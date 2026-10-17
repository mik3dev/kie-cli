// Package kiecli exposes assets embedded in the kie-cli binary.
package kiecli

import "embed"

// Assets holds the factory presets and the bundled agent skill.
//
//go:embed presets/default.yaml skills
var Assets embed.FS
