// Package catalog lists, filters and prices kie.ai models, with a 1h cache.
package catalog

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/cache"
)

const (
	catalogTTL = time.Hour
	cacheName  = "models"
)

// Model is one catalog entry.
type Model struct {
	Model       string   `json:"model"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Provider    string   `json:"provider"`
	TaskType    []string `json:"taskType"`
	Description *string  `json:"description"`
	PricingDesc string   `json:"pricingDesc"`
}

// Query filters the catalog; all comparisons are case-insensitive.
type Query struct{ Type, Provider, Text string }

// Service reads the catalog through the API with a disk cache.
type Service struct {
	API      *api.Client
	CacheDir string
	Now      func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// List returns the full unfiltered catalog, cached for 1h unless refresh.
func (s *Service) List(ctx context.Context, refresh bool) ([]Model, error) {
	var ms []Model
	if !refresh {
		if hit, err := cache.Get(s.CacheDir, cacheName, catalogTTL, s.now(), &ms); err != nil {
			return nil, err
		} else if hit {
			return ms, nil
		}
	}
	var data struct {
		Models []Model `json:"models"`
	}
	req := api.Request{Method: "GET", Path: "/api/v1/models", Class: api.ClassDiscovery}
	if err := s.API.Do(ctx, req, &data); err != nil {
		return nil, err
	}
	if err := cache.Put(s.CacheDir, cacheName, data.Models); err != nil {
		return nil, err
	}
	return data.Models, nil
}

func isChat(m Model) bool {
	for _, t := range m.TaskType {
		if strings.Contains(strings.ToLower(t), "chat") {
			return true
		}
	}
	return false
}

// Filter returns a new slice with the models matching q. Chat models are
// out of scope for kie-cli and are always excluded.
func Filter(ms []Model, q Query) []Model {
	typ, prov, text := strings.ToLower(q.Type), strings.ToLower(q.Provider), strings.ToLower(q.Text)
	out := []Model{}
	for _, m := range ms {
		if isChat(m) || !matches(m, typ, prov, text) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func matches(m Model, typ, prov, text string) bool {
	if prov != "" && strings.ToLower(m.Provider) != prov {
		return false
	}
	if typ != "" {
		found := false
		for _, t := range m.TaskType {
			if strings.Contains(strings.ToLower(t), typ) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if text != "" {
		desc := ""
		if m.Description != nil {
			desc = *m.Description
		}
		hay := strings.ToLower(m.Model + "\n" + m.Title + "\n" + desc)
		if !strings.Contains(hay, text) {
			return false
		}
	}
	return true
}

// Find returns the model with the exact id (chat models included).
func (s *Service) Find(ctx context.Context, model string) (Model, bool, error) {
	ms, err := s.List(ctx, false)
	if err != nil {
		return Model{}, false, err
	}
	for _, m := range ms {
		if m.Model == model {
			return m, true, nil
		}
	}
	return Model{}, false, nil
}

// Price returns the pricing description of a model.
func (s *Service) Price(ctx context.Context, model string) (string, error) {
	var data struct {
		PricingDesc string `json:"pricingDesc"`
	}
	req := api.Request{Method: "GET", Path: "/api/v1/models/" + model + "/price", Class: api.ClassDiscovery}
	if err := s.API.Do(ctx, req, &data); err != nil {
		return "", err
	}
	return data.PricingDesc, nil
}

// Health returns the success-rate points of a model as raw JSON.
func (s *Service) Health(ctx context.Context, model string) (json.RawMessage, error) {
	var data struct {
		Points json.RawMessage `json:"points"`
	}
	req := api.Request{Method: "GET", Path: "/api/v1/models/" + model + "/success-rate", Class: api.ClassDiscovery}
	if err := s.API.Do(ctx, req, &data); err != nil {
		return nil, err
	}
	if len(data.Points) == 0 {
		return json.RawMessage("[]"), nil
	}
	return data.Points, nil
}
