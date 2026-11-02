package reportservice

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/changereport"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
)

type lookupAnswer struct {
	result metadataprovider.LookupResult
	err    error
}

type lookupCall struct {
	query metadataprovider.Query
}

type fakeLookup struct {
	answers []lookupAnswer
	calls   []lookupCall
}

func (f *fakeLookup) Lookup(query metadataprovider.Query) (metadataprovider.LookupResult, error) {
	f.calls = append(f.calls, lookupCall{query: query})
	if len(f.calls) > len(f.answers) {
		return metadataprovider.LookupResult{}, errors.New("unexpected lookup")
	}
	answer := f.answers[len(f.calls)-1]
	return answer.result, answer.err
}

type valueLookup struct{}

func (valueLookup) Lookup(metadataprovider.Query) (metadataprovider.LookupResult, error) {
	return metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}, nil
}

func TestLookupAllowed(t *testing.T) {
	tests := []struct {
		name        string
		diagnostics []changereport.BindingDiagnostic
		want        bool
	}{
		{name: "empty", want: false},
		{name: "only snapshot missing", diagnostics: []changereport.BindingDiagnostic{{Code: changereport.CodeSnapshotMissing}, {Code: changereport.CodeSnapshotMissing}}, want: true},
		{name: "mixed", diagnostics: []changereport.BindingDiagnostic{{Code: changereport.CodeSnapshotMissing}, {Code: changereport.CodePackageMismatch}}, want: false},
		{name: "unknown", diagnostics: []changereport.BindingDiagnostic{{Code: "FUTURE_CODE"}}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := lookupAllowed(test.diagnostics); got != test.want {
				t.Fatalf("lookupAllowed() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestBuildFoundFoundUsesExactQueriesAndProducesReport(t *testing.T) {
	plan := testPlan()
	before, after := testSnapshot("1.0", "SLOT=1/1\nIUSE=old\n"), testSnapshot("2.0", "SLOT=2/2\nIUSE=new\n")
	provider := &fakeLookup{answers: []lookupAnswer{
		{result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: &before}},
		{result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: &after}},
	}}

	got, err := Build(plan, 0, provider)
	if err != nil {
		t.Fatal(err)
	}
	wantQueries := []metadataprovider.Query{
		{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "1.0"},
		{Repository: "gentoo", Category: "dev-libs", Name: "widget", Version: "2.0"},
	}
	if len(provider.calls) != 2 || provider.calls[0].query != wantQueries[0] || provider.calls[1].query != wantQueries[1] {
		t.Fatalf("lookup calls = %#v, want before then after %#v", provider.calls, wantQueries)
	}
	if got.BeforeLookup.Query == nil || *got.BeforeLookup.Query != wantQueries[0] || got.AfterLookup.Query == nil || *got.AfterLookup.Query != wantQueries[1] {
		t.Fatalf("outcome queries = %#v / %#v", got.BeforeLookup.Query, got.AfterLookup.Query)
	}
	if got.BeforeLookup.Result == nil || got.AfterLookup.Result == nil || got.BeforeLookup.Err != nil || got.AfterLookup.Err != nil {
		t.Fatalf("lookup outcomes = %#v / %#v", got.BeforeLookup, got.AfterLookup)
	}
	comparison := got.Report.Gentoo.Comparison
	if comparison == nil || got.Report.Gentoo.Status != metadatadiff.StatusAvailable {
		t.Fatalf("report = %#v", got.Report.Gentoo)
	}
	if comparison.Changes.Slot == nil || comparison.Changes.Slot.Before != "1" || comparison.Changes.Slot.After != "2" || comparison.Changes.Subslot == nil || comparison.Changes.Subslot.Before != "1" || comparison.Changes.Subslot.After != "2" {
		t.Fatalf("slot changes = %#v / %#v", comparison.Changes.Slot, comparison.Changes.Subslot)
	}
	if !reflect.DeepEqual(comparison.Changes.Slot.Evidence.Before.Lines, []int{2}) || !reflect.DeepEqual(comparison.Changes.Slot.Evidence.After.Lines, []int{2}) {
		t.Fatalf("slot evidence = %#v", comparison.Changes.Slot.Evidence)
	}
	if !reflect.DeepEqual(comparison.Changes.IUSE.Added, []metadatadiff.IUSEFlagChange{{Name: "new", Default: metadatadiff.DefaultUnspecified, Evidence: metadatadiff.EvidencePair{Before: metadatadiff.EvidenceRef{Side: metadatadiff.SideBefore, Field: metadatadiff.FieldIUSE, Lines: []int{3}}, After: metadatadiff.EvidenceRef{Side: metadatadiff.SideAfter, Field: metadatadiff.FieldIUSE, Lines: []int{3}}}}}) {
		t.Fatalf("IUSE changes = %#v", comparison.Changes.IUSE)
	}
}

func TestBuildLookupOutcomes(t *testing.T) {
	wrappedBefore := fmt.Errorf("before read: %w", metadataprovider.ErrRead)
	wrappedAfter := fmt.Errorf("after read: %w", metadataprovider.ErrUnsupportedEntry)
	tests := []struct {
		name        string
		before      lookupAnswer
		after       lookupAnswer
		beforeError error
		afterError  error
		beforeFound bool
		afterFound  bool
	}{
		{name: "not found before", before: lookupAnswer{result: metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}}, after: foundAnswer("2.0"), afterFound: true},
		{name: "repository unavailable before", before: lookupAnswer{result: metadataprovider.LookupResult{Status: metadataprovider.StatusRepositoryUnavailable}}, after: foundAnswer("2.0"), afterFound: true},
		{name: "not found after", before: foundAnswer("1.0"), beforeFound: true, after: lookupAnswer{result: metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}}},
		{name: "repository unavailable after", before: foundAnswer("1.0"), beforeFound: true, after: lookupAnswer{result: metadataprovider.LookupResult{Status: metadataprovider.StatusRepositoryUnavailable}}},
		{name: "before error after success", before: lookupAnswer{err: wrappedBefore, result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: ptrSnapshot("wrong")}}, after: foundAnswer("2.0"), afterFound: true, beforeError: metadataprovider.ErrRead},
		{name: "before success after error", before: foundAnswer("1.0"), after: lookupAnswer{err: wrappedAfter}, beforeFound: true, afterError: metadataprovider.ErrUnsupportedEntry},
		{name: "both errors", before: lookupAnswer{err: wrappedBefore}, after: lookupAnswer{err: wrappedAfter}, beforeError: metadataprovider.ErrRead, afterError: metadataprovider.ErrUnsupportedEntry},
		{name: "result plus error prefers error", before: lookupAnswer{err: wrappedBefore, result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: ptrSnapshot("1.0")}}, after: foundAnswer("2.0"), beforeError: metadataprovider.ErrRead, afterFound: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &fakeLookup{answers: []lookupAnswer{test.before, test.after}}
			got, err := Build(testPlan(), 0, provider)
			if err != nil {
				t.Fatal(err)
			}
			if len(provider.calls) != 2 {
				t.Fatalf("calls = %d, want 2", len(provider.calls))
			}
			assertOutcome(t, got.BeforeLookup, test.beforeError, test.beforeFound)
			assertOutcome(t, got.AfterLookup, test.afterError, test.afterFound)
			if !test.beforeFound || !test.afterFound {
				if got.Report.Gentoo.Comparison != nil || !containsCode(got.Report.Gentoo.Diagnostics, changereport.CodeSnapshotMissing) {
					t.Fatalf("missing snapshots not reflected by report: %#v", got.Report.Gentoo)
				}
			}
		})
	}
}

