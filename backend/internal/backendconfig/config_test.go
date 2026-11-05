package backendconfig

import (
	"context"
	"errors"
	"os"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func environment(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func testEnvironment(values map[string]string) func(string) string {
	result := map[string]string{Prefix + "DATABASE_URL": "postgres://test:test-only@127.0.0.1/test"}
	for key, value := range values {
		result[Prefix+key] = value
	}
	return environment(result)
}

func configFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backend.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultsAndPartialFile(t *testing.T) {
	config, err := Load("", testEnvironment(nil))
	if err != nil {
		t.Fatal(err)
	}
	if config.HTTP.ListenAddress != "127.0.0.1:8080" || config.Analysis.ConcurrencyLimit != 1 || config.Metadata.WorkerURL != "" || config.Overlays.Path != "" || config.Metadata.WorkerTimeout != Duration(20*time.Minute) {
		t.Fatalf("unexpected non-secret defaults")
	}
	config, err = Load(configFile(t, "analysis:\n  concurrency_limit: 0\nhttp:\n  idle_timeout: 90s\n"), testEnvironment(nil))
	if err != nil {
		t.Fatal(err)
	}
	if config.Analysis.ConcurrencyLimit != 0 || config.HTTP.IdleTimeout != Duration(90*time.Second) || config.HTTP.ReadHeaderTimeout != Duration(5*time.Second) {
		t.Fatal("partial YAML did not preserve defaults or explicit zero")
	}
	if _, err := Load("", environment(nil)); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatal("missing database env accepted")
	}
}

func TestEnvironmentOverridesAndEmptyValues(t *testing.T) {
	path := configFile(t, `startup_timeout: 22s
http:
  listen_address: "127.0.0.1:9090"
  read_header_timeout: 7s
  idle_timeout: 90s
analysis:
  concurrency_limit: 4
repository:
  name: gentoo
  source_url: "https://example.invalid/yaml"
  path: "relative-repository"
  commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
overlays:
  path: "relative-overlays"
  catalog:
    - name: yaml-overlay
      url: "https://example.invalid/yaml-overlay"
      masters: [gentoo]
metadata:
  worker_url: "http://yaml-worker:8090"
  worker_timeout: 25m
  profile: "profiles/yaml"
`)
	config, err := Load(path, testEnvironment(map[string]string{
		"LISTEN_ADDR": "127.0.0.1:9191", "STARTUP_TIMEOUT": "30s", "HTTP_READ_HEADER_TIMEOUT": "9s", "HTTP_IDLE_TIMEOUT": "100s", "METADATA_WORKER_TIMEOUT": "3m",
		"ANALYSIS_CONCURRENCY_LIMIT": "0", "REPOSITORY": "gentoo", "SOURCE_URL": "https://example.invalid/env", "REPOSITORY_PATH": "/env-repo", "COMMIT": strings.Repeat("b", 40),
		"OVERLAY_PATH": "/env-overlays", "OVERLAY_CATALOG": `[{"name":"env-overlay","url":"https://example.invalid/env-overlay","masters":["gentoo"]}]`,
		"METADATA_WORKER_URL": "http://env-worker:8090", "METADATA_PROFILE": "profiles/env",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if config.HTTP.ListenAddress != "127.0.0.1:9191" || config.StartupTimeout != Duration(30*time.Second) || config.HTTP.ReadHeaderTimeout != Duration(9*time.Second) || config.HTTP.IdleTimeout != Duration(100*time.Second) || config.Metadata.WorkerTimeout != Duration(3*time.Minute) || config.Analysis.ConcurrencyLimit != 0 {
		t.Fatal("numeric or HTTP env overrides were lost")
	}
	if config.Repository.SourceURL != "https://example.invalid/env" || config.Repository.Path != "/env-repo" || config.Repository.Commit != strings.Repeat("b", 40) || config.Overlays.Path != "/env-overlays" || config.Metadata.WorkerURL != "http://env-worker:8090" || config.Metadata.Profile != "profiles/env" {
		t.Fatal("string env overrides were lost")
	}
	if len(config.Overlays.Catalog) != 1 || config.Overlays.Catalog[0].Name != "env-overlay" {
		t.Fatal("env catalog did not replace YAML additions")
	}
	config, err = Load(path, testEnvironment(map[string]string{"LISTEN_ADDR": "", "ANALYSIS_CONCURRENCY_LIMIT": "", "OVERLAY_CATALOG": "", "METADATA_WORKER_URL": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if config.HTTP.ListenAddress != "127.0.0.1:9090" || config.Analysis.ConcurrencyLimit != 4 || config.Metadata.WorkerURL != "http://yaml-worker:8090" || config.Overlays.Catalog[0].Name != "yaml-overlay" {
		t.Fatal("empty env erased YAML")
	}
	// No environment interpolation; relative paths keep their process-CWD meaning.
	config, err = Load(configFile(t, "repository:\n  path: '${HOME}/literal'\nmetadata:\n  worker_url: ''\noverlays:\n  path: ''\n"), testEnvironment(nil))
	if err != nil || config.Repository.Path != "${HOME}/literal" || config.Metadata.WorkerURL != "" || config.Overlays.Path != "" {
		t.Fatal("literal paths or explicit disable changed")
	}
}

func TestStrictYAMLRejectsInvalidFilesWithoutLeakingValues(t *testing.T) {
	for name, content := range map[string]string{
		"empty": "", "null": "null", "list": "[]", "scalar": "text", "two documents": "{}\n---\n{}", "trailing empty document": "{}\n---\n",
		"unknown root": "database_url: DO_NOT_PRINT_THIS_SECRET", "unknown nested": "http:\n  unexpected: DO_NOT_PRINT_THIS_SECRET",
		"duplicate":       "analysis:\n  concurrency_limit: 1\n  concurrency_limit: 2",
		"wrong scalar":    "analysis:\n  concurrency_limit: DO_NOT_PRINT_THIS_SECRET",
		"string coercion": "metadata:\n  profile: 123",
		"boolean string":  "repository:\n  name: true",
		"catalog type":    "overlays:\n  catalog:\n    - name: true\n      url: https://example.invalid/repo",
		"float":           "analysis:\n  concurrency_limit: 1.5", "quoted integer": "analysis:\n  concurrency_limit: '2'", "boolean": "analysis:\n  concurrency_limit: true",
		"overflow": "analysis:\n  concurrency_limit: 9999999999999999999999999999", "negative": "analysis:\n  concurrency_limit: -1",
		"no duration unit": "startup_timeout: '15'", "duration integer": "startup_timeout: 15", "zero duration": "startup_timeout: 0s", "negative duration": "http:\n  idle_timeout: -1s",
		"null setting": "metadata:\n  worker_url: null", "syntax": "http: [",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(configFile(t, content), testEnvironment(map[string]string{"ANALYSIS_CONCURRENCY_LIMIT": "1"}))
			// Semantic negative values may legitimately be replaced by env;
			// test negative YAML without that override separately below.
			if name == "negative" {
				_, err = Load(configFile(t, content), testEnvironment(nil))
			}
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if strings.Contains(err.Error(), "DO_NOT_PRINT_THIS_SECRET") {
				t.Fatal("configuration value leaked in diagnostic")
			}
		})
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml"), testEnvironment(nil)); err == nil {
		t.Fatal("missing explicit file accepted")
	}
	if _, err := Load(t.TempDir(), testEnvironment(nil)); err == nil {
		t.Fatal("directory accepted as configuration file")
	}
}

func TestInvalidEnvironmentAndEffectiveSettings(t *testing.T) {
	for key, value := range map[string]string{
		"ANALYSIS_CONCURRENCY_LIMIT": "1.5", "STARTUP_TIMEOUT": "0s", "HTTP_READ_HEADER_TIMEOUT": "-1s", "HTTP_IDLE_TIMEOUT": "0", "METADATA_WORKER_TIMEOUT": "bad",
		"LISTEN_ADDR": "not-an-address", "REPOSITORY": "-invalid", "COMMIT": "short", "METADATA_WORKER_URL": "http://user:DO_NOT_PRINT_THIS_SECRET@example.invalid",
	} {
		t.Run(key, func(t *testing.T) {
			_, err := Load("", testEnvironment(map[string]string{key: value}))
			if err == nil || strings.Contains(err.Error(), "DO_NOT_PRINT_THIS_SECRET") {
				t.Fatal("invalid environment accepted or leaked")
			}
		})
	}
	for _, value := range []string{"-1", "9999999999999999999999999"} {
		if _, err := Load("", testEnvironment(map[string]string{"ANALYSIS_CONCURRENCY_LIMIT": value})); err == nil {
			t.Fatal("invalid integer accepted")
		}
	}
	for _, value := range []string{"null", "{}", "[] []", `[{"name":"test","url":"https://user:DO_NOT_PRINT_THIS_SECRET@example.invalid/repo"}]`, `[{"name":"test","url":"https://example.invalid/repo","extra":true}]`, `[{"name":"test","url":"https://example.invalid/repo","masters":["unknown"]}]`} {
		_, err := Load("", testEnvironment(map[string]string{"OVERLAY_CATALOG": value}))
		if err == nil || strings.Contains(err.Error(), "DO_NOT_PRINT_THIS_SECRET") {
			t.Fatal("invalid catalog accepted or leaked")
		}
	}
}

func TestCatalogKeepsGuruAndAmbiguity(t *testing.T) {
	config, err := Load("", testEnvironment(map[string]string{"OVERLAY_PATH": filepath.Join(t.TempDir(), "not-created"), "OVERLAY_CATALOG": `[{"name":"guru","url":"https://example.invalid/another-guru","masters":["gentoo"]}]`}))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := config.OverlayManager()
	if err != nil || manager == nil {
		t.Fatal("ambiguous catalog should remain constructible")
	}
	_, err = manager.PinAdditional(context.Background(), []string{"guru"}, map[string]repositorymanager.Revision{"gentoo": {Name: "gentoo", Commit: strings.Repeat("a", 40), Path: "unused"}})
	if !errors.Is(err, repositorymanager.ErrAmbiguousRepository) {
		t.Fatalf("built-in guru was not preserved as an ambiguous name: %v", err)
	}

	if _, err := os.Stat(config.Overlays.Path); !os.IsNotExist(err) {
		t.Fatal("validation touched overlay storage")
	}
}

func TestShippedComposeConfig(t *testing.T) {
	config, err := Load("../../../config/backend.yaml", testEnvironment(nil))
	if err != nil {
		t.Fatal(err)
	}
	if config.HTTP.ListenAddress != "0.0.0.0:8080" || config.Overlays.Path != "/overlays" || config.Metadata.WorkerURL != "http://metadata-worker:8090" {
		t.Fatal("Compose file settings are wrong")
	}
}
