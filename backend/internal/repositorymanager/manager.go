// Package repositorymanager pins known repositories for an analysis.
package repositorymanager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var ErrUnknownRepository = errors.New("unknown repository")
var ErrAmbiguousRepository = errors.New("ambiguous repository")

// Repository is trusted application configuration, never a plan-supplied URL.
type Repository struct {
	Name    string
	URL     string
	Masters []string
}

// KnownOverlays lists explicitly supported sources and their masters.
func KnownOverlays() []Repository {
	return []Repository{{Name: "guru", URL: "https://github.com/gentoo-mirror/guru.git", Masters: []string{"gentoo"}}}
}

type Revision struct {
	Name    string
	URL     string
	Path    string `json:"-"`
	Commit  string
	Masters []string
}

type Manager struct {
	root      string
	catalog   map[string]Repository
	ambiguous map[string]bool
	mu        sync.Mutex
}

var validName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)
var validCommit = regexp.MustCompile(`^([a-f0-9]{40}|[a-f0-9]{64})$`)

// New constructs a manager with an explicit, trusted catalog.
func New(root string, catalog []Repository) (*Manager, error) {
	if root == "" {
		return nil, errors.New("repository storage path is required")
	}
	m := &Manager{root: root, catalog: make(map[string]Repository, len(catalog)), ambiguous: make(map[string]bool)}
	for _, entry := range catalog {
		if !validName.MatchString(entry.Name) || entry.Name == "gentoo" || !validSource(entry.URL) {
			return nil, fmt.Errorf("invalid repository catalog entry %q", entry.Name)
		}
		if _, exists := m.catalog[entry.Name]; exists {
			m.ambiguous[entry.Name] = true
			continue
		}
		entry.Masters = append([]string(nil), entry.Masters...)
		m.catalog[entry.Name] = entry
	}
	for _, entry := range m.catalog {
		for _, master := range entry.Masters {
			if master != "gentoo" {
				if _, ok := m.catalog[master]; !ok {
					return nil, fmt.Errorf("unknown master %q for %q", master, entry.Name)
				}
			}
		}
	}
	return m, nil
}

