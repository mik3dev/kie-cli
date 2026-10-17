package catalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/catalog"
)

func fixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("../../testdata/live/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newSvc(t *testing.T, hits *atomic.Int32) *catalog.Service {
	t.Helper()
	models := fixture(t, "models.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var data json.RawMessage
		switch {
		case r.URL.Path == "/api/v1/models":
			data = models
		case strings.HasSuffix(r.URL.Path, "/price"):
			data = fixture(t, "price/google__imagen4.json")
		case strings.HasSuffix(r.URL.Path, "/success-rate"):
			data = fixture(t, "success-rate/ai-music-api__extend.json")
		default:
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": data})
	}))
	t.Cleanup(srv.Close)
	c := api.New("k", func(string) string { return "" })
	c.BaseURL = srv.URL
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return &catalog.Service{API: c, CacheDir: t.TempDir()}
}

func TestListUsesCacheWithinTTL(t *testing.T) {
	var hits atomic.Int32
	s := newSvc(t, &hits)
	a, err := s.List(context.Background(), false)
	if err != nil || len(a) != 227 {
		t.Fatalf("len=%d err=%v", len(a), err)
	}
	if _, err := s.List(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d", hits.Load())
	}
}

func TestListExpiredCacheRefetches(t *testing.T) {
	var hits atomic.Int32
	s := newSvc(t, &hits)
	_, _ = s.List(context.Background(), false)
	s.Now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	_, _ = s.List(context.Background(), false)
	if hits.Load() != 2 {
		t.Fatalf("hits = %d", hits.Load())
	}
}

func TestListRefreshBypassesCache(t *testing.T) {
	var hits atomic.Int32
	s := newSvc(t, &hits)
	_, _ = s.List(context.Background(), false)
	_, _ = s.List(context.Background(), true)
	if hits.Load() != 2 {
		t.Fatalf("hits = %d", hits.Load())
	}
}

func TestFilterTypeVideoSubstring(t *testing.T) {
	var hits atomic.Int32
	all, _ := newSvc(t, &hits).List(context.Background(), false)
	got := catalog.Filter(all, catalog.Query{Type: "VIDEO"})
	if len(got) == 0 {
		t.Fatal("no results")
	}
	for _, m := range got {
		ok := false
		for _, tt := range m.TaskType {
			ok = ok || strings.Contains(strings.ToLower(tt), "video")
		}
		if !ok {
			t.Errorf("%s has taskType %v", m.Model, m.TaskType)
		}
	}
}

func TestFilterProvider(t *testing.T) {
	var hits atomic.Int32
	all, _ := newSvc(t, &hits).List(context.Background(), false)
	got := catalog.Filter(all, catalog.Query{Provider: "kling"})
	if len(got) == 0 {
		t.Fatal("no kling models")
	}
	for _, m := range got {
		if !strings.EqualFold(m.Provider, "Kling") {
			t.Errorf("provider = %s", m.Provider)
		}
	}
}

func TestSearchText(t *testing.T) {
	var hits atomic.Int32
	all, _ := newSvc(t, &hits).List(context.Background(), false)
	got := catalog.Filter(all, catalog.Query{Text: "IMAGEN"})
	if len(got) == 0 {
		t.Fatal("no results")
	}
	for _, m := range got {
		d := ""
		if m.Description != nil {
			d = *m.Description
		}
		if !strings.Contains(strings.ToLower(m.Model+m.Title+d), "imagen") {
			t.Errorf("%s does not match", m.Model)
		}
	}
	if n := len(catalog.Filter(all, catalog.Query{Text: "zzz-no-such-model"})); n != 0 {
		t.Errorf("expected none, got %d", n)
	}
}

func isChat(m catalog.Model) bool {
	for _, tt := range m.TaskType {
		if strings.Contains(tt, "Chat") {
			return true
		}
	}
	return false
}

func TestChatModelsHidden(t *testing.T) {
	var hits atomic.Int32
	all, _ := newSvc(t, &hits).List(context.Background(), false)
	chat := map[string]bool{}
	for _, m := range all {
		if isChat(m) {
			chat[m.Model] = true
		}
	}
	if len(chat) == 0 {
		t.Fatal("fixture has no chat models")
	}
	for _, q := range []catalog.Query{{}, {Text: "claude"}, {Type: "chat"}, {Provider: "Anthropic"}} {
		for _, m := range catalog.Filter(all, q) {
			if isChat(m) || chat[m.Model] {
				t.Errorf("chat model %s leaked for %+v", m.Model, q)
			}
		}
	}
}

func TestFindIncludesChatModels(t *testing.T) {
	var hits atomic.Int32
	s := newSvc(t, &hits)
	m, ok, err := s.Find(context.Background(), "claude-sonnet-5-5")
	if err != nil || !ok || m.Model != "claude-sonnet-5-5" {
		t.Fatalf("m=%+v ok=%v err=%v", m, ok, err)
	}
	if _, ok, _ := s.Find(context.Background(), "nope/nope"); ok {
		t.Fatal("found nonexistent")
	}
}

func TestPriceAndHealth(t *testing.T) {
	var hits atomic.Int32
	s := newSvc(t, &hits)
	p, err := s.Price(context.Background(), "google/imagen4")
	if err != nil || !strings.Contains(p, "Imagen 4") {
		t.Fatalf("p=%q err=%v", p, err)
	}
	h, err := s.Health(context.Background(), "ai-music-api/extend")
	if err != nil {
		t.Fatal(err)
	}
	var pts []map[string]any
	if err := json.Unmarshal(h, &pts); err != nil || len(pts) == 0 {
		t.Fatalf("points: %v %s", err, h)
	}
}
