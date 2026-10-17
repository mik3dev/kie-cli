package cli_test

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"

	kiecli "github.com/mik3dev/kie-cli"
	"github.com/mik3dev/kie-cli/internal/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func testRoot() *cobra.Command {
	return cli.NewRoot(cli.Deps{
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		Env: func(string) string { return "" },
	})
}

// checkSkillText returns one problem per kie-cli command or flag in text that
// does not exist in the cobra tree rooted at root.
func checkSkillText(root *cobra.Command, text string) []string {
	var problems []string
	for _, line := range strings.Split(text, "\n") {
		rest := line
		for {
			i := strings.Index(rest, "kie-cli ")
			if i < 0 {
				break
			}
			rest = rest[i+len("kie-cli "):]
			seg := rest
			if j := strings.Index(seg, "`"); j >= 0 {
				seg = seg[:j]
			}
			problems = append(problems, checkInvocation(root, strings.Fields(seg))...)
		}
	}
	return problems
}

func checkInvocation(root *cobra.Command, tokens []string) []string {
	var problems []string
	cmd := root
	walking := true
	for _, tok := range tokens {
		switch {
		case strings.HasPrefix(tok, "-"):
			walking = false
			if p := checkFlag(cmd, tok); p != "" {
				problems = append(problems, p)
			}
		case walking && cmd.HasSubCommands():
			child := findChild(cmd, tok)
			if child == nil {
				return append(problems, "unknown command: "+cmd.CommandPath()+" "+tok)
			}
			cmd = child
		default:
			walking = false // positional args, <placeholders>, @presets, key=value
		}
	}
	return problems
}

func findChild(cmd *cobra.Command, name string) *cobra.Command {
	for _, c := range cmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func checkFlag(cmd *cobra.Command, tok string) string {
	name := strings.SplitN(strings.TrimLeft(tok, "-"), "=", 2)[0]
	if name == "" {
		return ""
	}
	var f *pflag.Flag
	if strings.HasPrefix(tok, "--") {
		f = cmd.Flag(name)
	} else {
		f = cmd.Flags().ShorthandLookup(name[:1])
	}
	if f == nil {
		return "unknown flag " + tok + " for " + cmd.CommandPath()
	}
	return ""
}

func TestCheckSkillTextCatchesBadRefs(t *testing.T) {
	root := testRoot()
	bad := []string{
		"`kie-cli run --bogus`",
		"`kie-cli nope`",
		"kie-cli models nope --refresh",
		"`kie-cli run -z`",
	}
	for _, text := range bad {
		if got := checkSkillText(root, text); len(got) != 1 {
			t.Errorf("%q: want 1 problem, got %v", text, got)
		}
	}
	good := []string{
		"`kie-cli models schema <model> --refresh`",
		"kie-cli run @musica -p prompt=x --no-wait",
		"kie-cli run <model> --out=dir -p k=v --dry-run",
		"no command here",
	}
	for _, text := range good {
		if got := checkSkillText(root, text); len(got) != 0 {
			t.Errorf("%q: want none, got %v", text, got)
		}
	}
}

func skillDocs(t *testing.T) map[string]string {
	t.Helper()
	docs := map[string]string{}
	err := fs.WalkDir(kiecli.Assets, "skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, rerr := kiecli.Assets.ReadFile(p)
		docs[p] = string(b)
		return rerr
	})
	if err != nil {
		t.Fatal(err)
	}
	return docs
}

func TestSkillsMentionOnlyRealCommands(t *testing.T) {
	docs := skillDocs(t)
	if len(docs) < 4 {
		t.Fatalf("want SKILL.md and 3 references, got %d files", len(docs))
	}
	for p, text := range docs {
		if got := checkSkillText(testRoot(), text); len(got) != 0 {
			t.Errorf("%s: %v", p, got)
		}
	}
}

func TestSkillDocumentsExitCodes(t *testing.T) {
	text := skillDocs(t)["skills/kie-generate/SKILL.md"]
	for _, code := range []string{"usage", "validation", "auth", "credits", "task_failed", "timeout", "rate_limit", "submit_uncertain", "api", "io"} {
		if !strings.Contains(text, code) {
			t.Errorf("SKILL.md does not mention error code %q", code)
		}
	}
}

func TestSkillFrontmatter(t *testing.T) {
	text := skillDocs(t)["skills/kie-generate/SKILL.md"]
	if !strings.HasPrefix(text, "---\n") {
		t.Fatal("no frontmatter")
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		t.Fatal("frontmatter not closed")
	}
	fm := text[4 : 4+end]
	if !strings.Contains(fm, "name: kie-generate") {
		t.Errorf("name missing in %q", fm)
	}
	var desc string
	for _, l := range strings.Split(fm, "\n") {
		if v, ok := strings.CutPrefix(l, "description:"); ok {
			desc = strings.TrimSpace(v)
		}
	}
	if desc == "" || desc == "TODO" {
		t.Errorf("description empty: %q", desc)
	}
}
