package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/backendconfig"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatagenerator"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
)

func TestConfiguredWorkerTimeoutCancelsHTTPWait(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	config := backendconfig.Defaults().Metadata
	config.WorkerURL = server.URL
	config.WorkerTimeout = backendconfig.Duration(100 * time.Millisecond)
	worker := workerSettingsFromConfig(config)
	_, err := callMetadataWorker(worker.URL, metadataprovider.Query{}, repositorymanager.Revision{}, nil, worker.Profile, worker.Client)
	var timeout net.Error
	if err == nil || !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("expected configured HTTP timeout, got %v", err)
	}
	select {
	case <-started:
	default:
		t.Fatal("request did not reach the controlled server")
	}
}

func TestFactoryUsesLoadedWorkerSettingsInsteadOfEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gentoo")
	commit := fixtureEmptyRepo(t, path)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Profile string }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Profile != "profiles/configured" {
			t.Errorf("profile = %q", request.Profile)
		}
		_ = json.NewEncoder(w).Encode(preparedMetadata{
			Result:       metadatagenerator.Result{Content: []byte("EAPI=8\nSLOT=0\nIUSE=generated\n"), Provenance: metadatagenerator.Provenance{CachePath: "metadata/md5-cache/dev-libs/widget-1"}},
			SourceCommit: commit,
		})
	}))
	defer server.Close()
	settings := backendconfig.Defaults().Metadata
	settings.WorkerURL, settings.Profile = server.URL, "profiles/configured"
	factory := newResumableProviderFactory(metadataprovider.Config{Repository: "gentoo", SourceURL: "https://example.invalid/gentoo", LocalPath: path, Commit: commit}, nil, workerSettingsFromConfig(settings))
	t.Setenv("GENTOO_UPDATE_INTELLIGENCE_METADATA_WORKER_URL", "http://127.0.0.1:1")
	t.Setenv("GENTOO_UPDATE_INTELLIGENCE_METADATA_PROFILE", "profiles/wrong-env")
	provider, err := factory(context.Background(), parser.Parse("[ebuild U] dev-libs/widget-1::gentoo"), nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Lookup(metadataprovider.Query{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "1"})
	if err != nil || result.Status != metadataprovider.StatusFound {
		t.Fatalf("configured worker lookup = %#v, %v", result, err)
	}
}
