// Package planservice computes the combined result for one text plan.
package planservice

import (
	"fmt"
	"reflect"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analyzer"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
)

// Result retains the complete outputs of Parser, Analyzer, and reportservice.
type Result struct {
	Plan     parser.Result
	Analysis analyzer.Result
	Reports  []reportservice.Result
}

type lookupResponse struct {
	result metadataprovider.LookupResult
	err    error
}

type analysisLookup struct {
	provider  reportservice.MetadataLookup
	results   map[metadataprovider.Query]lookupResponse
	completed int
	total     int
	progress  func(completed, total int)
}

func (p *analysisLookup) Lookup(query metadataprovider.Query) (metadataprovider.LookupResult, error) {
	if response, found := p.results[query]; found {
		return response.result, response.err
	}
	result, err := p.provider.Lookup(query)
	p.results[query] = lookupResponse{result: result, err: err}
	p.completed++
	if p.progress != nil {
		p.progress(p.completed, p.total)
	}
	return result, err
}

// RepositoryFailure preserves repository preparation errors for reportservice
// when it skips metadata lookups because a binding was rejected.
func (p *analysisLookup) RepositoryFailure(name string) error {
	source, ok := p.provider.(interface{ RepositoryFailure(string) error })
	if !ok {
		return nil
	}
	return source.RepositoryFailure(name)
}

func memoizeLookups(provider reportservice.MetadataLookup, total int, progress func(completed, total int)) reportservice.MetadataLookup {
	if provider == nil {
		return nil
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		if value.IsNil() {
			return provider
		}
	}
	return &analysisLookup{provider: provider, results: make(map[metadataprovider.Query]lookupResponse), total: total, progress: progress}
}

// Analyze parses a plan once, analyzes it once, and builds one report for each
// parsed entry in order. It performs no I/O except through provider.Lookup.
func Analyze(
	input string,
	context analyzer.Context,
	provider reportservice.MetadataLookup,
) (Result, error) {
	return AnalyzeWithProgress(input, context, provider, nil)
}

// AnalyzeWithProgress reports completed unique metadata queries and their total
// number. The callback runs in order on the analysis goroutine.
func AnalyzeWithProgress(
	input string,
	context analyzer.Context,
	provider reportservice.MetadataLookup,
	progress func(completed, total int),
) (Result, error) {
	return AnalyzeParsedWithProgress(input, parser.Parse(input), context, provider, progress)
}

// LookupUnitCount returns the number of distinct metadata Query values needed
// to build the plan's reports.
func LookupUnitCount(plan parser.Result) (int, error) {
	queries := make(map[metadataprovider.Query]struct{})
	for index := range plan.Entries {
		entryQueries, err := reportservice.LookupQueries(plan, index)
		if err != nil {
			return 0, fmt.Errorf("plan report lookups for entry %d: %w", index, err)
		}
		for _, query := range entryQueries {
			queries[query] = struct{}{}
		}
	}
	return len(queries), nil
}

// AnalyzeParsedWithProgress analyzes an already parsed plan and reports the
// number of completed unique metadata queries.
func AnalyzeParsedWithProgress(
	input string,
	plan parser.Result,
	context analyzer.Context,
	provider reportservice.MetadataLookup,
	progress func(completed, total int),
) (Result, error) {
	analysis := analyzer.Analyze(plan, context)
	total, err := LookupUnitCount(plan)
	if err != nil {
		return Result{}, err
	}
	if progress != nil {
		progress(0, total)
	}
	provider = memoizeLookups(provider, total, progress)
	reports := make([]reportservice.Result, 0, len(plan.Entries))
	for index := range plan.Entries {
		report, err := reportservice.Build(plan, index, provider)
		if err != nil {
			return Result{}, fmt.Errorf("build report for entry %d: %w", index, err)
		}
		reports = append(reports, report)
	}
	return Result{Plan: plan, Analysis: analysis, Reports: reports}, nil
}

