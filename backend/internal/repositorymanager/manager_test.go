package repositorymanager

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	args = append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, args...)
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestPinReusesMirrorAndRetainsRevisions(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, source, "init", "--quiet", "--initial-branch=main")
	gitTest(t, source, "commit", "--allow-empty", "-qm", "first")
	first := gitTest(t, source, "rev-parse", "HEAD")
	manager, err := New(filepath.Join(root, "mirrors"), []Repository{{Name: "overlay", URL: source, Masters: []string{"gentoo"}}})
	if err != nil {
		t.Fatal(err)
	}
	gentoo := Revision{Name: "gentoo", Path: "/pinned/gentoo", Commit: strings.Repeat("a", 40)}
	old, err := manager.Pin(context.Background(), []string{"overlay"}, gentoo)
	if err != nil {
		t.Fatal(err)
	}
	if old["overlay"].Commit != first || old["gentoo"].Commit != gentoo.Commit {
		t.Fatalf("unexpected revisions: %#v", old)
	}
	gitTest(t, source, "commit", "--allow-empty", "-qm", "second")
	second := gitTest(t, source, "rev-parse", "HEAD")
	reused, err := manager.Pin(context.Background(), []string{"overlay", "overlay"}, gentoo)
	if err != nil || reused["overlay"].Commit != second {
		t.Fatalf("reuse = %#v, %v", reused, err)
	}
	updated, err := manager.Pin(context.Background(), []string{"overlay"}, gentoo)
	if err != nil || updated["overlay"].Commit != second {
		t.Fatalf("updated = %#v, %v", updated, err)
	}
	if old["overlay"].Commit != first {
		t.Fatalf("previous analysis changed: %#v", old)
	}
	if got := gitTest(t, updated["overlay"].Path, "rev-parse", "refs/gentoo/snapshots/"+first); got != first {
		t.Fatalf("old ref = %s", got)
	}
}

func TestUnknownAndInvalidCatalog(t *testing.T) {
	manager, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Pin(context.Background(), []string{"../../unknown"}, Revision{Name: "gentoo", Path: "/gentoo", Commit: strings.Repeat("a", 40)})
	if !errors.Is(err, ErrUnknownRepository) {
		t.Fatalf("unknown = %v", err)
	}
	if _, err := New(t.TempDir(), []Repository{{Name: "../escape", URL: "source"}}); err == nil {
		t.Fatal("invalid catalog accepted")
	}
	if _, err := New(t.TempDir(), []Repository{{Name: "a", URL: "source", Masters: []string{"missing"}}}); err == nil {
		t.Fatal("unknown master accepted")
	}
}

func TestRefreshFailureRetainsPublishedCommit(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, source, "init", "--quiet", "--initial-branch=main")
	gitTest(t, source, "commit", "--allow-empty", "-qm", "first")
	manager, err := New(filepath.Join(root, "mirrors"), []Repository{{Name: "overlay", URL: source, Masters: []string{"gentoo"}}})
	if err != nil {
		t.Fatal(err)
	}
	gentoo := Revision{Name: "gentoo", Path: "/gentoo", Commit: strings.Repeat("a", 40)}
	first, err := manager.Pin(context.Background(), []string{"overlay"}, gentoo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, source+"-offline"); err != nil {
		t.Fatal(err)
	}
	again, err := manager.Pin(context.Background(), []string{"overlay"}, gentoo)
	if err != nil || again["overlay"].Commit != first["overlay"].Commit {
		t.Fatalf("fallback = %#v, %v", again, err)
	}
	if got := gitTest(t, first["overlay"].Path, "rev-parse", "refs/gentoo/published"); got != first["overlay"].Commit {
		t.Fatalf("published = %s", got)
	}
}

