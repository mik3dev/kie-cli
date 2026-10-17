package cli

import "runtime/debug"

// ResolveVersion exposes resolveVersion to external tests.
func ResolveVersion() string { return resolveVersion() }

// SetVersionSourcesForTest swaps Version and readBuildInfo; returns a restore func.
func SetVersionSourcesForTest(v string, rb func() (*debug.BuildInfo, bool)) func() {
	oldV, oldRB := Version, readBuildInfo
	Version, readBuildInfo = v, rb
	return func() { Version, readBuildInfo = oldV, oldRB }
}