// AnalyzeParsedParallelWithProgress performs at most two independent lookups at
// once. The provider must support concurrent Lookup calls; progress remains
// serialized on the caller goroutine and reports retain the plan order.
func AnalyzeParsedParallelWithProgress(input string, plan parser.Result, context analyzer.Context, provider reportservice.MetadataLookup, progress func(int, int)) (Result, error) {
	if progress == nil {
		return AnalyzeParsedProgressive(input, plan, context, provider, 2, nil)
	}
	lastCompleted := -1
	return AnalyzeParsedProgressive(input, plan, context, provider, 2, func(completed, total int, _ *Result) {
		if progress != nil && completed != lastCompleted {
			progress(completed, total)
		}
		lastCompleted = completed
	})
}

// AnalyzeParsedProgressive publishes completed entries in plan order. Provider
// calls run with the requested concurrency and all callbacks are serialized.
func AnalyzeParsedProgressive(input string, plan parser.Result, context analyzer.Context, provider reportservice.MetadataLookup, concurrency int, progress func(int, int, *Result)) (Result, error) {
	wrapped, ok := memoizeLookups(provider, 0, nil).(*analysisLookup)
	if !ok {
		return AnalyzeParsedWithProgress(input, plan, context, provider, nil)
	}
	if concurrency < 1 {
		concurrency = 1
	}
	queries := make([]metadataprovider.Query, 0)
	seen := make(map[metadataprovider.Query]bool)
	entryQueries := make([][]metadataprovider.Query, len(plan.Entries))
	for index := range plan.Entries {
		candidateQueries, err := reportservice.LookupQueries(plan, index)
		if err != nil {
			return Result{}, fmt.Errorf("plan report lookups for entry %d: %w", index, err)
		}
		entryQueries[index] = candidateQueries
		for _, query := range candidateQueries {
			if !seen[query] {
				seen[query] = true
				queries = append(queries, query)
			}
		}
	}
	if progress != nil {
		progress(0, len(queries), nil)
	}
	type completedLookup struct {
		query    metadataprovider.Query
		response lookupResponse
	}
	jobs := make(chan metadataprovider.Query)
	completed := make(chan completedLookup)
	for i := 0; i < concurrency; i++ {
		go func() {
			for query := range jobs {
				result, err := provider.Lookup(query)
				completed <- completedLookup{query, lookupResponse{result, err}}
			}
		}()
	}
	go func() {
		for _, query := range queries {
			jobs <- query
		}
		close(jobs)
	}()
	analysis := analyzer.Analyze(plan, context)
	ready := make([]*reportservice.Result, len(plan.Entries))
	publish := func(completed int) error {
		if progress == nil {
			return nil
		}
		changed := false
		for index, required := range entryQueries {
			if ready[index] != nil {
				continue
			}
			available := true
			for _, query := range required {
				if _, found := wrapped.results[query]; !found {
					available = false
					break
				}
			}
			if !available {
				continue
			}
			report, err := reportservice.Build(plan, index, wrapped)
			if err != nil {
				return fmt.Errorf("build report for entry %d: %w", index, err)
			}
			ready[index] = &report
			changed = true
		}
		if progress != nil && (changed || completed > 0) {
			var partial *Result
			if changed {
				value := Result{Plan: plan, Analysis: analysis, Reports: make([]reportservice.Result, 0, len(ready))}
				for _, report := range ready {
					if report != nil {
						value.Reports = append(value.Reports, *report)
					}
				}
				partial = &value
			}
			progress(completed, len(queries), partial)
		}
		return nil
	}
	publicationErr := publish(0)
	for i := range queries {
		outcome := <-completed
		wrapped.results[outcome.query] = outcome.response
		if publicationErr == nil {
			publicationErr = publish(i + 1)
		}
	}
	if publicationErr != nil {
		return Result{}, publicationErr
	}
	reports := make([]reportservice.Result, 0, len(plan.Entries))
	for index := range plan.Entries {
		report, err := reportservice.Build(plan, index, wrapped)
		if err != nil {
			return Result{}, fmt.Errorf("build report for entry %d: %w", index, err)
		}
		reports = append(reports, report)
	}
	return Result{Plan: plan, Analysis: analysis, Reports: reports}, nil
}
