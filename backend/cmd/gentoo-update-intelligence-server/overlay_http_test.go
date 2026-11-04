package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysishttp"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
)

type overlayTestStore struct{}

func (overlayTestStore) Save(_ context.Context, _ []byte) (string, error) {
	return "28b28f31-7e85-4e6a-a839-84658f32cc74", nil
}
func (overlayTestStore) Get(_ context.Context, _ string) ([]byte, bool, error) {
	return nil, false, nil
}

type overlayResponse struct {
	Result struct {
		Repositories map[string]repositorymanager.Revision
		Reports      []struct {
			BeforeLookup struct {
				Result *metadataprovider.LookupResult
				Err    *string
			}
			AfterLookup struct {
				Result *metadataprovider.LookupResult
				Err    *string
			}
		}
	} `json:"result"`
}

func postOverlayPlan(t *testing.T, handler http.Handler, plan string) overlayResponse {
	t.Helper()
	body, err := json.Marshal(map[string]string{"plan": plan})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/analyses", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST = %d: %s", response.Code, response.Body.String())
	}
	var result overlayResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestOverlayConfigurationAndRefreshThroughHTTP(t *testing.T) {
	root := t.TempDir()
	gentooPath := filepath.Join(root, "gentoo")
	gentooCommit := fixtureRepo(t, gentooPath, "EAPI=8\nSLOT=0\nIUSE=gentoo\n")
	overlayPath := filepath.Join(root, "zh")
	firstCommit := fixtureRepo(t, overlayPath, "EAPI=8\nSLOT=0\nIUSE=old\n")
	umbrielPath := filepath.Join(root, "umbriel")
	fixtureRepo(t, umbrielPath, "EAPI=8\nSLOT=0\nIUSE=umbriel\n")
	catalog, err := json.Marshal([]repositorymanager.Repository{
		{Name: "gentoo-zh", URL: overlayPath, Masters: []string{"gentoo"}},
		{Name: "umbriel-overlay", URL: umbrielPath, Masters: []string{"gentoo"}},
		{Name: "duplicate", URL: overlayPath, Masters: []string{"gentoo"}},
		{Name: "duplicate", URL: umbrielPath, Masters: []string{"gentoo"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := overlayManager(filepath.Join(root, "mirrors"), string(catalog))
	if err != nil {
		t.Fatal(err)
	}
	config := metadataprovider.Config{Repository: "gentoo", SourceURL: "https://example.invalid/gentoo", LocalPath: gentooPath, Commit: gentooCommit}
	factory := newProviderFactory(config, manager)
	handler := analysishttp.NewHandlerWithPlanProviderFactory(overlayTestStore{}, factory)
	plan := "[ebuild U] dev-libs/widget-1::gentoo-zh [1::gentoo-zh]"
	first := postOverlayPlan(t, handler, plan)
	if first.Result.Repositories["gentoo-zh"].Commit != firstCommit {
		t.Fatalf("first revision = %#v", first.Result.Repositories)
	}
	pinned, err := factory(context.Background(), parser.Parse(plan))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(overlayPath, "metadata/md5-cache/dev-libs/widget-2"), []byte("EAPI=8\nSLOT=0\nIUSE=new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, overlayPath, "add", ".")
	fixtureGit(t, overlayPath, "commit", "-qm", "new version")
	secondCommit := fixtureGit(t, overlayPath, "rev-parse", "HEAD")
	second := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-2::gentoo-zh [1::gentoo-zh]")
	if second.Result.Repositories["gentoo-zh"].Commit != secondCommit || second.Result.Reports[0].AfterLookup.Result == nil || second.Result.Reports[0].AfterLookup.Result.Status != metadataprovider.StatusFound {
		t.Fatalf("updated result = %#v", second.Result)
	}
	old, err := pinned.Lookup(metadataprovider.Query{Repository: "gentoo-zh", Category: "dev-libs", Name: "widget", Version: "1"})
	if err != nil || old.Snapshot == nil || old.Snapshot.Source.Commit != firstCommit {
		t.Fatalf("old lookup = %#v, %v", old, err)
	}

	if err := os.Rename(overlayPath, overlayPath+"-offline"); err != nil {
		t.Fatal(err)
	}
	fallback := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-2::gentoo-zh [1::gentoo-zh]")
	if fallback.Result.Repositories["gentoo-zh"].Commit != secondCommit || fallback.Result.Reports[0].AfterLookup.Result == nil || fallback.Result.Reports[0].AfterLookup.Result.Status != metadataprovider.StatusFound {
		t.Fatalf("fallback = %#v", fallback.Result)
	}

	umbriel := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-1::umbriel-overlay [1::umbriel-overlay]")
	if umbriel.Result.Repositories["umbriel-overlay"].Masters[0] != "gentoo" {
		t.Fatalf("umbriel = %#v", umbriel.Result.Repositories)
	}
	for _, tc := range []struct{ name, reason string }{{"unknown", "unknown repository"}, {"duplicate", "ambiguous repository"}} {
		result := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-1::"+tc.name+" [1::umbriel-overlay]")
		lookup := result.Result.Reports[0].AfterLookup
		if lookup.Err == nil || !strings.Contains(*lookup.Err, tc.reason) {
			t.Fatalf("%s reason = %#v", tc.name, lookup)
		}
	}
	missingInstalled := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-1::unknown")
	if got := missingInstalled.Result.Reports[0].AfterLookup.Err; got == nil || !strings.Contains(*got, "unknown repository") {
		t.Fatalf("unknown target with missing installed version = %v", got)
	}
	unknownBefore := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-1::umbriel-overlay [1::unknown]")
	if got := unknownBefore.Result.Reports[0].BeforeLookup.Err; got == nil || !strings.Contains(*got, "unknown repository") {
		t.Fatalf("unknown installed repository = %v", got)
	}
}

func TestOverlayManagerRejectsMalformedConfiguration(t *testing.T) {
	for _, value := range []string{"null", "{}", "[] []", `[{"Name":"test","URL":"https://user:secret@example.invalid/repo"}]`, `[{"Name":"test","URL":"https://example.invalid/repo","extra":true}]`} {
		if _, err := overlayManager(t.TempDir(), value); err == nil {
			t.Fatalf("accepted invalid catalog %q", value)
		}
	}
}
