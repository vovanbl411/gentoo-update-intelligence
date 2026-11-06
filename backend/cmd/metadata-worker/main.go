package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/ebuildresolver"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatacache"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatagenerator"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
)

type repository struct {
	Name, URL, Path, Commit string
}
type request struct {
	Repository repository
	Masters    []repository
	Category   string
	Name       string
	Version    string
	Profile    string
}
type response struct {
	Result       metadatagenerator.Result `json:"result"`
	Cached       bool                     `json:"cached"`
	SourceCommit string                   `json:"source_commit,omitempty"`
}

const defaultGenerationLimit = 2
const defaultPreparationLimit = 2

type preparationOutcome struct {
	result       metadatagenerator.Result
	cached       bool
	sourceCommit string
	err          error
}

type preparationCall struct {
	done    chan struct{}
	outcome preparationOutcome
	waiters int
}

type metadataWorker struct {
	mu      sync.Mutex
	calls   map[string]*preparationCall
	prepare func(request) preparationOutcome
}

func newMetadataWorkerWithPreparationLimit(limit, preparationLimit int) (*metadataWorker, error) {
	if limit <= 0 {
		return nil, errors.New("metadata generation limit must be positive")
	}
	if preparationLimit <= 0 {
		return nil, errors.New("metadata preparation limit must be positive")
	}
	limiter := make(chan struct{}, limit)
	return &metadataWorker{
		calls: make(map[string]*preparationCall),
		prepare: limitPreparation(preparationLimit, func(input request) preparationOutcome {
			result, cached, sourceCommit, err := prepareMetadata(input, limiter)
			return preparationOutcome{result: result, cached: cached, sourceCommit: sourceCommit, err: err}
		}),
	}, nil
}

func limitPreparation(limit int, prepare func(request) preparationOutcome) func(request) preparationOutcome {
	slots := make(chan struct{}, limit)
	return func(input request) preparationOutcome {
		slots <- struct{}{}
		defer func() { <-slots }()
		return prepare(input)
	}
}

func (w *metadataWorker) do(ctx context.Context, input request) preparationOutcome {
	keyBytes, _ := json.Marshal(input)
	key := string(keyBytes)

	w.mu.Lock()
	call := w.calls[key]
	if call == nil {
		call = &preparationCall{done: make(chan struct{})}
		w.calls[key] = call
		go func() {
			outcome := w.prepare(input)
			w.mu.Lock()
			call.outcome = outcome
			delete(w.calls, key)
			close(call.done)
			w.mu.Unlock()
		}()
	}
	call.waiters++
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		call.waiters--
		w.mu.Unlock()
	}()
	select {
	case <-call.done:
		return call.outcome
	case <-ctx.Done():
		return preparationOutcome{err: ctx.Err()}
	}
}

func (w *metadataWorker) servePrepare(wr http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(wr, "POST required", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(wr, r.Body, 1<<20)
	var input request
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(wr, err.Error(), http.StatusBadRequest)
		return
	}
	outcome := w.do(r.Context(), input)
	if outcome.err != nil {
		if errors.Is(outcome.err, context.Canceled) || errors.Is(outcome.err, context.DeadlineExceeded) {
			return
		}
		http.Error(wr, outcome.err.Error(), http.StatusUnprocessableEntity)
		return
	}
	wr.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(wr).Encode(response{Result: outcome.result, Cached: outcome.cached, SourceCommit: outcome.sourceCommit})
}

func generateWithLimit(slots chan struct{}, generate func() (metadatagenerator.Result, error)) (metadatagenerator.Result, error) {
	slots <- struct{}{}
	defer func() { <-slots }()
	return generate()
}

