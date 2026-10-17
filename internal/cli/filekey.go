package cli

import (
	"github.com/mik3dev/kie-cli/internal/schema"
	"github.com/mik3dev/kie-cli/internal/upload"
)

// fileKeyFunc decides which input keys may hold a local file to upload. When the
// schema declares the key, it must be a non-enum string (or array of strings) and
// either look like a file name or carry format uri. Keys the schema does not know,
// or a nil schema (--no-validate), fall back to the name heuristic alone.
func fileKeyFunc(sch map[string]any) func(string) bool {
	return func(k string) bool {
		known, ok := schema.FileCandidate(sch, k)
		if !known {
			return upload.FileKey(k)
		}
		return ok && (upload.FileKey(k) || schema.HasURIFormat(sch, k))
	}
}
