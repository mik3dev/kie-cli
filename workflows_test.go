package kiecli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWorkflowsNeverUseLiveKey(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 2 {
		t.Fatalf("want at least 2 workflow files, got %d", len(files))
	}
	forbidden := []string{"KIE_API_KEY", "KIE_E2E", "tags=live", "tags live"}
	secretRef := regexp.MustCompile(`secrets\.[A-Za-z0-9_]+`)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		content := string(raw)
		for _, bad := range forbidden {
			if strings.Contains(content, bad) {
				t.Errorf("%s contains forbidden %q", f, bad)
			}
		}
		for _, ref := range secretRef.FindAllString(content, -1) {
			if ref != "secrets.GITHUB_TOKEN" {
				t.Errorf("%s references %q; only secrets.GITHUB_TOKEN allowed", f, ref)
			}
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
