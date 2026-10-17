package cli

import "github.com/mik3dev/kie-cli/internal/upload"

// fileKeyFunc binds upload.SchemaFileKey to the input schema. The schema is nil
// only when doc.InputSchema() failed; the name heuristic then applies alone.
func fileKeyFunc(sch map[string]any) func(string) bool {
	return func(k string) bool { return upload.SchemaFileKey(sch, k) }
}
