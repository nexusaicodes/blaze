package config_test

import (
	"testing"

	"github.com/sakshamsaxena/blaze/internal/config"
	"github.com/stretchr/testify/require"
)

func validConfig() config.Config {
	return config.Config{
		Repo:      ".",
		Remote:    "origin",
		ModuleDir: ".",
		LogLevel:  "warn",
		Port:      0,
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c config.Config) config.Config
		wantErr string
	}{
		{
			name:    "valid",
			mutate:  func(c config.Config) config.Config { return c },
			wantErr: "",
		},
		{
			name: "PR with base",
			mutate: func(c config.Config) config.Config {
				c.PR = "482"
				c.Base = "main"
				return c
			},
			wantErr: "PR together with --base",
		},
		{
			name: "PR with head",
			mutate: func(c config.Config) config.Config {
				c.PR = "482"
				c.Head = "feature"
				return c
			},
			wantErr: "PR together with --base",
		},
		{
			name: "module-dir absolute",
			mutate: func(c config.Config) config.Config {
				c.ModuleDir = "/etc"
				return c
			},
			wantErr: "--module-dir",
		},
		{
			name: "module-dir contains dot dot",
			mutate: func(c config.Config) config.Config {
				c.ModuleDir = "../escape"
				return c
			},
			wantErr: "--module-dir",
		},
		{
			name: "port too low",
			mutate: func(c config.Config) config.Config {
				c.Port = -1
				return c
			},
			wantErr: "--port",
		},
		{
			name: "port too high",
			mutate: func(c config.Config) config.Config {
				c.Port = 65536
				return c
			},
			wantErr: "--port",
		},
		{
			name: "unknown log level",
			mutate: func(c config.Config) config.Config {
				c.LogLevel = "verbose"
				return c
			},
			wantErr: "unknown log level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.mutate(validConfig())
			err := cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
