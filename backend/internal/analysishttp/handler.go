// Package analysishttp exposes saved planservice results over HTTP.
package analysishttp

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"mime"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysisstore"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/analyzer"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/changereport"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/planservice"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
)

const maxRequestBody = 4 << 20

var (
	analysisIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	versionSuffix     = regexp.MustCompile(`-[0-9]+(\.[0-9]+)*[a-z]?(_(alpha|beta|pre|rc|p)[0-9]*)*(-r[0-9]+)?$`)
)

type Handler struct {
	store           analysisstore.Store
	providerFactory func(context.Context, parser.Result, map[string]repositorymanager.Revision) (reportservice.MetadataLookup, error)
	mux             http.Handler
	mu              sync.Mutex
	running         map[string]bool
}

type createRequest struct {
	Plan                   string           `json:"plan"`
	LocallyPatchedPackages []parser.Package `json:"locally_patched_packages"`
}

type jobResponse struct {
	ID       string          `json:"id"`
	Status   string          `json:"status"`
	Progress progressView    `json:"progress"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
}

type progressView struct {
	Stage     string `json:"stage"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
}

type resultView struct {
	Plan         parser.Result
	Analysis     analyzer.Result
	Reports      []reportView
	Repositories map[string]repositorymanager.Revision `json:",omitempty"`
}

type reportView struct {
	Report       changereport.Report
	BeforeLookup lookupView
	AfterLookup  lookupView
}

type lookupView struct {
	Query      *metadataprovider.Query
	Result     *metadataprovider.LookupResult
	Err        *string
	SkipReason reportservice.SkipReason
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewHandler(store analysisstore.Store, provider reportservice.MetadataLookup) *Handler {
	return NewHandlerWithProviderFactory(store, func() (reportservice.MetadataLookup, error) { return provider, nil })
}

// NewHandlerWithProviderFactory obtains one pinned provider per analysis.
func NewHandlerWithProviderFactory(store analysisstore.Store, factory func() (reportservice.MetadataLookup, error)) *Handler {
	return NewHandlerWithPinnedProviderFactory(store, func(_ context.Context, _ parser.Result, _ map[string]repositorymanager.Revision) (reportservice.MetadataLookup, error) {
		if factory == nil {
			return nil, nil
		}
		return factory()
	})
}

// NewHandlerWithPlanProviderFactory pins the repositories needed by the plan.
func NewHandlerWithPlanProviderFactory(store analysisstore.Store, factory func(context.Context, parser.Result) (reportservice.MetadataLookup, error)) *Handler {
	return NewHandlerWithPinnedProviderFactory(store, func(ctx context.Context, plan parser.Result, _ map[string]repositorymanager.Revision) (reportservice.MetadataLookup, error) {
		if factory == nil {
			return nil, nil
		}
		return factory(ctx, plan)
	})
}

// NewHandlerWithPinnedProviderFactory restores persisted repository revisions
// when resuming an interrupted analysis.
func NewHandlerWithPinnedProviderFactory(store analysisstore.Store, factory func(context.Context, parser.Result, map[string]repositorymanager.Revision) (reportservice.MetadataLookup, error)) *Handler {
	handler := &Handler{store: store, providerFactory: factory, running: make(map[string]bool)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /analyses", handler.create)
	mux.HandleFunc("GET /analyses/{id}", handler.get)
	handler.mux = mux
	return handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// Resume schedules every pending or interrupted job for execution. Call it
// before serving traffic so previously accepted work survives a restart.
func (h *Handler) Resume(ctx context.Context) error {
	jobs, err := h.store.UnfinishedJobs(ctx)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		var request createRequest
		if err := json.Unmarshal(job.Request, &request); err != nil {
			h.updateEventually(analysisstore.JobUpdate{ID: job.ID, Status: "failed", Stage: "failed", Completed: job.Completed, Total: job.Total, Error: "saved job request is invalid"})
			continue
		}
		h.launch(job.ID, request, job.Revisions)
	}
	return nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	var request createRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return
	}
	if strings.TrimSpace(request.Plan) == "" {
		writeError(w, http.StatusBadRequest, "plan must be a non-empty string")
		return
	}
	for _, pkg := range request.LocallyPatchedPackages {
		if !validPackage(pkg) {
			writeError(w, http.StatusBadRequest, "locally_patched_packages must contain exact Gentoo category/name pairs")
			return
		}
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "analysis storage is unavailable")
		return
	}
	serializedRequest, err := json.Marshal(request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "request serialization failed")
		return
	}
	id, err := h.store.CreateJob(r.Context(), serializedRequest)
	if err != nil || !analysisIDPattern.MatchString(id) {
		writeError(w, http.StatusServiceUnavailable, "analysis could not be saved")
		return
	}
	w.Header().Set("Location", "/analyses/"+id)
	writeJSON(w, http.StatusAccepted, jobResponse{ID: id, Status: "pending", Progress: progressView{Stage: "queued"}})
	h.launch(id, request, nil)
}

func (h *Handler) launch(id string, request createRequest, pinnedJSON []byte) {
	h.mu.Lock()
	if h.running[id] {
		h.mu.Unlock()
		return
	}
	h.running[id] = true
	h.mu.Unlock()
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.running, id)
			h.mu.Unlock()
		}()
		h.execute(id, request, pinnedJSON)
	}()
}

