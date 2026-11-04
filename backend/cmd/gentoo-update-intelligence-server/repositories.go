package main

import (
	"context"
	"errors"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
)

type pinnedRepositories struct {
	providers map[string]*metadataprovider.Provider
	failures  map[string]error
	revisions map[string]repositorymanager.Revision
}

func (p *pinnedRepositories) Lookup(query metadataprovider.Query) (metadataprovider.LookupResult, error) {
	if err := p.failures[query.Repository]; err != nil {
		return metadataprovider.LookupResult{}, err
	}
	provider := p.providers[query.Repository]
	if provider == nil {
		return metadataprovider.LookupResult{Status: metadataprovider.StatusRepositoryUnavailable}, nil
	}
	return provider.Lookup(query)
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
		revisions, err := manager.Pin(ctx, []string{name}, main)
		if errors.Is(err, repositorymanager.ErrUnknownRepository) {
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
