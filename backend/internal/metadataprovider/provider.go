// Package metadataprovider reads exact Gentoo md5-cache blobs from a pinned
// commit in a local Git repository.
package metadataprovider

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
)

var (
	ErrInvalidConfig    = errors.New("invalid metadata provider config")
	ErrInvalidQuery     = errors.New("invalid metadata query")
	ErrRepositoryAccess = errors.New("metadata repository unavailable")
	ErrInvalidCommit    = errors.New("invalid metadata commit")
	ErrRead             = errors.New("metadata read failed")
	ErrUnsupportedEntry = errors.New("unsupported metadata tree entry")
)

type Config struct {
	Repository string
	SourceURL  string
	LocalPath  string
	Commit     string
}

type Query struct {
	Repository string
	Category   string
	Name       string
	Version    string
}

type LookupStatus string

const (
	StatusFound                 LookupStatus = "found"
	StatusNotFound              LookupStatus = "not_found"
	StatusRepositoryUnavailable LookupStatus = "repository_unavailable"
)

type LookupResult struct {
	Status   LookupStatus
	Snapshot *metadatadiff.Snapshot
}

type Provider struct {
	repository string
	sourceURL  string
	localPath  string
	commit     string
}

var (
	versionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*[a-z]?(_(alpha|beta|pre|rc|p)[0-9]*)*(-r[0-9]+)?$`)
	versionSuffix  = regexp.MustCompile(`-[0-9]+(\.[0-9]+)*[a-z]?(_(alpha|beta|pre|rc|p)[0-9]*)*(-r[0-9]+)?$`)
)

func Open(config Config) (*Provider, error) {
	if !validRepository(config.Repository) || config.SourceURL == "" || config.LocalPath == "" || config.Commit == "" {
		return nil, fmt.Errorf("%w: Repository, SourceURL, LocalPath and Commit are required; Repository must be a Gentoo repository name", ErrInvalidConfig)
	}

	format, err := runGit(config.LocalPath, "rev-parse", "--show-object-format")
	if err != nil {
		return nil, fmt.Errorf("%w: git rev-parse --show-object-format: %v", ErrRepositoryAccess, err)
	}
	formatName := strings.TrimSpace(string(format))
	length := 0
	switch formatName {
	case "sha1":
		length = 40
	case "sha256":
		length = 64
	default:
		return nil, fmt.Errorf("%w: unsupported Git object format %q", ErrRepositoryAccess, formatName)
	}
	if !validObjectID(config.Commit, length) {
		return nil, fmt.Errorf("%w: Commit must be a full %s hexadecimal object ID", ErrInvalidCommit, formatName)
	}

	typ, err := runGit(config.LocalPath, "cat-file", "-t", config.Commit)
	if err != nil {
		return nil, fmt.Errorf("%w: commit object %s is unavailable: %v", ErrInvalidCommit, config.Commit, err)
	}
	if strings.TrimSpace(string(typ)) != "commit" {
		return nil, fmt.Errorf("%w: object %s has type %q, want commit", ErrInvalidCommit, config.Commit, strings.TrimSpace(string(typ)))
	}
	fullID, err := runGit(config.LocalPath, "rev-parse", "--verify", "--end-of-options", config.Commit)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve full commit ID: %v", ErrInvalidCommit, err)
	}
	fullID = bytes.TrimSpace(fullID)
	if !validObjectID(string(fullID), length) {
		return nil, fmt.Errorf("%w: Git returned a malformed full object ID", ErrRepositoryAccess)
	}

	return &Provider{
		repository: config.Repository,
		sourceURL:  config.SourceURL,
		localPath:  config.LocalPath,
		commit:     string(fullID),
	}, nil
}

func (p *Provider) Lookup(query Query) (LookupResult, error) {
	if p == nil {
		return LookupResult{}, fmt.Errorf("%w: nil Provider", ErrRepositoryAccess)
	}
	if !validRepository(query.Repository) || !validCategory(query.Category) || !validPackageName(query.Name) || !versionPattern.MatchString(query.Version) {
		return LookupResult{}, fmt.Errorf("%w: Repository, Category, Name or Version has invalid Gentoo syntax", ErrInvalidQuery)
	}
	if query.Repository != p.repository {
		return LookupResult{Status: StatusRepositoryUnavailable}, nil
	}

	cachePath := "metadata/md5-cache/" + query.Category + "/" + query.Name + "-" + query.Version
	output, err := runGit(p.localPath, "ls-tree", "-z", "--full-tree", p.commit, "--", ":(literal)"+cachePath)
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: ls-tree %s: %v", ErrRead, cachePath, err)
	}
	entry, found, err := parseTreeEntry(output, cachePath)
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: ls-tree entry %s: %v", ErrRead, cachePath, err)
	}
	if !found {
		return LookupResult{Status: StatusNotFound}, nil
	}
	if (entry.mode != "100644" && entry.mode != "100755") || entry.kind != "blob" {
		return LookupResult{}, fmt.Errorf("%w: %s has mode %s and type %s", ErrUnsupportedEntry, cachePath, entry.mode, entry.kind)
	}
	content, err := runGit(p.localPath, "cat-file", "blob", entry.objectID)
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: cat-file blob %s at %s: %v", ErrRead, entry.objectID, cachePath, err)
	}
	source := metadatadiff.Source{
		Category: query.Category, Name: query.Name, Version: query.Version,
		Repository: p.repository, SourceURL: p.sourceURL, Commit: p.commit, CachePath: cachePath,
	}
	snapshot := metadatadiff.Normalize(source, string(content))
	return LookupResult{Status: StatusFound, Snapshot: &snapshot}, nil
}

type treeEntry struct {
	mode     string
	kind     string
	objectID string
}

func parseTreeEntry(output []byte, wantedPath string) (treeEntry, bool, error) {
	if len(output) == 0 {
		return treeEntry{}, false, nil
	}
	records := bytes.Split(output, []byte{0})
	var match treeEntry
	found := false
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		metadata, path, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return treeEntry{}, false, fmt.Errorf("malformed NUL-delimited ls-tree record")
		}
		if string(path) != wantedPath {
			continue
		}
		fields := strings.Fields(string(metadata))
		if len(fields) != 3 || !validHexObjectID(fields[2]) {
			return treeEntry{}, false, fmt.Errorf("malformed metadata for exact path")
		}
		if found {
			return treeEntry{}, false, fmt.Errorf("multiple records for exact path")
		}
		match = treeEntry{mode: fields[0], kind: fields[1], objectID: fields[2]}
		found = true
	}
	return match, found, nil
}

func runGit(localPath string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"--no-replace-objects", "--no-lazy-fetch", "-C", localPath}, args...)
	command := exec.Command("git", commandArgs...)
	output, err := command.Output()
	if err == nil {
		return output, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil, fmt.Errorf("%s (exit %d): %s", strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
	}
	return nil, fmt.Errorf("%s: %w", strings.Join(args, " "), err)
}

func validObjectID(value string, length int) bool {
	return len(value) == length && validHexObjectID(value)
}

func validHexObjectID(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func validRepository(value string) bool {
	if value == "" || value[0] == '-' {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !(isASCIIAlnum(c) || c == '_' || c == '-') {
			return false
		}
	}
	return validPackageName(value)
}

func validCategory(value string) bool {
	if value == "" || value[0] == '-' || value[0] == '.' || value[0] == '+' {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !(isASCIIAlnum(c) || c == '+' || c == '_' || c == '.' || c == '-') {
			return false
		}
	}
	return true
}

func validPackageName(value string) bool {
	if value == "" || value[0] == '-' || value[0] == '+' {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !(isASCIIAlnum(c) || c == '+' || c == '_' || c == '-') {
			return false
		}
	}
	return !versionSuffix.MatchString(value)
}

func isASCIIAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
