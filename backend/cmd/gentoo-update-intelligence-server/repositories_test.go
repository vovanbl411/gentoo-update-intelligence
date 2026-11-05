package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatagenerator"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
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

func TestPinnedLookupUsesExactCacheBeforeWorkerAndSendsPinnedMasterCommits(t *testing.T) {
	root := t.TempDir()
	gentooPath := filepath.Join(root, "gentoo")
	gentooCommit := fixtureRepo(t, gentooPath, "EAPI=8\nSLOT=0\nIUSE=gentoo\n")
	overlayPath := filepath.Join(root, "overlay")
	selectedCommit := fixtureEmptyRepo(t, overlayPath)
	pinnedCommit := fixtureGit(t, overlayPath, "commit", "--allow-empty", "-qm", "newer revision")
	config := metadataprovider.Config{Repository: "gentoo", SourceURL: "https://example.invalid/gentoo", LocalPath: gentooPath, Commit: gentooCommit}
	gentoo, err := metadataprovider.Open(config)
	if err != nil {
		t.Fatal(err)
	}
	plan := parser.Parse("[ebuild U] dev-libs/widget-1::overlay [1::gentoo]")
	var calls int
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var input struct {
			Masters []struct {
				Name   string
				Commit string
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if len(input.Masters) != 1 || input.Masters[0].Name != "gentoo" || input.Masters[0].Commit != gentooCommit {
			t.Errorf("worker masters = %#v", input.Masters)
		}
		content := []byte("EAPI=8\nSLOT=0\nIUSE=generated\n")
		digest := sha256.Sum256(content)
		result := metadatagenerator.Result{Content: content, SHA256: hex.EncodeToString(digest[:]), Provenance: metadatagenerator.Provenance{CachePath: "metadata/md5-cache/dev-libs/widget-1", SelectedCommit: selectedCommit, MasterRevisions: []metadatagenerator.MasterRevision{{Name: "gentoo", Commit: gentooCommit}}}}
		data, _ := json.Marshal(map[string]any{"result": result, "cached": false, "source_commit": selectedCommit})
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	manager, err := repositorymanager.New(filepath.Join(root, "mirrors"), []repositorymanager.Repository{{Name: "overlay", URL: overlayPath, Masters: []string{"gentoo"}}})
	if err != nil {
		t.Fatal(err)
	}
	pinned := prepareRepositories(context.Background(), plan, config, gentoo, manager).(*pinnedRepositories)
	pinned.workerURL = "http://metadata-worker/"
	pinned.workerClient = &http.Client{Transport: transport}
	query := metadataprovider.Query{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "1"}
	// The exact pinned Gentoo cache is returned without contacting the worker.
	if got, err := pinned.Lookup(query); err != nil || got.Status != metadataprovider.StatusFound || calls != 0 {
		t.Fatalf("cache lookup = %#v, %v, calls=%d", got, err, calls)
	}
	query.Repository = "overlay"
	got, err := pinned.Lookup(query)
	if err != nil || got.Status != metadataprovider.StatusFound || got.Provenance == nil || got.Snapshot.Source.Commit != selectedCommit || calls != 1 {
		t.Fatalf("generated lookup = %#v, %v, calls=%d", got, err, calls)
	}
	if got.Snapshot.Source.Commit == pinnedCommit {
		t.Fatalf("historical cache source commit = pinned commit %s", pinnedCommit)
	}
	if got.Provenance.MasterRevisions[0].Commit != gentooCommit {
		t.Fatalf("provenance masters = %#v", got.Provenance.MasterRevisions)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

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

func fixtureEmptyRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "init", "--quiet", "--initial-branch=main")
	fixtureGit(t, dir, "commit", "--allow-empty", "-qm", "fixture")
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

func TestResumableProviderFactoryDoesNotReusePinnedCommit(t *testing.T) {
	root := t.TempDir()
	gentooPath := filepath.Join(root, "gentoo")
	oldCommit := fixtureRepo(t, gentooPath, "EAPI=8\nSLOT=0\nIUSE=old\n")
	fixtureGit(t, gentooPath, "update-ref", "refs/gentoo/published", oldCommit)
	config := metadataprovider.Config{Repository: "gentoo", SourceURL: "https://example.invalid/gentoo", LocalPath: gentooPath}
	factory := newResumableProviderFactory(config, nil, workerSettings{})
	plan := parser.Parse("[ebuild U] dev-libs/widget-1::gentoo")
	pinned, err := factory(context.Background(), plan, map[string]repositorymanager.Revision{
		"gentoo": {Name: "gentoo", URL: config.SourceURL, Commit: oldCommit},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := pinned.(interface {
		RepositoryRevisions() map[string]repositorymanager.Revision
	}).RepositoryRevisions()["gentoo"].Commit; got != oldCommit {
		t.Fatalf("resumed commit = %s, want %s", got, oldCommit)
	}

	cachePath := filepath.Join(gentooPath, "metadata/md5-cache/dev-libs/widget-1")
	if err := os.WriteFile(cachePath, []byte("EAPI=8\nSLOT=0\nIUSE=new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, gentooPath, "add", ".")
	fixtureGit(t, gentooPath, "commit", "-qm", "published update")
	newCommit := fixtureGit(t, gentooPath, "rev-parse", "HEAD")
	fixtureGit(t, gentooPath, "update-ref", "refs/gentoo/published", newCommit)

	newAnalysis, err := factory(context.Background(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := newAnalysis.(interface {
		RepositoryRevisions() map[string]repositorymanager.Revision
	}).RepositoryRevisions()["gentoo"].Commit; got != newCommit {
		t.Fatalf("new analysis commit = %s, want newly published %s", got, newCommit)
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

func TestNilOverlayManagerReportsUnavailableSideAndKeepsGentooLookup(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "gentoo")
	commit := fixtureRepo(t, path, "EAPI=8\nSLOT=0\nIUSE=gentoo\n")
	config := metadataprovider.Config{Repository: "gentoo", SourceURL: "https://example.invalid/gentoo", LocalPath: path, Commit: commit}
	provider, err := metadataprovider.Open(config)
	if err != nil {
		t.Fatal(err)
	}
	plan := parser.Parse("[ebuild U] dev-libs/widget-1::not-configured [1::gentoo]")
	pinned := prepareRepositories(context.Background(), plan, config, provider, nil)
	report, err := reportservice.Build(plan, 0, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if report.BeforeLookup.Result == nil || report.BeforeLookup.Result.Status != metadataprovider.StatusFound {
		t.Fatalf("gentoo before lookup = %#v", report.BeforeLookup)
	}
	if report.AfterLookup.Err == nil || !strings.Contains(report.AfterLookup.Err.Error(), "overlay preparation is disabled") {
		t.Fatalf("unconfigured overlay error = %v", report.AfterLookup.Err)
	}
}

// Fixture-only convenience for tests that do not enable the metadata worker.
func prepareRepositories(ctx context.Context, plan parser.Result, config metadataprovider.Config, gentoo *metadataprovider.Provider, manager *repositorymanager.Manager) reportservice.MetadataLookup {
	result, _ := preparePinnedRepositories(ctx, plan, config, gentoo, manager, nil, workerSettings{})
	return result
}
