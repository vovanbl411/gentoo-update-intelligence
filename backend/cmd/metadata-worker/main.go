package main

import (
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

func main() {
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
	http.HandleFunc("/prepare", prepare)
	address := os.Getenv("METADATA_WORKER_LISTEN_ADDR")
	if address == "" {
		address = ":8090"
	}
	if err := http.ListenAndServe(address, nil); err != nil {
		panic(err)
	}
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

func prepare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var input request
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, cached, sourceCommit, err := prepareMetadata(input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response{Result: result, Cached: cached, SourceCommit: sourceCommit})
}

func prepareMetadata(input request) (metadatagenerator.Result, bool, string, error) {
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
	generated, err := metadatagenerator.New(metadatagenerator.Config{
		PortageUserName:  os.Getenv("METADATA_WORKER_PORTAGE_USER"),
		PortageGroupName: os.Getenv("METADATA_WORKER_PORTAGE_GROUP"),
	}).Generate(metadatagenerator.Request{
		Resolution: resolution, Repository: input.Repository.Name, Category: input.Category,
		Name: input.Name, Version: input.Version, Checkout: mainCheckout, Profile: profile,
		TempDir: tempDir, Masters: masterCheckouts,
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
