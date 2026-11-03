package planservice

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analyzer"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/changereport"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
)

type lookupAnswer struct {
	result metadataprovider.LookupResult
	err    error
}

type fakeLookup struct {
	answers []lookupAnswer
	calls   []metadataprovider.Query
}

func (f *fakeLookup) Lookup(query metadataprovider.Query) (metadataprovider.LookupResult, error) {
	f.calls = append(f.calls, query)
	if len(f.calls) > len(f.answers) {
		return metadataprovider.LookupResult{}, errors.New("unexpected lookup")
	}
	answer := f.answers[len(f.calls)-1]
	return answer.result, answer.err
}

func TestAnalyzeBuildsReportsSequentiallyAndPreservesFullOutcomes(t *testing.T) {
	input := strings.Join([]string{
		"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]",
		"[ebuild U] dev-libs/widget-3::gentoo [2::gentoo]",
		"[ebuild U] dev-libs/widget-4::gentoo [3::gentoo]",
		"[ebuild U] dev-libs/widget-5::gentoo [4::gentoo]",
	}, "\n")
	lookupErrBefore := fmt.Errorf("synthetic before failure: %w", metadataprovider.ErrRead)
	lookupErrAfter := fmt.Errorf("synthetic after failure: %w", metadataprovider.ErrUnsupportedEntry)
	answers := []lookupAnswer{
		{err: lookupErrBefore},
		{result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: &metadatadiff.Snapshot{}}},
		{result: metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}},
		{result: metadataprovider.LookupResult{Status: metadataprovider.StatusRepositoryUnavailable}},
		{result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound}}, // Invalid: found requires a snapshot.
		{result: metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}},
		{result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: &metadatadiff.Snapshot{}}},
		{err: lookupErrAfter},
	}
	provider := &fakeLookup{answers: answers}
	context := analyzer.Context{LocallyPatchedPackages: []parser.Package{{Category: "dev-libs", Name: "widget"}}}
	contextBefore := analyzer.Context{LocallyPatchedPackages: append([]parser.Package(nil), context.LocallyPatchedPackages...)}

	got, err := Analyze(input, context, provider)
	if err != nil {
		t.Fatal(err)
	}
	wantQueries := []metadataprovider.Query{
		{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "1"},
		{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "2"},
		{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "2"},
		{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "3"},
		{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "3"},
		{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "4"},
		{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "4"},
		{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "5"},
	}
	if !reflect.DeepEqual(provider.calls, wantQueries) {
		t.Fatalf("lookup order = %#v, want %#v", provider.calls, wantQueries)
	}
	if !reflect.DeepEqual(got.Plan, parser.Parse(input)) || !reflect.DeepEqual(got.Analysis, analyzer.Analyze(parser.Parse(input), context)) {
		t.Fatal("complete Parser or Analyzer result differs from direct call")
	}
	if len(got.Reports) != len(got.Plan.Entries) || len(got.Reports) != 4 {
		t.Fatalf("reports = %d for entries %d", len(got.Reports), len(got.Plan.Entries))
	}
	for index, report := range got.Reports {
		if report.Report.EntryIndex != index || report.Report.SourceLines[0] != index+1 {
			t.Errorf("report[%d] index/source lines = %d/%v", index, report.Report.EntryIndex, report.Report.SourceLines)
		}
	}
	if !errors.Is(got.Reports[0].BeforeLookup.Err, metadataprovider.ErrRead) || got.Reports[0].AfterLookup.Result == nil {
		t.Fatalf("first Lookup outcomes lost: %#v", got.Reports[0])
	}
	if got.Reports[1].BeforeLookup.Result == nil || got.Reports[1].BeforeLookup.Result.Status != metadataprovider.StatusNotFound || got.Reports[1].AfterLookup.Result == nil || got.Reports[1].AfterLookup.Result.Status != metadataprovider.StatusRepositoryUnavailable {
		t.Fatalf("not_found/repository_unavailable outcomes lost: %#v", got.Reports[1])
	}
	if !errors.Is(got.Reports[2].BeforeLookup.Err, reportservice.ErrInvalidLookupResult) || got.Reports[2].AfterLookup.Result == nil {
		t.Fatalf("invalid Provider response or following Lookup lost: %#v", got.Reports[2])
	}
	if got.Reports[3].BeforeLookup.Result == nil || !errors.Is(got.Reports[3].AfterLookup.Err, metadataprovider.ErrUnsupportedEntry) {
		t.Fatalf("later Lookup outcomes lost: %#v", got.Reports[3])
	}
	if !reflect.DeepEqual(context, contextBefore) {
		t.Fatalf("Analyze mutated context: got %#v, want %#v", context, contextBefore)
	}

	// Compare every complete report with the same public reportservice call.
	directProvider := &fakeLookup{answers: answers}
	for index := range got.Plan.Entries {
		want, err := reportservice.Build(got.Plan, index, directProvider)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Reports[index], want) {
			t.Errorf("report[%d] differs from full direct reportservice result", index)
		}
	}

	first, err := Analyze(input, context, &fakeLookup{answers: answers})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Analyze(input, context, &fakeLookup{answers: answers})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same input and Provider answers produced different results")
	}
}

