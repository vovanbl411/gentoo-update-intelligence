package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysishttp"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysisstore"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/backendconfig"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Printf("backend stopped: %v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	settings, err := loadServerConfig(args, os.Getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(settings.StartupTimeout))
	defer cancel()

	store, err := analysisstore.Open(ctx, settings.DatabaseURL)
	if err != nil {
		return errors.New("PostgreSQL connection failed; check DATABASE_URL and database availability")
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}

	config := metadataprovider.Config{
		Repository: settings.Repository.Name,
		SourceURL:  settings.Repository.SourceURL,
		LocalPath:  settings.Repository.Path,
		Commit:     settings.Repository.Commit,
	}
	var overlays *repositorymanager.Manager
	if settings.Overlays.Path != "" {
		if config.Repository != "gentoo" {
			return errors.New("overlay preparation requires the gentoo repository")
		}
		overlays, err = settings.OverlayManager()
		if err != nil {
			return err
		}
	}
	providerFactory := newResumableProviderFactory(config, overlays, workerSettingsFromConfig(settings.Metadata))
	handler, err := analysishttp.NewHandlerWithConcurrencyLimit(store, providerFactory, int(settings.Analysis.ConcurrencyLimit))
	if err != nil {
		return err
	}
	if err := handler.Resume(ctx); err != nil {
		return fmt.Errorf("resume analysis jobs: %w", err)
	}

	server := &http.Server{
		Addr:              settings.HTTP.ListenAddress,
		Handler:           handler,
		ReadHeaderTimeout: time.Duration(settings.HTTP.ReadHeaderTimeout),
		IdleTimeout:       time.Duration(settings.HTTP.IdleTimeout),
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func loadServerConfig(args []string, getenv func(string) string) (backendconfig.Config, error) {
	flags := flag.NewFlagSet("backend", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	filename := flags.String("config", "", "YAML configuration file")
	if err := flags.Parse(args); err != nil {
		return backendconfig.Config{}, errors.New("invalid command line; use --config <path>")
	}
	if flags.NArg() != 0 {
		return backendconfig.Config{}, errors.New("unexpected positional arguments")
	}
	provided := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			provided = true
		}
	})
	if provided && *filename == "" {
		return backendconfig.Config{}, errors.New("--config requires a nonempty file path")
	}
	return backendconfig.Load(*filename, getenv)
}

func workerSettingsFromConfig(config backendconfig.Metadata) workerSettings {
	return workerSettings{URL: config.WorkerURL, Profile: config.Profile, Client: &http.Client{Timeout: time.Duration(config.WorkerTimeout)}}
}

func newProviderFactory(config metadataprovider.Config, overlays *repositorymanager.Manager, worker workerSettings) func(context.Context, parser.Result) (reportservice.MetadataLookup, error) {
	resumable := newResumableProviderFactory(config, overlays, worker)
	return func(ctx context.Context, plan parser.Result) (reportservice.MetadataLookup, error) {
		return resumable(ctx, plan, nil)
	}
}

func newResumableProviderFactory(config metadataprovider.Config, overlays *repositorymanager.Manager, worker workerSettings) func(context.Context, parser.Result, map[string]repositorymanager.Revision) (reportservice.MetadataLookup, error) {
	return func(ctx context.Context, plan parser.Result, pinned map[string]repositorymanager.Revision) (reportservice.MetadataLookup, error) {
		analysisConfig := config
		if gentoo, ok := pinned["gentoo"]; ok {
			if gentoo.Name != "gentoo" || gentoo.Commit == "" {
				return nil, errors.New("saved gentoo revision is invalid")
			}
			analysisConfig.Commit = gentoo.Commit
		}
		var provider *metadataprovider.Provider
		var err error
		if analysisConfig.Commit == "" {
			provider, err = metadataprovider.OpenPublished(analysisConfig)
		} else {
			provider, err = metadataprovider.Open(analysisConfig)
		}
		if err != nil {
			log.Printf("Metadata Provider unavailable: %v", err)
			return nil, err
		}
		if overlays == nil {
			return preparePinnedRepositories(ctx, plan, analysisConfig, provider, nil, pinned, worker)
		}
		return preparePinnedRepositories(ctx, plan, analysisConfig, provider, overlays, pinned, worker)
	}
}
