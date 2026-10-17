package kiecli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	forbiddenSubstrings = []string{
		"KIE_API_KEY", "KIE_E2E", "secrets: inherit", "secrets:inherit",
		"secrets[", "toJSON(secrets)", "tojson(secrets)",
	}
	secretRef = regexp.MustCompile(`secrets\.[A-Za-z0-9_]+`)
	// tagsFlag captures the value following -tags / --tags (with = or space).
	tagsFlag = regexp.MustCompile(`--?tags(?:=|\s+)(?:"([^"]*)"|'([^']*)'|(\S+))`)
	liveWord = regexp.MustCompile(`(^|[^A-Za-z0-9_])live([^A-Za-z0-9_]|$)`)
)

// workflowViolations returns every guard violation found in workflow content.
func workflowViolations(content string) []string {
	var out []string
	for _, bad := range forbiddenSubstrings {
		if strings.Contains(content, bad) {
			out = append(out, "forbidden "+bad)
		}
	}
	for _, ref := range secretRef.FindAllString(content, -1) {
		if ref != "secrets.GITHUB_TOKEN" {
			out = append(out, "secret reference "+ref)
		}
	}
	for _, m := range tagsFlag.FindAllStringSubmatch(content, -1) {
		if liveWord.MatchString(m[1] + m[2] + m[3]) {
			out = append(out, "live build tag in "+m[0])
		}
	}
	return out
}

func TestWorkflowGuardRejectsBadSnippets(t *testing.T) {
	bad := map[string]string{
		"api key":         "env:\n  KIE_API_KEY: x",
		"e2e flag":        "KIE_E2E=1 go test ./...",
		"tags equals":     "go test -tags=live ./...",
		"tags space":      "go test -tags live ./...",
		"tags quoted":     "go test -tags 'live' ./...",
		"tags dquoted":    `go test -tags "integration live" ./...`,
		"tags list":       "go test -tags=integration,live ./...",
		"double dash":     "go test --tags=live ./...",
		"other secret":    "token: ${{ secrets.KIE_KEY }}",
		"secrets inherit": "    secrets: inherit",
		"secrets index":   "x: ${{ secrets['KIE_KEY'] }}",
		"to json":         "x: ${{ toJSON(secrets) }}",
	}
	for name, snippet := range bad {
		if len(workflowViolations(snippet)) == 0 {
			t.Errorf("%s: snippet not rejected: %q", name, snippet)
		}
	}
	good := []string{
		"GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}",
		"go test -race -tags=integration ./...",
		"go test -tags=delivery ./...",
	}
	for _, snippet := range good {
		if v := workflowViolations(snippet); len(v) != 0 {
			t.Errorf("good snippet rejected %q: %v", snippet, v)
		}
	}
}

func TestWorkflowsNeverUseLiveKey(t *testing.T) {
	var files []string
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		m, err := filepath.Glob(filepath.Join(".github", "workflows", pattern))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) < 2 {
		t.Fatalf("want at least 2 workflow files, got %d", len(files))
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range workflowViolations(string(raw)) {
			t.Errorf("%s: %s", f, v)
		}
	}
}

func TestGoreleaserInjectsVersion(t *testing.T) {
	raw, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	for _, want := range []string{
		"github.com/mik3dev/kie-cli/internal/cli.Version={{.Version}}",
		"windows", "darwin", "linux", "arm64", "CGO_ENABLED=0",
	} {
		if !strings.Contains(content, want) {
			t.Errorf(".goreleaser.yaml missing %q", want)
		}
	}
}
