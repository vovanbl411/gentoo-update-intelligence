// Package ebuildresolver selects an ebuild from a commit's first-parent history.
package ebuildresolver

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrInvalidInput     = errors.New("invalid ebuild resolver input")
	ErrRepositoryAccess = errors.New("ebuild repository unavailable")
	ErrInvalidReference = errors.New("invalid ebuild reference commit")
	ErrRead             = errors.New("ebuild history read failed")
	ErrUnsupportedEntry = errors.New("unsupported ebuild tree entry")
	versionPattern      = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*[a-z]?(_(alpha|beta|pre|rc|p)[0-9]*)*(-r[0-9]+)?$`)
	versionSuffix       = regexp.MustCompile(`-[0-9]+(\.[0-9]+)*[a-z]?(_(alpha|beta|pre|rc|p)[0-9]*)*(-r[0-9]+)?$`)
)

type Input struct {
	Repository string
	Reference  string
	Category   string
	Name       string
	Version    string
}

type Status string

const (
	StatusFound             Status = "found"
	StatusNotFound          Status = "not_found"
	StatusHistoryIncomplete Status = "history_incomplete"
)

type Reason string

const (
	ReasonAtReference        Reason = "at_reference"
	ReasonFirstParentHistory Reason = "first_parent_history"
)

type Result struct {
	Status          Status
	ReferenceCommit string
	SelectedCommit  string
	EbuildPath      string
	BlobID          string
	Reason          Reason
}

type Resolver struct {
	localPath string
	objectLen int
}

func Open(localPath string) (*Resolver, error) {
	if localPath == "" {
		return nil, fmt.Errorf("%w: LocalPath is required", ErrInvalidInput)
	}
	format, err := runGit(localPath, "rev-parse", "--show-object-format")
	if err != nil {
		return nil, fmt.Errorf("%w: git repository: %v", ErrRepositoryAccess, err)
	}
	length := 0
	switch strings.TrimSpace(string(format)) {
	case "sha1":
		length = 40
	case "sha256":
		length = 64
	default:
		return nil, fmt.Errorf("%w: unsupported Git object format %q", ErrRepositoryAccess, strings.TrimSpace(string(format)))
	}
	return &Resolver{localPath: localPath, objectLen: length}, nil
}

func (r *Resolver) Resolve(input Input) (Result, error) {
	return r.resolve(input, true)
}

func (r *Resolver) resolve(input Input, fast bool) (Result, error) {
	if r == nil {
		return Result{}, fmt.Errorf("%w: nil Resolver", ErrRepositoryAccess)
	}
	if input.Repository == "" || !validRepository(input.Repository) || !validCategory(input.Category) || !validPackageName(input.Name) || !versionPattern.MatchString(input.Version) {
		return Result{}, fmt.Errorf("%w: Repository, Category, Name or Version has invalid Gentoo syntax", ErrInvalidInput)
	}
	if !validObjectID(input.Reference, r.objectLen) {
		return Result{}, fmt.Errorf("%w: Reference must be a full hexadecimal object ID", ErrInvalidReference)
	}
	typ, err := runGit(r.localPath, "cat-file", "-t", input.Reference)
	if err != nil {
		return Result{}, fmt.Errorf("%w: reference object %s is unavailable: %v", ErrInvalidReference, input.Reference, err)
	}
	if strings.TrimSpace(string(typ)) != "commit" {
		return Result{}, fmt.Errorf("%w: object %s has type %q, want commit", ErrInvalidReference, input.Reference, strings.TrimSpace(string(typ)))
	}
	reference, err := runGit(r.localPath, "rev-parse", "--verify", "--end-of-options", input.Reference)
	if err != nil {
		return Result{}, fmt.Errorf("%w: resolve reference: %v", ErrInvalidReference, err)
	}
	reference = bytes.TrimSpace(reference)
	if !validObjectID(string(reference), r.objectLen) {
		return Result{}, fmt.Errorf("%w: Git returned malformed object ID", ErrRepositoryAccess)
	}
	path := input.Category + "/" + input.Name + "/" + input.Name + "-" + input.Version + ".ebuild"
	shallow, err := shallowCommits(r.localPath, r.objectLen)
	if err != nil {
		return Result{}, fmt.Errorf("%w: read shallow boundary: %v", ErrRead, err)
	}
	commit := string(reference)
	first := true
	for {
		entry, found, err := lookup(r.localPath, commit, path)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %s at %s: %v", ErrRead, path, commit, err)
		}
		if found {
			if entry.mode != "100644" && entry.mode != "100755" || entry.kind != "blob" {
				return Result{}, fmt.Errorf("%w: %s has mode %s and type %s", ErrUnsupportedEntry, path, entry.mode, entry.kind)
			}
			if _, err := runGit(r.localPath, "cat-file", "blob", entry.objectID); err != nil {
				return Result{}, fmt.Errorf("%w: blob %s at %s: %v", ErrRead, entry.objectID, path, err)
			}
			reason := ReasonFirstParentHistory
			if first {
				reason = ReasonAtReference
			}
			return Result{Status: StatusFound, ReferenceCommit: string(reference), SelectedCommit: commit, EbuildPath: path, BlobID: entry.objectID, Reason: reason}, nil
		}
		if first && fast {
			if selected, ok := r.findDeletedPath(string(reference), path); ok {
				return selected, nil
			}
		}
		rawCommit, err := runGit(r.localPath, "cat-file", "commit", commit)
		if err != nil {
			return Result{}, fmt.Errorf("%w: commit %s: %v", ErrRead, commit, err)
		}
		parents, err := parseParents(rawCommit, r.objectLen)
		if err != nil {
			return Result{}, fmt.Errorf("%w: commit %s: %v", ErrRead, commit, err)
		}
		if _, isShallow := shallow[commit]; isShallow {
			return Result{Status: StatusHistoryIncomplete, ReferenceCommit: string(reference), EbuildPath: path}, nil
		}
		if len(parents) == 0 {
			return Result{Status: StatusNotFound, ReferenceCommit: string(reference), EbuildPath: path}, nil
		}
		commit = parents[0]
		parentType, err := runGit(r.localPath, "cat-file", "-t", commit)
		if err != nil {
			return Result{}, fmt.Errorf("%w: first parent %s of %s unavailable: %v", ErrRead, commit, string(reference), err)
		}
		if strings.TrimSpace(string(parentType)) != "commit" {
			return Result{}, fmt.Errorf("%w: first parent %s of %s has type %q, want commit", ErrRead, commit, string(reference), strings.TrimSpace(string(parentType)))
		}
		first = false
	}
}

// findDeletedPath uses one path-limited first-parent walk when Git can prove
// the immediately preceding state. Ambiguous or incomplete history falls
// back to the exhaustive walk, which preserves its detailed error handling.
func (r *Resolver) findDeletedPath(reference, path string) (Result, bool) {
	output, err := runGit(r.localPath, "log", "--first-parent", "--full-history", "-1", "--format=%H %P", reference, "--", ":(literal)"+path)
	if err != nil {
		return Result{}, false
	}
	fields := strings.Fields(string(output))
	if len(fields) < 2 || !validObjectID(fields[0], r.objectLen) || !validObjectID(fields[1], r.objectLen) {
		return Result{}, false
	}
	parent := fields[1]
	typ, err := runGit(r.localPath, "cat-file", "-t", parent)
	if err != nil || strings.TrimSpace(string(typ)) != "commit" {
		return Result{}, false
	}
	entry, found, err := lookup(r.localPath, parent, path)
	if err != nil || !found || (entry.mode != "100644" && entry.mode != "100755") || entry.kind != "blob" {
		return Result{}, false
	}
	if _, err := runGit(r.localPath, "cat-file", "blob", entry.objectID); err != nil {
		return Result{}, false
	}
	return Result{Status: StatusFound, ReferenceCommit: reference, SelectedCommit: parent, EbuildPath: path, BlobID: entry.objectID, Reason: ReasonFirstParentHistory}, true
}

type treeEntry struct{ mode, kind, objectID string }

func lookup(repo, commit, path string) (treeEntry, bool, error) {
	output, err := runGit(repo, "ls-tree", "-z", "--full-tree", commit, "--", ":(literal)"+path)
	if err != nil {
		return treeEntry{}, false, err
	}
	if len(output) == 0 {
		return treeEntry{}, false, nil
	}
	var found treeEntry
	matched := false
	for _, record := range bytes.Split(output, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		metadata, filePath, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return treeEntry{}, false, fmt.Errorf("malformed ls-tree record")
		}
		if string(filePath) != path {
			continue
		}
		fields := strings.Fields(string(metadata))
		if len(fields) != 3 || !validHexObjectID(fields[2]) {
			return treeEntry{}, false, fmt.Errorf("malformed ls-tree entry")
		}
		if matched {
			return treeEntry{}, false, fmt.Errorf("duplicate exact tree path")
		}
		found = treeEntry{mode: fields[0], kind: fields[1], objectID: fields[2]}
		matched = true
	}
	return found, matched, nil
}

func shallowCommits(repo string, objectLen int) (map[string]struct{}, error) {
	output, err := runGit(repo, "rev-parse", "--git-path", "shallow")
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(string(output))
	if !filepath.IsAbs(path) {
		path = filepath.Join(repo, path)
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]struct{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{})
	for _, line := range strings.Fields(string(data)) {
		if !validObjectID(line, objectLen) {
			return nil, fmt.Errorf("malformed shallow object ID")
		}
		result[line] = struct{}{}
	}
	return result, nil
}

func parseParents(commit []byte, length int) ([]string, error) {
	var parents []string
	header := bytes.SplitN(commit, []byte("\n\n"), 2)[0]
	for _, line := range bytes.Split(header, []byte{'\n'}) {
		if !bytes.HasPrefix(line, []byte("parent ")) {
			continue
		}
		id := string(bytes.TrimPrefix(line, []byte("parent ")))
		if !validObjectID(id, length) {
			return nil, fmt.Errorf("malformed parent object ID")
		}
		parents = append(parents, id)
	}
	return parents, nil
}

func runGit(repo string, args ...string) ([]byte, error) {
	all := append([]string{"--no-replace-objects", "--no-lazy-fetch", "-C", repo}, args...)
	cmd := exec.Command("git", all...)
	output, err := cmd.Output()
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
	for i := range value {
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
	for i := range value {
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
	for i := range value {
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
	for i := range value {
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
