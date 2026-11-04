package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysishttp"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysisstore"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
)

func main() {
	if err := run(); err != nil {
		log.Printf("backend stopped: %v", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	store, err := analysisstore.Open(ctx, os.Getenv("GENTOO_UPDATE_INTELLIGENCE_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}

	config := metadataprovider.Config{
		Repository: os.Getenv("GENTOO_UPDATE_INTELLIGENCE_REPOSITORY"),
		SourceURL:  os.Getenv("GENTOO_UPDATE_INTELLIGENCE_SOURCE_URL"),
		LocalPath:  os.Getenv("GENTOO_UPDATE_INTELLIGENCE_REPOSITORY_PATH"),
		Commit:     os.Getenv("GENTOO_UPDATE_INTELLIGENCE_COMMIT"),
	}
	var overlays *repositorymanager.Manager
	if path := os.Getenv("GENTOO_UPDATE_INTELLIGENCE_OVERLAY_PATH"); path != "" {
		if config.Repository != "gentoo" {
			return errors.New("overlay preparation requires the gentoo repository")
		}
		overlays, err = overlayManager(path, os.Getenv("GENTOO_UPDATE_INTELLIGENCE_OVERLAY_CATALOG"))
		if err != nil {
			return err
		}
	}
	providerFactory := newProviderFactory(config, overlays)

	address := os.Getenv("GENTOO_UPDATE_INTELLIGENCE_LISTEN_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	server := &http.Server{
		Addr:              address,
		Handler:           analysishttp.NewHandlerWithPlanProviderFactory(store, providerFactory),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newProviderFactory(config metadataprovider.Config, overlays *repositorymanager.Manager) func(context.Context, parser.Result) (reportservice.MetadataLookup, error) {
	return func(ctx context.Context, plan parser.Result) (reportservice.MetadataLookup, error) {
		var provider *metadataprovider.Provider
		var err error
		if config.Commit == "" {
			provider, err = metadataprovider.OpenPublished(config)
		} else {
			provider, err = metadataprovider.Open(config)
		}
		if err != nil {
			log.Printf("Metadata Provider unavailable: %v", err)
			return nil, err
		}
		if overlays == nil {
			return provider, nil
		}
		return prepareRepositories(ctx, plan, config, provider, overlays), nil
	}
}

// overlayManager accepts only operator-provided catalog configuration.
func overlayManager(path, catalogJSON string) (*repositorymanager.Manager, error) {
	catalog := repositorymanager.KnownOverlays()
	if catalogJSON != "" {
		decoder := json.NewDecoder(strings.NewReader(catalogJSON))
		decoder.DisallowUnknownFields()
		var configured []repositorymanager.Repository
		if err := decoder.Decode(&configured); err != nil {
			return nil, fmt.Errorf("invalid overlay catalog: %w", err)
		}
		if configured == nil {
			return nil, errors.New("overlay catalog must be a JSON array")
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, errors.New("overlay catalog must contain one JSON array")
		}
		catalog = append(catalog, configured...)
	}
	return repositorymanager.New(path, catalog)
}
