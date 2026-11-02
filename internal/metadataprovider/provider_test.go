package metadataprovider

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
)

type testRepo struct {
	path string
	sha  string
}

func newTestRepo(t *testing.T, objectFormat string) *testRepo {
	t.Helper()
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	args := []string{"init", "--quiet", "--initial-branch=main"}
	if objectFormat != "" {
		args = append(args, "--object-format="+objectFormat)
	}
	args = append(args, repoPath)
	if _, err := testGit(root, args...); err != nil {
		t.Fatalf("git init: %v", err)
	}
	return &testRepo{path: repoPath}
}

func (r *testRepo) commit(t *testing.T) string {
	t.Helper()
	if _, err := testGit(r.path, "add", "--all"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	_, err := testGit(r.path, "commit", "--quiet", "--allow-empty", "-m", "fixture")
	if err != nil {
		t.Fatalf("git commit: %v", err)
	}
	id, err := testGit(r.path, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	r.sha = strings.TrimSpace(string(id))
	return r.sha
}

func (r *testRepo) open(t *testing.T) *Provider {
	t.Helper()
	provider, err := Open(Config{Repository: "gentoo", SourceURL: "https://example.invalid/gentoo", LocalPath: r.path, Commit: r.sha})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return provider
}

func testGit(dir string, args ...string) ([]byte, error) {
	commandArgs := []string{"-c", "user.name=Metadata Provider Test", "-c", "user.email=metadata-provider@example.invalid", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-C", dir}
	commandArgs = append(commandArgs, args...)
	command := exec.Command("git", commandArgs...)
	return command.CombinedOutput()
}

func writeCache(t *testing.T, repoPath, category, name, version, content string, mode os.FileMode) string {
	t.Helper()
	cachePath := filepath.Join(repoPath, "metadata", "md5-cache", category, name+"-"+version)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	return cachePath
}

func TestLookupFoundPreservesContentDigestAndProvenance(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode os.FileMode
	}{
		{name: "regular", mode: 0o644},
		{name: "executable", mode: 0o755},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newTestRepo(t, "sha1")
			content := "EAPI=8\nSLOT=0\nIUSE=\nopaque=unchanged \t\n"
			writeCache(t, repo.path, "dev-libs", "libxml2", "2.14.6-r2", content, tc.mode)
			commit := repo.commit(t)
			provider := repo.open(t)
			result, err := provider.Lookup(Query{Repository: "gentoo", Category: "dev-libs", Name: "libxml2", Version: "2.14.6-r2"})
			if err != nil || result.Status != StatusFound || result.Snapshot == nil {
				t.Fatalf("Lookup = %#v, %v", result, err)
			}
			snapshot := result.Snapshot
			if snapshot.Content != content {
				t.Fatalf("content changed: %q", snapshot.Content)
			}
			digest := sha256.Sum256([]byte(content))
			if snapshot.SHA256 != hex.EncodeToString(digest[:]) {
				t.Fatalf("SHA256 = %s", snapshot.SHA256)
			}
			wantSource := metadatadiff.Source{
				Category: "dev-libs", Name: "libxml2", Version: "2.14.6-r2", Repository: "gentoo",
				SourceURL: "https://example.invalid/gentoo", Commit: commit,
				CachePath: "metadata/md5-cache/dev-libs/libxml2-2.14.6-r2",
			}
			if snapshot.Source != wantSource {
				t.Fatalf("Source = %#v, want %#v", snapshot.Source, wantSource)
			}
			if snapshot.EAPI.Value != "8" || len(snapshot.Diagnostics) != 0 {
				t.Fatalf("Normalize output = %#v", snapshot)
			}
		})
	}
}

func TestLookupNotFoundAndRepositoryUnavailable(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	repo.commit(t)
	provider := repo.open(t)
	missing, err := provider.Lookup(Query{Repository: "gentoo", Category: "dev-libs", Name: "missing", Version: "1.0"})
	if err != nil || missing.Status != StatusNotFound || missing.Snapshot != nil {
		t.Fatalf("missing = %#v, %v", missing, err)
	}
	unavailable, err := provider.Lookup(Query{Repository: "overlay", Category: "dev-libs", Name: "missing", Version: "1.0"})
	if err != nil || unavailable.Status != StatusRepositoryUnavailable || unavailable.Snapshot != nil {
		t.Fatalf("unavailable = %#v, %v", unavailable, err)
	}
}