func TestAnalyzeRequiresProviderForAnyEntryIncludingNS(t *testing.T) {
	inputs := []string{
		"[ebuild U] dev-libs/widget-2 [1]",
		"[ebuild NS] app-misc/new-1",
	}
	var typedNil *fakeLookup
	providers := []reportservice.MetadataLookup{nil, typedNil}
	for _, input := range inputs {
		for _, provider := range providers {
			got, err := Analyze(input, analyzer.Context{}, provider)
			if !errors.Is(err, reportservice.ErrProviderRequired) || !reflect.DeepEqual(got, Result{}) {
				t.Errorf("input %q provider %#v: result=%#v error=%v", input, provider, got, err)
			}
		}
	}
}

func TestAnalyzeSkipsProviderCallsForEmptyPlansAndRejectedBindings(t *testing.T) {
	t.Run("no entries", func(t *testing.T) {
		inputs := []string{
			"",
			"unrecognized significant text",
			"[blocks B] cat/old",
			"[uninstall] cat/old-1",
			"[blocks B] cat/old\n[uninstall] cat/old-1",
		}
		for _, input := range inputs {
			got, err := Analyze(input, analyzer.Context{}, nil)
			if err != nil {
				t.Fatalf("input %q: %v", input, err)
			}
			if len(got.Plan.Entries) != 0 || len(got.Reports) != 0 || !reflect.DeepEqual(got.Plan, parser.Parse(input)) || !reflect.DeepEqual(got.Analysis, analyzer.Analyze(parser.Parse(input), analyzer.Context{})) {
				t.Errorf("input %q: result=%#v", input, got)
			}
		}
	})

	t.Run("NS and rejected relationship", func(t *testing.T) {
		input := strings.Join([]string{
			"[ebuild NS] app-misc/new-1",
			"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo] [0::gentoo]",
		}, "\n")
		provider := &fakeLookup{}
		got, err := Analyze(input, analyzer.Context{}, provider)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Reports) != 2 || len(provider.calls) != 0 {
			t.Fatalf("reports/calls = %d/%d", len(got.Reports), len(provider.calls))
		}
		for index, report := range got.Reports {
			if report.Report.EntryIndex != index || report.BeforeLookup.SkipReason != reportservice.SkipBindingRejected || report.AfterLookup.SkipReason != reportservice.SkipBindingRejected {
				t.Errorf("report[%d] skip outcomes = %#v", index, report)
			}
		}
	})
}