func (h *Handler) execute(id string, request createRequest, pinnedJSON []byte) {
	update := func(status, stage string, completed, total int, result, revisions []byte, message string) {
		h.updateEventually(analysisstore.JobUpdate{
			ID: id, Status: status, Stage: stage, Completed: completed, Total: total, Result: result, Revisions: revisions, Error: message,
		})
	}
	var pinned map[string]repositorymanager.Revision
	if len(pinnedJSON) > 0 {
		if err := json.Unmarshal(pinnedJSON, &pinned); err != nil || len(pinned) == 0 {
			update("failed", "failed", 0, 0, nil, nil, "saved repository revisions are invalid")
			return
		}
	}
	parsed := parser.Parse(request.Plan)
	total, err := planservice.LookupUnitCount(parsed)
	if err != nil {
		update("failed", "failed", 0, 0, nil, nil, "analysis plan could not be prepared")
		return
	}
	update("running", "preparing", 0, total, nil, nil, "")
	if h.providerFactory == nil {
		update("failed", "failed", 0, total, nil, nil, "metadata provider is unavailable")
		return
	}
	provider, err := h.providerFactory(context.Background(), parsed, pinned)
	if err != nil || isNilProvider(provider) {
		if len(pinned) > 0 {
			log.Printf("analysis job %s could not restore pinned repositories: %v", id, err)
			update("failed", "failed", 0, total, nil, nil, "pinned repository revisions could not be restored")
		} else {
			update("failed", "failed", 0, total, nil, nil, "metadata provider is unavailable")
		}
		return
	}
	if source, ok := provider.(interface {
		RepositoryRevisions() map[string]repositorymanager.Revision
	}); ok {
		revisions, marshalErr := json.Marshal(source.RepositoryRevisions())
		if marshalErr != nil {
			update("failed", "failed", 0, total, nil, nil, "repository revisions could not be saved")
			return
		}
		if len(source.RepositoryRevisions()) > 0 {
			update("running", "preparing", 0, total, nil, revisions, "")
		}
	}
	lastCompleted, lastTotal := 0, total
	result, err := planservice.AnalyzeParsedWithProgress(request.Plan, parsed, analyzer.Context{LocallyPatchedPackages: request.LocallyPatchedPackages}, provider, func(completed, total int) {
		lastCompleted, lastTotal = completed, total
		update("running", "analyzing", completed, total, nil, nil, "")
	})
	if err != nil {
		update("failed", "failed", lastCompleted, lastTotal, nil, nil, "analysis failed")
		return
	}
	view := presentResult(result)
	if source, ok := provider.(interface {
		RepositoryRevisions() map[string]repositorymanager.Revision
	}); ok {
		view.Repositories = source.RepositoryRevisions()
	}
	serialized, err := json.Marshal(view)
	if err != nil {
		update("failed", "failed", lastCompleted, lastTotal, nil, nil, "analysis serialization failed")
		return
	}
	update("completed", "completed", lastTotal, lastTotal, serialized, nil, "")
}

func (h *Handler) updateEventually(update analysisstore.JobUpdate) {
	delay := 100 * time.Millisecond
	logged := false
	for {
		if err := h.store.UpdateJob(context.Background(), update); err == nil {
			return
		} else if !logged {
			log.Printf("analysis job %s state update failed; retrying: %v", update.ID, err)
			logged = true
		}
		time.Sleep(delay)
		if delay < 5*time.Second {
			delay *= 2
		}
	}
}

func isNilProvider(provider reportservice.MetadataLookup) bool {
	if provider == nil {
		return true
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !analysisIDPattern.MatchString(id) {
		writeError(w, http.StatusNotFound, "analysis not found")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "analysis storage is unavailable")
		return
	}
	job, found, err := h.store.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "analysis storage is unavailable")
		return
	}
	if found {
		if len(job.Result) > 0 && !json.Valid(job.Result) {
			writeError(w, http.StatusInternalServerError, "stored analysis is invalid")
			return
		}
		writeJSON(w, http.StatusOK, jobResponse{ID: id, Status: job.Status, Progress: progressView{Stage: job.Stage, Completed: job.Completed, Total: job.Total}, Result: job.Result, Error: job.Error})
		return
	}
	// Rows created by the synchronous API predate analysis_jobs. Keep them
	// readable through the new endpoint as completed analyses.
	result, found, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "analysis storage is unavailable")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "analysis not found")
		return
	}
	if !json.Valid(result) {
		writeError(w, http.StatusInternalServerError, "stored analysis is invalid")
		return
	}
	writeJSON(w, http.StatusOK, jobResponse{
		ID: id, Status: "completed", Progress: progressView{Stage: "completed", Completed: 1, Total: 1}, Result: result,
	})
}

func presentResult(result planservice.Result) resultView {
	view := resultView{
		Plan: result.Plan, Analysis: result.Analysis, Reports: make([]reportView, len(result.Reports)),
	}
	for index, report := range result.Reports {
		view.Reports[index] = reportView{
			Report: report.Report, BeforeLookup: presentLookup(report.BeforeLookup), AfterLookup: presentLookup(report.AfterLookup),
		}
	}
	return view
}

func presentLookup(outcome reportservice.LookupOutcome) lookupView {
	view := lookupView{Query: outcome.Query, Result: outcome.Result, SkipReason: outcome.SkipReason}
	if outcome.Err != nil {
		message := outcome.Err.Error()
		view.Err = &message
	}
	return view
}

func validPackage(pkg parser.Package) bool {
	if pkg.Category == "" || pkg.Name == "" || pkg.Category[0] == '-' || pkg.Category[0] == '.' || pkg.Category[0] == '+' || pkg.Name[0] == '-' || pkg.Name[0] == '+' || versionSuffix.MatchString(pkg.Name) {
		return false
	}
	for index := 0; index < len(pkg.Category); index++ {
		char := pkg.Category[index]
		if !asciiAlphanumeric(char) && char != '+' && char != '_' && char != '.' && char != '-' {
			return false
		}
	}
	for index := 0; index < len(pkg.Name); index++ {
		char := pkg.Name[index]
		if !asciiAlphanumeric(char) && char != '+' && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func asciiAlphanumeric(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
}

func isJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/json"
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
