package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigCLIAndFailFast(t *testing.T) {
	getenv := func(name string) string {
		if name == "GENTOO_UPDATE_INTELLIGENCE_DATABASE_URL" {
			return "postgres://test:test-only@127.0.0.1/test"
		}
		return ""
	}
	config, err := loadServerConfig(nil, getenv)
	if err != nil || config.Analysis.ConcurrencyLimit != 1 {
		t.Fatal("env-only default startup failed")
	}
	for _, args := range [][]string{{"--unknown"}, {"positional"}, {"--config"}, {"--config", ""}} {
		if _, err := loadServerConfig(args, getenv); err == nil {
			t.Fatal("invalid command line accepted")
		}
	}
	path := filepath.Join(t.TempDir(), "backend.yaml")
	if err := os.WriteFile(path, []byte("http:\n  unexpected: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// No DATABASE_URL or working PostgreSQL is needed to observe this error.
	if err := run([]string{"--config", path}); err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("configuration was not rejected before database startup: %v", err)
	}
}