func TestFailedVerificationDoesNotPublishCandidate(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, source, "init", "--quiet", "--initial-branch=main")
	gitTest(t, source, "commit", "--allow-empty", "-qm", "first")
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "fail-verification")
	wrapper := "#!/bin/sh\ncase \" $* \" in *' fsck '*) [ ! -e \"$SYNC_TEST_FAIL\" ] || exit 1 ;; esac\nexec '" + strings.ReplaceAll(gitPath, "'", "'\\''") + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SYNC_TEST_FAIL", marker)
	manager, err := New(filepath.Join(root, "mirrors"), []Repository{{Name: "overlay", URL: source, Masters: []string{"gentoo"}}})
	if err != nil {
		t.Fatal(err)
	}
	gentoo := Revision{Name: "gentoo", Path: "/gentoo", Commit: strings.Repeat("a", 40)}
	first, err := manager.Pin(context.Background(), []string{"overlay"}, gentoo)
	if err != nil {
		t.Fatal(err)
	}
	gitTest(t, source, "commit", "--allow-empty", "-qm", "second")
	second := gitTest(t, source, "rev-parse", "HEAD")
	if err := os.WriteFile(marker, []byte("fail"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Pin(context.Background(), []string{"overlay"}, gentoo)
	if err != nil || result["overlay"].Commit != first["overlay"].Commit {
		t.Fatalf("result = %#v, %v", result, err)
	}
	if got := gitTest(t, first["overlay"].Path, "rev-parse", "refs/gentoo/candidate"); got != second {
		t.Fatalf("candidate = %s", got)
	}
	if got := gitTest(t, first["overlay"].Path, "rev-parse", "refs/gentoo/published"); got != first["overlay"].Commit {
		t.Fatalf("published = %s", got)
	}
}

func TestLegacyMirrorMigratesBeforeRefresh(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, source, "init", "--quiet", "--initial-branch=main")
	gitTest(t, source, "commit", "--allow-empty", "-qm", "first")
	first := gitTest(t, source, "rev-parse", "HEAD")
	storage := filepath.Join(root, "mirrors")
	if err := os.Mkdir(storage, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, root, "clone", "--mirror", "--", source, filepath.Join(storage, "overlay.git"))
	gitTest(t, source, "commit", "--allow-empty", "-qm", "second")
	second := gitTest(t, source, "rev-parse", "HEAD")
	manager, err := New(storage, []Repository{{Name: "overlay", URL: source, Masters: []string{"gentoo"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Pin(context.Background(), []string{"overlay"}, Revision{Name: "gentoo", Path: "/gentoo", Commit: strings.Repeat("a", 40)})
	if err != nil || result["overlay"].Commit != second {
		t.Fatalf("pin = %#v, %v", result, err)
	}
	if got := gitTest(t, result["overlay"].Path, "rev-parse", "refs/gentoo/snapshots/"+first); got != first {
		t.Fatalf("legacy ref = %s", got)
	}
}

func TestAmbiguousCatalogEntryIsReported(t *testing.T) {
	root := t.TempDir()
	manager, err := New(root, []Repository{{Name: "overlay", URL: filepath.Join(root, "one")}, {Name: "overlay", URL: filepath.Join(root, "two")}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Pin(context.Background(), []string{"overlay"}, Revision{Name: "gentoo", Path: "/gentoo", Commit: strings.Repeat("a", 40)})
	if !errors.Is(err, ErrAmbiguousRepository) {
		t.Fatalf("ambiguous = %v", err)
	}
}

func TestPinAdditionalKeepsPreviouslyPinnedMaster(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	child := filepath.Join(root, "child")
	for _, path := range []string{parent, child} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		gitTest(t, path, "init", "--quiet", "--initial-branch=main")
		gitTest(t, path, "commit", "--allow-empty", "-qm", "first")
	}
	first := gitTest(t, parent, "rev-parse", "HEAD")
	manager, err := New(filepath.Join(root, "mirrors"), []Repository{
		{Name: "parent", URL: parent, Masters: []string{"gentoo"}},
		{Name: "child", URL: child, Masters: []string{"parent"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	gentoo := Revision{Name: "gentoo", Path: "/gentoo", Commit: strings.Repeat("a", 40)}
	pinned, err := manager.Pin(context.Background(), []string{"parent"}, gentoo)
	if err != nil {
		t.Fatal(err)
	}
	gitTest(t, parent, "commit", "--allow-empty", "-qm", "second")
	second := gitTest(t, parent, "rev-parse", "HEAD")
	sameAnalysis, err := manager.PinAdditional(context.Background(), []string{"child"}, pinned)
	if err != nil || sameAnalysis["parent"].Commit != first {
		t.Fatalf("same analysis = %#v, %v", sameAnalysis, err)
	}
	newAnalysis, err := manager.Pin(context.Background(), []string{"child"}, gentoo)
	if err != nil || newAnalysis["parent"].Commit != second {
		t.Fatalf("new analysis = %#v, %v", newAnalysis, err)
	}
}

func TestConcurrentPinsShareMirror(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, source, "init", "--quiet", "--initial-branch=main")
	gitTest(t, source, "commit", "--allow-empty", "-qm", "fixture")
	commit := gitTest(t, source, "rev-parse", "HEAD")
	manager, err := New(filepath.Join(root, "mirrors"), []Repository{{Name: "overlay", URL: source, Masters: []string{"gentoo"}}})
	if err != nil {
		t.Fatal(err)
	}
	gentoo := Revision{Name: "gentoo", Path: "/gentoo", Commit: strings.Repeat("a", 40)}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pinned, err := manager.Pin(context.Background(), []string{"overlay"}, gentoo)
			if err == nil && pinned["overlay"].Commit != commit {
				err = errors.New("unexpected commit")
			}
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
}
