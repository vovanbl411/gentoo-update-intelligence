// Package metadatagenerator generates an exact Gentoo md5-cache entry from a
// resolver-selected ebuild in an isolated checkout.
package metadatagenerator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/ebuildresolver"
)

var (
	ErrInvalidInput = errors.New("invalid metadata generator input")
	ErrCheckout     = errors.New("invalid metadata source checkout")
	ErrEgencache    = errors.New("egencache failed")
	ErrCacheMissing = errors.New("exact generated metadata cache is missing")
)

// Config contains executable overrides and optional Portage identity settings
// for local runs. Identity settings are deliberately empty by default.
type Config struct {
	EgencachePath    string
	EmergePath       string
	PortageUserName  string
	PortageGroupName string
}

type Request struct {
	Resolution ebuildresolver.Result
	Category   string
	Name       string
	Version    string
	Checkout   string
	Profile    string
	TempDir    string
	Repository string
	Masters    []Master
}

// Master is a clean checkout pinned to the commit used for generation.
type Master struct {
	Name     string
	Checkout string
	Commit   string
}

type MasterRevision struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

type Provenance struct {
	ReferenceCommit  string           `json:"reference_commit"`
	SelectedCommit   string           `json:"selected_commit"`
	SelectionReason  string           `json:"selection_reason"`
	EbuildPath       string           `json:"ebuild_path"`
	EbuildBlobID     string           `json:"ebuild_blob_id"`
	Profile          string           `json:"profile"`
	PortageVersion   string           `json:"portage_version"`
	CachePath        string           `json:"cache_path"`
	CacheSHA256      string           `json:"cache_sha256"`
	EgencachePath    string           `json:"egencache_path"`
	EgencacheArgs    []string         `json:"egencache_args"`
	EgencacheStdout  string           `json:"egencache_stdout"`
	EgencacheStderr  string           `json:"egencache_stderr"`
	Repositories     string           `json:"repositories_configuration"`
	MakeConf         string           `json:"make_conf"`
	Path             string           `json:"path"`
	PortageUserName  string           `json:"portage_username,omitempty"`
	PortageGroupName string           `json:"portage_group_name,omitempty"`
	MasterRevisions  []MasterRevision `json:"master_revisions,omitempty"`
}

type Result struct {
	Content    []byte
	SHA256     string
	Provenance Provenance
}

type Generator struct {
	egencachePath string
	emergePath    string
	userName      string
	groupName     string
}

var versionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*[a-z]?(_(alpha|beta|pre|rc|p)[0-9]*)*(-r[0-9]+)?$`)
var portageIdentityPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
var portageVersionPattern = regexp.MustCompile(`(?m)^Portage ([^ ]+)`)
var repositoryNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)
var fullCommitPattern = regexp.MustCompile(`^([a-f0-9]{40}|[a-f0-9]{64})$`)

func New(config Config) *Generator {
	egencache := config.EgencachePath
	if egencache == "" {
		egencache = "egencache"
	}
	emerge := config.EmergePath
	if emerge == "" {
		emerge = "emerge"
	}
	return &Generator{egencachePath: egencache, emergePath: emerge, userName: config.PortageUserName, groupName: config.PortageGroupName}
}

func (g *Generator) Generate(request Request) (Result, error) {
	if request.Repository == "" {
		request.Repository = "gentoo"
	}
	if g == nil || !validPair(request.Category, request.Name) || !versionPattern.MatchString(request.Version) ||
		!repositoryNamePattern.MatchString(request.Repository) ||
		g.userName != "" && !portageIdentityPattern.MatchString(g.userName) || g.groupName != "" && !portageIdentityPattern.MatchString(g.groupName) ||
		request.Resolution.Status != ebuildresolver.StatusFound || request.Resolution.ReferenceCommit == "" ||
		request.Resolution.SelectedCommit == "" || request.Resolution.BlobID == "" || request.Resolution.EbuildPath == "" ||
		(request.Resolution.Reason != ebuildresolver.ReasonAtReference && request.Resolution.Reason != ebuildresolver.ReasonFirstParentHistory) ||
		request.Checkout == "" || request.Profile == "" || request.TempDir == "" {
		return Result{}, fmt.Errorf("%w: found resolution, exact package/version, checkout, profile and temp directory are required", ErrInvalidInput)
	}
	wantEbuild := filepath.ToSlash(filepath.Join(request.Category, request.Name, request.Name+"-"+request.Version+".ebuild"))
	if request.Resolution.EbuildPath != wantEbuild {
		return Result{}, fmt.Errorf("%w: resolver path %q does not match requested path %q", ErrInvalidInput, request.Resolution.EbuildPath, wantEbuild)
	}
	checkout, err := filepath.Abs(request.Checkout)
	if err != nil {
		return Result{}, fmt.Errorf("%w: resolve checkout path: %v", ErrCheckout, err)
	}
	if strings.ContainsAny(checkout, "\x00\r\n") {
		return Result{}, fmt.Errorf("%w: checkout path cannot contain NUL or line breaks", ErrInvalidInput)
	}
	profile, err := filepath.Abs(request.Profile)
	if err != nil {
		return Result{}, fmt.Errorf("%w: resolve profile path: %v", ErrInvalidInput, err)
	}
	tempDir, err := filepath.Abs(request.TempDir)
	if err != nil {
		return Result{}, fmt.Errorf("%w: resolve temp directory: %v", ErrInvalidInput, err)
	}
	profileRoots := []string{checkout}
	masterRevisions := make([]MasterRevision, 0, len(request.Masters))
	repositories := ""
	seenRepositories := map[string]bool{request.Repository: true}
	for _, master := range request.Masters {
		if !repositoryNamePattern.MatchString(master.Name) || seenRepositories[master.Name] || !fullCommitPattern.MatchString(master.Commit) || master.Checkout == "" {
			return Result{}, fmt.Errorf("%w: invalid or duplicate master repository %q", ErrInvalidInput, master.Name)
		}
		masterPath, pathErr := filepath.Abs(master.Checkout)
		if pathErr != nil || strings.ContainsAny(masterPath, "\x00\r\n") {
			return Result{}, fmt.Errorf("%w: invalid master checkout path", ErrInvalidInput)
		}
		if err := validateMaster(masterPath, master.Commit); err != nil {
			return Result{}, fmt.Errorf("%w: master %s: %v", ErrCheckout, master.Name, err)
		}
		seenRepositories[master.Name] = true
		profileRoots = append(profileRoots, masterPath)
		masterRevisions = append(masterRevisions, MasterRevision{Name: master.Name, Commit: master.Commit})
		repositories += "[" + master.Name + "]\nlocation = " + masterPath + "\n"
	}
	if err := validateCheckout(checkout, profile, request, profileRoots); err != nil {
		return Result{}, err
	}
	resolvedCheckout, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		return Result{}, fmt.Errorf("%w: resolve checkout: %v", ErrCheckout, err)
	}
	resolvedTemp, err := filepath.EvalSymlinks(tempDir)
	if err != nil {
		return Result{}, fmt.Errorf("%w: temp directory must already exist: %v", ErrInvalidInput, err)
	}
	if relative, relErr := filepath.Rel(resolvedCheckout, resolvedTemp); relErr != nil || relative == "." || !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && relative != ".." {
		return Result{}, fmt.Errorf("%w: temp directory must resolve outside the source checkout", ErrInvalidInput)
	}
	if info, err := os.Stat(resolvedTemp); err != nil || !info.IsDir() {
		return Result{}, fmt.Errorf("%w: temp directory is not a directory", ErrInvalidInput)
	}
	resolvedProfile, err := filepath.EvalSymlinks(profile)
	if err != nil {
		return Result{}, fmt.Errorf("%w: resolve selected profile: %v", ErrInvalidInput, err)
	}
	profileRelative, err := filepath.Rel(resolvedCheckout, resolvedProfile)
	if err != nil {
		return Result{}, fmt.Errorf("%w: resolve selected profile relative to checkout: %v", ErrInvalidInput, err)
	}
	if profileRelative == ".." || strings.HasPrefix(profileRelative, ".."+string(os.PathSeparator)) {
		for _, master := range request.Masters {
			root, rootErr := filepath.EvalSymlinks(master.Checkout)
			if rootErr != nil {
				continue
			}
			rel, relErr := filepath.Rel(root, resolvedProfile)
			if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				profileRelative = master.Name + ":" + filepath.ToSlash(rel)
				break
			}
		}
	}
	configRoot := filepath.Join(tempDir, "config-root")
	cacheDir := filepath.Join(tempDir, "intermediate-cache")
	for _, path := range []string{configRoot, cacheDir} {
		if _, err := os.Lstat(path); err == nil {
			return Result{}, fmt.Errorf("%w: generator path already exists: %s", ErrInvalidInput, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return Result{}, fmt.Errorf("%w: inspect generator path %s: %v", ErrInvalidInput, path, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(configRoot, "etc", "portage"), 0o700); err != nil {
		return Result{}, fmt.Errorf("%w: create config root: %v", ErrInvalidInput, err)
	}
	makeConf := ""
	if g.userName != "" {
		makeConf += "PORTAGE_USERNAME=\"" + g.userName + "\"\n"
	}
	if g.groupName != "" {
		makeConf += "PORTAGE_GRPNAME=\"" + g.groupName + "\"\n"
	}
	if err := os.WriteFile(filepath.Join(configRoot, "etc", "portage", "make.conf"), []byte(makeConf), 0o600); err != nil {
		return Result{}, fmt.Errorf("%w: write isolated make.conf: %v", ErrInvalidInput, err)
	}
	if err := os.Symlink(profile, filepath.Join(configRoot, "etc", "portage", "make.profile")); err != nil {
		return Result{}, fmt.Errorf("%w: select profile: %v", ErrInvalidInput, err)
	}
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("%w: create intermediate cache: %v", ErrInvalidInput, err)
	}
	repositories += "[" + request.Repository + "]\nlocation = " + checkout + "\n"
	versionCommand := exec.Command(g.emergePath, "--version")
	versionCommand.Env = isolatedEnvironment(tempDir)
	portageOutput, err := versionCommand.Output()
	if err != nil {
		return Result{}, fmt.Errorf("%w: read Portage version with %q --version: %v", ErrEgencache, g.emergePath, err)
	}
	portageVersion := strings.TrimSpace(string(portageOutput))
	if portageVersion == "" {
		return Result{}, fmt.Errorf("%w: %q --version returned empty output", ErrEgencache, g.emergePath)
	}
	versionMatch := portageVersionPattern.FindStringSubmatch(portageVersion)
	if len(versionMatch) != 2 {
		return Result{}, fmt.Errorf("%w: cannot parse Portage version from %q --version output", ErrEgencache, g.emergePath)
	}
	portageVersion = versionMatch[1]
	args := []string{
		"--ignore-default-opts", "--repo", request.Repository, "--update", request.Category + "/" + request.Name,
		"--config-root", configRoot, "--cache-dir", cacheDir, "--repositories-configuration", repositories,
	}
	command := exec.Command(g.egencachePath, args...)
	command.Dir = checkout
	command.Env = isolatedEnvironment(tempDir)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if err != nil {
		return Result{}, fmt.Errorf("%w: %q %s: %v: %s %s", ErrEgencache, g.egencachePath, strings.Join(args, " "), err, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}
	cachePath := filepath.Join(checkout, "metadata", "md5-cache", request.Category, request.Name+"-"+request.Version)
	info, err := os.Lstat(cachePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{}, fmt.Errorf("%w: %s (egencache stdout: %q; stderr: %q)", ErrCacheMissing,
				filepath.ToSlash(filepath.Join("metadata/md5-cache", request.Category, request.Name+"-"+request.Version)),
				strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
		}
		return Result{}, fmt.Errorf("%w: read %s: %v", ErrCacheMissing, cachePath, err)
	}
	if !info.Mode().IsRegular() {
		return Result{}, fmt.Errorf("%w: exact cache path is not a regular file", ErrCacheMissing)
	}
	content, err := os.ReadFile(cachePath)
	if err != nil {
		return Result{}, fmt.Errorf("%w: read %s: %v", ErrCacheMissing, cachePath, err)
	}
	digest := sha256.Sum256(content)
	sha := hex.EncodeToString(digest[:])
	return Result{
		Content: content,
		SHA256:  sha,
		Provenance: Provenance{
			ReferenceCommit:  request.Resolution.ReferenceCommit,
			SelectedCommit:   request.Resolution.SelectedCommit,
			SelectionReason:  string(request.Resolution.Reason),
			EbuildPath:       request.Resolution.EbuildPath,
			EbuildBlobID:     request.Resolution.BlobID,
			Profile:          filepath.ToSlash(profileRelative),
			PortageVersion:   portageVersion,
			CachePath:        filepath.ToSlash(filepath.Join("metadata/md5-cache", request.Category, request.Name+"-"+request.Version)),
			CacheSHA256:      sha,
			EgencachePath:    g.egencachePath,
			EgencacheArgs:    args,
			EgencacheStdout:  strings.TrimSpace(stdout.String()),
			EgencacheStderr:  strings.TrimSpace(stderr.String()),
			Repositories:     repositories,
			MakeConf:         makeConf,
			Path:             os.Getenv("PATH"),
			PortageUserName:  g.userName,
			PortageGroupName: g.groupName,
			MasterRevisions:  masterRevisions,
		},
	}, nil
}

func validateCheckout(checkout, profile string, request Request, profileRoots []string) error {
	root, err := git(checkout, "rev-parse", "--show-toplevel")
	if err != nil || filepath.Clean(strings.TrimSpace(string(root))) != filepath.Clean(checkout) {
		return fmt.Errorf("%w: %s is not a Git checkout root", ErrCheckout, checkout)
	}
	status, err := git(checkout, "status", "--porcelain", "--untracked-files=all", "--ignored")
	if err != nil {
		return fmt.Errorf("%w: read checkout status: %v", ErrCheckout, err)
	}
	if len(status) != 0 {
		return fmt.Errorf("%w: source checkout is not clean: %s", ErrCheckout, strings.TrimSpace(string(status)))
	}
	referenceType, err := git(checkout, "cat-file", "-t", request.Resolution.ReferenceCommit)
	if err != nil || strings.TrimSpace(string(referenceType)) != "commit" {
		return fmt.Errorf("%w: resolver reference is not an available commit", ErrCheckout)
	}
	head, err := git(checkout, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != request.Resolution.SelectedCommit {
		return fmt.Errorf("%w: HEAD does not match selected commit %s", ErrCheckout, request.Resolution.SelectedCommit)
	}
	entry, err := git(checkout, "ls-tree", "-z", "--full-tree", request.Resolution.SelectedCommit, "--", ":(literal)"+request.Resolution.EbuildPath)
	if err != nil {
		return fmt.Errorf("%w: inspect selected ebuild: %v", ErrCheckout, err)
	}
	fields, path, ok := bytes.Cut(bytes.TrimSuffix(entry, []byte{0}), []byte{'\t'})
	metadata := strings.Fields(string(fields))
	if !ok || string(path) != request.Resolution.EbuildPath || len(metadata) != 3 || metadata[0] != "100644" && metadata[0] != "100755" || metadata[1] != "blob" || metadata[2] != request.Resolution.BlobID {
		return fmt.Errorf("%w: selected tree entry does not match resolver blob %s", ErrCheckout, request.Resolution.BlobID)
	}
	blob, err := git(checkout, "cat-file", "blob", request.Resolution.BlobID)
	if err != nil {
		return fmt.Errorf("%w: read selected ebuild blob: %v", ErrCheckout, err)
	}
	ebuildPath := filepath.Join(checkout, filepath.FromSlash(request.Resolution.EbuildPath))
	working, err := os.ReadFile(ebuildPath)
	if err != nil || !bytes.Equal(working, blob) {
		return fmt.Errorf("%w: checkout ebuild contents do not match selected blob", ErrCheckout)
	}
	resolvedProfile, err := filepath.EvalSymlinks(profile)
	if err != nil {
		return fmt.Errorf("%w: selected profile is unavailable: %v", ErrInvalidInput, err)
	}
	profileAllowed := false
	for _, root := range profileRoots {
		resolvedRoot, rootErr := filepath.EvalSymlinks(root)
		if rootErr != nil {
			return fmt.Errorf("%w: resolve checkout: %v", ErrCheckout, rootErr)
		}
		rel, relErr := filepath.Rel(resolvedRoot, resolvedProfile)
		if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			profileAllowed = true
			break
		}
	}
	if !profileAllowed {
		return fmt.Errorf("%w: profile must resolve inside a pinned checkout", ErrInvalidInput)
	}
	info, err := os.Stat(resolvedProfile)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%w: selected profile is not a directory", ErrInvalidInput)
	}
	cachePath := filepath.Join(checkout, "metadata", "md5-cache", request.Category, request.Name+"-"+request.Version)
	if _, err := os.Lstat(cachePath); err == nil {
		return fmt.Errorf("%w: requested cache already exists in source checkout", ErrCheckout)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: inspect requested cache: %v", ErrCheckout, err)
	}
	return nil
}

func validateMaster(checkout, commit string) error {
	root, err := git(checkout, "rev-parse", "--show-toplevel")
	if err != nil || filepath.Clean(strings.TrimSpace(string(root))) != filepath.Clean(checkout) {
		return errors.New("not a Git checkout root")
	}
	head, err := git(checkout, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != commit {
		return errors.New("HEAD does not match pinned commit")
	}
	status, err := git(checkout, "status", "--porcelain", "--untracked-files=all", "--ignored")
	if err != nil || len(status) != 0 {
		return errors.New("checkout is not clean")
	}
	return nil
}

func git(repo string, args ...string) ([]byte, error) {
	all := append([]string{"--no-replace-objects", "--no-lazy-fetch", "-C", repo}, args...)
	command := exec.Command("git", all...)
	output, err := command.Output()
	if err == nil {
		return output, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil, fmt.Errorf("git %s (exit %d): %s", strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
	}
	return nil, err
}

func isolatedEnvironment(tempDir string) []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + tempDir, "TMPDIR=" + tempDir, "LANG=C"}
}

func validPair(category, name string) bool {
	if category == "" || name == "" || strings.Contains(name, "-") && versionSuffix.MatchString(name) {
		return false
	}
	if category[0] == '-' || category[0] == '.' || category[0] == '+' || name[0] == '-' || name[0] == '+' {
		return false
	}
	for _, c := range category {
		if !asciiAlphaNumeric(c) && c != '+' && c != '_' && c != '.' && c != '-' {
			return false
		}
	}
	for _, c := range name {
		if !asciiAlphaNumeric(c) && c != '+' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

var versionSuffix = regexp.MustCompile(`-[0-9]+(\.[0-9]+)*[a-z]?(_(alpha|beta|pre|rc|p)[0-9]*)*(-r[0-9]+)?$`)

func asciiAlphaNumeric(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
