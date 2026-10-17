package cli_test

import (
	"os"
	"strings"
	"testing"
)

func readReadme(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("README.md: %v", err)
	}
	return string(b)
}

func TestReadmeMentionsOnlyRealCommands(t *testing.T) {
	text := readReadme(t)
	if !strings.Contains(text, "kie-cli ") {
		t.Fatal("README has no kie-cli examples")
	}
	if got := checkSkillText(testRoot(), text); len(got) != 0 {
		t.Errorf("README references non-existent commands or flags: %v", got)
	}
}

func TestReadmeDocumentsExitCodes(t *testing.T) {
	text := readReadme(t)
	for _, code := range []string{"usage", "validation", "auth", "credits", "task_failed", "timeout", "rate_limit", "submit_uncertain", "api", "io", "kie.ai/logs"} {
		if !strings.Contains(text, code) {
			t.Errorf("README does not mention %q", code)
		}
	}
}
