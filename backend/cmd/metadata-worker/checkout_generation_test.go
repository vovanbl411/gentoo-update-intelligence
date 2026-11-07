package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatagenerator"
)

func TestPooledGeneration(t *testing.T) {
	for _, real := range []bool{false, true} {
		name := "stub"
		if real {
			name = "real"
		}
		t.Run(name, func(t *testing.T) {
			if real && os.Getenv("METADATA_WORKER_TEST_REAL_EGENCACHE") != "1" {
				t.Skip("set METADATA_WORKER_TEST_REAL_EGENCACHE=1 for real Portage acceptance")
			}
			root := t.TempDir()
			repo := filepath.Join(root, "repo")
			write := func(path, content string, mode os.FileMode) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), mode); err != nil {
					t.Fatal(err)
				}
			}
			files := map[string]string{"profiles/repo_name": "pool-test\n", "profiles/test/make.defaults": "ARCH=amd64\n", "profiles/categories": "app-test\n", "metadata/layout.conf": "masters =\nthin-manifests = true\n", ".gitignore": "ignored\n"}
			for _, pkg := range []string{"alpha", "beta", "failure"} {
				files["app-test/"+pkg+"/"+pkg+"-1.ebuild"] = "EAPI=8\nDESCRIPTION=\"pool fixture\"\nHOMEPAGE=\"https://example.invalid\"\nSLOT=0\nLICENSE=MIT\nKEYWORDS=\"~amd64\"\n"
			}
			for path, content := range files {
				write(filepath.Join(repo, path), content, 0o644)
			}
			workerGit(t, repo, "init", "--quiet", "--initial-branch=main")
			workerGit(t, repo, "add", ".")
			workerGit(t, repo, "commit", "-qm", "fixture")
			commit := workerGit(t, repo, "rev-parse", "HEAD")
			config := metadatagenerator.Config{}
			if !real {
				bin := filepath.Join(root, "bin")
				write(filepath.Join(bin, "emerge"), "#!/bin/sh\nprintf 'Portage 3.0.1 test\\n'\n", 0o755)
				write(filepath.Join(bin, "egencache"), "#!/bin/sh\nprintf changed > profiles/test/make.defaults\nprintf dirty > ignored\nprintf dirty > untracked\nfor pkg in alpha beta; do mkdir -p metadata/md5-cache/app-test; printf 'EAPI=8\\nSLOT=0\\n' > metadata/md5-cache/app-test/$pkg-1; done\ncase \"$*\" in *app-test/failure*) exit 1;; esac\n", 0o755)
				config.EgencachePath = filepath.Join(bin, "egencache")
				config.EmergePath = filepath.Join(bin, "emerge")
			} else {
				if _, err := exec.LookPath("egencache"); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("METADATA_WORKER_CACHE_PATH", filepath.Join(root, "cache"))
			t.Setenv("METADATA_WORKER_TEMP_PATH", root)
			pool := testPool(t, defaultCheckoutCacheMaxBytes)
			generator := metadatagenerator.New(config)
			slots := make(chan struct{}, 2)
			input := request{Repository: repository{Name: "pool-test", URL: "local", Path: repo, Commit: commit}, Category: "app-test", Name: "alpha", Version: "1", Profile: "profiles/test"}
			var checkoutRoot, generationConfig string
			for _, pkg := range []string{"alpha", "beta"} {
				input.Name = pkg
				result, cached, _, err := prepareMetadataWithPool(input, slots, generator, pool)
				if err != nil || cached {
					t.Fatalf("%s: cached=%v err=%v", pkg, cached, err)
				}
				if len(pool.idle) != 1 {
					t.Fatal("idle count", len(pool.idle))
				}
				if checkoutRoot != "" && checkoutRoot != pool.idle[0].root {
					t.Fatal("different packages did not reuse")
				}
				checkoutRoot = pool.idle[0].root
				configRoot := ""
				for i, arg := range result.Provenance.EgencacheArgs {
					if arg == "--config-root" && i+1 < len(result.Provenance.EgencacheArgs) {
						configRoot = result.Provenance.EgencacheArgs[i+1]
						break
					}
				}
				if configRoot == "" || configRoot == generationConfig {
					t.Fatal("generation config root is missing or reused")
				}
				if _, err := os.Stat(configRoot); !os.IsNotExist(err) {
					t.Fatalf("generation directory was not removed: %v", err)
				}
				generationConfig = configRoot
				for _, path := range pool.idle[0].paths {
					if state := workerGit(t, path, "status", "--porcelain", "--untracked-files=all", "--ignored"); state != "" {
						t.Fatal(state)
					}
				}
			}
			if !real {
				input.Name = "failure"
				if _, _, _, err := prepareMetadataWithPool(input, slots, generator, pool); err == nil {
					t.Fatal("expected generation failure")
				}
				if len(pool.idle) != 1 || pool.idle[0].root != checkoutRoot {
					t.Fatal("failed generation did not restore set")
				}
				for _, path := range pool.idle[0].paths {
					if state := workerGit(t, path, "status", "--porcelain", "--untracked-files=all", "--ignored"); state != "" {
						t.Fatalf("dirty checkout after failed generation: %s", state)
					}
				}
			}
			// Cache hits remain available even if subsequent allocations are blocked.
			pool.mu.Lock()
			pool.blocked = context.Canceled
			pool.mu.Unlock()
			input.Name = "alpha"
			if _, cached, _, err := prepareMetadataWithPool(input, slots, generator, pool); err != nil || !cached {
				t.Fatal("cache hit blocked", cached, err)
			}
		})
	}
}
