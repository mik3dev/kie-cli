// Package result extracts downloadable URLs from a finished task response.
package result

import "encoding/json"

// URLs returns resultUrls followed by lastFrameUrl, deduplicated, in order.
// ponytail: Suno data[].audio_url lands in stage 4.
func URLs(response json.RawMessage) []string {
	var doc struct {
		ResultURLs   json.RawMessage `json:"resultUrls"`
		LastFrameURL json.RawMessage `json:"lastFrameUrl"`
	}
	if err := json.Unmarshal(response, &doc); err != nil {
		return []string{}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, field := range []json.RawMessage{doc.ResultURLs, doc.LastFrameURL} {
		for _, u := range strings(field) {
			if u != "" && !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
		}
	}
	return out
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
