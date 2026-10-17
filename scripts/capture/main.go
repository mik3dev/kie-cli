// Command capture records read-only kie.ai discovery responses as fixtures.
// One-off tool: calls only GET /api/v1/models*, and GET /api/v1/chat/credit.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
)

type model struct {
	Model    string          `json:"model"`
	Provider string          `json:"provider"`
	TaskType []string        `json:"taskType"`
	Raw      json.RawMessage `json:"-"`
}

type catalog struct {
	Total  int               `json:"total"`
	Models []json.RawMessage `json:"models"`
}

type schemaData struct {
	Model   string          `json:"model"`
	OpenAPI json.RawMessage `json:"openapi"`
}

var fileKeyRe = regexp.MustCompile(`(?i)url|image|video|audio|file|mask`)

func main() {
	out := flag.String("out", "testdata/live", "output directory")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "capture:", err)
		os.Exit(1)
	}
}

func run(out string) error {
	key := os.Getenv("KIE_API_KEY")
	if key == "" {
		return fmt.Errorf("KIE_API_KEY not set")
	}
	c := api.New(key, os.Getenv)
	ctx := context.Background()

	get := func(path string, v *json.RawMessage) error {
		return c.Do(ctx, api.Request{Method: "GET", Path: path, Class: api.ClassDiscovery, Host: api.HostAPI}, v)
	}

	var creditBefore json.RawMessage
	if err := get("/api/v1/chat/credit", &creditBefore); err != nil {
		return fmt.Errorf("credit: %w", err)
	}
	if err := writeJSON(filepath.Join(out, "credit.json"), creditBefore); err != nil {
		return err
	}

	var rawCat json.RawMessage
	if err := get("/api/v1/models", &rawCat); err != nil {
		return fmt.Errorf("catalog: %w", err)
	}
	if err := writeJSON(filepath.Join(out, "models.json"), rawCat); err != nil {
		return err
	}
	var cat catalog
	if err := json.Unmarshal(rawCat, &cat); err != nil {
		return fmt.Errorf("catalog shape: %w", err)
	}
	models := make([]model, 0, len(cat.Models))
	for _, r := range cat.Models {
		var m model
		if err := json.Unmarshal(r, &m); err != nil {
			return fmt.Errorf("model shape: %w", err)
		}
		models = append(models, m)
	}

	rep := newReport(cat.Total, models)
	for i, m := range models {
		fmt.Fprintf(os.Stderr, "schema %d/%d %s\n", i+1, len(models), m.Model)
		var raw json.RawMessage
		if err := get("/api/v1/models/"+m.Model+"/schema", &raw); err != nil {
			rep.schemaErrors = append(rep.schemaErrors, fmt.Sprintf("%s: %v", m.Model, err))
			continue
		}
		if err := writeJSON(filepath.Join(out, "schemas", fileName(m.Model)+".json"), raw); err != nil {
			return err
		}
		var sd schemaData
		if err := json.Unmarshal(raw, &sd); err != nil {
			rep.schemaErrors = append(rep.schemaErrors, fmt.Sprintf("%s: shape: %v", m.Model, err))
			continue
		}
		rep.addSchema(m.Model, sd.OpenAPI)
	}

	for _, m := range pickSamples(models) {
		for _, ep := range []string{"price", "success-rate"} {
			var raw json.RawMessage
			if err := get("/api/v1/models/"+m.Model+"/"+ep, &raw); err != nil {
				rep.schemaErrors = append(rep.schemaErrors, fmt.Sprintf("%s %s: %v", m.Model, ep, err))
				continue
			}
			if err := writeJSON(filepath.Join(out, ep, fileName(m.Model)+".json"), raw); err != nil {
				return err
			}
		}
		rep.samples = append(rep.samples, m.Model)
	}

	var creditAfter json.RawMessage
	if err := get("/api/v1/chat/credit", &creditAfter); err != nil {
		return fmt.Errorf("credit after: %w", err)
	}
	rep.creditBefore, rep.creditAfter = string(creditBefore), string(creditAfter)
	return os.WriteFile(filepath.Join(out, "REPORT.md"), []byte(rep.render()), 0o644)
}

func fileName(m string) string { return strings.ReplaceAll(m, "/", "__") }

func writeJSON(path string, raw json.RawMessage) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return fmt.Errorf("indent %s: %w", path, err)
	}
	buf.WriteByte('\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func hasType(m model, sub string) bool {
	for _, t := range m.TaskType {
		if strings.Contains(strings.ToLower(t), sub) {
			return true
		}
	}
	return false
}