func TestQueryValidationPrecedesRepositoryMismatch(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	repo.commit(t)
	provider := repo.open(t)
	result, err := provider.Lookup(Query{Repository: "overlay", Category: "../dev-libs", Name: "pkg", Version: "1"})
	if !errors.Is(err, ErrInvalidQuery) || result != (LookupResult{}) {
		t.Fatalf("Lookup = %#v, %v; want zero result and ErrInvalidQuery", result, err)
	}
}

func TestInvalidQueries(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	repo.commit(t)
	provider := repo.open(t)
	tests := []Query{
		{Repository: "gentoo", Category: "../dev-libs", Name: "pkg", Version: "1"},
		{Repository: "gentoo", Category: "/dev-libs", Name: "pkg", Version: "1"},
		{Repository: "gentoo", Category: "dev/libs", Name: "pkg", Version: "1"},
		{Repository: "gentoo", Category: "dev-libs\n", Name: "pkg", Version: "1"},
		{Repository: "gentoo", Category: "dev-libs", Name: "-pkg", Version: "1"},
		{Repository: "gentoo", Category: "dev-libs", Name: "pkg/../../escape", Version: "1"},
		{Repository: "gentoo", Category: "dev-libs", Name: "pkg\x00", Version: "1"},
		{Repository: "gentoo", Category: "dev-libs", Name: "pkg-1.2", Version: "1"},
		{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "../1"},
		{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "1/2"},
		{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "1\n"},
		{Repository: "../gentoo", Category: "dev-libs", Name: "pkg", Version: "1"},
	}
	for _, query := range tests {
		result, err := provider.Lookup(query)
		if !errors.Is(err, ErrInvalidQuery) || result != (LookupResult{}) {
			t.Errorf("Lookup(%#v) = %#v, %v; want zero result and ErrInvalidQuery", query, result, err)
		}
	}
}

func TestValidGentooRevisionsSuffixesAndDoubleDot(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	category, name, version := "dev..libs", "libxml2-compat", "1.2.3_alpha2_beta_rc1_p4-r12"
	writeCache(t, repo.path, category, name, version, "EAPI=8\n", 0o644)
	repo.commit(t)
	result, err := repo.open(t).Lookup(Query{Repository: "gentoo", Category: category, Name: name, Version: version})
	if err != nil || result.Status != StatusFound || result.Snapshot == nil {
		t.Fatalf("Lookup = %#v, %v", result, err)
	}
}

func TestOpenRejectsEmptyConfigAndInvalidCommitInputs(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	commit := repo.commit(t)
	base := Config{Repository: "gentoo", SourceURL: "https://example.invalid/gentoo", LocalPath: repo.path, Commit: commit}
	configs := []Config{
		{},
		{SourceURL: base.SourceURL, LocalPath: repo.path, Commit: commit},
		{Repository: "gentoo", LocalPath: repo.path, Commit: commit},
		{Repository: "gentoo", SourceURL: base.SourceURL, Commit: commit},
		{Repository: "gentoo", SourceURL: base.SourceURL, LocalPath: repo.path},
		{Repository: "-bad", SourceURL: base.SourceURL, LocalPath: repo.path, Commit: commit},
	}
	for _, config := range configs {
		if provider, err := Open(config); !errors.Is(err, ErrInvalidConfig) || provider != nil {
			t.Errorf("Open(%#v) = %p, %v; want nil and ErrInvalidConfig", config, provider, err)
		}
	}
	for _, value := range []string{strings.Repeat("a", 39), strings.Repeat("g", 40), strings.Repeat("a", 64), "main", "HEAD", "refs/heads/main"} {
		config := base
		config.Commit = value
		if provider, err := Open(config); !errors.Is(err, ErrInvalidCommit) || provider != nil {
			t.Errorf("Open Commit %q = %p, %v; want nil and ErrInvalidCommit", value, provider, err)
		}
	}
}

