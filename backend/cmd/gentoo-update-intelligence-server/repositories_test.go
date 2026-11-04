package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
)

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	args = append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, args...)
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func fixtureRepo(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "metadata/md5-cache/dev-libs"), 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "init", "--quiet", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(dir, "metadata/md5-cache/dev-libs/widget-1"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-qm", "fixture")
	return fixtureGit(t, dir, "rev-parse", "HEAD")
}

func TestPrepareRepositoriesPinsAndRoutesPlan(t *testing.T) {
	root := t.TempDir()
	gentooPath := filepath.Join(root, "gentoo")
	gentooCommit := fixtureRepo(t, gentooPath, "EAPI=8\nSLOT=0\nIUSE=gentoo\n")
	overlayPath := filepath.Join(root, "overlay")
	overlayCommit := fixtureRepo(t, overlayPath, "EAPI=8\nSLOT=0\nIUSE=overlay\n")
	config := metadataprovider.Config{Repository: "gentoo", SourceURL: "https://example.invalid/gentoo", LocalPath: gentooPath, Commit: gentooCommit}
	provider, err := metadataprovider.Open(config)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := repositorymanager.New(filepath.Join(root, "mirrors"), []repositorymanager.Repository{{Name: "overlay", URL: overlayPath, Masters: []string{"gentoo"}}})
	if err != nil {
		t.Fatal(err)
	}
	plan := parser.Parse("[ebuild U] dev-libs/widget-1::overlay [1::gentoo]")
	pinned := prepareRepositories(context.Background(), plan, config, provider, manager).(*pinnedRepositories)
	if pinned.revisions["overlay"].Commit != overlayCommit || pinned.revisions["gentoo"].Commit != gentooCommit {
		t.Fatalf("revisions: %#v", pinned.revisions)
	}
	if got := pinned.revisions["overlay"].Masters; len(got) != 1 || got[0] != "gentoo" {
		t.Fatalf("masters: %v", got)
	}
	query := metadataprovider.Query{Repository: "overlay", Category: "dev-libs", Name: "widget", Version: "1"}
	result, err := pinned.Lookup(query)
	if err != nil || result.Status != metadataprovider.StatusFound || result.Snapshot.Source.Commit != overlayCommit {
		t.Fatalf("lookup = %#v, %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(overlayPath, "metadata/md5-cache/dev-libs/widget-1"), []byte("EAPI=8\nSLOT=0\nIUSE=updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, overlayPath, "add", ".")
	fixtureGit(t, overlayPath, "commit", "-qm", "updated")
	updatedCommit := fixtureGit(t, overlayPath, "rev-parse", "HEAD")
	stillOld, err := pinned.Lookup(query)
	if err != nil || stillOld.Snapshot == nil || stillOld.Snapshot.Source.Commit != overlayCommit {
		t.Fatalf("old analysis after update = %#v, %v", stillOld, err)
	}
	newAnalysis := prepareRepositories(context.Background(), plan, config, provider, manager).(*pinnedRepositories)
	newResult, err := newAnalysis.Lookup(query)
	if err != nil || newResult.Snapshot == nil || newResult.Snapshot.Source.Commit != updatedCommit {
		t.Fatalf("new analysis after update = %#v, %v", newResult, err)
	}
	query.Repository = "unknown"
	result, err = pinned.Lookup(query)
	if !errors.Is(err, repositorymanager.ErrUnknownRepository) {
		t.Fatalf("unknown = %#v, %v", result, err)
	}
}

func TestOverlayFailureDoesNotBlockGentooLookup(t *testing.T) {
	root := t.TempDir()
	gentooPath := filepath.Join(root, "gentoo")
	commit := fixtureRepo(t, gentooPath, "EAPI=8\nSLOT=0\nIUSE=gentoo\n")
	config := metadataprovider.Config{Repository: "gentoo", SourceURL: "https://example.invalid/gentoo", LocalPath: gentooPath, Commit: commit}
	provider, err := metadataprovider.Open(config)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := repositorymanager.New(filepath.Join(root, "mirrors"), []repositorymanager.Repository{{Name: "overlay", URL: filepath.Join(root, "missing"), Masters: []string{"gentoo"}}})
	if err != nil {
		t.Fatal(err)
	}
	plan := parser.Parse("[ebuild U] dev-libs/widget-1::overlay [1::gentoo]")
	pinned := prepareRepositories(context.Background(), plan, config, provider, manager)
	query := metadataprovider.Query{Repository: "overlay", Category: "dev-libs", Name: "widget", Version: "1"}
	if _, err := pinned.Lookup(query); err == nil {
		t.Fatal("failed overlay returned no error")
	}
	query.Repository = "gentoo"
	result, err := pinned.Lookup(query)
	if err != nil || result.Status != metadataprovider.StatusFound {
		t.Fatalf("gentoo lookup = %#v, %v", result, err)
	}
}
