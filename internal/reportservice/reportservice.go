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
	if !lookupAllowed(preliminary.Gentoo.Diagnostics) {
		outcome := LookupOutcome{SkipReason: SkipBindingRejected}
		return Result{Report: preliminary, BeforeLookup: outcome, AfterLookup: outcome}, nil
	}

	entry := plan.Entries[entryIndex]
	installed := entry.Installed.Versions[0]
	beforeQuery := metadataprovider.Query{
		Repository: *installed.Repository,
		Category:   entry.Package.Category,
		Name:       entry.Package.Name,
		Version:    installed.Version,
	}
	afterQuery := metadataprovider.Query{
		Repository: *entry.Target.Repository,
		Category:   entry.Package.Category,
		Name:       entry.Package.Name,
		Version:    entry.Target.Version,
	}

	beforeOutcome, before := lookup(provider, beforeQuery)
	afterOutcome, after := lookup(provider, afterQuery)

	report, err := changereport.Build(plan, entryIndex, before, after)
	if err != nil {
		return Result{}, err
	}
	return Result{Report: report, BeforeLookup: beforeOutcome, AfterLookup: afterOutcome}, nil
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
