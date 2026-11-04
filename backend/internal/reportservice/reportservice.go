// Package reportservice coordinates metadata lookups for one Parser entry and
// builds its ChangeReport.
package reportservice

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/changereport"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
)

var (
	ErrProviderRequired    = errors.New("metadata provider is required")
	ErrInvalidLookupResult = errors.New("invalid metadata lookup result")
)

// MetadataLookup supplies metadata for an exact query.
type MetadataLookup interface {
	Lookup(metadataprovider.Query) (metadataprovider.LookupResult, error)
}

// SkipReason explains why a lookup was not attempted.
type SkipReason string

const SkipBindingRejected SkipReason = "BINDING_REJECTED"

// LookupOutcome retains the exact request and outcome for one side.
type LookupOutcome struct {
	Query      *metadataprovider.Query
	Result     *metadataprovider.LookupResult
	Err        error
	SkipReason SkipReason
}

// Result contains the ChangeReport and the outcomes for both metadata lookups.
type Result struct {
	Report       changereport.Report
	BeforeLookup LookupOutcome
	AfterLookup  LookupOutcome
}

// Build coordinates metadata lookups for one plan entry and builds its report.
// It performs no I/O except through provider.Lookup.
func Build(plan parser.Result, entryIndex int, provider MetadataLookup) (Result, error) {
	preliminary, err := changereport.Build(plan, entryIndex, nil, nil)
	if err != nil {
		return Result{}, err
	}
	if isNilProvider(provider) {
		return Result{}, fmt.Errorf("%w", ErrProviderRequired)
	}
	queries, err := queriesForEntry(plan, entryIndex, preliminary)
	if err != nil {
		return Result{}, err
	}
	if len(queries) == 0 {
		before := LookupOutcome{SkipReason: SkipBindingRejected}
		after := LookupOutcome{SkipReason: SkipBindingRejected}
		if source, ok := provider.(interface{ RepositoryFailure(string) error }); ok {
			entry := plan.Entries[entryIndex]
			for _, installed := range entry.Installed.Versions {
				if installed.Repository != nil {
					before.Err = errors.Join(before.Err, source.RepositoryFailure(*installed.Repository))
				}
			}
			if entry.Target.Repository != nil {
				after.Err = source.RepositoryFailure(*entry.Target.Repository)
			}
		}
		return Result{Report: preliminary, BeforeLookup: before, AfterLookup: after}, nil
	}

	beforeOutcome, before := lookup(provider, queries[0])
	afterOutcome, after := lookup(provider, queries[1])

	report, err := changereport.Build(plan, entryIndex, before, after)
	if err != nil {
		return Result{}, err
	}
	return Result{Report: report, BeforeLookup: beforeOutcome, AfterLookup: afterOutcome}, nil
}

// LookupQueries returns the metadata lookup candidates Build may request for
// one entry, in before/after order. Invalid bindings return no queries.
func LookupQueries(plan parser.Result, entryIndex int) ([]metadataprovider.Query, error) {
	preliminary, err := changereport.Build(plan, entryIndex, nil, nil)
	if err != nil {
		return nil, err
	}
	return queriesForEntry(plan, entryIndex, preliminary)
}

func queriesForEntry(plan parser.Result, entryIndex int, preliminary changereport.Report) ([]metadataprovider.Query, error) {
	if !lookupAllowed(preliminary.Gentoo.Diagnostics) {
		return nil, nil
	}
	entry := plan.Entries[entryIndex]
	if len(entry.Installed.Versions) == 0 || entry.Installed.Versions[0].Repository == nil || entry.Target.Repository == nil {
		return nil, errors.New("metadata lookup bindings are incomplete")
	}
	installed := entry.Installed.Versions[0]
	return []metadataprovider.Query{
		{Repository: *installed.Repository, Category: entry.Package.Category, Name: entry.Package.Name, Version: installed.Version},
		{Repository: *entry.Target.Repository, Category: entry.Package.Category, Name: entry.Package.Name, Version: entry.Target.Version},
	}, nil
}

func isNilProvider(provider MetadataLookup) bool {
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

func lookupAllowed(diagnostics []changereport.BindingDiagnostic) bool {
	if len(diagnostics) == 0 {
		return false
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code != changereport.CodeSnapshotMissing {
			return false
		}
	}
	return true
}

func lookup(provider MetadataLookup, query metadataprovider.Query) (LookupOutcome, *metadatadiff.Snapshot) {
	queryCopy := query
	outcome := LookupOutcome{Query: &queryCopy}
	result, err := provider.Lookup(query)
	if err != nil {
		outcome.Err = err
		return outcome, nil
	}
	if !validLookupResult(result) {
		outcome.Err = fmt.Errorf("%w: status %q and snapshot presence %t", ErrInvalidLookupResult, result.Status, result.Snapshot != nil)
		return outcome, nil
	}
	resultCopy := result
	outcome.Result = &resultCopy
	if result.Status == metadataprovider.StatusFound {
		return outcome, result.Snapshot
	}
	return outcome, nil
}

func validLookupResult(result metadataprovider.LookupResult) bool {
	switch result.Status {
	case metadataprovider.StatusFound:
		return result.Snapshot != nil
	case metadataprovider.StatusNotFound, metadataprovider.StatusRepositoryUnavailable:
		return result.Snapshot == nil
	default:
		return false
	}
}
