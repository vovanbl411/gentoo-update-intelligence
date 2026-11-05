package analysishttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysisstore"
)

func mustV1Result(t *testing.T, stored []byte) []byte {
	t.Helper()
	result, err := decodeStoredResult(stored)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestV1StoredFixtureWithoutProvider(t *testing.T) {
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
	expected, err := os.ReadFile("testdata/v1-found.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(expected, &golden); err != nil {
		t.Fatal(err)
	}
	if actual := mustV1Result(t, legacy.Result); !jsonEqual(actual, golden.Result) {
		var got, want any
		json.Unmarshal(actual, &got)
		json.Unmarshal(golden.Result, &want)
		t.Fatal("public fixture differences: ", jsonDifferences("result", got, want))
	}
	for _, source := range []string{"analyses", "analysis_jobs"} {
		t.Run(source, func(t *testing.T) {
			store := &memoryStore{id: testID, result: legacy.Result}
			if source == "analysis_jobs" {
				store.jobs = map[string]analysisstore.Job{testID: {ID: testID, Status: "completed", Stage: "completed", Result: legacy.Result}}
			}
			handler := NewHandler(store, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/analyses/"+testID, nil))
			if response.Code != 200 {
				t.Fatal(response.Body.String())
			}
			var job struct {
				Result struct {
					Reports []reportDTO `json:"reports"`
				} `json:"result"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
				t.Fatal(err)
			}
			lookup := job.Result.Reports[0].AfterLookup
			if lookup.Result == nil || lookup.Result.Snapshot == nil {
				t.Fatal("snapshot lost")
			}
			snapshot := lookup.Result.Snapshot
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(legacy.Result, &raw); err != nil {
				t.Fatal(err)
			}
			var reports []storedReport
			if err := json.Unmarshal(raw["Reports"], &reports); err != nil {
				t.Fatal(err)
			}
			original := reports[0].AfterLookup.Result.Snapshot
			if snapshot.Content != original.Content || snapshot.SHA256 != original.SHA256 {
				t.Fatal("metadata content or digest changed")
			}
			if !bytes.Equal(store.result, legacy.Result) || store.createCalls != 0 {
				t.Fatal("GET mutated storage")
			}
			for _, forbidden := range []string{`"BeforeLookup":`, `"SourceLines":`, `"Plan":`, `"Path":`} {
				if bytes.Contains(response.Body.Bytes(), []byte(forbidden)) {
					t.Fatalf("legacy field leaked: %s", forbidden)
				}
			}
		})
	}
}

func TestV1EvidenceAndNullableValues(t *testing.T) {
	stored := []byte(`{"Plan":{"Status":"partial","Entries":[{"Package":{"Category":"a","Name":"b"},"Target":{"Repository":"","Slot":null}}]},"Analysis":{"Status":"partial","Findings":[
	{"RuleID":"LOCAL_PATCHED_PACKAGE","EntryIndex":0,"Evidence":{"ContextIndex":0}},
	{"RuleID":"USE_CHANGED","Evidence":{"Tokens":[{"GroupIndex":0,"ValueIndex":0}]}},
	{"RuleID":"SLOT_CHANGED","Evidence":{"InstalledVersionIndex":0}},
	{"RuleID":"DOWNGRADE_PLANNED","Evidence":{"EntryIndex":0,"Operation":"downgrade","InstalledVersions":["2"],"TargetVersion":"1"}},
	{"RuleID":"UNRESOLVED_BLOCKER","EntryIndex":null,"Evidence":{"BlockerIndex":0}}
	]},"Reports":null}`)
	data := mustV1Result(t, stored)
	for _, expected := range []string{`"context_index":0`, `"group_index":0`, `"value_index":0`, `"installed_version_index":0`, `"blocker_index":0`, `"entry_index":null`, `"entry_index":0`, `"repository":""`, `"slot":null`, `"reports":[]`, `"repositories":{}`, `"diagnostics":[]`, `"removals":[]`} {
		if !bytes.Contains(data, []byte(expected)) {
			t.Fatalf("missing %s in %s", expected, data)
		}
	}
	if !bytes.Contains(data, []byte(`"installed_versions":["2"]`)) {
		t.Fatal("downgrade evidence lost")
	}
	for _, invalid := range []string{`null`, `{}`, `[]`, `{"Plan":{"Status":"parsed"}}`, `{"Plan":{"Status":"parsed"},"Analysis":{"Status":"parsed","Findings":[{"RuleID":"USE_CHANGED","Evidence":42}]}}`} {
		if _, err := decodeStoredResult([]byte(invalid)); err == nil {
			t.Fatalf("invalid stored result accepted: %s", invalid)
		}
	}
}

func TestV1RoutesErrorsAndLocation(t *testing.T) {
	store := &memoryStore{id: testID}
	handler := NewHandler(store, nil)
	for _, tc := range []struct {
		method, path, body, contentType string
		status                          int
		code, allow                     string
	}{
		{"POST", "/analyses", `{"plan":"test"}`, "application/json", 404, "NOT_FOUND", ""},
		{"POST", "/api/analyses", `{"plan":"test"}`, "application/json", 404, "NOT_FOUND", ""},
		{"GET", "/api/v2/analyses", "", "", 404, "NOT_FOUND", ""},
		{"GET", "/api/v1/analyses", "", "", 405, "METHOD_NOT_ALLOWED", "POST"},
		{"POST", "/api/v1/analyses/" + testID, "", "", 405, "METHOD_NOT_ALLOWED", "GET, HEAD"},
		{"POST", "/api/v1/analyses", `{}`, "application/json", 400, "INVALID_REQUEST", ""},
		{"POST", "/api/v1/analyses", `{}`, "text/plain", 415, "UNSUPPORTED_MEDIA_TYPE", ""},
		{"GET", "/api/v1/analyses/not-a-uuid", "", "", 404, "ANALYSIS_NOT_FOUND", ""},
	} {
		request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		request.Header.Set("Content-Type", tc.contentType)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		var body errorResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != tc.status || body.Error == nil || body.Error.Code != tc.code || response.Header().Get("Allow") != tc.allow {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
	if store.createCalls != 0 {
		t.Fatal("invalid or old routes created a job")
	}
	response := newPost(handler, `{"plan":"test"}`)
	if response.Code != 202 || response.Header().Get("Location") != "/api/v1/analyses/"+testID {
		t.Fatal(response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"result":null`) || !strings.Contains(response.Body.String(), `"error":null`) {
		t.Fatal(response.Body.String())
	}
	job := waitJob(t, handler, testID)
	if job.Error == nil || job.Error.Code != "ANALYSIS_FAILED" {
		t.Fatalf("job: %#v", job)
	}
}

func TestV1PreservesGeneratedProvenanceAndPartialComparison(t *testing.T) {
	stored := []byte(`{"Plan":{"Status":"partial"},"Analysis":{"Status":"partial"},"Repositories":{"Name":{"Name":"Name","URL":"https://example.invalid/repo.git","Commit":"abc","Path":"/private-repository-path","Masters":["gentoo"]}},"Reports":[{
	"Report":{"EntryIndex":0,"Gentoo":{"Status":"partial","Comparison":{"Status":"partial","ComparedFields":["EAPI"],"Limitations":[{"Code":"IUSE_MISSING","Field":"IUSE","Side":"after","Evidence":[{"Side":"after","Field":"IUSE","WholeSnapshot":true}]}]}}},
	"BeforeLookup":{"Err":"cache read failed"},
	"AfterLookup":{"Result":{"Status":"found","Snapshot":{"Content":"EAPI=8\nSLOT=2/16\n","SHA256":"fixture-sha"},"provenance":{"reference_commit":"reference","selected_commit":"selected","selection_reason":"at_reference","ebuild_path":"dev-libs/a/a-1.ebuild","ebuild_blob_id":"blob","profile":"profiles/default/linux/amd64/23.0","portage_version":"3.0.82.2","cache_path":"metadata/md5-cache/dev-libs/a-1","cache_sha256":"fixture-sha","egencache_path":"egencache","egencache_args":["--repo","gentoo"],"egencache_stdout":"","egencache_stderr":"","repositories_configuration":"[gentoo]\nlocation = /checkout\n","make_conf":"PORTAGE_USERNAME=\"portage\"\n","path":"/usr/bin:/bin","portage_username":"portage","portage_group_name":"portage","master_revisions":[{"name":"gentoo","commit":"master"}]}}}
	}]}`)
	result, err := decodeStoredResult(stored)
	if err != nil {
		t.Fatal(err)
	}
	lookup := result.Reports[0].AfterLookup.Result
	provenance := lookup.Provenance
	if provenance == nil || provenance.SelectedCommit != "selected" || provenance.CacheSHA256 != "fixture-sha" || provenance.PortageUserName == nil || *provenance.PortageUserName != "portage" || provenance.MasterRevisions[0].Commit != "master" {
		t.Fatalf("provenance lost: %#v", provenance)
	}
	if provenance.MakeConf != "PORTAGE_USERNAME=\"portage\"\n" || provenance.Repositories != "[gentoo]\nlocation = /checkout\n" || lookup.Snapshot.Content != "EAPI=8\nSLOT=2/16\n" {
		t.Fatal("raw metadata/provenance text changed")
	}
	if result.Reports[0].BeforeLookup.Error.Code != "METADATA_LOOKUP_FAILED" || result.Reports[0].BeforeLookup.Error.Message != "cache read failed" {
		t.Fatal("lookup error lost")
	}
	comparison := result.Reports[0].Report.Gentoo.Comparison
	if comparison.Status != "partial" || comparison.ComparedFields[0] != "EAPI" || comparison.Limitations[0].Evidence[0].WholeSnapshot != true {
		t.Fatal("partial comparison evidence lost")
	}
	if result.Repositories["Name"].Name != "Name" {
		t.Fatal("dictionary key was renamed")
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("/private-repository-path")) {
		t.Fatal("private repository path leaked")
	}
}

func TestV1InvalidStoredResultHasMachineCode(t *testing.T) {
	handler := NewHandler(&memoryStore{id: testID, result: []byte(`{"Plan":{}}`)}, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/analyses/"+testID, nil))
	var body errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != 500 || body.Error == nil || body.Error.Code != "INVALID_STORED_RESULT" {
		t.Fatal(response.Body.String())
	}
}

func TestV1NewRequestKeepsLegacyStorageAndContextEvidence(t *testing.T) {
	store := &memoryStore{id: testID}
	handler := NewHandler(store, &sequenceProvider{})
	response := newPost(handler, `{"plan":"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]","locally_patched_packages":[{"category":"dev-libs","name":"widget"}]}`)
	if response.Code != 202 {
		t.Fatal(response.Body.String())
	}
	job := waitJob(t, handler, testID)
	if job.Status != "completed" {
		t.Fatalf("job failed: %#v", job)
	}
	var public struct {
		Analysis struct {
			Findings []struct {
				Rule     string `json:"rule_id"`
				Index    *int   `json:"entry_index"`
				Evidence struct {
					Context *int `json:"context_index"`
				} `json:"evidence"`
			} `json:"findings"`
		} `json:"analysis"`
	}
	if err := json.Unmarshal(job.Result, &public); err != nil {
		t.Fatal(err)
	}
	finding := public.Analysis.Findings[0]
	if finding.Rule != "LOCAL_PATCHED_PACKAGE" || finding.Index == nil || *finding.Index != 0 || finding.Evidence.Context == nil || *finding.Evidence.Context != 0 {
		t.Fatalf("context evidence lost: %#v", finding)
	}
	saved, found, err := store.GetJob(context.Background(), testID)
	if err != nil || !found {
		t.Fatal("saved job missing")
	}
	if !bytes.Contains(saved.Result, []byte(`"Plan":`)) || bytes.Contains(saved.Result, []byte(`"plan":`)) {
		t.Fatal("storage switched to public DTO")
	}
	var request createRequest
	if err := json.Unmarshal(saved.Request, &request); err != nil {
		t.Fatal(err)
	}
	if request.LocallyPatchedPackages[0].Name != "widget" {
		t.Fatal("saved request lost patched package")
	}
}

func jsonDifferences(path string, got, want any) []string {
	if reflect.DeepEqual(got, want) {
		return nil
	}
	var differences []string
	if g, ok := got.(map[string]any); ok {
		if w, ok := want.(map[string]any); ok {
			for key, value := range g {
				if expected, ok := w[key]; ok {
					differences = append(differences, jsonDifferences(path+"."+key, value, expected)...)
				} else {
					differences = append(differences, path+"."+key+" unexpected")
				}
			}
			for key := range w {
				if _, ok := g[key]; !ok {
					differences = append(differences, path+"."+key+" missing")
				}
			}
			return differences
		}
	}
	if g, ok := got.([]any); ok {
		if w, ok := want.([]any); ok && len(g) == len(w) {
			for i := range g {
				differences = append(differences, jsonDifferences(fmt.Sprintf("%s[%d]", path, i), g[i], w[i])...)
			}
			return differences
		}
	}
	return []string{fmt.Sprintf("%s: got %v, want %v", path, got, want)}
}
