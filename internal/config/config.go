// Package config resolves the kie-cli home directory and stores credentials.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mik3dev/kie-cli/internal/output"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600
	// minRedactLen is the shortest key of which any suffix may be shown.
	minRedactLen = 12
	visibleTail  = 4
)

// Paths are the locations of every file kie-cli keeps on disk.
type Paths struct {
	Home, Config, Presets, Cache, Ledger string
}

// Config is the persisted user configuration.
type Config struct {
	APIKey string `json:"api_key,omitempty"`
	OutDir string `json:"out_dir,omitempty"`
}

// ResolvePaths uses KIE_CLI_HOME or, when unset, $HOME/.kie-cli.
func ResolvePaths(env func(string) string) (Paths, error) {
	home := env("KIE_CLI_HOME")
	if home == "" {
		h := env("HOME")
		if h == "" {
			return Paths{}, &output.Error{Code: output.CodeIO, Message: "cannot resolve home: set KIE_CLI_HOME or HOME"}
		}
		home = filepath.Join(h, ".kie-cli")
	}
	return Paths{
		Home:    home,
		Config:  filepath.Join(home, "config.json"),
		Presets: filepath.Join(home, "presets.yaml"),
		Cache:   filepath.Join(home, "cache"),
		Ledger:  filepath.Join(home, "tasks.jsonl"),
	}, nil
}

// EnsureHome creates the home directory with mode 0700.
func EnsureHome(p Paths) error {
	if err := os.MkdirAll(p.Home, dirPerm); err != nil {
		return fmt.Errorf("create %s: %w", p.Home, err)
	}
	return nil
}

// Load reads config.json. A missing file yields an empty Config. Loose
// permissions produce a warning on warn but do not fail.
func Load(p Paths, warn io.Writer) (Config, error) {
	fi, err := os.Stat(p.Config)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("stat %s: %w", p.Config, err)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		fmt.Fprintf(warn, "warning: %s permissions are %04o, should be 0600\n", p.Config, perm)
	}
	data, err := os.ReadFile(p.Config)
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", p.Config, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", p.Config, err)
	}
	return c, nil
}

// Save writes config.json atomically (temp file with 0600, then rename).
func Save(p Paths, c Config) error {
	if err := EnsureHome(p); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(p.Home, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if err := tmp.Chmod(filePerm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), p.Config); err != nil {
		return fmt.Errorf("replace %s: %w", p.Config, err)
	}
	return nil
}

// ResolveKey returns the API key from KIE_API_KEY or the config. Both are
// trimmed; whitespace-only counts as unset.
func ResolveKey(env func(string) string, c Config) (key, source string, err error) {
	if k := strings.TrimSpace(env("KIE_API_KEY")); k != "" {
		return k, "env", nil
	}
	if k := strings.TrimSpace(c.APIKey); k != "" {
		return k, "config", nil
	}
	return "", "", &output.Error{Code: output.CodeAuth, Message: "no API key: set KIE_API_KEY or run 'kie-cli auth login'"}
}

// Redact hides a key, keeping only its last 4 characters ("****" if shorter than 12).
func Redact(key string) string {
	r := []rune(key)
	if len(r) < minRedactLen {
		return "****"
	}
	return "…" + string(r[len(r)-visibleTail:])
}