func validSource(source string) bool {
	if strings.ContainsAny(source, "\x00\r\n") {
		return false
	}
	if filepath.IsAbs(source) {
		return true
	}
	parsed, err := url.Parse(source)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

// Pin returns immutable commit IDs for the named overlays and their masters.
// gentoo must already be pinned by the caller before this method is called.
// An existing mirror is refreshed before its published revision is pinned.
func (m *Manager) Pin(ctx context.Context, names []string, gentoo Revision) (map[string]Revision, error) {
	return m.PinAdditional(ctx, names, map[string]Revision{"gentoo": gentoo})
}

// PinAdditional preserves revisions already pinned for the same analysis.
func (m *Manager) PinAdditional(ctx context.Context, names []string, pinned map[string]Revision) (map[string]Revision, error) {
	gentoo := pinned["gentoo"]
	if gentoo.Name != "gentoo" || !validCommit.MatchString(gentoo.Commit) || gentoo.Path == "" {
		return nil, errors.New("pinned gentoo revision is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]Revision, len(pinned))
	for name, revision := range pinned {
		result[name] = revision
	}
	visiting := make(map[string]bool)
	var visit func(string) error
	visit = func(name string) error {
		if _, ok := result[name]; ok {
			return nil
		}
		if m.ambiguous[name] {
			return fmt.Errorf("%w: %s", ErrAmbiguousRepository, name)
		}
		entry, ok := m.catalog[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownRepository, name)
		}
		if visiting[name] {
			return fmt.Errorf("repository master cycle at %s", name)
		}
		visiting[name] = true
		defer delete(visiting, name)
		for _, master := range entry.Masters {
			if err := visit(master); err != nil {
				return err
			}
		}
		path := filepath.Join(m.root, entry.Name+".git")
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(m.root, 0o755); err != nil {
				return err
			}
			staging, err := os.MkdirTemp(m.root, ".clone-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(staging)
			if _, err = runGit(ctx, "clone", "--mirror", "--", entry.URL, staging); err != nil {
				return fmt.Errorf("clone %s: %w", name, err)
			}
			id, err := resolveCommit(ctx, staging, "HEAD^{commit}")
			if err != nil {
				return fmt.Errorf("pin %s: %w", name, err)
			}
			if err = publish(ctx, staging, id); err != nil {
				return fmt.Errorf("publish %s: %w", name, err)
			}
			if err = os.Chmod(staging, 0o755); err != nil {
				return fmt.Errorf("make %s mirror readable: %w", name, err)
			}
			if err = os.Rename(staging, path); err != nil {
				return fmt.Errorf("publish %s: %w", name, err)
			}
		} else if err != nil {
			return err
		} else {
			origin, err := runGit(ctx, "-C", path, "config", "--get", "remote.origin.url")
			if err != nil || strings.TrimSpace(string(origin)) != entry.URL {
				return fmt.Errorf("repository %s source does not match configured URL", name)
			}
			if _, err := runGit(ctx, "-C", path, "show-ref", "--verify", "--quiet", "refs/gentoo/published"); err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
					return fmt.Errorf("read publication %s: %w", name, err)
				}
				// Mirrors from the previous implementation have only HEAD.
				id, headErr := resolveCommit(ctx, path, "HEAD^{commit}")
				if headErr != nil {
					return fmt.Errorf("pin %s: %w", name, headErr)
				}
				if err := publish(ctx, path, id); err != nil {
					return fmt.Errorf("publish %s: %w", name, err)
				}
			}
			if _, err := resolveCommit(ctx, path, "refs/gentoo/published^{commit}"); err != nil {
				return fmt.Errorf("invalid publication %s: %w", name, err)
			}
			if err := os.Chmod(path, 0o755); err != nil {
				return fmt.Errorf("make %s mirror readable: %w", name, err)
			}
			_, fetchErr := runGit(ctx, "-C", path, "fetch", "--no-tags", "--no-write-fetch-head", "--refmap=", "--force", "--", entry.URL, "+HEAD:refs/gentoo/candidate")
			if fetchErr == nil {
				id, resolveErr := resolveCommit(ctx, path, "refs/gentoo/candidate^{commit}")
				if resolveErr == nil {
					resolveErr = publish(ctx, path, id)
				}
				fetchErr = resolveErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// An unsuccessful refresh leaves the previous published ref intact.
			if fetchErr != nil {
				log.Printf("overlay %s refresh failed; using last published revision", name)
			}
		}
		id, err := resolveCommit(ctx, path, "refs/gentoo/published^{commit}")
		if err != nil {
			return fmt.Errorf("pin %s: %w", name, err)
		}
		result[name] = Revision{Name: name, URL: entry.URL, Path: path, Commit: id, Masters: append([]string(nil), entry.Masters...)}
		return nil
	}
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// RestorePinned validates and opens persisted revisions without refreshing
// their mirrors. The caller supplies the local Gentoo path in its revision.
func (m *Manager) RestorePinned(ctx context.Context, pinned map[string]Revision) (map[string]Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]Revision, len(pinned))
	for name, revision := range pinned {
		if name == "gentoo" {
			if revision.Name != "gentoo" || !validCommit.MatchString(revision.Commit) || revision.Path == "" {
				return nil, errors.New("invalid pinned gentoo revision")
			}
			result[name] = revision
			continue
		}
		if m.ambiguous[name] {
			return nil, fmt.Errorf("%w: %s", ErrAmbiguousRepository, name)
		}
		entry, ok := m.catalog[name]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownRepository, name)
		}
		if revision.Name != name || revision.URL != entry.URL || !sameNames(revision.Masters, entry.Masters) || !validCommit.MatchString(revision.Commit) {
			return nil, fmt.Errorf("pinned repository %s does not match configured source", name)
		}
		path := filepath.Join(m.root, name+".git")
		origin, err := runGit(ctx, "-C", path, "config", "--get", "remote.origin.url")
		if err != nil || strings.TrimSpace(string(origin)) != entry.URL {
			return nil, fmt.Errorf("pinned repository %s mirror is unavailable or has a different source", name)
		}
		commit, err := resolveCommit(ctx, path, revision.Commit+"^{commit}")
		if err != nil || commit != revision.Commit {
			return nil, fmt.Errorf("pinned commit %s for repository %s is unavailable", revision.Commit, name)
		}
		revision.Path = path
		result[name] = revision
	}
	for name, revision := range result {
		for _, master := range revision.Masters {
			if _, ok := result[master]; !ok {
				return nil, fmt.Errorf("pinned master %s for repository %s is unavailable", master, name)
			}
		}
	}
	return result, nil
}

func sameNames(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func resolveCommit(ctx context.Context, path, ref string) (string, error) {
	output, err := runGit(ctx, "-C", path, "rev-parse", "--verify", ref)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(output))
	if !validCommit.MatchString(id) {
		return "", errors.New("invalid full commit ID")
	}
	return id, nil
}

func publish(ctx context.Context, path, id string) error {
	if _, err := runGit(ctx, "-C", path, "fsck", "--connectivity-only", "--no-reflogs", "--no-dangling", id); err != nil {
		return err
	}
	if _, err := runGit(ctx, "-C", path, "update-ref", "refs/gentoo/snapshots/"+id, id); err != nil {
		return err
	}
	_, err := runGit(ctx, "-C", path, "update-ref", "refs/gentoo/published", id)
	return err
}

func runGit(ctx context.Context, args ...string) ([]byte, error) {
	args = append([]string{"--no-replace-objects", "--no-lazy-fetch", "-c", "credential.helper=", "-c", "gc.auto=0", "-c", "maintenance.auto=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