func TestBuildRejectsMalformedProviderResultsAndContinues(t *testing.T) {
	invalid := []struct {
		name   string
		result metadataprovider.LookupResult
	}{
		{name: "unknown status", result: metadataprovider.LookupResult{Status: "future"}},
		{name: "empty status", result: metadataprovider.LookupResult{}},
		{name: "found without snapshot", result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound}},
		{name: "snapshot with not found", result: metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound, Snapshot: ptrSnapshot("1.0")}},
		{name: "snapshot with unavailable", result: metadataprovider.LookupResult{Status: metadataprovider.StatusRepositoryUnavailable, Snapshot: ptrSnapshot("1.0")}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			provider := &fakeLookup{answers: []lookupAnswer{{result: test.result}, {result: foundAnswer("2.0").result}}}
			got, err := Build(testPlan(), 0, provider)
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(got.BeforeLookup.Err, ErrInvalidLookupResult) || got.BeforeLookup.Query == nil || got.BeforeLookup.Result != nil {
				t.Fatalf("invalid before outcome = %#v", got.BeforeLookup)
			}
			if len(provider.calls) != 2 || got.AfterLookup.Result == nil {
				t.Fatalf("second lookup did not proceed: calls=%d outcome=%#v", len(provider.calls), got.AfterLookup)
			}
			if got.Report.Gentoo.Comparison != nil || !containsCode(got.Report.Gentoo.Diagnostics, changereport.CodeSnapshotMissing) {
				t.Fatalf("invalid snapshot reached report: %#v", got.Report.Gentoo)
			}
		})
	}
}

