package ebuildresolver

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type testRepo struct {
	t     *testing.T
	path  string
	clock int
}

func newRepo(t *testing.T) *testRepo {
	t.Helper()
	path := t.TempDir()
	r := &testRepo{t: t, path: path}
	r.git("init", "-q")
	r.git("config", "user.name", "Resolver Test")
	r.git("config", "user.email", "resolver@example.invalid")
	r.git("config", "commit.gpgsign", "false")
	return r
}

func (r *testRepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.path}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (r *testRepo) commit(message string, files map[string]string, remove ...string) string {
	r.t.Helper()
	for _, path := range remove {
		if err := os.Remove(filepath.Join(r.path, path)); err != nil && !errors.Is(err, os.ErrNotExist) {
			r.t.Fatal(err)
		}
	}
	for path, content := range files {
		full := filepath.Join(r.path, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	r.git("add", "-A")
	r.clock++
	date := fmt.Sprintf("2001-01-%02dT00:00:00Z", r.clock%27+1)
	cmd := exec.Command("git", "-C", r.path, "-c", "user.name=Resolver Test", "-c", "user.email=resolver@example.invalid", "commit", "-q", "-m", message)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	if output, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git commit: %v\n%s", err, output)
	}
	return r.git("rev-parse", "HEAD")
}

func (r *testRepo) branch(name string, commit string) { r.git("branch", name, commit) }

func inputFor(reference string, category, name, version string) Input {
	return Input{Repository: "gentoo", Reference: reference, Category: category, Name: name, Version: version}
}

func TestResolveFirstParentStateTransitions(t *testing.T) {
	r := newRepo(t)
	path := "dev-libs/libei/libei-1.6.0.ebuild"
	old := r.commit("add", map[string]string{path: "old"})
	removed := r.commit("remove", nil, path)
	r.commit("readd", map[string]string{path: "new"})
	updated := r.commit("update same version", map[string]string{path: "updated"})
	reference := r.commit("edit other path", map[string]string{"README": "unrelated"})
	resolver, err := Open(r.path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := resolver.Resolve(inputFor(reference, "dev-libs", "libei", "1.6.0"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFound || result.SelectedCommit != reference || result.Reason != ReasonAtReference {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(r.path, path)); err != nil {
		t.Fatalf("fixture disappeared: %v", err)
	}

	// A later deletion selects the nearest state with the file, even if the
	// selected commit edited a different path than the ebuild.
	deleted := r.commit("delete again", nil, path)
	result, err = resolver.Resolve(inputFor(deleted, "dev-libs", "libei", "1.6.0"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFound || result.SelectedCommit != reference || result.Reason != ReasonFirstParentHistory {
		t.Fatalf("unexpected prior-state selection: %+v", result)
	}
	wantBlob := r.git("rev-parse", updated+":"+path)
	if result.BlobID != wantBlob {
		t.Fatalf("selected blob %s, want updated same-version blob %s", result.BlobID, wantBlob)
	}
	removedResult, err := resolver.Resolve(inputFor(removed, "dev-libs", "libei", "1.6.0"))
	if err != nil {
		t.Fatal(err)
	}
	if removedResult.Status != StatusFound || removedResult.SelectedCommit != old || removedResult.Reason != ReasonFirstParentHistory {
		t.Fatalf("re-added file must not affect the earlier deletion: %+v", removedResult)
	}
	if result.BlobID == "" || result.ReferenceCommit != deleted {
		t.Fatalf("missing provenance: %+v", result)
	}
	if removed == "" || old == "" {
		t.Fatal("fixture commits missing")
	}

	// Repeated resolution leaves HEAD, index and worktree untouched.
	r.git("checkout", "--detach", deleted)
	if err := os.WriteFile(filepath.Join(r.path, "local-change"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.git("add", "local-change")
	beforeHead := r.git("rev-parse", "HEAD")
	beforeIndex := r.git("diff", "--cached", "--binary")
	beforeWorktree := r.git("status", "--porcelain=v1", "-uall")
	for i := 0; i < 2; i++ {
		if _, err := resolver.Resolve(inputFor(deleted, "dev-libs", "libei", "1.6.0")); err != nil {
			t.Fatal(err)
		}
	}
	if got := r.git("rev-parse", "HEAD"); got != beforeHead {
		t.Fatalf("HEAD changed: %s", got)
	}
	if got := r.git("diff", "--cached", "--binary"); got != beforeIndex {
		t.Fatalf("index changed: %q", got)
	}
	if got := r.git("status", "--porcelain=v1", "-uall"); got != beforeWorktree {
		t.Fatalf("worktree changed: %q", got)
	}
}

func TestResolveMergeUsesOnlyFirstParent(t *testing.T) {
	r := newRepo(t)
	base := r.commit("base", map[string]string{"README": "base"})
	r.branch("mainline", base)
	main := r.commit("main line", map[string]string{"main": "main"})
	r.git("checkout", "-q", "-b", "side", base)
	side := r.commit("side version", map[string]string{"cat/pkg/pkg-2.0.ebuild": "side"})
	r.git("checkout", "-q", "mainline")
	r.git("merge", "--no-ff", "-q", "side", "-m", "merge")
	r.git("rm", "-q", "cat/pkg/pkg-2.0.ebuild")
	r.git("-c", "user.name=Resolver Test", "-c", "user.email=resolver@example.invalid", "commit", "--amend", "--no-edit", "-q")
	merge := r.git("rev-parse", "HEAD")
	resolver, err := Open(r.path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := resolver.Resolve(inputFor(merge, "cat", "pkg", "2.0"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusNotFound {
		t.Fatalf("side-parent version was selected: %+v", result)
	}
	if main == side {
		t.Fatal("fixture branches unexpectedly match")
	}
}

func TestResolveIgnoresCommitDates(t *testing.T) {
	r := newRepo(t)
	path := "cat/pkg/pkg-1.ebuild"
	first := r.commit("first", map[string]string{path: "first"})
	second := r.commit("second", map[string]string{path: "second"})
	cmd := exec.Command("git", "-C", r.path, "commit", "--amend", "--no-edit", "-q")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=1999-01-01T00:00:00Z", "GIT_COMMITTER_DATE=1999-01-01T00:00:00Z")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git amend: %v\n%s", err, output)
	}
	second = r.git("rev-parse", "HEAD")
	deleted := r.commit("delete", nil, path)
	resolver, err := Open(r.path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := resolver.Resolve(inputFor(deleted, "cat", "pkg", "1"))
	if err != nil {
		t.Fatal(err)
	}
	wantBlob := r.git("rev-parse", second+":"+path)
	if result.Status != StatusFound || result.SelectedCommit != second || result.BlobID != wantBlob || result.Reason != ReasonFirstParentHistory {
		t.Fatalf("date affected selection: %+v (first=%s)", result, first)
	}
}

func TestResolveShallowBoundaryChecksTreeFirst(t *testing.T) {
	r := newRepo(t)
	path := "cat/pkg/pkg-1.ebuild"
	boundary := r.commit("boundary", map[string]string{path: "at boundary"})
	reference := r.commit("reference", nil, path)
	markShallow(t, r, boundary)
	resolver, err := Open(r.path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := resolver.Resolve(inputFor(reference, "cat", "pkg", "1"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFound || result.SelectedCommit != boundary || result.Reason != ReasonFirstParentHistory {
		t.Fatalf("expected boundary file to be found: %+v", result)
	}

	withoutFile := newRepo(t)
	emptyBoundary := withoutFile.commit("boundary without ebuild", map[string]string{"README": "boundary"})
	emptyReference := withoutFile.commit("reference", map[string]string{"reference": "child"})
	markShallow(t, withoutFile, emptyBoundary)
	emptyResolver, err := Open(withoutFile.path)
	if err != nil {
		t.Fatal(err)
	}
	result, err = emptyResolver.Resolve(inputFor(emptyReference, "cat", "pkg", "1"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusHistoryIncomplete {
		t.Fatalf("expected incomplete history: %+v", result)
	}
}

func TestResolveErrorsForInvalidInputsAndUnavailableBlob(t *testing.T) {
	r := newRepo(t)
	path := "cat/pkg/pkg-1.ebuild"
	reference := r.commit("add", map[string]string{path: "content"})
	resolver, err := Open(r.path)
	if err != nil {
		t.Fatal(err)
	}
	invalid := inputFor(reference, "../cat", "pkg", "1")
	if _, err := resolver.Resolve(invalid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid path input error = %v", err)
	}
	if _, err := resolver.Resolve(inputFor(strings.Repeat("0", 40), "cat", "pkg", "1")); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("missing ref error = %v", err)
	}
	blob := r.git("rev-parse", reference+":"+path)
	if _, err := resolver.Resolve(inputFor(blob, "cat", "pkg", "1")); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("blob reference error = %v", err)
	}
	objectPath := filepath.Join(r.path, ".git", "objects", blob[:2], blob[2:])
	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(inputFor(reference, "cat", "pkg", "1")); !errors.Is(err, ErrRead) {
		t.Fatalf("unavailable blob must be technical error, got %v", err)
	}
}

func TestResolveMissingParentOutsideShallowBoundaryIsError(t *testing.T) {
	r := newRepo(t)
	parent := r.commit("parent", map[string]string{"README": "parent"})
	reference := r.commit("child", map[string]string{"README": "child"})
	resolver, err := Open(r.path)
	if err != nil {
		t.Fatal(err)
	}
	objectPath := filepath.Join(r.path, ".git", "objects", parent[:2], parent[2:])
	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(inputFor(reference, "cat", "pkg", "1")); !errors.Is(err, ErrRead) {
		t.Fatalf("missing non-shallow parent must be a technical error, got %v", err)
	}
}

func TestResolveRejectsUnsupportedEntry(t *testing.T) {
	r := newRepo(t)
	path := "cat/pkg/pkg-1.ebuild"
	if err := os.MkdirAll(filepath.Join(r.path, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(r.path, path)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.path, "target", "file"), []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target/file", filepath.Join(r.path, path)); err != nil {
		t.Fatal(err)
	}
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "symlink")
	reference := r.git("rev-parse", "HEAD")
	resolver, err := Open(r.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(inputFor(reference, "cat", "pkg", "1")); !errors.Is(err, ErrUnsupportedEntry) {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestResolveRejectsTreeAtExactPath(t *testing.T) {
	r := newRepo(t)
	path := "cat/pkg/pkg-1.ebuild"
	if err := os.MkdirAll(filepath.Join(r.path, path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.path, path, "nested"), []byte("not an ebuild"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "tree at ebuild path")
	resolver, err := Open(r.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(inputFor(r.git("rev-parse", "HEAD"), "cat", "pkg", "1")); !errors.Is(err, ErrUnsupportedEntry) {
		t.Fatalf("tree entry error = %v", err)
	}
}

func TestResolveRejectsSubmoduleAtExactPath(t *testing.T) {
	r := newRepo(t)
	child := newRepo(t)
	childCommit := child.commit("submodule revision", map[string]string{"README": "child"})
	path := "cat/pkg/pkg-1.ebuild"
	r.git("update-index", "--add", "--cacheinfo", "160000,"+childCommit+","+path)
	r.git("commit", "-q", "-m", "submodule at ebuild path")
	resolver, err := Open(r.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(inputFor(r.git("rev-parse", "HEAD"), "cat", "pkg", "1")); !errors.Is(err, ErrUnsupportedEntry) {
		t.Fatalf("submodule entry error = %v", err)
	}
}

func markShallow(t *testing.T, r *testRepo, commit string) {
	t.Helper()
	path := strings.TrimSpace(r.git("rev-parse", "--git-path", "shallow"))
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.path, path)
	}
	if err := os.WriteFile(path, []byte(commit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
