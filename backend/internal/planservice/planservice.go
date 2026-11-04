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
	provider reportservice.MetadataLookup
	results  map[metadataprovider.Query]lookupResponse
}

func (p *analysisLookup) Lookup(query metadataprovider.Query) (metadataprovider.LookupResult, error) {
	if response, found := p.results[query]; found {
		return response.result, response.err
	}
	result, err := p.provider.Lookup(query)
	p.results[query] = lookupResponse{result: result, err: err}
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

func memoizeLookups(provider reportservice.MetadataLookup) reportservice.MetadataLookup {
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
	return &analysisLookup{provider: provider, results: make(map[metadataprovider.Query]lookupResponse)}
}

// Analyze parses a plan once, analyzes it once, and builds one report for each
// parsed entry in order. It performs no I/O except through provider.Lookup.
func Analyze(
	input string,
	context analyzer.Context,
	provider reportservice.MetadataLookup,
) (Result, error) {
	plan := parser.Parse(input)
	analysis := analyzer.Analyze(plan, context)
	provider = memoizeLookups(provider)
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
