package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatagenerator"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
)

type pinnedRepositories struct {
	providers    map[string]*metadataprovider.Provider
	failures     map[string]error
	revisions    map[string]repositorymanager.Revision
	workerURL    string
	profile      string
	workerClient *http.Client
}

type preparedMetadata struct {
	Result       metadatagenerator.Result `json:"result"`
	SourceCommit string                   `json:"source_commit"`
}

func (p *pinnedRepositories) RepositoryFailure(name string) error {
	if err := p.failures[name]; err != nil {
		return err
	}
	if p.providers[name] == nil {
		return fmt.Errorf("%w: %s", repositorymanager.ErrUnknownRepository, name)
	}
	return nil
}

func (p *pinnedRepositories) Lookup(query metadataprovider.Query) (metadataprovider.LookupResult, error) {
	if err := p.RepositoryFailure(query.Repository); err != nil {
		return metadataprovider.LookupResult{}, err
	}
	provider := p.providers[query.Repository]
	result, err := provider.Lookup(query)
	if err != nil || result.Status != metadataprovider.StatusNotFound || p.workerURL == "" {
		return result, err
	}
	revision := p.revisions[query.Repository]
	masters, err := pinnedMasters(revision, p.revisions)
	if err != nil {
		return metadataprovider.LookupResult{}, err
	}
	profile := p.profile
	if profile == "" {
		profile = "profiles/default/linux/amd64/23.0"
	}
	if query.Repository != "gentoo" && !strings.Contains(profile, ":") {
		profile = "gentoo:" + profile
	}
	workerResult, err := callMetadataWorker(p.workerURL, query, revision, masters, profile, p.workerClient)
	if err != nil {
		return metadataprovider.LookupResult{}, err
	}
	sourceCommit := workerResult.SourceCommit
	if sourceCommit == "" {
		sourceCommit = revision.Commit
	}
	source := metadatadiff.Source{Category: query.Category, Name: query.Name, Version: query.Version,
		Repository: query.Repository, SourceURL: revision.URL, Commit: sourceCommit,
		CachePath: workerResult.Result.Provenance.CachePath}
	snapshot := metadatadiff.Normalize(source, string(workerResult.Result.Content))
	provenance := workerResult.Result.Provenance
	return metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: &snapshot, Provenance: &provenance}, nil
}

func pinnedMasters(source repositorymanager.Revision, revisions map[string]repositorymanager.Revision) ([]repositorymanager.Revision, error) {
	seen := make(map[string]bool)
	ordered := make([]repositorymanager.Revision, 0)
	var visit func(string) error
	visit = func(name string) error {
		if seen[name] {
			return nil
		}
		master, ok := revisions[name]
		if !ok {
			return fmt.Errorf("pinned master %s is unavailable", name)
		}
		seen[name] = true
		for _, parent := range master.Masters {
			if err := visit(parent); err != nil {
				return err
			}
		}
		ordered = append(ordered, master)
		return nil
	}
	for _, name := range source.Masters {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func callMetadataWorker(endpoint string, query metadataprovider.Query, source repositorymanager.Revision, masters []repositorymanager.Revision, profile string, client *http.Client) (preparedMetadata, error) {
	type workerRepository struct {
		Name   string `json:"Name"`
		URL    string `json:"URL"`
		Path   string `json:"Path"`
		Commit string `json:"Commit"`
	}
	type workerRequest struct {
		Repository workerRepository   `json:"Repository"`
		Masters    []workerRepository `json:"Masters"`
		Category   string             `json:"Category"`
		Name       string             `json:"Name"`
		Version    string             `json:"Version"`
		Profile    string             `json:"Profile"`
	}
	input := workerRequest{Repository: workerRepository{source.Name, source.URL, source.Path, source.Commit}, Category: query.Category, Name: query.Name, Version: query.Version, Profile: profile}
	for _, master := range masters {
		input.Masters = append(input.Masters, workerRepository{master.Name, master.URL, master.Path, master.Commit})
	}
	data, err := json.Marshal(input)
	if err != nil {
		return preparedMetadata{}, err
	}
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(endpoint, "/")+"/prepare", bytes.NewReader(data))
	if err != nil {
		return preparedMetadata{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Minute}
	}
	response, err := client.Do(request)
	if err != nil {
		return preparedMetadata{}, fmt.Errorf("metadata worker: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var message bytes.Buffer
		_, _ = message.ReadFrom(http.MaxBytesReader(nil, response.Body, 64<<10))
		return preparedMetadata{}, fmt.Errorf("metadata worker returned %s: %s", response.Status, strings.TrimSpace(message.String()))
	}
	var output preparedMetadata
	if err := json.NewDecoder(response.Body).Decode(&output); err != nil {
		return preparedMetadata{}, fmt.Errorf("decode metadata worker result: %w", err)
	}
	return output, nil
}

func (p *pinnedRepositories) RepositoryRevisions() map[string]repositorymanager.Revision {
	return p.revisions
}

func prepareRepositories(ctx context.Context, plan parser.Result, config metadataprovider.Config, gentoo *metadataprovider.Provider, manager *repositorymanager.Manager) reportservice.MetadataLookup {
	main := repositorymanager.Revision{Name: "gentoo", URL: config.SourceURL, Path: config.LocalPath, Commit: gentoo.Commit()}
	result := &pinnedRepositories{
		providers: map[string]*metadataprovider.Provider{"gentoo": gentoo},
		failures:  make(map[string]error),
		revisions: map[string]repositorymanager.Revision{"gentoo": main},
		workerURL: os.Getenv("GENTOO_UPDATE_INTELLIGENCE_METADATA_WORKER_URL"),
		profile:   os.Getenv("GENTOO_UPDATE_INTELLIGENCE_METADATA_PROFILE"),
	}
	names := make(map[string]bool)
	for _, entry := range plan.Entries {
		if entry.Target.Repository != nil {
			names[*entry.Target.Repository] = true
		}
		for _, installed := range entry.Installed.Versions {
			if installed.Repository != nil {
				names[*installed.Repository] = true
			}
		}
	}
	for name := range names {
		if name == "gentoo" {
			continue
		}
		if manager == nil {
			result.failures[name] = fmt.Errorf("overlay preparation is disabled; repository %s cannot be resolved", name)
			continue
		}
		revisions, err := manager.PinAdditional(ctx, []string{name}, result.revisions)
		if errors.Is(err, repositorymanager.ErrUnknownRepository) {
			result.failures[name] = err
			continue
		}
		if err != nil {
			result.failures[name] = err
			continue
		}
		for repoName, revision := range revisions {
			if repoName == "gentoo" {
				continue
			}
			provider, err := metadataprovider.Open(metadataprovider.Config{Repository: repoName, SourceURL: revision.URL, LocalPath: revision.Path, Commit: revision.Commit})
			if err != nil {
				result.failures[repoName] = err
				continue
			}
			result.providers[repoName] = provider
			result.revisions[repoName] = revision
		}
	}
	return result
}