func TestAnalyzePreservesPartialFactsAndFindingReferences(t *testing.T) {
	input := strings.Join([]string{
		"[ebuild UD] dev-libs/widget-1::gentoo [2::gentoo] USE=\"flag*\"",
		"[blocks B] <dev-libs/old-2 (is hard blocking dev-libs/widget-1)",
		"[uninstall] app-misc/old-1",
		"WARNING: One or more updates/rebuilds have been skipped due to a dependency conflict:",
	}, "\n")
	lookupError := fmt.Errorf("synthetic lookup: %w", metadataprovider.ErrRead)
	provider := &fakeLookup{answers: []lookupAnswer{
		{err: lookupError},
		{result: metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}},
	}}
	context := analyzer.Context{LocallyPatchedPackages: []parser.Package{{Category: "dev-libs", Name: "widget"}}}
	contextBefore := analyzer.Context{LocallyPatchedPackages: append([]parser.Package(nil), context.LocallyPatchedPackages...)}
	got, err := Analyze(input, context, provider)
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan.Status != parser.StatusPartial || len(got.Plan.Entries) != 1 || len(got.Plan.Blockers) != 1 || len(got.Plan.Removals) != 1 || len(got.Plan.Diagnostics) != 1 || !reflect.DeepEqual(got.Plan.Diagnostics[0].SourceLines, []int{4}) {
		t.Fatalf("partial plan facts = %#v", got.Plan)
	}
	if !reflect.DeepEqual(got.Plan, parser.Parse(input)) || !reflect.DeepEqual(got.Analysis, analyzer.Analyze(got.Plan, context)) {
		t.Fatal("partial Parser/Analyzer outputs were altered")
	}
	if len(got.Reports) != 1 || !errors.Is(got.Reports[0].BeforeLookup.Err, metadataprovider.ErrRead) {
		t.Fatalf("report collection/outcome = %#v", got.Reports)
	}
	wantRules := []analyzer.RuleID{
		analyzer.RuleDowngradePlanned,
		analyzer.RuleLocalPatchedPackage,
		analyzer.RuleUseChanged,
		analyzer.RuleUnresolvedBlocker,
	}
	if len(got.Analysis.Findings) != len(wantRules) {
		t.Fatalf("findings = %#v", got.Analysis.Findings)
	}
	for index, finding := range got.Analysis.Findings {
		if finding.RuleID != wantRules[index] {
			t.Errorf("finding[%d] rule = %s, want %s", index, finding.RuleID, wantRules[index])
		}
		if index < 3 && (finding.EntryIndex == nil || *finding.EntryIndex != 0) {
			t.Errorf("package finding[%d] entry reference = %v", index, finding.EntryIndex)
		}
	}
	blocker := got.Analysis.Findings[3]
	evidence, ok := blocker.Evidence.(analyzer.UnresolvedBlockerEvidence)
	if !ok || blocker.EntryIndex != nil || evidence.BlockerIndex != 0 {
		t.Fatalf("blocker finding reference = %#v evidence=%#v", blocker, blocker.Evidence)
	}
	if !reflect.DeepEqual(context, contextBefore) {
		t.Fatalf("Analyze mutated context: got %#v, want %#v", context, contextBefore)
	}
}

func TestAnalyzeWrapsBuildErrorAndReturnsZeroResult(t *testing.T) {
	var typedNil *fakeLookup
	got, err := Analyze("[ebuild U] dev-libs/widget-2 [1]", analyzer.Context{}, typedNil)
	if !errors.Is(err, reportservice.ErrProviderRequired) || !strings.Contains(err.Error(), "entry 0") || !reflect.DeepEqual(got, Result{}) {
		t.Fatalf("result=%#v error=%v", got, err)
	}
}