func TestOpenRejectsMissingCommitTagAndNonCommitObject(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	writeCache(t, repo.path, "dev-libs", "pkg", "1", "EAPI=8\n", 0o644)
	commit := repo.commit(t)
	if _, err := testGit(repo.path, "tag", "-a", "v1", "-m", "tag"); err != nil {
		t.Fatal(err)
	}
	blobID, err := testGit(repo.path, "hash-object", "metadata/md5-cache/dev-libs/pkg-1")
	if err != nil {
		t.Fatal(err)
	}
	tagID, err := testGit(repo.path, "rev-parse", "refs/tags/v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{strings.Repeat("0", 40), "refs/tags/v1", "HEAD~0", strings.TrimSpace(string(tagID)), strings.TrimSpace(string(blobID))} {
		config := Config{Repository: "gentoo", SourceURL: "x", LocalPath: repo.path, Commit: value}
		if provider, err := Open(config); !errors.Is(err, ErrInvalidCommit) || provider != nil {
			t.Errorf("Open Commit %q = %p, %v; want nil and ErrInvalidCommit", value, provider, err)
		}
	}
	if provider, err := Open(Config{Repository: "gentoo", SourceURL: "x", LocalPath: repo.path, Commit: commit[:12]}); !errors.Is(err, ErrInvalidCommit) || provider != nil {
		t.Fatalf("abbreviated SHA Open = %p, %v", provider, err)
	}
}

