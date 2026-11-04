package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysishttp"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysisstore"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
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
	providerFactory := func() (reportservice.MetadataLookup, error) {
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
		return provider, nil
	}

	address := os.Getenv("GENTOO_UPDATE_INTELLIGENCE_LISTEN_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	server := &http.Server{
		Addr:              address,
		Handler:           analysishttp.NewHandlerWithProviderFactory(store, providerFactory),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
