// Package analysishttp exposes saved planservice results over HTTP.
package analysishttp

import (
	"context"
	"encoding/json"
	"errors"
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
	store            analysisstore.Store
	providerFactory  func(context.Context, parser.Result, map[string]repositorymanager.Revision) (reportservice.MetadataLookup, error)
	mux              http.Handler
	mu               sync.Mutex
	running          map[string]bool
	concurrencyLimit int
	dispatching      bool
	wakeRequested    bool
}

type createRequest struct {
	Plan                   string           `json:"plan"`
	LocallyPatchedPackages []parser.Package `json:"locally_patched_packages"`
}

type jobResponse struct {
	ID       string          `json:"id"`
	Status   string          `json:"status"`
	Progress progressView    `json:"progress"`
	Result   json.RawMessage `json:"result"`
	Error    *apiErrorDTO    `json:"error"`
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
	Error *apiErrorDTO `json:"error"`
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
	handler, _ := NewHandlerWithConcurrencyLimit(store, factory, 1)
	return handler
}

// NewHandlerWithConcurrencyLimit bounds active analyses in this backend.
// Zero disables the bound; negative limits are invalid.
func NewHandlerWithConcurrencyLimit(store analysisstore.Store, factory func(context.Context, parser.Result, map[string]repositorymanager.Revision) (reportservice.MetadataLookup, error), limit int) (*Handler, error) {
	if limit < 0 {
		return nil, errors.New("analysis concurrency limit must be non-negative")
	}
	handler := &Handler{store: store, providerFactory: factory, running: make(map[string]bool), concurrencyLimit: limit}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/analyses", handler.create)
	mux.HandleFunc("GET /api/v1/analyses/{id}", handler.get)
	mux.HandleFunc("/api/v1/analyses", methodNotAllowed("POST"))
	mux.HandleFunc("/api/v1/analyses/{id}", methodNotAllowed("GET, HEAD"))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "API endpoint not found")
	})
	handler.mux = mux
	return handler, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// Resume schedules every pending or interrupted job for execution. Call it
// before serving traffic so previously accepted work survives a restart.
func (h *Handler) Resume(ctx context.Context) error {
	if err := h.store.RequeueInterruptedJobs(ctx); err != nil {
		return err
	}
	h.schedule()
	return nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	var input createRequestDTO
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain one JSON object")
		return
	}
	request := createRequest{Plan: input.Plan, LocallyPatchedPackages: make([]parser.Package, len(input.LocallyPatchedPackages))}
	for i, pkg := range input.LocallyPatchedPackages {
		request.LocallyPatchedPackages[i] = parser.Package{Category: pkg.Category, Name: pkg.Name}
	}
	if strings.TrimSpace(request.Plan) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "plan must be a non-empty string")
		return
	}
	for _, pkg := range request.LocallyPatchedPackages {
		if !validPackage(pkg) {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "locally_patched_packages must contain exact Gentoo category/name pairs")
			return
		}
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "analysis storage is unavailable")
		return
	}
	serializedRequest, err := json.Marshal(request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "request serialization failed")
		return
	}
	id, err := h.store.CreateJob(r.Context(), serializedRequest)
	if err != nil || !analysisIDPattern.MatchString(id) {
		writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "analysis could not be saved")
		return
	}
	w.Header().Set("Location", "/api/v1/analyses/"+id)
	writeJSON(w, http.StatusAccepted, jobResponse{ID: id, Status: "pending", Progress: progressView{Stage: "queued"}})
	h.schedule()
}

// schedule coalesces notifications into one dispatcher. Waiting requests remain
// in PostgreSQL; no goroutine or in-memory request queue is allocated per job.
func (h *Handler) schedule() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.wakeRequested = true
	if h.dispatching {
		return
	}
	h.dispatching = true
	go h.dispatch()
}

func (h *Handler) dispatch() {
	for {
		h.mu.Lock()
		h.wakeRequested = false
		if h.concurrencyLimit > 0 && len(h.running) >= h.concurrencyLimit {
			h.dispatching = false
			h.mu.Unlock()
			return
		}
		h.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		job, found, err := h.store.ClaimNextJob(ctx)
		cancel()
		if err != nil {
			log.Printf("analysis queue read failed; retrying: %v", err)
			time.Sleep(time.Second)
			continue
		}
		if !found {
			h.mu.Lock()
			if h.wakeRequested {
				h.mu.Unlock()
				continue
			}
			h.dispatching = false
			h.mu.Unlock()
			return
		}
		h.mu.Lock()
		h.running[job.ID] = true
		h.mu.Unlock()
		h.launch(job)
	}
}

func (h *Handler) launch(job analysisstore.Job) {
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.running, job.ID)
			h.mu.Unlock()
			h.schedule()
		}()
		var request createRequest
		if err := json.Unmarshal(job.Request, &request); err != nil {
			h.updateEventually(analysisstore.JobUpdate{ID: job.ID, Status: "failed", Stage: "failed", Error: "saved job request is invalid"})
			return
		}
		h.execute(job.ID, request, job.Revisions)
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
		writeError(w, http.StatusNotFound, "ANALYSIS_NOT_FOUND", "analysis not found")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "analysis storage is unavailable")
		return
	}
	job, found, err := h.store.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "analysis storage is unavailable")
		return
	}
	if found {
		h.writeJob(w, job.ID, job.Status, progressView{Stage: job.Stage, Completed: job.Completed, Total: job.Total}, job.Result, job.Error)
		return
	}
	// Rows created by the synchronous API predate analysis_jobs. Keep them
	// readable through the new endpoint as completed analyses.
	result, found, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "analysis storage is unavailable")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "ANALYSIS_NOT_FOUND", "analysis not found")
		return
	}
	h.writeJob(w, id, "completed", progressView{Stage: "completed", Completed: 1, Total: 1}, result, "")
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

func methodNotAllowed(allow string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h *Handler) writeJob(w http.ResponseWriter, id, status string, progress progressView, stored []byte, message string) {
	response := jobResponse{ID: id, Status: status, Progress: progress}
	if status == "completed" {
		result, err := decodeStoredResult(stored)
		if err != nil {
			log.Printf("analysis %s stored result cannot be decoded: %v", id, err)
			writeError(w, http.StatusInternalServerError, "INVALID_STORED_RESULT", "stored analysis is invalid")
			return
		}
		serialized, err := json.Marshal(result)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "analysis response serialization failed")
			return
		}
		response.Result = serialized
	}
	if status == "failed" {
		response.Error = &apiErrorDTO{Code: "ANALYSIS_FAILED", Message: message}
	}
	writeJSON(w, http.StatusOK, response)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: &apiErrorDTO{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