func TestPinnedCommitIgnoresHeadWorkingTreeAndUntrackedFiles(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	file := writeCache(t, repo.path, "dev-libs", "pkg", "1", "EAPI=8\nSLOT=0\n", 0o644)
	pinned := repo.commit(t)
	provider := repo.open(t)
	if err := os.WriteFile(file, []byte("EAPI=7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo.path, "untracked"), []byte("noise"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := testGit(repo.path, "add", "--", "metadata/md5-cache/dev-libs/pkg-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := testGit(repo.path, "commit", "--quiet", "-m", "second"); err != nil {
		t.Fatal(err)
	}
	result, err := provider.Lookup(Query{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "1"})
	if err != nil || result.Status != StatusFound || result.Snapshot.Content != "EAPI=8\nSLOT=0\n" || result.Snapshot.Source.Commit != pinned {
		t.Fatalf("Lookup = %#v, %v", result, err)
	}
}

func TestReplaceRefCannotSubstitutePinnedBlob(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	writeCache(t, repo.path, "dev-libs", "pkg", "1", "EAPI=8\n", 0o644)
	repo.commit(t)
	provider := repo.open(t)
	replacementPath := filepath.Join(t.TempDir(), "replacement")
	if err := os.WriteFile(replacementPath, []byte("EAPI=7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	replacementID, err := testGit(repo.path, "hash-object", "-w", replacementPath)
	if err != nil {
		t.Fatal(err)
	}
	originalID, err := testGit(repo.path, "rev-parse", "HEAD:metadata/md5-cache/dev-libs/pkg-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testGit(repo.path, "replace", strings.TrimSpace(string(originalID)), strings.TrimSpace(string(replacementID))); err != nil {
		t.Fatal(err)
	}
	result, err := provider.Lookup(Query{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "1"})
	if err != nil || result.Status != StatusFound || result.Snapshot.Content != "EAPI=8\n" {
		t.Fatalf("Lookup = %#v, %v; replace ref changed the pinned blob", result, err)
	}
}

func TestUnsupportedTreeEntries(t *testing.T) {
	tests := []string{"symlink", "directory", "submodule"}
	for _, entryType := range tests {
		t.Run(entryType, func(t *testing.T) {
			repo := newTestRepo(t, "sha1")
			base := filepath.Join(repo.path, "metadata", "md5-cache", "dev-libs")
			if err := os.MkdirAll(base, 0o755); err != nil {
				t.Fatal(err)
			}
			cachePath := filepath.Join(base, "pkg-1")
			if entryType == "symlink" {
				if err := os.Symlink("elsewhere", cachePath); err != nil {
					t.Fatal(err)
				}
				if _, err := testGit(repo.path, "add", "--", "metadata/md5-cache/dev-libs/pkg-1"); err != nil {
					t.Fatal(err)
				}
			} else if entryType == "directory" {
				if err := os.Mkdir(cachePath, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(cachePath, "child"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := testGit(repo.path, "add", "--all"); err != nil {
					t.Fatal(err)
				}
			} else {
				nested := cachePath
				if _, err := testGit(filepath.Dir(nested), "init", "--quiet", nested); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(nested, "child"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := testGit(nested, "add", "--all"); err != nil {
					t.Fatal(err)
				}
				if _, err := testGit(nested, "commit", "--quiet", "-m", "nested"); err != nil {
					t.Fatal(err)
				}
			}
			repo.commit(t)
			result, err := repo.open(t).Lookup(Query{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "1"})
			if !errors.Is(err, ErrUnsupportedEntry) || result != (LookupResult{}) {
				t.Fatalf("Lookup = %#v, %v; want zero result and ErrUnsupportedEntry", result, err)
			}
		})
	}
}

func TestCorruptMetadataIsFoundWithNormalizeDiagnostics(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	writeCache(t, repo.path, "dev-libs", "pkg", "1", "EAPI=8\nSLOT=bad/value/extra\n", 0o644)
	repo.commit(t)
	result, err := repo.open(t).Lookup(Query{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "1"})
	if err != nil || result.Status != StatusFound || result.Snapshot == nil || len(result.Snapshot.Diagnostics) == 0 {
		t.Fatalf("Lookup = %#v, %v", result, err)
	}
}

func TestBlobReadFailureIsNotNotFound(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	writeCache(t, repo.path, "dev-libs", "pkg", "1", "EAPI=8\n", 0o644)
	repo.commit(t)
	provider := repo.open(t)
	blobID, err := testGit(repo.path, "rev-parse", "HEAD:metadata/md5-cache/dev-libs/pkg-1")
	if err != nil {
		t.Fatal(err)
	}
	objectPath := filepath.Join(repo.path, ".git", "objects", strings.TrimSpace(string(blobID))[:2], strings.TrimSpace(string(blobID))[2:])
	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	result, err := provider.Lookup(Query{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "1"})
	if !errors.Is(err, ErrRead) || result != (LookupResult{}) {
		t.Fatalf("Lookup = %#v, %v; want zero result and ErrRead", result, err)
	}
}

func TestProviderDoesNotModifyRepositoryAndIsRepeatable(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	writeCache(t, repo.path, "dev-libs", "pkg", "1", "EAPI=8\nSLOT=0\n", 0o644)
	repo.commit(t)
	provider := repo.open(t)
	statusBefore, err := testGit(repo.path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		t.Fatal(err)
	}
	refsBefore, err := testGit(repo.path, "show-ref")
	if err != nil {
		t.Fatal(err)
	}
	indexBefore, err := os.ReadFile(filepath.Join(repo.path, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := provider.Lookup(Query{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Lookup(Query{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("results differ: %#v / %#v", first, second)
	}
	statusAfter, err := testGit(repo.path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		t.Fatal(err)
	}
	refsAfter, err := testGit(repo.path, "show-ref")
	if err != nil {
		t.Fatal(err)
	}
	indexAfter, err := os.ReadFile(filepath.Join(repo.path, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	if string(statusBefore) != string(statusAfter) || string(refsBefore) != string(refsAfter) || string(indexBefore) != string(indexAfter) {
		t.Fatalf("repository changed: status %q => %q, refs %q => %q, index equal=%t", statusBefore, statusAfter, refsBefore, refsAfter, string(indexBefore) == string(indexAfter))
	}
}

func TestSHA256ObjectFormatWhenSupported(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo-sha256")
	if _, err := testGit(root, "init", "--quiet", "--object-format=sha256", repoPath); err != nil {
		t.Skipf("SHA-256 Git object format unavailable in Git %s: %v", gitVersion(t), err)
	}
	repo := &testRepo{path: repoPath}
	writeCache(t, repo.path, "dev-libs", "pkg", "1", "EAPI=8\n", 0o644)
	commit := repo.commit(t)
	if len(commit) != 64 {
		t.Fatalf("SHA-256 commit length = %d, want 64", len(commit))
	}
	result, err := repo.open(t).Lookup(Query{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "1"})
	if err != nil || result.Status != StatusFound || result.Snapshot.Source.Commit != commit || result.Snapshot.Content != "EAPI=8\n" {
		t.Fatalf("SHA-256 Lookup = %#v, %v", result, err)
	}
}

func gitVersion(t *testing.T) string {
	t.Helper()
	output, err := exec.Command("git", "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("git --version: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func TestOpenMissingGitRepositoryIsRepositoryAccessError(t *testing.T) {
	config := Config{Repository: "gentoo", SourceURL: "x", LocalPath: filepath.Join(t.TempDir(), "missing"), Commit: strings.Repeat("a", 40)}
	if provider, err := Open(config); !errors.Is(err, ErrRepositoryAccess) || provider != nil {
		t.Fatalf("Open = %p, %v; want nil and ErrRepositoryAccess", provider, err)
	}
}

func TestOpenRequiresCommitTypeForFullObjectID(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	writeCache(t, repo.path, "dev-libs", "pkg", "1", "x", 0o644)
	repo.commit(t)
	blobID, err := testGit(repo.path, "hash-object", "metadata/md5-cache/dev-libs/pkg-1")
	if err != nil {
		t.Fatal(err)
	}
	if provider, err := Open(Config{Repository: "gentoo", SourceURL: "x", LocalPath: repo.path, Commit: strings.TrimSpace(string(blobID))}); !errors.Is(err, ErrInvalidCommit) || provider != nil {
		t.Fatalf("Open blob = %p, %v", provider, err)
	}
}

func TestRepositoryNameValidation(t *testing.T) {
	for _, valid := range []string{"gentoo", "my_overlay", "repo_1"} {
		if !validRepository(valid) {
			t.Errorf("validRepository(%q) = false", valid)
		}
	}
	for _, invalid := range []string{"", "-repo", "repo/other", "repo.name", "repo-1.2", "repo\n"} {
		if validRepository(invalid) {
			t.Errorf("validRepository(%q) = true", invalid)
		}
	}
}

func TestObjectIDValidationMatchesFormats(t *testing.T) {
	for _, tc := range []struct {
		value  string
		length int
		want   bool
	}{{strings.Repeat("a", 40), 40, true}, {strings.Repeat("A", 64), 64, true}, {strings.Repeat("a", 40), 64, false}, {strings.Repeat("a", 64), 40, false}} {
		if got := validObjectID(tc.value, tc.length); got != tc.want {
			t.Errorf("validObjectID(%d chars, %d) = %v, want %v", len(tc.value), tc.length, got, tc.want)
		}
	}
}

func TestParseTreeEntryRequiresExactPathAndValidRecord(t *testing.T) {
	line := []byte("100644 blob aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\tmetadata/md5-cache/dev-libs/pkg-1\x00")
	entry, found, err := parseTreeEntry(line, "metadata/md5-cache/dev-libs/pkg-1")
	if err != nil || !found || entry.mode != "100644" || entry.kind != "blob" {
		t.Fatalf("parseTreeEntry = %#v, %v, %v", entry, found, err)
	}
	if _, found, err := parseTreeEntry(line, "metadata/md5-cache/dev-libs/pkg-10"); err != nil || found {
		t.Fatalf("prefix path matched: found=%v err=%v", found, err)
	}
	if _, _, err := parseTreeEntry([]byte("broken\x00"), "x"); err == nil {
		t.Fatal("malformed tree record accepted")
	}
}

func TestDiagnosticContentRemainsAvailableOnFound(t *testing.T) {
	repo := newTestRepo(t, "sha1")
	content := "EAPI=8\nSLOT=bad/value/extra\n"
	writeCache(t, repo.path, "dev-libs", "pkg", "1", content, 0o644)
	repo.commit(t)
	result, err := repo.open(t).Lookup(Query{Repository: "gentoo", Category: "dev-libs", Name: "pkg", Version: "1"})
	if err != nil || result.Status != StatusFound || result.Snapshot.Content != content || len(result.Snapshot.Diagnostics) == 0 {
		t.Fatalf("Lookup = %#v, %v", result, err)
	}
}