func TestAnalyzeWithRealMetadataProviderAndSyntheticGitRepository(t *testing.T) {
	root := t.TempDir()
	runGitForTest(t, root, "init", "-q")
	cache := map[string]string{
		"metadata/md5-cache/dev-libs/widget-1": "EAPI=8\nSLOT=1/1\nIUSE=old\n",
		"metadata/md5-cache/dev-libs/widget-2": "EAPI=8\nSLOT=2/2\nIUSE=new\n",
		"metadata/md5-cache/dev-libs/gadget-1": "EAPI=8\nSLOT=1/1\nIUSE=after\n",
		"metadata/md5-cache/dev-libs/gadget-2": "EAPI=8\nSLOT=2/2\nIUSE=before\n",
	}
	for path, content := range cache {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitForTest(t, root, "add", "metadata")
	runGitForTest(t, root, "-c", "user.name=Synthetic Test", "-c", "user.email=synthetic@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "synthetic plan metadata")
	commitBytes, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(commitBytes))
	provider, err := metadataprovider.Open(metadataprovider.Config{
		Repository: "gentoo", SourceURL: "synthetic://local", LocalPath: root, Commit: commit,
	})
	if err != nil {
		t.Fatal(err)
	}

	input := strings.Join([]string{
		"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]",
		"[ebuild UD] dev-libs/gadget-1::gentoo [2::gentoo]",
		"[ebuild U] dev-libs/missing-2::gentoo [1::gentoo]",
		"[ebuild NS] app-misc/new-1::gentoo",
		"[blocks B] <dev-libs/old-2 (is hard blocking dev-libs/widget-2)",
		"[uninstall] app-misc/old-1",
		"WARNING: One or more updates/rebuilds have been skipped due to a dependency conflict:",
	}, "\n")
	got, err := Analyze(input, analyzer.Context{}, provider)
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan.Status != parser.StatusPartial || len(got.Plan.Entries) != 4 || len(got.Plan.Blockers) != 1 || len(got.Plan.Removals) != 1 || len(got.Plan.Diagnostics) != 1 || !reflect.DeepEqual(got.Plan.Diagnostics[0].SourceLines, []int{7}) {
		t.Fatalf("parsed facts = %#v", got.Plan)
	}
	for index, entry := range got.Plan.Entries {
		if !reflect.DeepEqual(entry.SourceLines, []int{index + 1}) {
			t.Errorf("entry[%d] source lines = %v", index, entry.SourceLines)
		}
	}
	if got.Plan.Blockers[0].RawLine != "[blocks B] <dev-libs/old-2 (is hard blocking dev-libs/widget-2)" || !reflect.DeepEqual(got.Plan.Blockers[0].SourceLines, []int{5}) || !reflect.DeepEqual(got.Plan.Removals[0].SourceLines, []int{6}) {
		t.Fatalf("blocker/removal source facts = %#v / %#v", got.Plan.Blockers[0], got.Plan.Removals[0])
	}
	if len(got.Reports) != 4 {
		t.Fatalf("reports = %d", len(got.Reports))
	}
	for index, report := range got.Reports {
		if report.Report.EntryIndex != index || !reflect.DeepEqual(report.Report.SourceLines, got.Plan.Entries[index].SourceLines) {
			t.Errorf("report[%d] order/source lines = %#v", index, report.Report)
		}
	}

	upgrade := got.Reports[0]
	upgradeComparison := upgrade.Report.Gentoo.Comparison
	if upgradeComparison == nil || upgrade.Report.Gentoo.Status != metadatadiff.StatusAvailable {
		t.Fatalf("upgrade report = %#v", upgrade.Report.Gentoo)
	}
	if upgrade.BeforeLookup.Query == nil || *upgrade.BeforeLookup.Query != (metadataprovider.Query{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "1"}) || upgrade.AfterLookup.Query == nil || *upgrade.AfterLookup.Query != (metadataprovider.Query{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "2"}) {
		t.Fatalf("upgrade queries = %#v / %#v", upgrade.BeforeLookup.Query, upgrade.AfterLookup.Query)
	}
	if upgradeComparison.Before.Source.Commit != commit || upgradeComparison.After.Source.Commit != commit || upgradeComparison.Before.Source.Version != "1" || upgradeComparison.After.Source.Version != "2" {
		t.Fatalf("upgrade provenance = %#v / %#v", upgradeComparison.Before.Source, upgradeComparison.After.Source)
	}
	if upgradeComparison.Changes.Slot == nil || upgradeComparison.Changes.Slot.Before != "1" || upgradeComparison.Changes.Slot.After != "2" || len(upgradeComparison.Changes.IUSE.Added) != 1 || upgradeComparison.Changes.IUSE.Added[0].Name != "new" || len(upgradeComparison.Changes.IUSE.Removed) != 1 || upgradeComparison.Changes.IUSE.Removed[0].Name != "old" {
		t.Fatalf("upgrade changes = %#v", upgradeComparison.Changes)
	}
	if !reflect.DeepEqual(upgradeComparison.Changes.Slot.Evidence.Before.Lines, []int{2}) || !reflect.DeepEqual(upgradeComparison.Changes.IUSE.Added[0].Evidence.After.Lines, []int{3}) {
		t.Fatalf("upgrade evidence = slot %#v IUSE %#v", upgradeComparison.Changes.Slot.Evidence, upgradeComparison.Changes.IUSE.Added[0].Evidence)
	}

	downgrade := got.Reports[1]
	downgradeComparison := downgrade.Report.Gentoo.Comparison
	if downgradeComparison == nil || downgradeComparison.Before.Source.Version != "2" || downgradeComparison.After.Source.Version != "1" || downgradeComparison.Changes.Slot == nil || downgradeComparison.Changes.Slot.Before != "2" || downgradeComparison.Changes.Slot.After != "1" {
		t.Fatalf("downgrade report = %#v", downgrade.Report.Gentoo)
	}
	missing := got.Reports[2]
	if missing.BeforeLookup.Result == nil || missing.BeforeLookup.Result.Status != metadataprovider.StatusNotFound || missing.AfterLookup.Result == nil || missing.AfterLookup.Result.Status != metadataprovider.StatusNotFound || missing.Report.Gentoo.Comparison != nil || !hasDiagnostic(missing.Report.Gentoo.Diagnostics, changereport.CodeSnapshotMissing) {
		t.Fatalf("missing metadata outcome = %#v", missing)
	}
	if got.Reports[3].BeforeLookup.SkipReason != reportservice.SkipBindingRejected || got.Reports[3].AfterLookup.SkipReason != reportservice.SkipBindingRejected || got.Reports[3].BeforeLookup.Query != nil || got.Reports[3].AfterLookup.Query != nil {
		t.Fatalf("NS Lookup was not skipped: %#v", got.Reports[3])
	}

	var downgradeFinding, blockerFinding *analyzer.Finding
	for index := range got.Analysis.Findings {
		finding := &got.Analysis.Findings[index]
		switch finding.RuleID {
		case analyzer.RuleDowngradePlanned:
			downgradeFinding = finding
		case analyzer.RuleUnresolvedBlocker:
			blockerFinding = finding
		}
	}
	if downgradeFinding == nil || downgradeFinding.EntryIndex == nil || *downgradeFinding.EntryIndex != 1 || !strings.Contains(downgradeFinding.Message, "2 → 1") {
		t.Fatalf("downgrade finding = %#v", downgradeFinding)
	}
	if blockerFinding == nil || blockerFinding.EntryIndex != nil {
		t.Fatalf("blocker finding = %#v", blockerFinding)
	}
	blockerEvidence, ok := blockerFinding.Evidence.(analyzer.UnresolvedBlockerEvidence)
	if !ok || blockerEvidence.BlockerIndex != 0 {
		t.Fatalf("blocker evidence = %#v", blockerFinding.Evidence)
	}
}

func runGitForTest(t *testing.T, root string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", root}, args...)
	command := exec.Command("git", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func hasDiagnostic(diagnostics []changereport.BindingDiagnostic, code changereport.BindingCode) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
