package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func workerGit(t *testing.T, path string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", path, "-c", "user.name=Worker Test", "-c", "user.email=worker@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestHistoricalExactCacheIsReturnedWithoutGeneration(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repositoryPath, "dev-libs/widget"), 0o755); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(repositoryPath, "metadata/md5-cache/dev-libs/widget-1")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	workerGit(t, repositoryPath, "init", "--quiet", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(repositoryPath, "dev-libs/widget/widget-1.ebuild"), []byte("EAPI=8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cacheContent := "EAPI=8\nSLOT=0\nIUSE=historical\n"
	if err := os.WriteFile(cachePath, []byte(cacheContent), 0o644); err != nil {
		t.Fatal(err)
	}
	workerGit(t, repositoryPath, "add", ".")
	workerGit(t, repositoryPath, "commit", "-qm", "version with cache")
	selectedCommit := workerGit(t, repositoryPath, "rev-parse", "HEAD")
	if err := os.Remove(filepath.Join(repositoryPath, "dev-libs/widget/widget-1.ebuild")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cachePath); err != nil {
		t.Fatal(err)
	}
	workerGit(t, repositoryPath, "add", "--all")
	workerGit(t, repositoryPath, "commit", "-qm", "remove old version")
	referenceCommit := workerGit(t, repositoryPath, "rev-parse", "HEAD")

	result, cached, cacheCommit, err := prepareMetadata(request{
		Repository: repository{Name: "overlay", URL: "https://example.invalid/overlay", Path: repositoryPath, Commit: referenceCommit},
		Category:   "dev-libs", Name: "widget", Version: "1", Profile: "gentoo:profiles/test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cached {
		t.Fatal("historical Git cache was reported as generated Store cache")
	}
	if cacheCommit != selectedCommit {
		t.Fatalf("cache commit = %s, want selected historical commit %s", cacheCommit, selectedCommit)
	}
	if string(result.Content) != cacheContent {
		t.Fatalf("metadata = %q, want %q", result.Content, cacheContent)
	}
	if result.Provenance.ReferenceCommit != referenceCommit || result.Provenance.SelectedCommit != selectedCommit || result.Provenance.SelectionReason != "first_parent_history" {
		t.Fatalf("historical cache provenance = %#v", result.Provenance)
	}
}