func main() {
	limit, err := generationLimitFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	preparationLimit, err := preparationLimitFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	worker, err := newMetadataWorkerWithPreparationLimit(limit, preparationLimit)
	if err != nil {
		log.Fatal(err)
	}
	if err := dropToPortage(); err != nil {
		panic(err)
	}
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET required", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	http.HandleFunc("/prepare", worker.servePrepare)
	address := os.Getenv("METADATA_WORKER_LISTEN_ADDR")
	if address == "" {
		address = ":8090"
	}
	if err := http.ListenAndServe(address, nil); err != nil {
		panic(err)
	}
}

func preparationLimitFromEnv() (int, error) {
	value := os.Getenv("METADATA_WORKER_PREPARATION_LIMIT")
	if value == "" {
		return defaultPreparationLimit, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("METADATA_WORKER_PREPARATION_LIMIT must be a positive integer, got %q", value)
	}
	return limit, nil
}

func generationLimitFromEnv() (int, error) {
	value := os.Getenv("METADATA_WORKER_GENERATION_LIMIT")
	if value == "" {
		return defaultGenerationLimit, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("METADATA_WORKER_GENERATION_LIMIT must be a positive integer, got %q", value)
	}
	return limit, nil
}

func dropToPortage() error {
	name := os.Getenv("METADATA_WORKER_PORTAGE_USER")
	if name == "" || os.Geteuid() != 0 {
		return nil
	}
	account, err := user.Lookup(name)
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return err
	}
	if err := os.Chown(os.Getenv("METADATA_WORKER_CACHE_PATH"), uid, gid); err != nil {
		return err
	}
	if err := syscall.Setgid(gid); err != nil {
		return err
	}
	return syscall.Setuid(uid)
}