func TestBuildSkipsUnsupportedBindingsAndRetainsPreliminaryReport(t *testing.T) {
	tests := []struct {
		name string
		edit func(*parser.Result)
	}{
		{name: "NS install", edit: func(plan *parser.Result) { plan.Entries[0].Operation = parser.OperationInstall }},
		{name: "ambiguous installed", edit: func(plan *parser.Result) {
			plan.Entries[0].Installed.Versions = append(plan.Entries[0].Installed.Versions, plan.Entries[0].Installed.Versions[0])
		}},
		{name: "unknown before repository", edit: func(plan *parser.Result) { plan.Entries[0].Installed.Versions[0].Repository = nil }},
		{name: "unknown after repository", edit: func(plan *parser.Result) { plan.Entries[0].Target.Repository = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := testPlan()
			test.edit(&plan)
			preliminary, err := changereport.Build(plan, 0, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			provider := &fakeLookup{}
			got, err := Build(plan, 0, provider)
			if err != nil {
				t.Fatal(err)
			}
			if len(provider.calls) != 0 || !reflect.DeepEqual(got.Report, preliminary) {
				t.Fatalf("got calls=%d report=%#v, want preliminary %#v", len(provider.calls), got.Report, preliminary)
			}
			for _, outcome := range []LookupOutcome{got.BeforeLookup, got.AfterLookup} {
				if outcome.Query != nil || outcome.Result != nil || outcome.Err != nil || outcome.SkipReason != SkipBindingRejected {
					t.Fatalf("skip outcome = %#v", outcome)
				}
			}
		})
	}
}

func TestBuildProviderAndIndexErrors(t *testing.T) {
	plan := testPlan()
	for _, index := range []int{-1, 1} {
		got, err := Build(plan, index, nil)
		if !errors.Is(err, changereport.ErrInvalidEntryIndex) || !reflect.DeepEqual(got, Result{}) {
			t.Fatalf("index %d: result=%#v error=%v", index, got, err)
		}
	}
	var typedNil *fakeLookup
	for _, provider := range []MetadataLookup{nil, typedNil} {
		got, err := Build(plan, 0, provider)
		if !errors.Is(err, ErrProviderRequired) || !reflect.DeepEqual(got, Result{}) {
			t.Fatalf("provider %#v: result=%#v error=%v", provider, got, err)
		}
	}
	if _, err := Build(plan, 0, valueLookup{}); err != nil {
		t.Fatalf("non-nil value receiver provider panicked or failed: %v", err)
	}
	unsupported := testPlan()
	unsupported.Entries[0].Operation = parser.OperationInstall
	if got, err := Build(unsupported, 0, nil); !errors.Is(err, ErrProviderRequired) || !reflect.DeepEqual(got, Result{}) {
		t.Fatalf("missing provider did not take priority for unsupported binding: result=%#v error=%v", got, err)
	}
}

func TestBuildPreservesPartialParserDiagnosticsAndInputs(t *testing.T) {
	plan := testPlan()
	plan.Status = parser.StatusPartial
	plan.Diagnostics = []parser.Diagnostic{{Code: "UNRECOGNIZED_LINE", Message: "synthetic", SourceLines: []int{9}}}
	plan.Entries[0].SourceLines = []int{3, 4}
	plan.Entries[0].FlagGroups = []parser.FlagGroup{{Name: "USE", Values: []parser.FlagValue{{Name: "feature", State: parser.FlagEnabled, Markers: []string{"*"}}}}}
	original := clonePlan(plan)
	before, after := *ptrSnapshot("1.0"), *ptrSnapshot("2.0")
	originalBefore, originalAfter := cloneSnapshot(before), cloneSnapshot(after)
	provider := &fakeLookup{answers: []lookupAnswer{foundAnswerValue(before), foundAnswerValue(after)}}
	got, err := Build(plan, 0, provider)
	if err != nil {
		t.Fatal(err)
	}
	if got.Report.ParserStatus != parser.StatusPartial || !reflect.DeepEqual(got.Report.ParserDiagnostics, plan.Diagnostics) || !reflect.DeepEqual(got.Report.SourceLines, []int{3, 4}) {
		t.Fatalf("Parser facts lost: %#v", got.Report)
	}
	if !reflect.DeepEqual(plan, original) {
		t.Fatalf("Build mutated input plan:\n got %#v\nwant %#v", plan, original)
	}
	if !reflect.DeepEqual(*provider.answers[0].result.Snapshot, originalBefore) || !reflect.DeepEqual(*provider.answers[1].result.Snapshot, originalAfter) {
		t.Fatal("Build mutated a Provider snapshot or one of its nested slices")
	}
}

func TestBuildRejectsForeignSnapshotsThroughChangeReport(t *testing.T) {
	foreignBefore := testSnapshot("9.0", "SLOT=1/1\nIUSE=old\n")
	foreignAfter := testSnapshot("2.0", "SLOT=2/2\nIUSE=new\n")
	provider := &fakeLookup{answers: []lookupAnswer{
		{result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: &foreignBefore}},
		{result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: &foreignAfter}},
	}}
	got, err := Build(testPlan(), 0, provider)
	if err != nil {
		t.Fatal(err)
	}
	if got.Report.Gentoo.Comparison != nil || !containsCode(got.Report.Gentoo.Diagnostics, changereport.CodeVersionMismatch) {
		t.Fatalf("foreign snapshot was not rejected by ChangeReport: %#v", got.Report.Gentoo)
	}
}

