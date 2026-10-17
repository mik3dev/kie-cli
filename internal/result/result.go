// Package result extracts downloadable URLs and chaining ids from a finished task response.
package result

import (
	"encoding/json"
	"sort"
	stdstrings "strings"
)

// IDs are the identifiers a finished task exposes to chained follow-up commands.
type IDs struct {
	TaskID       string   `json:"task_id,omitempty"`
	AudioIDs     []string `json:"audio_ids,omitempty"`
	CharacterIDs []string `json:"character_ids,omitempty"`
	LastFrameURL string   `json:"last_frame_url,omitempty"`
}

// track is one entry of a Suno response's data[] array.
type track struct {
	ID       string `json:"id"`
	AudioURL string `json:"audio_url"`
}

// payload is the subset of a task response this package reads.
type payload struct {
	ResultURLs   json.RawMessage `json:"resultUrls"`
	Data         json.RawMessage `json:"data"`
	LastFrameURL json.RawMessage `json:"lastFrameUrl"`
	ResultObject json.RawMessage `json:"resultObject"`
}

// URLs returns resultUrls, then data[].audio_url, then lastFrameUrl, then the media urls of
// resultObject, deduplicated, in order.
func URLs(response json.RawMessage) []string {
	doc := parse(response)
	seen := map[string]bool{}
	out := []string{}
	for _, u := range urlsOf(doc) {
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// Extract returns the chaining ids of a task response: audio ids from data[] and the
// first non-empty lastFrameUrl. Missing or malformed fields are left empty.
func Extract(taskID string, response json.RawMessage) IDs {
	doc := parse(response)
	return IDs{
		TaskID:       taskID,
		AudioIDs:     audioIDs(tracks(doc.Data)),
		LastFrameURL: firstNonEmpty(strings(doc.LastFrameURL)),
	}
}

func parse(response json.RawMessage) payload {
	var doc payload
	if err := json.Unmarshal(response, &doc); err != nil {
		return payload{}
	}
	return doc
}

func urlsOf(doc payload) []string {
	var out []string
	out = append(out, strings(doc.ResultURLs)...)
	for _, t := range tracks(doc.Data) {
		out = append(out, t.AudioURL)
	}
	out = append(out, strings(doc.LastFrameURL)...)
	return append(out, MediaURLs(doc.ResultObject)...)
}

// tracks decodes data[] when it is an array of objects; anything else yields nil.
func tracks(raw json.RawMessage) []track {
	var list []track
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	return list
}

func audioIDs(list []track) []string {
	var out []string
	for _, t := range list {
		if t.ID != "" {
			out = append(out, t.ID)
		}
	}
	return out
}

func firstNonEmpty(list []string) string {
	for _, s := range list {
		if s != "" {
			return s
		}
	}
	return ""
}

// strings decodes a JSON array of strings, or a single string; anything else yields nil.
func strings(raw json.RawMessage) []string {
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}
	}
	return nil
}

// SyncIDs returns the chaining ids of a synchronous response's data object: audioId and
// characterId. Anything missing or malformed is left empty.
func SyncIDs(data json.RawMessage) IDs {
	var doc struct {
		AudioID     string `json:"audioId"`
		CharacterID string `json:"characterId"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return IDs{}
	}
	var ids IDs
	if doc.AudioID != "" {
		ids.AudioIDs = []string{doc.AudioID}
	}
	if doc.CharacterID != "" {
		ids.CharacterIDs = []string{doc.CharacterID}
	}
	return ids
}

// MediaURLs collects the http(s) strings under keys ending in "url" (any case) or inside
// arrays under keys ending in "urls". Keys containing "callback" or "stream" are skipped.
// Keys are walked alphabetically; the result is deduplicated and never nil.
func MediaURLs(v json.RawMessage) []string {
	out := []string{}
	var doc any
	if json.Unmarshal(v, &doc) != nil {
		return out
	}
	seen := map[string]bool{}
	for _, u := range walkMedia(doc, "") {
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// walkMedia descends maps (sorted keys) and arrays; key is the nearest enclosing map key.
func walkMedia(node any, key string) []string {
	switch n := node.(type) {
	case map[string]any:
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out []string
		for _, k := range keys {
			if excludedKey(k) {
				continue
			}
			out = append(out, walkMedia(n[k], k)...)
		}
		return out
	case []any:
		var out []string
		for _, e := range n {
			out = append(out, walkMedia(e, key)...)
		}
		return out
	case string:
		lk := stdstrings.ToLower(key)
		isURL := stdstrings.HasSuffix(lk, "url") || stdstrings.HasSuffix(lk, "urls")
		if isURL && (stdstrings.HasPrefix(n, "http://") || stdstrings.HasPrefix(n, "https://")) {
			return []string{n}
		}
	}
	return nil
}

func excludedKey(k string) bool {
	lk := stdstrings.ToLower(k)
	return stdstrings.Contains(lk, "callback") || stdstrings.Contains(lk, "stream")
}
