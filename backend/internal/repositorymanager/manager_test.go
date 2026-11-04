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
	if err != nil || reused["overlay"].Commit != first {
		t.Fatalf("reuse = %#v, %v", reused, err)
	}
	gitTest(t, reused["overlay"].Path, "fetch", "--force", source, "+HEAD:refs/heads/main")
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
