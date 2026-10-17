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

func TestURLsIncludesSunoAudio(t *testing.T) {
	raw := json.RawMessage(`{"data":[{"id":"a1","audio_url":"https://c/1.mp3","stream_audio_url":"https://s/1","image_url":"https://c/1.jpg"},{"id":"a2","audio_url":"https://c/2.mp3"}]}`)
	want := []string{"https://c/1.mp3", "https://c/2.mp3"}
	if got := result.URLs(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestURLsOrderAndDedupAcrossShapes(t *testing.T) {
	raw := json.RawMessage(`{"resultUrls":["https://a/x.mp4"],"data":[{"audio_url":"https://a/x.mp4"},{"audio_url":"https://a/y.mp3"}],"lastFrameUrl":"https://a/f.png"}`)
	want := []string{"https://a/x.mp4", "https://a/y.mp3", "https://a/f.png"}
	if got := result.URLs(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestExtractSuno(t *testing.T) {
	raw := json.RawMessage(`{"data":[{"id":"a1","audio_url":"https://c/1.mp3"},{"id":"a2","audio_url":"https://c/2.mp3"},{"audio_url":"https://c/3.mp3"},{"id":"a3"}]}`)
	want := result.IDs{TaskID: "t1", AudioIDs: []string{"a1", "a2", "a3"}}
	if got := result.Extract("t1", raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	wantURLs := []string{"https://c/1.mp3", "https://c/2.mp3", "https://c/3.mp3"}
	if got := result.URLs(raw); !reflect.DeepEqual(got, wantURLs) {
		t.Fatalf("urls got %v", got)
	}
}

func TestExtractLastFrame(t *testing.T) {
	if got := result.Extract("t1", json.RawMessage(`{"lastFrameUrl":["","https://a/f.png"]}`)).LastFrameURL; got != "https://a/f.png" {
		t.Fatalf("array: got %q", got)
	}
	if got := result.Extract("t1", json.RawMessage(`{"lastFrameUrl":"https://a/g.png"}`)).LastFrameURL; got != "https://a/g.png" {
		t.Fatalf("string: got %q", got)
	}
}

func TestExtractEmptyResponses(t *testing.T) {
	for _, raw := range []string{``, `null`, `{}`, `not json`, `{"data":"x"}`} {
		if got := result.Extract("t1", json.RawMessage(raw)); !reflect.DeepEqual(got, result.IDs{TaskID: "t1"}) {
			t.Fatalf("%q: got %+v", raw, got)
		}
	}
	b, err := json.Marshal(result.IDs{TaskID: "t1"})
	if err != nil || string(b) != `{"task_id":"t1"}` {
		t.Fatalf("marshal: %s %v", b, err)
	}
}
