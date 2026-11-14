// Package config holds the Config struct shared by the review and plan
// commands, and the validation rules common to both.
package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Config holds one field per CLI flag common to `review` and `plan`, the
// flags specific to `review`, and the optional PR argument.
type Config struct {
	Repo      string // --repo
	Base      string // --base
	Head      string // --head
	Remote    string // --remote
	ModuleDir string // --module-dir
	LogLevel  string // --log-level
	PR        string // positional <pr> argument

	// review only
	Port    int  // --port
	NoOpen  bool // --no-open
	NoState bool // --no-state
}

// logLevels are the values accepted by --log-level.
var logLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// Validate rejects:
//   - PR together with --base or --head
//   - a --module-dir that is absolute or contains ..
//   - a port outside 0-65535
//   - an unknown log level
func (c Config) Validate() error {
	if c.PR != "" && (c.Base != "" || c.Head != "") {
		return fmt.Errorf("PR together with --base or --head: pass either a PR or --base/--head, not both")
	}
	if filepath.IsAbs(c.ModuleDir) {
		return fmt.Errorf("--module-dir must be relative to the repo root, got %q", c.ModuleDir)
	}
	if moduleDirHasDotDot(c.ModuleDir) {
		return fmt.Errorf("--module-dir must not contain \"..\", got %q", c.ModuleDir)
	}
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("--port must be between 0 and 65535, got %d", c.Port)
	}
	if !logLevels[c.LogLevel] {
		return fmt.Errorf("unknown log level %q: want debug, info, warn or error", c.LogLevel)
	}
	return nil
}

// moduleDirHasDotDot reports whether any slash-separated element of dir is
// "..".
func moduleDirHasDotDot(dir string) bool {
	for _, part := range strings.Split(filepath.ToSlash(dir), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
