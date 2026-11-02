// Package planservice computes the combined result for one text plan.
package planservice

import (
	"fmt"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analyzer"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
)

// Result retains the complete outputs of Parser, Analyzer, and reportservice.
type Result struct {
	Plan     parser.Result
	Analysis analyzer.Result
	Reports  []reportservice.Result
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
