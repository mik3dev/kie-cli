package result_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/mik3dev/kie-cli/internal/result"
)

func TestURLsResultUrlsAndLastFrame(t *testing.T) {
	raw := json.RawMessage(`{"resultUrls":["https://a/1.mp4","https://a/2.mp4","https://a/1.mp4"],"lastFrameUrl":["https://a/f.png","https://a/2.mp4"]}`)
	want := []string{"https://a/1.mp4", "https://a/2.mp4", "https://a/f.png"}
	if got := result.URLs(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestURLsLastFrameAsString(t *testing.T) {
	raw := json.RawMessage(`{"lastFrameUrl":"https://a/f.png"}`)
	if got := result.URLs(raw); !reflect.DeepEqual(got, []string{"https://a/f.png"}) {
		t.Fatalf("got %v", got)
	}
}

func TestURLsEmptyOnNoResult(t *testing.T) {
	for _, raw := range []string{``, `null`, `{}`, `not json`, `{"resultUrls":null}`, `{"resultUrls":[1,""]}`} {
		if got := result.URLs(json.RawMessage(raw)); len(got) != 0 {
			t.Fatalf("%q: got %v", raw, got)
		}
	}
}
