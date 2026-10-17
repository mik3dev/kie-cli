// Package schema fetches kie model OpenAPI schemas and summarizes their input.
package schema

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/cache"
)

const (
	schemaTTL   = 24 * time.Hour
	jobPath     = "/api/v1/jobs/createTask"
	defaultVerb = "POST"
)

var httpVerbs = []string{"get", "post", "put", "patch", "delete"}

// Doc is the `data` of /api/v1/models/{m}/schema. OpenAPI is nil when kie
// has not synced the schema (it arrives as null).
type Doc struct {
	Model   string         `json:"model"`
	OpenAPI map[string]any `json:"openapi"`
}

// Null reports whether kie returned no schema for the model.
func (d Doc) Null() bool { return d.OpenAPI == nil }

// Fetch returns the model schema, cached 24h unless refresh.
func Fetch(ctx context.Context, c *api.Client, cacheDir, model string, refresh bool, now time.Time) (Doc, error) {
	name := "schema/" + model
	var d Doc
	if !refresh {
		if hit, err := cache.Get(cacheDir, name, schemaTTL, now, &d); err != nil {
			return Doc{}, err
		} else if hit {
			return d, nil
		}
		d = Doc{}
	}
	req := api.Request{Method: "GET", Path: "/api/v1/models/" + model + "/schema", Class: api.ClassDiscovery}
	if err := c.Do(ctx, req, &d); err != nil {
		return Doc{}, err
	}
	if d.Model == "" {
		d.Model = model
	}
	if err := cache.Put(cacheDir, name, d); err != nil {
		return Doc{}, err
	}
	return d, nil
}

// Endpoint returns the method and path of the first operation (paths sorted
// for determinism). A null schema reports POST createTask.
func (d Doc) Endpoint() (method, path string, err error) {
	if d.Null() {
		return defaultVerb, jobPath, nil
	}
	paths, _ := d.OpenAPI["paths"].(map[string]any)
	if len(paths) == 0 {
		return "", "", errors.New("schema has no paths")
	}
	keys := make([]string, 0, len(paths))
	for k := range paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	path = keys[0]
	item, _ := paths[path].(map[string]any)
	for _, v := range httpVerbs {
		if _, ok := item[v]; ok {
			return strings.ToUpper(v), path, nil
		}
	}
	return "", "", fmt.Errorf("path %s has no operation", path)
}

// IsJob reports whether the model is submitted through createTask.
func (d Doc) IsJob() bool {
	_, p, err := d.Endpoint()
	return err == nil && p == jobPath
}

// bodySchema returns the resolved application/json request body schema.
func (d Doc) bodySchema() (map[string]any, error) {
	if d.Null() {
		return nil, nil
	}
	method, path, err := d.Endpoint()
	if err != nil {
		return nil, err
	}
	paths, _ := d.OpenAPI["paths"].(map[string]any)
	item, _ := paths[path].(map[string]any)
	op, _ := item[strings.ToLower(method)].(map[string]any)
	rb, err := Resolve(d.OpenAPI, op["requestBody"])
	if err != nil {
		return nil, err
	}
	rbm, _ := rb.(map[string]any)
	content, _ := rbm["content"].(map[string]any)
	js, _ := content["application/json"].(map[string]any)
	sch, ok := js["schema"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s %s has no application/json request schema", method, path)
	}
	return sch, nil
}

// InputSchema returns the resolved `properties.input` of the createTask body.
// Other operations (chat, omni) have no input wrapper (a body field named
// `input` there is an ordinary field), so the whole body schema is returned.
// A null schema returns nil.
func (d Doc) InputSchema() (map[string]any, error) {
	body, err := d.bodySchema()
	if err != nil || body == nil {
		return nil, err
	}
	if !d.IsJob() {
		return body, nil
	}
	props, _ := body["properties"].(map[string]any)
	if in, ok := props["input"].(map[string]any); ok {
		return in, nil
	}
	return body, nil
}
