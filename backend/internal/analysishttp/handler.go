// Package analysishttp exposes saved planservice results over HTTP.
package analysishttp

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"reflect"
	"regexp"
	"strings"

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
	providerFactory func(context.Context, parser.Result) (reportservice.MetadataLookup, error)
}

type createRequest struct {
	Plan                   string           `json:"plan"`
	LocallyPatchedPackages []parser.Package `json:"locally_patched_packages"`
}

type response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
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

func NewHandler(store analysisstore.Store, provider reportservice.MetadataLookup) http.Handler {
	return NewHandlerWithProviderFactory(store, func() (reportservice.MetadataLookup, error) { return provider, nil })
}

// NewHandlerWithProviderFactory obtains one pinned provider per analysis.
func NewHandlerWithProviderFactory(store analysisstore.Store, factory func() (reportservice.MetadataLookup, error)) http.Handler {
	return NewHandlerWithPlanProviderFactory(store, func(_ context.Context, _ parser.Result) (reportservice.MetadataLookup, error) {
		if factory == nil {
			return nil, nil
		}
		return factory()
	})
}

// NewHandlerWithPlanProviderFactory pins the repositories needed by the plan.
func NewHandlerWithPlanProviderFactory(store analysisstore.Store, factory func(context.Context, parser.Result) (reportservice.MetadataLookup, error)) http.Handler {
	handler := &Handler{store: store, providerFactory: factory}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /analyses", handler.create)
	mux.HandleFunc("GET /analyses/{id}", handler.get)
	return mux
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
	if h.providerFactory == nil {
		writeError(w, http.StatusServiceUnavailable, "metadata provider is unavailable")
		return
	}
	provider, err := h.providerFactory(r.Context(), parser.Parse(request.Plan))
	if err != nil || isNilProvider(provider) {
		writeError(w, http.StatusServiceUnavailable, "metadata provider is unavailable")
		return
	}

	result, err := planservice.Analyze(request.Plan, analyzer.Context{LocallyPatchedPackages: request.LocallyPatchedPackages}, provider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "analysis failed")
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
		writeError(w, http.StatusInternalServerError, "analysis serialization failed")
		return
	}
	id, err := h.store.Save(r.Context(), serialized)
	if err != nil || !analysisIDPattern.MatchString(id) {
		writeError(w, http.StatusServiceUnavailable, "analysis could not be saved")
		return
	}
	writeJSON(w, http.StatusCreated, response{ID: id, Result: serialized})
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
	writeJSON(w, http.StatusOK, response{ID: id, Result: result})
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
