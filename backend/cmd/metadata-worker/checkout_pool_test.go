package main

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func poolFixture(t *testing.T) request {
	t.Helper()
	root := t.TempDir()
	workerGit(t, root, "init", "--quiet", "--initial-branch=main")
	for name, content := range map[string]string{"tracked": "original", ".gitignore": "ignored\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	workerGit(t, root, "add", ".")
	workerGit(t, root, "commit", "-qm", "fixture")
	return request{Repository: repository{Name: "main", URL: "local", Path: root, Commit: workerGit(t, root, "rev-parse", "HEAD")}, Profile: "profiles/test"}
}
func testPool(t *testing.T, budget int64) *checkoutPool {
	t.Helper()
	p, err := openCheckoutPool(t.TempDir(), budget)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.close() })
	return p
}
func acquireSet(t *testing.T, p *checkoutPool, r request) *checkoutSet {
	t.Helper()
	s, err := p.acquire(context.Background(), r, r.Repository.Commit, "runtime")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func releaseSet(t *testing.T, p *checkoutPool, s *checkoutSet) {
	t.Helper()
	if err := p.release(s, false); err != nil {
		t.Fatal(err)
	}
}
func TestCheckoutBudgetConfig(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
		bad   bool
	}{{"", 4294967296, false}, {"0", 0, false}, {"123", 123, false}, {"-1", 0, true}, {"bad", 0, true}, {"9223372036854775808", 0, true}} {
		t.Setenv("METADATA_WORKER_CHECKOUT_CACHE_MAX_BYTES", tc.value)
		got, err := checkoutCacheMaxBytesFromEnv()
		if (err != nil) != tc.bad || got != tc.want {
			t.Fatalf("%q: %d %v", tc.value, got, err)
		}
	}
}
func TestCheckoutKey(t *testing.T) {
	r := poolFixture(t)
	r.Masters = []repository{{Name: "a", URL: "a", Path: "a", Commit: "a"}, {Name: "b", URL: "b", Path: "b", Commit: "b"}}
	original := checkoutKey(r, r.Repository.Commit, "runtime")
	other := r
	other.Name = "other"
	other.Version = "2"
	other.Category = "different"
	if checkoutKey(other, r.Repository.Commit, "runtime") != original {
		t.Fatal("package participates in key")
	}
	for _, field := range []string{"name", "url", "path", "profile", "master-name", "master-url", "master-path", "master-commit", "order", "commit", "runtime"} {
		other := r
		other.Masters = append([]repository(nil), r.Masters...)
		commit, runtime := r.Repository.Commit, "runtime"
		switch field {
		case "name":
			other.Repository.Name += "x"
		case "url":
			other.Repository.URL += "x"
		case "path":
			other.Repository.Path += "x"
		case "profile":
			other.Profile += "x"
		case "master-name":
			other.Masters[0].Name += "x"
		case "master-url":
			other.Masters[0].URL += "x"
		case "master-path":
			other.Masters[0].Path += "x"
		case "master-commit":
			other.Masters[0].Commit += "x"
		case "order":
			other.Masters[0], other.Masters[1] = other.Masters[1], other.Masters[0]
		case "commit":
			commit = "other"
		case "runtime":
			runtime = "other"
		}
		if checkoutKey(other, commit, runtime) == original {
			t.Fatal(field, "shares key")
		}
	}
}
func TestCheckoutRestoreReuseAndWaitCancellation(t *testing.T) {
	r := poolFixture(t)
	r.Masters = []repository{r.Repository}
	p := testPool(t, defaultCheckoutCacheMaxBytes)
	s := acquireSet(t, p, r)
	for _, path := range s.paths {
		for _, name := range []string{"tracked", "untracked", "ignored"} {
			if err := os.WriteFile(filepath.Join(path, name), []byte("changed"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := p.acquire(ctx, r, r.Repository.Commit, "runtime"); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel blocked")
	}
	other := r
	other.Profile = "other"
	parallel := acquireSet(t, p, other)
	if parallel.root == s.root {
		t.Fatal("shared active set")
	}
	releaseSet(t, p, parallel)
	releaseSet(t, p, s)
	r.Name = "different"
	reused := acquireSet(t, p, r)
	if reused.root != s.root {
		t.Fatal("did not reuse")
	}
	for _, path := range reused.paths {
		if got := workerGit(t, path, "status", "--porcelain", "--untracked-files=all", "--ignored"); got != "" {
			t.Fatal(got)
		}
		data, err := os.ReadFile(filepath.Join(path, "tracked"))
		if err != nil || string(data) != "original" {
			t.Fatal("not restored", err)
		}
	}
	releaseSet(t, p, reused)
}
func TestCheckoutOwnershipAndStaleCleanup(t *testing.T) {
	root := t.TempDir()
	p, err := openCheckoutPool(root, 100)
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(p.root, "stale")
	if err := os.WriteFile(stale, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if second, err := openCheckoutPool(root, 100); err == nil {
		second.close()
		t.Fatal("second owner accepted")
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatal("second owner removed data")
	}
	p.close()
	next, err := openCheckoutPool(root, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer next.close()
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale survived", err)
	}
}
func TestCheckoutLimitsAndBlockedCleanup(t *testing.T) {
	r := poolFixture(t)
	for _, budget := range []int64{0, 1} {
		p := testPool(t, budget)
		s := acquireSet(t, p, r)
		releaseSet(t, p, s)
		if len(p.idle) != 0 {
			t.Fatal("oversized retained")
		}
		if _, err := os.Stat(s.root); !os.IsNotExist(err) {
			t.Fatal("not removed", err)
		}
	}
	p := testPool(t, defaultCheckoutCacheMaxBytes)
	var first string
	for i := 0; i < 17; i++ {
		r.Profile = string(rune('a' + i))
		s := acquireSet(t, p, r)
		if i == 0 {
			first = s.root
		}
		releaseSet(t, p, s)
	}
	if len(p.idle) != 16 {
		t.Fatal("count", len(p.idle))
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("oldest not evicted")
	}
	s := acquireSet(t, p, r)
	p.remove = func(string) error { return errors.New("injected removal failure") }
	if err := p.release(s, true); err == nil {
		t.Fatal("cleanup error lost")
	}
	if _, err := p.acquire(context.Background(), r, r.Repository.Commit, "runtime"); err == nil {
		t.Fatal("allocation after failure")
	}
}
func TestAllocatedBytesDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	before, err := allocatedBytes(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/", filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	after, err := allocatedBytes(root)
	if err != nil || after-before > 8192 {
		t.Fatal(after, before, err)
	}
}

func TestCheckoutByteLRUAndRecency(t *testing.T) {
	r := poolFixture(t)
	p := testPool(t, defaultCheckoutCacheMaxBytes)
	a := acquireSet(t, p, r)
	releaseSet(t, p, a)
	other := r
	other.Profile = "b"
	b := acquireSet(t, p, other)
	releaseSet(t, p, b)
	// Touch a: b must become the oldest idle entry.
	a = acquireSet(t, p, r)
	releaseSet(t, p, a)
	p.maxBytes = a.bytes + b.bytes
	other.Profile = "c"
	c := acquireSet(t, p, other)
	releaseSet(t, p, c)
	var total int64
	for _, s := range p.idle {
		total += s.bytes
		if s.root == b.root {
			t.Fatal("least recently used entry retained")
		}
	}
	if total > p.maxBytes {
		t.Fatal("byte budget exceeded", total, p.maxBytes)
	}
}
func TestCheckoutWaitingLeaseIsExclusive(t *testing.T) {
	r := poolFixture(t)
	p := testPool(t, defaultCheckoutCacheMaxBytes)
	first := acquireSet(t, p, r)
	received := make(chan *checkoutSet, 1)
	failed := make(chan error, 1)
	go func() {
		s, err := p.acquire(context.Background(), r, r.Repository.Commit, "runtime")
		if err != nil {
			failed <- err
			return
		}
		received <- s
	}()
	select {
	case <-received:
		t.Fatal("lease shared while active")
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(50 * time.Millisecond):
	}
	releaseSet(t, p, first)
	select {
	case s := <-received:
		if s.root != first.root {
			t.Fatal("waiter did not reuse")
		}
		releaseSet(t, p, s)
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("waiter blocked")
	}
}
func TestCheckoutCorruptionIsDiscarded(t *testing.T) {
	r := poolFixture(t)
	p := testPool(t, defaultCheckoutCacheMaxBytes)
	s := acquireSet(t, p, r)
	if err := os.RemoveAll(filepath.Join(s.paths[0], ".git")); err != nil {
		t.Fatal(err)
	}
	if err := p.release(s, false); err == nil {
		t.Fatal("restoration error lost")
	}
	if len(p.idle) != 0 || p.blocked != nil {
		t.Fatal("corruption not removed")
	}
	next := acquireSet(t, p, r)
	if next.root == s.root {
		t.Fatal("corrupt set reused")
	}
	releaseSet(t, p, next)
}
func TestCheckoutCrashRestart(t *testing.T) {
	if root := os.Getenv("CHECKOUT_POOL_TEST_CHILD"); root != "" {
		p, err := openCheckoutPool(root, 100)
		if err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(filepath.Join(p.root, "stale"), []byte("stale"), 0o600); err != nil {
			os.Exit(3)
		}
		os.Stdout.Write([]byte("ready\n"))
		select {}
	}
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCheckoutCrashRestart$")
	cmd.Env = append(os.Environ(), "CHECKOUT_POOL_TEST_CHILD="+root)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(pipe).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if line != "ready\n" {
			t.Fatal("child failed", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child startup timeout")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	p, err := openCheckoutPool(root, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	if _, err := os.Stat(filepath.Join(p.root, "stale")); !os.IsNotExist(err) {
		t.Fatal("crash leftovers survived", err)
	}
}
