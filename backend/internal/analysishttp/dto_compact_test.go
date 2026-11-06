package analysishttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysisstore"
)

func TestCompactReportSavedResult(t *testing.T) {
	fixture, err := os.ReadFile("testdata/legacy-found.json")
	if err != nil {
		t.Fatal(err)
	}
	var legacy struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(fixture, &legacy); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(&memoryStore{id: testID, result: legacy.Result}, nil)
	request := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
		return response
	}
	full := request("/api/v1/analyses/" + testID)
	compact := request("/api/v1/analyses/" + testID + "/report")
	var normal, small struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(full.Body.Bytes(), &normal); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(compact.Body.Bytes(), &small); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"plan", "analysis", "repositories"} {
		if !jsonEqual(normal.Result[key], small.Result[key]) {
			t.Fatalf("%s changed", key)
		}
	}
	var fullReports, compactReports []map[string]any
	if err := json.Unmarshal(normal.Result["reports"], &fullReports); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(small.Result["reports"], &compactReports); err != nil {
		t.Fatal(err)
	}
	for _, report := range fullReports {
		details := report["report"].(map[string]any)
		delete(details, "parser_status")
		delete(details, "parser_diagnostics")
		for _, side := range []string{"before_lookup", "after_lookup"} {
			lookup := report[side].(map[string]any)
			if result, ok := lookup["result"].(map[string]any); ok {
				delete(result, "snapshot")
				delete(result, "provenance")
			}
		}
	}
	if !reflect.DeepEqual(fullReports, compactReports) {
		t.Fatal("compact reports differ from full reports beyond omitted fields")
	}
	if !bytes.Equal(h.store.(*memoryStore).result, legacy.Result) {
		t.Fatal("GET changed saved result")
	}
	var raw struct {
		Result struct {
			Reports []struct {
				Report       map[string]json.RawMessage `json:"report"`
				BeforeLookup struct {
					Result map[string]json.RawMessage `json:"result"`
				} `json:"before_lookup"`
				AfterLookup struct {
					Result map[string]json.RawMessage `json:"result"`
				} `json:"after_lookup"`
			} `json:"reports"`
		} `json:"result"`
	}
	if err := json.Unmarshal(compact.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, report := range raw.Result.Reports {
		if _, ok := report.Report["parser_diagnostics"]; ok {
			t.Fatal("duplicate parser diagnostics")
		}
		for _, lookup := range []map[string]json.RawMessage{report.BeforeLookup.Result, report.AfterLookup.Result} {
			if _, ok := lookup["snapshot"]; ok {
				t.Fatal("snapshot present")
			}
			if _, ok := lookup["provenance"]; ok {
				t.Fatal("provenance present")
			}
		}
	}
	if compact.Body.Len() >= full.Body.Len() {
		t.Fatal("compact response did not shrink")
	}
	t.Logf("full: %d bytes; compact: %d bytes", full.Body.Len(), compact.Body.Len())
}

func TestCompactReportErrors(t *testing.T) {
	for _, tc := range []struct {
		name, path, method string
		store              *memoryStore
		status             int
		code               string
	}{
		{"invalid id", "/api/v1/analyses/invalid/report", "GET", &memoryStore{}, 404, "ANALYSIS_NOT_FOUND"},
		{"missing", "/api/v1/analyses/" + testID + "/report", "GET", &memoryStore{}, 404, "ANALYSIS_NOT_FOUND"},
		{"invalid stored result", "/api/v1/analyses/" + testID + "/report", "GET", &memoryStore{id: testID, result: []byte(`{}`)}, 500, "INVALID_STORED_RESULT"},
		{"method", "/api/v1/analyses/" + testID + "/report", "POST", &memoryStore{}, 405, "METHOD_NOT_ALLOWED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			NewHandler(tc.store, nil).ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			var body errorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != tc.status || body.Error == nil || body.Error.Code != tc.code {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
		})
	}
	h := NewHandler(nil, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/analyses/"+testID+"/report", nil))
	if w.Code != 503 {
		t.Fatalf("missing storage: %d %s", w.Code, w.Body.String())
	}
}

func TestCompactReportJobStates(t *testing.T) {
	for _, status := range []string{"pending", "running", "failed"} {
		t.Run(status, func(t *testing.T) {
			store := &memoryStore{id: testID, jobs: map[string]analysisstore.Job{testID: {ID: testID, Status: status, Stage: "queued", Error: "failure"}}}
			h := NewHandler(store, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/analyses/"+testID+"/report", nil))
			var job jobResponse
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &job) != nil || job.Status != status || len(job.Result) != 0 && string(job.Result) != "null" {
				t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
			}
			if (status == "failed") != (job.Error != nil) {
				t.Fatalf("unexpected error: %#v", job.Error)
			}
		})
	}
}
