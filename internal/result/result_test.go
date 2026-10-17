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

func TestMediaURLs(t *testing.T) {
	in := `{"characterId":"c1","imageUrl":"https://a/i.png","bodyImageUrl":"https://a/b.png","callbackUrl":"https://cb","x":{"stream_audio_url":"https://s"},"assetUrl":"asset://x","list":{"maskUrls":["https://a/m1.png","https://a/m1.png"]}}`
	got := result.MediaURLs(json.RawMessage(in))
	want := []string{"https://a/b.png", "https://a/i.png", "https://a/m1.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestURLsIncludesResultObjectMedia(t *testing.T) {
	in := `{"resultObject":{"segments_count":2,"segments":[{"maskUrl":"https://a/0.png","name":"sky","index":0},{"maskUrl":"https://a/1.png","index":1}]}}`
	got := result.URLs(json.RawMessage(in))
	want := []string{"https://a/0.png", "https://a/1.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestURLsTextOnlyResultObject(t *testing.T) {
	in := `{"resultObject":{"lyricsData":[{"text":"la la","title":"t"}]}}`
	got := result.URLs(json.RawMessage(in))
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v want empty non-nil", got)
	}
}

func TestSyncIDs(t *testing.T) {
	cases := []struct {
		in   string
		want result.IDs
		js   string
	}{
		{`{"audioId":"au1","name":"v"}`, result.IDs{AudioIDs: []string{"au1"}}, `{"audio_ids":["au1"]}`},
		{`{"characterId":"ch1"}`, result.IDs{CharacterIDs: []string{"ch1"}}, `{"character_ids":["ch1"]}`},
		{`null`, result.IDs{}, `{}`},
		{`{}`, result.IDs{}, `{}`},
	}
	for _, c := range cases {
		got := result.SyncIDs(json.RawMessage(c.in))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %#v want %#v", c.in, got, c.want)
		}
		b, _ := json.Marshal(got)
		if string(b) != c.js {
			t.Errorf("%s: json %s want %s", c.in, b, c.js)
		}
	}
}
