// Package result extracts downloadable URLs and chaining ids from a finished task response.
package result

import "encoding/json"

// IDs are the identifiers a finished task exposes to chained follow-up commands.
type IDs struct {
	TaskID       string   `json:"task_id"`
	AudioIDs     []string `json:"audio_ids,omitempty"`
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
}

// URLs returns resultUrls, then data[].audio_url, then lastFrameUrl, deduplicated, in order.
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
	return append(out, strings(doc.LastFrameURL)...)
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