func pickSamples(models []model) []model {
	preds := []func(model) bool{
		func(m model) bool { return hasType(m, "text to image") },
		func(m model) bool { return hasType(m, "video") && !strings.EqualFold(m.Provider, "suno") },
		func(m model) bool { return strings.EqualFold(m.Provider, "suno") || strings.Contains(m.Model, "suno") },
		func(m model) bool { return hasType(m, "text to speech") },
		func(m model) bool { return m.Model == "veo-3-1" },
	}
	var picked []model
	seen := map[string]bool{}
	for _, p := range preds {
		for _, m := range models {
			if p(m) && !seen[m.Model] {
				seen[m.Model] = true
				picked = append(picked, m)
				break
			}
		}
	}
	return picked
}

type report struct {
	total        int
	models       []model
	byType       map[string]int
	nullSchemas  []string
	otherPaths   map[string][]string
	fileKeys     map[string][]string
	schemaErrors []string
	samples      []string
	creditBefore string
	creditAfter  string
}

func newReport(total int, models []model) *report {
	r := &report{total: total, models: models, byType: map[string]int{},
		otherPaths: map[string][]string{}, fileKeys: map[string][]string{}}
	for _, m := range models {
		for _, t := range m.TaskType {
			r.byType[t]++
		}
	}
	return r
}

func (r *report) addSchema(name string, openapi json.RawMessage) {
	if len(openapi) == 0 || string(openapi) == "null" {
		r.nullSchemas = append(r.nullSchemas, name)
		return
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(openapi, &doc); err != nil {
		r.schemaErrors = append(r.schemaErrors, name+": openapi shape: "+err.Error())
		return
	}
	for p, ops := range doc.Paths {
		if p != "/api/v1/jobs/createTask" {
			r.otherPaths[p] = append(r.otherPaths[p], name)
		}
		for _, op := range ops {
			for _, k := range inputKeys(op) {
				if fileKeyRe.MatchString(k) {
					r.fileKeys[k] = appendUniq(r.fileKeys[k], name)
				}
			}
		}
	}
}

func appendUniq(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// inputKeys finds property names under the `input` object of an operation's request schema.
func inputKeys(op json.RawMessage) []string {
	var any interface{}
	if json.Unmarshal(op, &any) != nil {
		return nil
	}
	var keys []string
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch t := v.(type) {
		case map[string]interface{}:
			if props, ok := t["properties"].(map[string]interface{}); ok {
				if in, ok := props["input"].(map[string]interface{}); ok {
					if ip, ok := in["properties"].(map[string]interface{}); ok {
						for k := range ip {
							keys = append(keys, k)
						}
					}
				}
			}
			for _, c := range t {
				walk(c)
			}
		case []interface{}:
			for _, c := range t {
				walk(c)
			}
		}
	}
	walk(any)
	return keys
}

func (r *report) has(sub string) []string {
	var hits []string
	for _, m := range r.models {
		if strings.Contains(strings.ToLower(m.Model), sub) || strings.Contains(strings.ToLower(m.Provider), sub) {
			hits = append(hits, m.Model)
		}
	}
	for p := range r.otherPaths {
		if strings.Contains(strings.ToLower(p), sub) {
			hits = append(hits, "path:"+p)
		}
	}
	sort.Strings(hits)
	return hits
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func (r *report) render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Live capture report\n\nGenerated %s (read-only).\n\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Total (data.total): %d\n- Models listed: %d\n\n## Count by taskType\n\n", r.total, len(r.models))
	for _, k := range sortedKeys(r.byType) {
		fmt.Fprintf(&b, "- %s: %d\n", k, r.byType[k])
	}
	fmt.Fprintf(&b, "\n## Null schemas (%d of %d)\n\n", len(r.nullSchemas), len(r.models))
	for _, n := range r.nullSchemas {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	fmt.Fprintf(&b, "\n## Schema/other call errors (%d)\n\n", len(r.schemaErrors))
	for _, e := range r.schemaErrors {
		fmt.Fprintf(&b, "- %s\n", e)
	}
	b.WriteString("\n## Paths other than /api/v1/jobs/createTask\n\n")
	for _, p := range sortedKeys(r.otherPaths) {
		fmt.Fprintf(&b, "- %s: %s\n", p, strings.Join(r.otherPaths[p], ", "))
	}
	b.WriteString("\n## Presence\n\n")
	for _, s := range []string{"ai-music-api", "suno", "veo", "runway"} {
		h := r.has(s)
		fmt.Fprintf(&b, "- %s: %v (%d hits) %s\n", s, len(h) > 0, len(h), strings.Join(h, ", "))
	}
	b.WriteString("\n## input property names matching url|image|video|audio|file|mask\n\n")
	for _, k := range sortedKeys(r.fileKeys) {
		fmt.Fprintf(&b, "- %s (%d models)\n", k, len(r.fileKeys[k]))
	}
	fmt.Fprintf(&b, "\n## Price/success-rate samples\n\n%s\n", strings.Join(r.samples, ", "))
	fmt.Fprintf(&b, "\n## Credit check\n\n- before: %s\n- after: %s\n- unchanged: %v\n", r.creditBefore, r.creditAfter, r.creditBefore == r.creditAfter)
	return b.String()
}
