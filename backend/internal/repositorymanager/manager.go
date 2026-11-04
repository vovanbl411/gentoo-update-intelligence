// Package repositorymanager pins known repositories for an analysis.
package repositorymanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var ErrUnknownRepository = errors.New("unknown repository")

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
	root    string
	catalog map[string]Repository
	mu      sync.Mutex
}

var validName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)
var validCommit = regexp.MustCompile(`^([a-f0-9]{40}|[a-f0-9]{64})$`)

// New constructs a manager with an explicit, trusted catalog.
func New(root string, catalog []Repository) (*Manager, error) {
	if root == "" {
		return nil, errors.New("repository storage path is required")
	}
	m := &Manager{root: root, catalog: make(map[string]Repository, len(catalog))}
	for _, entry := range catalog {
		if !validName.MatchString(entry.Name) || entry.Name == "gentoo" || entry.URL == "" || strings.HasPrefix(entry.URL, "-") {
			return nil, fmt.Errorf("invalid repository catalog entry %q", entry.Name)
		}
		if _, exists := m.catalog[entry.Name]; exists {
			return nil, fmt.Errorf("duplicate repository %q", entry.Name)
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

// Pin returns immutable commit IDs for the named overlays and their masters.
// gentoo must already be pinned by the caller before this method is called.
// An existing local mirror is reused; it is not implicitly fetched again.
func (m *Manager) Pin(ctx context.Context, names []string, gentoo Revision) (map[string]Revision, error) {
	if gentoo.Name != "gentoo" || !validCommit.MatchString(gentoo.Commit) || gentoo.Path == "" {
		return nil, errors.New("pinned gentoo revision is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := map[string]Revision{"gentoo": gentoo}
	visiting := make(map[string]bool)
	var visit func(string) error
	visit = func(name string) error {
		if _, ok := result[name]; ok {
			return nil
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
			if err = os.Rename(staging, path); err != nil {
				return fmt.Errorf("publish %s: %w", name, err)
			}
		} else if err != nil {
			return err
		}
		commit, err := runGit(ctx, "-C", path, "rev-parse", "--verify", "HEAD^{commit}")
		if err != nil {
			return fmt.Errorf("pin %s: %w", name, err)
		}
		id := strings.TrimSpace(string(commit))
		if !validCommit.MatchString(id) {
			return fmt.Errorf("invalid commit returned for %s", name)
		}
		if _, err := runGit(ctx, "-C", path, "update-ref", "refs/gentoo/snapshots/"+id, id); err != nil {
			return fmt.Errorf("retain %s: %w", name, err)
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
