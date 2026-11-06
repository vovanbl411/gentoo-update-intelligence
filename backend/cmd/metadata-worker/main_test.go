package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatacache"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatagenerator"
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

func TestPinnedTreeProfileOnlyAcceptsGitDirectories(t *testing.T) {
	root := t.TempDir()
	workerGit(t, root, "init", "--quiet", "--initial-branch=main")
	if err := os.MkdirAll(filepath.Join(root, "profiles", "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "profiles", "real", "make.defaults"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(root, "profiles", "alias")); err != nil {
		t.Fatal(err)
	}
	workerGit(t, root, "add", ".")
	workerGit(t, root, "commit", "-qm", "profiles")
	commit := workerGit(t, root, "rev-parse", "HEAD")
	input := request{Repository: repository{Name: "gentoo", Path: root, Commit: commit}}
	for _, tc := range []struct {
		profile, key string
		ok           bool
	}{
		{"profiles/real", "profiles/real", true},
		{"gentoo:profiles/real", "profiles/real", true},
		{"profiles/alias", "", false},
		{"profiles/alias/make.defaults", "", false},
		{"profiles/real/make.defaults", "", false},
		{"../profiles/real", "", false},
		{"/profiles/real", "", false},
	} {
		input.Profile = tc.profile
		key, ok := pinnedTreeProfile(input, commit)
		if key != tc.key || ok != tc.ok {
			t.Errorf("profile %q: got %q, %v", tc.profile, key, ok)
		}
	}
	input.Masters = []repository{{Name: "master", Path: root, Commit: commit}}
	input.Profile = "master:profiles/real"
	if key, ok := pinnedTreeProfile(input, commit); !ok || key != input.Profile {
		t.Fatalf("master profile: %q, %v", key, ok)
	}
}

func TestGeneratedCacheHitDoesNotCreateCheckout(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	for _, dir := range []string{"profiles/test", "dev-libs/widget"} {
		if err := os.MkdirAll(filepath.Join(repo, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "profiles/test/make.defaults"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "dev-libs/widget/widget-1.ebuild"), []byte("EAPI=8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workerGit(t, repo, "init", "--quiet", "--initial-branch=main")
	workerGit(t, repo, "add", ".")
	workerGit(t, repo, "commit", "-qm", "fixture")
	commit := workerGit(t, repo, "rev-parse", "HEAD")
	blob := workerGit(t, repo, "rev-parse", "HEAD:dev-libs/widget/widget-1.ebuild")
	cache, err := metadatacache.NewStore(filepath.Join(root, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	inputs := metadatacache.Inputs{Repository: "gentoo", SourceURL: "https://example.invalid/repo", Category: "dev-libs", Name: "widget", Version: "1", ReferenceCommit: commit, SourceCommit: commit, EbuildBlob: blob, Profile: "profiles/test", GeneratorVersion: "metadata-worker-v1", PortageVersion: "3.0"}
	content := []byte("EAPI=8\nSLOT=0\n")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	result := metadatagenerator.Result{Content: content, SHA256: digest, Provenance: metadatagenerator.Provenance{ReferenceCommit: commit, SelectedCommit: commit, EbuildBlobID: blob, Profile: inputs.Profile, PortageVersion: inputs.PortageVersion, EbuildPath: "dev-libs/widget/widget-1.ebuild", CachePath: "metadata/md5-cache/dev-libs/widget-1", CacheSHA256: digest}}
	if err := cache.Save(inputs, result); err != nil {
		t.Fatal(err)
	}
	emerge := filepath.Join(root, "emerge")
	if err := os.WriteFile(emerge, []byte("#!/bin/sh\nprintf 'Portage 3.0 test\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMERGE_PATH", emerge)
	t.Setenv("METADATA_WORKER_CACHE_PATH", filepath.Join(root, "cache"))
	// A nonexistent temp root makes any checkout attempt fail.
	t.Setenv("METADATA_WORKER_TEMP_PATH", filepath.Join(root, "missing"))
	got, cached, _, err := prepareMetadata(request{Repository: repository{Name: "gentoo", URL: inputs.SourceURL, Path: repo, Commit: commit}, Category: inputs.Category, Name: inputs.Name, Version: inputs.Version, Profile: inputs.Profile}, nil)
	if err != nil || !cached || string(got.Content) != string(content) {
		t.Fatalf("cache hit: cached=%v content=%q err=%v", cached, got.Content, err)
	}
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
	}, nil)
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

func testWorker(prepare func(request) preparationOutcome) *metadataWorker {
	return &metadataWorker{calls: make(map[string]*preparationCall), prepare: prepare}
}

func waitForWaiters(t *testing.T, worker *metadataWorker, input request, want int) {
	t.Helper()
	keyBytes, _ := json.Marshal(input)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		worker.mu.Lock()
		call := worker.calls[string(keyBytes)]
		waiters := 0
		if call != nil {
			waiters = call.waiters
		}
		worker.mu.Unlock()
		if waiters == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waiters did not reach %d", want)
}

func TestMetadataWorkerDeduplicatesConcurrentPreparationAndSharesResult(t *testing.T) {
	const count = 12
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	want := preparationOutcome{result: metadatagenerator.Result{Content: []byte("SLOT=0\n")}, cached: true, sourceCommit: "abc"}
	worker := testWorker(func(request) preparationOutcome {
		calls.Add(1)
		close(started)
		<-release
		return want
	})
	input := request{Repository: repository{Name: "gentoo", Path: "/repo", Commit: "abc"}, Category: "dev-libs", Name: "widget", Version: "1", Profile: "profile"}
	results := make(chan preparationOutcome, count)
	var group sync.WaitGroup
	for i := 0; i < count; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- worker.do(context.Background(), input)
		}()
	}
	<-started
	waitForWaiters(t, worker, input, count)
	close(release)
	group.Wait()
	close(results)
	for got := range results {
		if got.err != nil || got.cached != want.cached || got.sourceCommit != want.sourceCommit || string(got.result.Content) != string(want.result.Content) {
			t.Fatalf("shared outcome = %#v, want %#v", got, want)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("preparation calls = %d, want 1", got)
	}
}

func TestMetadataWorkerSharesPreparationErrorWithAllWaiters(t *testing.T) {
	const count = 8
	started := make(chan struct{})
	release := make(chan struct{})
	wantErr := errors.New("generation failed")
	var calls atomic.Int32
	worker := testWorker(func(request) preparationOutcome {
		calls.Add(1)
		close(started)
		<-release
		return preparationOutcome{err: wantErr}
	})
	input := request{Name: "widget"}
	results := make(chan preparationOutcome, count)
	var group sync.WaitGroup
	for i := 0; i < count; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- worker.do(context.Background(), input)
		}()
	}
	<-started
	waitForWaiters(t, worker, input, count)
	close(release)
	group.Wait()
	close(results)
	for got := range results {
		if !errors.Is(got.err, wantErr) {
			t.Fatalf("shared error = %v, want %v", got.err, wantErr)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("preparation calls = %d, want 1", got)
	}
}

func TestMetadataWorkerUsesSharedGenerationLimitForDifferentKeys(t *testing.T) {
	const limit = 2
	const count = 10
	slots := make(chan struct{}, limit)
	started := make(chan struct{}, count)
	release := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	worker := testWorker(func(input request) preparationOutcome {
		result, err := generateWithLimit(slots, func() (metadatagenerator.Result, error) {
			current := active.Add(1)
			for old := maximum.Load(); current > old && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
			}
			started <- struct{}{}
			<-release
			active.Add(-1)
			return metadatagenerator.Result{Content: []byte(input.Name)}, nil
		})
		return preparationOutcome{result: result, err: err}
	})
	var group sync.WaitGroup
	results := make(chan preparationOutcome, count)
	for i := 0; i < count; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			results <- worker.do(context.Background(), request{Name: fmt.Sprintf("pkg-%d", i)})
		}(i)
	}
	for i := 0; i < limit; i++ {
		<-started
	}
	if got := maximum.Load(); got != limit {
		t.Fatalf("simultaneous generations = %d, want to reach limit %d", got, limit)
	}
	close(release)
	group.Wait()
	close(results)
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
	}
	if got := maximum.Load(); got > limit {
		t.Fatalf("maximum simultaneous generations = %d, limit %d", got, limit)
	}
}

