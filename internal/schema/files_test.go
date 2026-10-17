package schema_test

import (
	"testing"

	"github.com/mik3dev/kie-cli/internal/schema"
)

func TestHasURIFormat(t *testing.T) {
	sch := inputSchema(t, "bytedance__seedance-2-5")
	cases := map[string]bool{
		"reference_image_urls": true,
		"first_frame_url":      false,
		"prompt":               false,
		"missing":              false,
	}
	for key, want := range cases {
		if got := schema.HasURIFormat(sch, key); got != want {
			t.Errorf("HasURIFormat(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestHasURIFormatOneOf(t *testing.T) {
	sch := inputSchema(t, "ai-music-api__separate-vocals")
	if !schema.HasURIFormat(sch, "audio_url") {
		t.Fatal("audio_url should be a uri field (oneOf branch 2)")
	}
}

func TestHasURIFormatNestedAndNil(t *testing.T) {
	sch := map[string]any{"properties": map[string]any{
		"obj": map[string]any{"type": "object", "properties": map[string]any{
			"u": map[string]any{"type": "string", "format": "uri"}}},
		"alt": map[string]any{"anyOf": []any{
			map[string]any{"type": "string"},
			map[string]any{"type": "string", "format": "uri"}}},
		"plain": map[string]any{"type": "string", "format": "date"},
	}}
	for key, want := range map[string]bool{"obj": true, "alt": true, "plain": false} {
		if got := schema.HasURIFormat(sch, key); got != want {
			t.Errorf("HasURIFormat(%q) = %v, want %v", key, got, want)
		}
	}
	if schema.HasURIFormat(nil, "x") {
		t.Fatal("nil schema must be false")
	}
}
