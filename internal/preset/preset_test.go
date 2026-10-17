package preset_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kiecli "github.com/mik3dev/kie-cli"
	"github.com/mik3dev/kie-cli/internal/preset"
	"github.com/mik3dev/kie-cli/internal/schema"
)

func TestParseValid(t *testing.T) {
	got, err := preset.Parse([]byte("presets:\n- name: a\n  description: d\n  model: m/x\n  params: {n: 5, s: \"5\", b: true}\n"), "factory")
	if err != nil || len(got) != 1 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	p := got[0]
	if p.Params["n"] != json.Number("5") || p.Params["s"] != "5" || p.Params["b"] != true || p.Source != "factory" {
		t.Fatalf("bad preset: %#v", p)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"invalid yaml":  "presets: [",
		"unknown key":   "presets:\n- name: a\n  modle: m\n",
		"missing model": "presets:\n- name: a\n",
		"bad name":      "presets:\n- name: Foo Bar\n  model: m\n",
		"duplicate":     "presets:\n- name: a\n  model: m\n- name: a\n  model: n\n",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := preset.Parse([]byte(doc), "user"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseEmptyAndUnmarshalableParams(t *testing.T) {
	got, err := preset.Parse([]byte(""), "user")
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if _, err := preset.Parse([]byte("presets:\n- name: a\n  model: m\n  params: {k: .inf}\n"), "user"); err == nil {
		t.Fatal("expected error for unencodable float")
	}
}

const factory = "presets:\n- name: a\n  model: m1\n  params: {x: 1}\n- name: b\n  model: m1\n"

func writeUser(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "presets.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadUserOverridesAndAdds(t *testing.T) {
	p := writeUser(t, "presets:\n- name: a\n  model: m2\n- name: c\n  model: m3\n")
	got, err := preset.Load([]byte(factory), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Name != "a" || got[0].Model != "m2" || got[0].Source != "user" ||
		got[0].Params != nil || got[1].Name != "b" || got[1].Source != "factory" ||
		got[2].Name != "c" || got[2].Source != "user" {
		t.Fatalf("got %#v", got)
	}
	f, ok := preset.Find(got, "b")
	if !ok || f.Name != "b" {
		t.Fatalf("find: %v %v", f, ok)
	}
	if _, ok := preset.Find(got, "zzz"); ok {
		t.Fatal("unexpected find")
	}
}

func TestLoadMissingUserFile(t *testing.T) {
	got, err := preset.Load([]byte(factory), filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil || len(got) != 2 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestLoadBrokenUserFileNamesPath(t *testing.T) {
	p := writeUser(t, "presets: [")
	_, err := preset.Load([]byte(factory), p)
	if err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadBrokenFactory(t *testing.T) {
	if _, err := preset.Load([]byte("presets: ["), ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadUserPathUnreadable(t *testing.T) {
	if _, err := preset.Load([]byte(factory), t.TempDir()); err == nil {
		t.Fatal("expected error reading a directory")
	}
}

func TestFactoryPresetsMatchLiveSchemas(t *testing.T) {
	data, err := kiecli.Assets.ReadFile("presets/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	list, err := preset.Parse(data, "factory")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"imagen-rapida", "imagen-calidad", "imagen-edicion", "video-calidad", "video-imagen",
		"video-inicio-fin", "video-barato", "musica", "instrumental", "voz"}
	for _, n := range want {
		if _, ok := preset.Find(list, n); !ok {
			t.Errorf("missing factory preset %s", n)
		}
	}
	mb, err := os.ReadFile("../../testdata/live/models.json")
	if err != nil {
		t.Fatal(err)
	}
	var models struct {
		Models []struct {
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.Unmarshal(mb, &models); err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, m := range models.Models {
		known[m.Model] = true
	}
	for _, p := range list {
		if !known[p.Model] {
			t.Errorf("%s: model %s not in models.json", p.Name, p.Model)
			continue
		}
		sb, err := os.ReadFile("../../testdata/live/schemas/" + strings.ReplaceAll(p.Model, "/", "__") + ".json")
		if err != nil {
			t.Errorf("%s: %v", p.Name, err)
			continue
		}
		var doc schema.Doc
		if err := json.Unmarshal(sb, &doc); err != nil {
			t.Errorf("%s: %v", p.Name, err)
			continue
		}
		sch, err := doc.InputSchema()
		if err != nil {
			t.Errorf("%s: %v", p.Name, err)
			continue
		}
		probs, err := schema.Validate(sch, schema.Coerce(sch, p.Params))
		if err != nil {
			t.Errorf("%s: %v", p.Name, err)
		}
		for _, pr := range probs {
			if !strings.HasSuffix(pr.Message, "missing required field") {
				t.Errorf("%s: %+v", p.Name, pr)
			}
		}
	}
}