func TestBuildDeterministicForSameProviderAnswers(t *testing.T) {
	build := func() Result {
		before, after := testSnapshot("1.0", "SLOT=1/1\nIUSE=old\n"), testSnapshot("2.0", "SLOT=2/2\nIUSE=new\n")
		provider := &fakeLookup{answers: []lookupAnswer{foundAnswerValue(before), foundAnswerValue(after)}}
		result, err := Build(testPlan(), 0, provider)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first, second := build(), build()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated result differs:\nfirst %#v\nsecond %#v", first, second)
	}
}

func TestBuildWithRealMetadataProviderAndSyntheticGitRepository(t *testing.T) {
	root := t.TempDir()
	if err := runTestGit(root, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	writeCache := func(path, content string) {
		t.Helper()
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeCache("metadata/md5-cache/dev-libs/widget-1.0", "EAPI=8\nSLOT=1/1\nIUSE=old\n")
	writeCache("metadata/md5-cache/dev-libs/widget-2.0", "EAPI=8\nSLOT=2/2\nIUSE=new\n")
	if err := runTestGit(root, "add", "metadata"); err != nil {
		t.Fatal(err)
	}
	if err := runTestGit(root, "-c", "user.name=Synthetic Test", "-c", "user.email=synthetic@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "synthetic metadata"); err != nil {
		t.Fatal(err)
	}
	commitBytes, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(commitBytes))
	provider, err := metadataprovider.Open(metadataprovider.Config{Repository: "gentoo", SourceURL: "synthetic://local", LocalPath: root, Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Build(testPlan(), 0, provider)
	if err != nil {
		t.Fatal(err)
	}
	comparison := got.Report.Gentoo.Comparison
	if comparison == nil || got.Report.Gentoo.Status != metadatadiff.StatusAvailable {
		t.Fatalf("Gentoo report = %#v", got.Report.Gentoo)
	}
	if comparison.Before.Source.Commit != commit || comparison.After.Source.Commit != commit || comparison.Before.Source.Version != "1.0" || comparison.After.Source.Version != "2.0" {
		t.Fatalf("provenance = before %#v after %#v", comparison.Before.Source, comparison.After.Source)
	}
	if comparison.Changes.Slot == nil || comparison.Changes.Slot.Before != "1" || comparison.Changes.Slot.After != "2" || comparison.Changes.Subslot == nil || comparison.Changes.Subslot.Before != "1" || comparison.Changes.Subslot.After != "2" || len(comparison.Changes.IUSE.Added) != 1 || comparison.Changes.IUSE.Added[0].Name != "new" || len(comparison.Changes.IUSE.Removed) != 1 || comparison.Changes.IUSE.Removed[0].Name != "old" {
		t.Fatalf("synthetic metadata changes = %#v", comparison.Changes)
	}
	if !reflect.DeepEqual(comparison.Changes.Slot.Evidence.Before.Lines, []int{2}) || !reflect.DeepEqual(comparison.Changes.IUSE.Added[0].Evidence.After.Lines, []int{3}) {
		t.Fatalf("evidence = slot %#v IUSE %#v", comparison.Changes.Slot.Evidence, comparison.Changes.IUSE.Added[0].Evidence)
	}
}

func testPlan() parser.Result {
	installedRepo, targetRepo := "gentoo", "gentoo"
	return parser.Result{
		Status: parser.StatusParsed,
		Entries: []parser.Entry{{
			Package: parser.Package{Category: "dev-libs", Name: "widget"}, Operation: parser.OperationUpgrade,
			Target:      parser.Target{Version: "2.0", Repository: &targetRepo},
			Installed:   parser.InstalledContext{Versions: []parser.InstalledVersion{{Version: "1.0", Repository: &installedRepo}}},
			SourceLines: []int{1},
		}},
	}
}

func testSnapshot(version, body string) metadatadiff.Snapshot {
	return metadatadiff.Normalize(metadatadiff.Source{
		Category: "dev-libs", Name: "widget", Version: version, Repository: "gentoo",
		SourceURL: "synthetic://test", Commit: strings.Repeat("a", 40), CachePath: "metadata/md5-cache/dev-libs/widget-" + version,
	}, "EAPI=8\n"+body)
}

func ptrSnapshot(version string) *metadatadiff.Snapshot {
	snapshot := testSnapshot(version, "SLOT=1/1\nIUSE=old\n")
	return &snapshot
}

func foundAnswer(version string) lookupAnswer {
	return foundAnswerValue(*ptrSnapshot(version))
}

func foundAnswerValue(snapshot metadatadiff.Snapshot) lookupAnswer {
	return lookupAnswer{result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: &snapshot}}
}

func assertOutcome(t *testing.T, outcome LookupOutcome, wantErr error, wantFound bool) {
	t.Helper()
	if outcome.Query == nil || outcome.SkipReason != "" {
		t.Fatalf("executed outcome lacks query or has skip reason: %#v", outcome)
	}
	if wantErr != nil {
		if !errors.Is(outcome.Err, wantErr) || outcome.Result != nil {
			t.Fatalf("error outcome = %#v, want errors.Is(%v)", outcome, wantErr)
		}
		return
	}
	if outcome.Err != nil || outcome.Result == nil || (outcome.Result.Status == metadataprovider.StatusFound) != wantFound {
		t.Fatalf("successful outcome = %#v, want found=%t", outcome, wantFound)
	}
}

func containsCode(diagnostics []changereport.BindingDiagnostic, code changereport.BindingCode) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func clonePlan(plan parser.Result) parser.Result {
	cloned := plan
	cloned.Entries = append([]parser.Entry(nil), plan.Entries...)
	for i := range cloned.Entries {
		cloned.Entries[i].SourceLines = append([]int(nil), plan.Entries[i].SourceLines...)
		cloned.Entries[i].Installed.Versions = append([]parser.InstalledVersion(nil), plan.Entries[i].Installed.Versions...)
		cloned.Entries[i].FlagGroups = append([]parser.FlagGroup(nil), plan.Entries[i].FlagGroups...)
		for j := range cloned.Entries[i].FlagGroups {
			cloned.Entries[i].FlagGroups[j].Values = append([]parser.FlagValue(nil), plan.Entries[i].FlagGroups[j].Values...)
			for k := range cloned.Entries[i].FlagGroups[j].Values {
				cloned.Entries[i].FlagGroups[j].Values[k].Markers = append([]string(nil), plan.Entries[i].FlagGroups[j].Values[k].Markers...)
			}
		}
	}
	cloned.Diagnostics = append([]parser.Diagnostic(nil), plan.Diagnostics...)
	for i := range cloned.Diagnostics {
		cloned.Diagnostics[i].SourceLines = append([]int(nil), plan.Diagnostics[i].SourceLines...)
	}
	return cloned
}

func cloneSnapshot(snapshot metadatadiff.Snapshot) metadatadiff.Snapshot {
	cloned := snapshot
	cloned.EAPI.Lines = append([]int(nil), snapshot.EAPI.Lines...)
	cloned.Slot.Lines = append([]int(nil), snapshot.Slot.Lines...)
	cloned.IUSE.Lines = append([]int(nil), snapshot.IUSE.Lines...)
	cloned.IUSE.Flags = append([]metadatadiff.IUSEFlag(nil), snapshot.IUSE.Flags...)
	for i := range cloned.IUSE.Flags {
		cloned.IUSE.Flags[i].Declarations = append([]metadatadiff.IUSEDeclaration(nil), snapshot.IUSE.Flags[i].Declarations...)
	}
	cloned.Diagnostics = append([]metadatadiff.Diagnostic(nil), snapshot.Diagnostics...)
	for i := range cloned.Diagnostics {
		cloned.Diagnostics[i].Lines = append([]int(nil), snapshot.Diagnostics[i].Lines...)
	}
	return cloned
}

func runTestGit(root string, args ...string) error {
	commandArgs := append([]string{"-C", root}, args...)
	command := exec.Command("git", commandArgs...)
	command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Synthetic Test", "GIT_AUTHOR_EMAIL=synthetic@example.invalid", "GIT_COMMITTER_NAME=Synthetic Test", "GIT_COMMITTER_EMAIL=synthetic@example.invalid")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
