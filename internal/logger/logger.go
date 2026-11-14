// Package logger provides a single global zerolog logger behind an atomic
// pointer, configured once at startup and read by every other package.
package logger

import (
	"fmt"
	"os"
	"sync/atomic"

	"github.com/rs/zerolog"
)

var current atomic.Pointer[zerolog.Logger]

// Init configures the global logger to write to stderr through a
// zerolog.ConsoleWriter at the given level. level is one of "debug", "info",
// "warn", "error". It returns an error for any other value.
func Init(level string) error {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("parse log level: %w", err)
	}
	l := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(lvl).With().Timestamp().Logger()
	current.Store(&l)
	return nil
}

// InitNop configures the global logger to discard everything. Tests call it
// in TestMain.
func InitNop() {
	l := zerolog.Nop()
	current.Store(&l)
}

// Get returns the current global logger. It panics if neither Init nor
// InitNop has been called.
func Get() *zerolog.Logger {
	l := current.Load()
	if l == nil {
		panic("logger: Get called before Init or InitNop")
	}
	return l
}