func TestMetadataWorkerCancelingOneWaiterDoesNotCancelSharedPreparation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	worker := testWorker(func(request) preparationOutcome {
		close(started)
		<-release
		return preparationOutcome{result: metadatagenerator.Result{Content: []byte("ready")}}
	})
	input := request{Name: "widget"}
	ctx, cancel := context.WithCancel(context.Background())
	canceledResult := make(chan preparationOutcome, 1)
	go func() { canceledResult <- worker.do(ctx, input) }()
	<-started
	remainingResult := make(chan preparationOutcome, 1)
	go func() { remainingResult <- worker.do(context.Background(), input) }()
	waitForWaiters(t, worker, input, 2)
	cancel()
	if got := <-canceledResult; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v, want context canceled", got.err)
	}
	close(release)
	if got := <-remainingResult; got.err != nil || string(got.result.Content) != "ready" {
		t.Fatalf("remaining waiter outcome = %#v", got)
	}
}

func TestGenerationLimitFromEnv(t *testing.T) {
	t.Setenv("METADATA_WORKER_GENERATION_LIMIT", "3")
	if got, err := generationLimitFromEnv(); err != nil || got != 3 {
		t.Fatalf("generationLimitFromEnv() = %d, %v; want 3, nil", got, err)
	}
	for _, value := range []string{"0", "-1", "invalid"} {
		t.Setenv("METADATA_WORKER_GENERATION_LIMIT", value)
		if _, err := generationLimitFromEnv(); err == nil {
			t.Errorf("generationLimitFromEnv(%q) succeeded, want error", value)
		}
	}
}

func TestPreparationLimit(t *testing.T) {
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	worker := testWorker(limitPreparation(2, func(request) preparationOutcome {
		started <- struct{}{}
		<-release
		return preparationOutcome{}
	}))
	var done sync.WaitGroup
	for _, name := range []string{"one", "two", "three"} {
		done.Add(1)
		go func(name string) { defer done.Done(); worker.do(context.Background(), request{Name: name}) }(name)
	}
	<-started
	<-started
	select {
	case <-started:
		t.Error("third preparation started before a slot was released")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	done.Wait()
	select {
	case <-started:
	default:
		t.Error("third preparation did not start")
	}
}

func TestPreparationLimitFromEnv(t *testing.T) {
	for _, tc := range []struct {
		value   string
		want    int
		invalid bool
	}{
		{"", 2, false}, {"3", 3, false}, {"0", 0, true}, {"-1", 0, true}, {"invalid", 0, true},
	} {
		t.Setenv("METADATA_WORKER_PREPARATION_LIMIT", tc.value)
		got, err := preparationLimitFromEnv()
		if got != tc.want || (err != nil) != tc.invalid {
			t.Errorf("preparationLimitFromEnv(%q) = %d, %v", tc.value, got, err)
		}
	}
}