func prepareMetadata(input request, generationSlots chan struct{}) (metadatagenerator.Result, bool, string, error) {
	if input.Repository.Name == "" || input.Repository.Path == "" || input.Repository.Commit == "" || input.Profile == "" {
		return metadatagenerator.Result{}, false, "", errors.New("repository, pinned commit and profile are required")
	}
	resolver, err := ebuildresolver.Open(input.Repository.Path)
	if err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	resolution, err := resolver.Resolve(ebuildresolver.Input{Repository: input.Repository.Name, Reference: input.Repository.Commit, Category: input.Category, Name: input.Name, Version: input.Version})
	if err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	if resolution.Status != ebuildresolver.StatusFound {
		return metadatagenerator.Result{}, false, "", fmt.Errorf("exact ebuild resolution: %s", resolution.Status)
	}
	selectedProvider, err := metadataprovider.Open(metadataprovider.Config{
		Repository: input.Repository.Name, SourceURL: input.Repository.URL,
		LocalPath: input.Repository.Path, Commit: resolution.SelectedCommit,
	})
	if err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	selectedCache, err := selectedProvider.Lookup(metadataprovider.Query{
		Repository: input.Repository.Name, Category: input.Category, Name: input.Name, Version: input.Version,
	})
	if err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	if selectedCache.Status == metadataprovider.StatusFound {
		masters := make([]metadatagenerator.MasterRevision, 0, len(input.Masters))
		for _, master := range input.Masters {
			masters = append(masters, metadatagenerator.MasterRevision{Name: master.Name, Commit: master.Commit})
		}
		return metadatagenerator.Result{
			Content: []byte(selectedCache.Snapshot.Content), SHA256: selectedCache.Snapshot.SHA256,
			Provenance: metadatagenerator.Provenance{
				ReferenceCommit: resolution.ReferenceCommit, SelectedCommit: resolution.SelectedCommit,
				SelectionReason: string(resolution.Reason), EbuildPath: resolution.EbuildPath,
				EbuildBlobID: resolution.BlobID, CachePath: selectedCache.Snapshot.Source.CachePath,
				CacheSHA256: selectedCache.Snapshot.SHA256, MasterRevisions: masters,
			},
		}, false, resolution.SelectedCommit, nil
	}
	portageVersion, err := readPortageVersion()
	if err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	profileKey := input.Profile
	masters := make([]metadatacache.Master, 0, len(input.Masters))
	for _, master := range input.Masters {
		masters = append(masters, metadatacache.Master{Name: master.Name, Commit: master.Commit})
	}
	cacheInputs := metadatacache.Inputs{
		Repository: input.Repository.Name, SourceURL: input.Repository.URL,
		Category: input.Category, Name: input.Name, Version: input.Version,
		ReferenceCommit: resolution.ReferenceCommit, SourceCommit: resolution.SelectedCommit,
		EbuildBlob: resolution.BlobID, Masters: masters, Profile: profileKey,
		GeneratorVersion: "metadata-worker-v1", PortageVersion: portageVersion,
	}
	store, err := metadatacache.NewStore(os.Getenv("METADATA_WORKER_CACHE_PATH"))
	if err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	// A tree entry proves that Git can traverse the complete profile path
	// without following a worktree symlink. Other cases retain the checkout path.
	if key, ok := pinnedTreeProfile(input, resolution.SelectedCommit); ok {
		cacheInputs.Profile = key
		if result, found, loadErr := store.Load(cacheInputs); loadErr != nil {
			return metadatagenerator.Result{}, false, "", loadErr
		} else if found {
			log.Printf("metadata cache hit repository=%s package=%s/%s version=%s", input.Repository.Name, input.Category, input.Name, input.Version)
			return result, true, resolution.SelectedCommit, nil
		}
	}
	tempRoot, err := os.MkdirTemp(os.Getenv("METADATA_WORKER_TEMP_PATH"), "metadata-")
	if err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	defer os.RemoveAll(tempRoot)
	mainCheckout := filepath.Join(tempRoot, input.Repository.Name)
	if err := checkout(input.Repository.Path, resolution.SelectedCommit, mainCheckout); err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	masterCheckouts := make([]metadatagenerator.Master, 0, len(input.Masters))
	masterPaths := make(map[string]string, len(input.Masters))
	for i, master := range input.Masters {
		path := filepath.Join(tempRoot, fmt.Sprintf("master-%d", i))
		if err := checkout(master.Path, master.Commit, path); err != nil {
			return metadatagenerator.Result{}, false, "", fmt.Errorf("checkout master %s: %w", master.Name, err)
		}
		masterCheckouts = append(masterCheckouts, metadatagenerator.Master{Name: master.Name, Checkout: path, Commit: master.Commit})
		masterPaths[master.Name] = path
	}
	profile := input.Profile
	if masterName, relative, found := strings.Cut(profile, ":"); found {
		masterPath := masterPaths[masterName]
		if masterName == input.Repository.Name {
			masterPath = mainCheckout
		}
		if masterPath == "" {
			return metadatagenerator.Result{}, false, "", fmt.Errorf("profile master %s is not pinned", masterName)
		}
		profile = filepath.Join(masterPath, filepath.FromSlash(relative))
	} else if !filepath.IsAbs(profile) {
		profile = filepath.Join(mainCheckout, filepath.FromSlash(profile))
	}
	profileKey, err = canonicalProfile(profile, mainCheckout, masterPaths, input.Masters)
	if err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	cacheInputs.Profile = profileKey
	if result, found, loadErr := store.Load(cacheInputs); loadErr != nil {
		return metadatagenerator.Result{}, false, "", loadErr
	} else if found {
		log.Printf("metadata cache hit repository=%s package=%s/%s version=%s", input.Repository.Name, input.Category, input.Name, input.Version)
		return result, true, resolution.SelectedCommit, nil
	}
	tempDir := filepath.Join(tempRoot, "generation")
	if err := os.Mkdir(tempDir, 0o700); err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	generated, err := generateWithLimit(generationSlots, func() (metadatagenerator.Result, error) {
		return metadatagenerator.New(metadatagenerator.Config{
			PortageUserName:  os.Getenv("METADATA_WORKER_PORTAGE_USER"),
			PortageGroupName: os.Getenv("METADATA_WORKER_PORTAGE_GROUP"),
		}).Generate(metadatagenerator.Request{
			Resolution: resolution, Repository: input.Repository.Name, Category: input.Category,
			Name: input.Name, Version: input.Version, Checkout: mainCheckout, Profile: profile,
			TempDir: tempDir, Masters: masterCheckouts,
		})
	})
	if err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	if err := store.Save(cacheInputs, generated); err != nil {
		return metadatagenerator.Result{}, false, "", err
	}
	log.Printf("metadata generated repository=%s package=%s/%s version=%s source_commit=%s", input.Repository.Name, input.Category, input.Name, input.Version, resolution.SelectedCommit)
	return generated, false, resolution.SelectedCommit, nil
}

