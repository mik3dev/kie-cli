package cli

import (
	"testing"
)

func TestFileKeyFuncSchemaAware(t *testing.T) {
	sch := map[string]any{"properties": map[string]any{
		"image_urls":     map[string]any{"type": "array", "items": map[string]any{"type": "string", "format": "uri"}},
		"image":          map[string]any{"type": "string"},
		"audio_id":       map[string]any{"type": "string"},
		"image_size":     map[string]any{"type": "string", "enum": []any{"square", "wide"}},
		"generate_audio": map[string]any{"type": "boolean"},
		"max_images":     map[string]any{"type": "integer"},
		"reference":      map[string]any{"type": "string", "format": "uri"},
		"image_enum":     map[string]any{"type": "string", "enum": []any{"a"}},
		"video_list": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
			"url": map[string]any{"type": "string", "format": "uri"}}}},
	}}
	tests := map[string]bool{
		"video_list": true, "image_urls": true, "image": true, "reference": true,
		"audio_id": false, "image_size": false, "generate_audio": false, "max_images": false, "image_enum": false,
		"unknown_url": true, // not in schema: name heuristic
		"prompt":      false,
	}
	isFile := fileKeyFunc(sch)
	for k, want := range tests {
		if got := isFile(k); got != want {
			t.Errorf("isFile(%q) = %v, want %v", k, got, want)
		}
	}
}

func TestFileKeyFuncNoSchemaFallsBackToName(t *testing.T) {
	isFile := fileKeyFunc(nil)
	if !isFile("image_url") || isFile("prompt") || isFile("audio_id") {
		t.Fatal("name heuristic fallback broken")
	}
}
