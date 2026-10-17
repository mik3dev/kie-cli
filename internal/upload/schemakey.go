package upload

import "github.com/mik3dev/kie-cli/internal/schema"

// SchemaFileKey decides whether input key k may hold a local file to upload.
// A key the schema declares qualifies when any part of its subtree has format
// uri (covers nested objects and arrays of objects), or when it is a non-enum
// string / array of strings whose name looks like a file. Keys the schema does
// not know, or a nil schema (InputSchema failed), use the name heuristic alone.
func SchemaFileKey(sch map[string]any, k string) bool {
	known, ok := schema.FileCandidate(sch, k)
	if !known {
		return FileKey(k)
	}
	if schema.HasURIFormat(sch, k) {
		return true
	}
	return ok && FileKey(k)
}