// pinnedTreeProfile recognizes only paths whose canonical checkout path can be
// proven directly from Git. Symlinks, absolute paths and errors fall back.
func pinnedTreeProfile(input request, selectedCommit string) (string, bool) {
	// The checkout path verifies each master commit even when the profile is
	// elsewhere. Do not bypass that check for a cache hit.
	for _, master := range input.Masters {
		cmd := exec.Command("git", "--no-replace-objects", "--no-lazy-fetch", "-C", master.Path, "cat-file", "-t", master.Commit)
		typ, err := cmd.Output()
		if err != nil || strings.TrimSpace(string(typ)) != "commit" {
			return "", false
		}
	}
	profile := input.Profile
	repo := input.Repository
	commit := selectedCommit
	prefix := ""
	if name, relative, found := strings.Cut(profile, ":"); found {
		profile = relative
		if name != repo.Name {
			foundMaster := false
			for _, master := range input.Masters {
				if master.Name == name {
					repo, commit, prefix, foundMaster = master, master.Commit, name+":", true
					break
				}
			}
			if !foundMaster {
				return "", false
			}
		}
	}
	if profile == "" || filepath.IsAbs(profile) || filepath.Clean(profile) != profile || profile == "." || profile == ".." || strings.HasPrefix(profile, "../") || strings.ContainsAny(profile, "\\\n\r") {
		return "", false
	}
	cmd := exec.Command("git", "--no-replace-objects", "--no-lazy-fetch", "-C", repo.Path, "cat-file", "-t", commit+":"+profile)
	typ, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(typ)) != "tree" {
		return "", false
	}
	return prefix + profile, true
}

func canonicalProfile(profile, mainCheckout string, masterPaths map[string]string, masters []repository) (string, error) {
	resolved, err := filepath.EvalSymlinks(profile)
	if err != nil {
		return "", fmt.Errorf("resolve profile %s: %w", profile, err)
	}
	mainRoot, err := filepath.EvalSymlinks(mainCheckout)
	if err != nil {
		return "", err
	}
	if relative, err := filepath.Rel(mainRoot, resolved); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return filepath.ToSlash(relative), nil
	}
	for _, master := range masters {
		root := masterPaths[master.Name]
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(resolvedRoot, resolved)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return master.Name + ":" + filepath.ToSlash(relative), nil
		}
	}
	return "", errors.New("selected profile is outside the pinned repositories")
}

func checkout(mirror, commit, destination string) error {
	cmd := exec.Command("git", "--no-replace-objects", "--no-lazy-fetch", "clone", "--shared", "--no-checkout", "--", mirror, destination)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clone pinned checkout: %w: %s", err, strings.TrimSpace(string(output)))
	}
	cmd = exec.Command("git", "--no-replace-objects", "--no-lazy-fetch", "-C", destination, "checkout", "--detach", "--force", commit)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("checkout %s: %w: %s", commit, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func readPortageVersion() (string, error) {
	emerge := os.Getenv("EMERGE_PATH")
	if emerge == "" {
		emerge = "emerge"
	}
	command := exec.Command(emerge, "--version")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read Portage version: %w", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "Portage ") {
			fields := strings.Fields(strings.TrimPrefix(line, "Portage "))
			if len(fields) > 0 {
				return fields[0], nil
			}
		}
	}
	return "", errors.New("cannot parse Portage version")
}
