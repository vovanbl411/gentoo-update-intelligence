package analysishttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
)

type rebuildHTTPProvider struct {
	mu    sync.Mutex
	calls map[metadataprovider.Query]int
}

func (p *rebuildHTTPProvider) Lookup(q metadataprovider.Query) (metadataprovider.LookupResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls[q]++
	return metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}, nil
}

func TestRebuildPlanHTTP(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/parser/ebuild-updates-3.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, withSummary := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged fixture", true: "synthetic summary"}[withSummary], func(t *testing.T) {
			input := string(data)
			if withSummary {
				input += "\nTotal: 43 packages (36 upgrades, 7 reinstalls), Size of downloads: 1,079,819 KiB"
			}
			store := &memoryStore{id: testID}
			provider := &rebuildHTTPProvider{calls: map[metadataprovider.Query]int{}}
			handler := NewHandler(store, provider)
			body, _ := json.Marshal(map[string]string{"plan": input})
			post := newPost(handler, string(body))
			if post.Code != http.StatusAccepted {
				t.Fatal(post.Body.String())
			}
			job := waitJob(t, handler, testID)
			if job.Status != "completed" || job.Progress.Completed != 79 || job.Progress.Total != 79 {
				t.Fatalf("%+v", job)
			}
			var result resultDTO
			if err := json.Unmarshal(job.Result, &result); err != nil {
				t.Fatal(err)
			}
			if result.Plan.Status != "parsed" || len(result.Plan.Diagnostics) != 0 || len(result.Plan.Entries) != 43 || len(result.Reports) != 43 {
				t.Fatalf("status=%s entries=%d reports=%d", result.Plan.Status, len(result.Plan.Entries), len(result.Reports))
			}
			if withSummary {
				if result.Plan.Summary == nil || result.Plan.Summary.Reinstalls != 7 || result.Plan.Summary.Upgrades != 36 {
					t.Fatal(result.Plan.Summary)
				}
			} else if result.Plan.Summary != nil {
				t.Fatal("invented summary")
			}
			rebuilds := 0
			for i, e := range result.Plan.Entries {
				r := result.Reports[i]
				if r.Report.EntryIndex != i || r.AfterLookup.Query == nil || r.AfterLookup.Query.Version != e.Target.Version || r.AfterLookup.Query.Repository != *e.Target.Repository {
					t.Fatalf("row %d: %+v", i, r)
				}
				if e.Operation == "reinstall" {
					rebuilds++
					if len(e.Installed.Versions) != 0 || r.BeforeLookup.Query != nil || r.BeforeLookup.Result != nil || r.BeforeLookup.SkipReason != "BINDING_REJECTED" || r.Report.Gentoo.Comparison != nil {
						t.Fatalf("invented before: %+v", r)
					}
				}
			}
			if rebuilds != 7 {
				t.Fatalf("rebuilds=%d", rebuilds)
			}
			provider.mu.Lock()
			defer provider.mu.Unlock()
			if len(provider.calls) != 79 {
				t.Fatalf("calls=%d", len(provider.calls))
			}
			for q, count := range provider.calls {
				if count != 1 {
					t.Errorf("%v called %d times", q, count)
				}
			}
			for _, suffix := range []string{"/report", "/partial-report"} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/analyses/"+testID+suffix, nil))
				if response.Code != http.StatusOK {
					t.Fatal(response.Body.String())
				}
				var compact struct {
					Result   compactResultDTO
					Progress progressView
				}
				if err := json.Unmarshal(response.Body.Bytes(), &compact); err != nil {
					t.Fatal(err)
				}
				if len(compact.Result.Reports) != 43 || len(compact.Result.Plan.Entries) != 43 || compact.Progress.Completed != 79 {
					t.Fatal("compact report lost records or progress")
				}
			}
		})
	}
}

func TestLegacySummaryDefaultsReinstallsWithoutReparsing(t *testing.T) {
	// The saved partial parser result deliberately differs from what parsing its
	// source today would produce; GET must preserve it without consulting Provider.
	original := []byte(`{"Plan":{"Status":"partial","Entries":[],"Summary":{"Packages":43,"Upgrades":36},"Diagnostics":[{"Code":"UNSUPPORTED_OPERATION","SourceLines":[11]}]},"Analysis":{"Status":"partial","Findings":[]},"Reports":[]}`)
	store := &memoryStore{id: testID, result: original}
	provider := &rebuildHTTPProvider{calls: map[metadataprovider.Query]int{}}
	handler := NewHandler(store, provider)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/analyses/"+testID, nil))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var job struct{ Result resultDTO }
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	p := job.Result.Plan
	if p.Status != "partial" || len(p.Entries) != 0 || len(p.Diagnostics) != 1 || p.Summary.Reinstalls != 0 || p.Summary.Packages != 43 {
		t.Fatalf("%+v", p)
	}
	if len(provider.calls) != 0 || string(store.result) != string(original) || store.createCalls != 0 {
		t.Fatal("GET reanalyzed or mutated saved result")
	}
}
