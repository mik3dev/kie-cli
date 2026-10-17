// Package preset loads named model+params presets from the embedded factory
// file and the optional user file, merging them by name.
package preset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Preset sources.
const (
	SourceFactory = "factory"
	SourceUser    = "user"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Preset is a named model plus default params.
type Preset struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Model       string         `json:"model"`
	Params      map[string]any `json:"params,omitempty"`
	Source      string         `json:"source"`
}

type rawPreset struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Model       string         `yaml:"model"`
	Params      map[string]any `yaml:"params"`
}

type rawFile struct {
	Presets []*rawPreset `yaml:"presets"`
}

// Parse decodes and validates a presets document, tagging each preset with source.
func Parse(data []byte, source string) ([]Preset, error) {
	var f rawFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse presets: %w", err)
	}
	seen := map[string]bool{}
	out := make([]Preset, 0, len(f.Presets))
	for i, r := range f.Presets {
		if r == nil {
			return nil, fmt.Errorf("preset #%d: empty entry", i+1)
		}
		if !namePattern.MatchString(r.Name) {
			return nil, fmt.Errorf("preset #%d: invalid name %q (want %s)", i+1, r.Name, namePattern)
		}
		if seen[r.Name] {
			return nil, fmt.Errorf("preset %q: duplicate name", r.Name)
		}
		seen[r.Name] = true
		if strings.TrimSpace(r.Model) == "" {
			return nil, fmt.Errorf("preset %q: model is required", r.Name)
		}
		params, err := normalize(r.Params)
		if err != nil {
			return nil, fmt.Errorf("preset %q: params: %w", r.Name, err)
		}
		out = append(out, Preset{Name: r.Name, Description: r.Description, Model: r.Model, Params: params, Source: source})
	}
	return out, nil
}

// normalize round-trips params through JSON so numbers become json.Number.
func normalize(in map[string]any) (map[string]any, error) {
	if len(in) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return out, nil
}

// Load merges the factory presets with the user file at userPath (user wins by
// name, whole-preset replacement) and returns them sorted by name. A missing
// user file is not an error.
func Load(factory []byte, userPath string) ([]Preset, error) {
	base, err := Parse(factory, SourceFactory)
	if err != nil {
		return nil, fmt.Errorf("factory presets: %w", err)
	}
	merged := make(map[string]Preset, len(base))
	for _, p := range base {
		merged[p.Name] = p
	}
	data, err := os.ReadFile(userPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("read presets file %s: %w", userPath, err)
	default:
		user, perr := Parse(data, SourceUser)
		if perr != nil {
			return nil, fmt.Errorf("presets file %s: %w", userPath, perr)
		}
		for _, p := range user {
			merged[p.Name] = p
		}
	}
	out := make([]Preset, 0, len(merged))
	for _, p := range merged {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Find returns the preset with the given name.
func Find(list []Preset, name string) (Preset, bool) {
	for _, p := range list {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}
