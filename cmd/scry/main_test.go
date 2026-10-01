package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeRejectsSemanticKeyFallbackBeforeOpeningDatabase(t *testing.T) {
	for _, tc := range []struct {
		name, semanticKey, want string
	}{
		{"missing dedicated key", "", "require SCRY_SEMANTIC_API_KEY"},
		{"blank semantic key", "  ", "require SCRY_SEMANTIC_API_KEY"},
		{"short semantic key", "short-key", "bounded non-control"},
		{"control character in semantic key", strings.Repeat("x", 32)+"\n", "bounded non-control"},
		{"generation key copied into semantic binding", "generation-key", "must differ from SCRY_MODEL_API_KEY"},
		{"generic key copied into semantic binding", "mixed-key", "must differ from OPENROUTER_API_KEY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SCRY_MODEL_API_KEY", "generation-key")
			t.Setenv("OPENROUTER_API_KEY", "mixed-key")
			t.Setenv("SCRY_SEMANTIC_ENDPOINT", "https://openrouter.ai/api/alpha/decisions")
			t.Setenv("SCRY_SEMANTIC_API_KEY", tc.semanticKey)
			t.Setenv("SCRY_SEMANTIC_RESERVATION_MICROS", "2000")
			t.Setenv("SCRY_GENERATION_DAILY_BUDGET_MICROS", "3500000")
			t.Setenv("SCRY_GENERATION_RESERVATION_MICROS", "")
			t.Setenv("SCRY_TRUSTED_PROXY_IPS", "")
			t.Setenv("SCRY_BACKUP_INTERVAL", "")
			t.Setenv("SCRY_BACKUP_KEEP", "")
			path := filepath.Join(t.TempDir(), "scry.sqlite")
			err := serve([]string{"--dev", "--db", path, "--addr", "127.0.0.1:0"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("serve error = %v, want %s", err, tc.want)
			}
			if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("invalid credentials must fail before creating the database, stat error = %v", statErr)
			}
		})
	}
}
