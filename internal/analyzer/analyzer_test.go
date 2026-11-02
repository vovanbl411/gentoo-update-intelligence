package analyzer

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
)

func TestAnalyzeRealParserFixtureWithoutContext(t *testing.T) {
	input, err := os.ReadFile("../../testdata/parser/ebuild-updates.txt")
	if err != nil {
		t.Fatal(err)
	}
	parsed := parser.Parse(string(input))
	if parsed.Status != parser.StatusParsed {
		t.Fatalf("fixture Parser status = %q, want %q", parsed.Status, parser.StatusParsed)
	}
	got := Analyze(parsed, Context{})

	if got.Status != parsed.Status || !reflect.DeepEqual(got.Diagnostics, parsed.Diagnostics) {
		t.Fatalf("Parser outcome not preserved: status=%q diagnostics=%+v", got.Status, got.Diagnostics)
	}
	if len(got.Findings) != 0 {
		t.Fatalf("fixture findings = %+v, want none", got.Findings)
	}
}

func TestLocalPatchedPackageMatchingAndEvidence(t *testing.T) {
	plan := parser.Result{Status: parser.StatusParsed, Entries: []parser.Entry{
		{Package: parser.Package{Category: "dev-libs", Name: "example"}},
		{Package: parser.Package{Category: "dev-libs", Name: "example"}},
		{Package: parser.Package{Category: "sys-kernel", Name: "gentoo-kernel"}},
	}}
	context := Context{LocallyPatchedPackages: []parser.Package{
		{Category: "dev-libs", Name: "other"},
		{Category: "dev-libs", Name: "example"},
		{Category: "dev-libs", Name: "example"},
		{Category: "other", Name: "example"},
	}}
	got := Analyze(plan, context)
	if len(got.Findings) != 2 {
		t.Fatalf("findings = %+v, want one for each exact matching entry", got.Findings)
	}
	for index, finding := range got.Findings {
		if finding.RuleID != RuleLocalPatchedPackage || finding.EntryIndex != index {
			t.Errorf("finding[%d] = %+v", index, finding)
		}
		evidence, ok := finding.Evidence.(LocalPatchedPackageEvidence)
		if !ok || evidence.ContextIndex != 1 {
			t.Errorf("finding[%d] evidence = %#v, want first match at context index 1", index, finding.Evidence)
		}
		if finding.Category != CategoryReview || finding.Message == "" {
			t.Errorf("finding[%d] category/message = %q / %q", index, finding.Category, finding.Message)
		}
	}
	if got.Findings[0].Message == "" || contains(got.Findings[0].Message, "слом") {
		t.Errorf("local patch message must ask for review without asserting failure: %q", got.Findings[0].Message)
	}
}

func TestUseChangedEvidenceAndExclusions(t *testing.T) {
	plan := parser.Result{Entries: []parser.Entry{{
		FlagGroups: []parser.FlagGroup{
			{Name: "PYTHON_TARGETS", Values: []parser.FlagValue{{RawToken: "python3_13*", Markers: []string{"*"}}}},
			{Name: "USE", Values: []parser.FlagValue{
				{RawToken: "foo*", Markers: []string{"*"}},
				{RawToken: "bar%", Markers: []string{"%"}},
				{RawToken: "both*%", Markers: []string{"*", "%"}},
				{RawToken: "(wrapped)", Markers: []string{"(", ")"}},
				{RawToken: "(-disabled)", Markers: []string{"(", ")", "-"}},
				{RawToken: "~unstable", Markers: []string{"~"}},
			}},
		},
	}}}
	got := Analyze(plan, Context{})
	if len(got.Findings) != 1 || got.Findings[0].RuleID != RuleUseChanged {
		t.Fatalf("findings = %+v, want one USE_CHANGED", got.Findings)
	}
	finding := got.Findings[0]
	if !contains(finding.Message, "foo*") || !contains(finding.Message, "bar%") || !contains(finding.Message, "both*%") || contains(finding.Message, "python3_13*") {
		t.Errorf("message does not list only changed USE tokens: %q", finding.Message)
	}
	evidence, ok := finding.Evidence.(UseChangedEvidence)
	if !ok {
		t.Fatalf("evidence type = %T, want UseChangedEvidence", finding.Evidence)
	}
	want := []UseTokenRef{{GroupIndex: 1, ValueIndex: 0}, {GroupIndex: 1, ValueIndex: 1}, {GroupIndex: 1, ValueIndex: 2}}
	if !reflect.DeepEqual(evidence.Tokens, want) {
		t.Errorf("token refs = %+v, want %+v", evidence.Tokens, want)
	}
}

func TestUseChangedDoesNotTriggerOnOtherMarkersOrGroups(t *testing.T) {
	plan := parser.Result{Entries: []parser.Entry{{UnstableMarker: true, FlagGroups: []parser.FlagGroup{
		{Name: "PYTHON_TARGETS", Values: []parser.FlagValue{{RawToken: "python*", Markers: []string{"*"}}}},
		{Name: "USE", Values: []parser.FlagValue{
			{RawToken: "(foo)", Markers: []string{"(", ")"}},
			{RawToken: "(-bar)", Markers: []string{"(", ")", "-"}},
			{RawToken: "~baz", Markers: []string{"~"}},
		}},
	}}}}
	if got := Analyze(plan, Context{}); len(got.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", got.Findings)
	}
}

func TestSlotChangedRequiresUnambiguousUpgradeAndDifferentKnownSlots(t *testing.T) {
	entry := func(operation parser.Operation, targetSlot *string, installedSlots ...*string) parser.Entry {
		versions := make([]parser.InstalledVersion, len(installedSlots))
		for index, slot := range installedSlots {
			versions[index].Slot = slot
		}
		return parser.Entry{
			Package:   parser.Package{Category: "cat", Name: "pkg"},
			Operation: operation,
			NewSlot:   operation == parser.OperationInstall,
			Target:    parser.Target{Slot: targetSlot},
			Installed: parser.InstalledContext{Versions: versions},
		}
	}
	oldSlot, newSlot, sameSlot, subslotOnly := "0", "1", "0", "0"
	oldSubslot, newSubslot := "1", "2"
	tests := []struct {
		name  string
		entry parser.Entry
		want  bool
	}{
		{"different explicit slots", entry(parser.OperationUpgrade, &newSlot, &oldSlot), true},
		{"same slots", entry(parser.OperationUpgrade, &sameSlot, &oldSlot), false},
		{"subslot-only change", withSubslots(entry(parser.OperationUpgrade, &subslotOnly, &oldSlot), &newSubslot, &oldSubslot), false},
		{"unknown target slot", entry(parser.OperationUpgrade, nil, &oldSlot), false},
		{"unknown installed slot", entry(parser.OperationUpgrade, &newSlot, nil), false},
		{"no installed versions", entry(parser.OperationUpgrade, &newSlot), false},
		{"multiple installed versions", entry(parser.OperationUpgrade, &newSlot, &oldSlot, &sameSlot), false},
		{"install into new slot", entry(parser.OperationInstall, &newSlot, &oldSlot), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Analyze(parser.Result{Entries: []parser.Entry{test.entry}}, Context{})
			if (len(got.Findings) == 1) != test.want {
				t.Fatalf("findings = %+v, want SLOT_CHANGED=%v", got.Findings, test.want)
			}
			if test.want {
				finding := got.Findings[0]
				if finding.RuleID != RuleSlotChanged || !contains(finding.Message, "0") || !contains(finding.Message, "1") {
					t.Errorf("finding = %+v", finding)
				}
				evidence, ok := finding.Evidence.(SlotChangedEvidence)
				if !ok || evidence.InstalledVersionIndex != 0 {
					t.Errorf("evidence = %#v, want sole installed version at index 0", finding.Evidence)
				}
			}
		})
	}
}

func TestAnalyzeOrderingPartialAndUnsupported(t *testing.T) {
	oldSlot, newSlot := "0", "1"
	plan := parser.Result{
		Status: parser.StatusPartial,
		Entries: []parser.Entry{{
			Package:    parser.Package{Category: "cat", Name: "pkg"},
			Operation:  parser.OperationUpgrade,
			Target:     parser.Target{Slot: &newSlot},
			Installed:  parser.InstalledContext{Versions: []parser.InstalledVersion{{Slot: &oldSlot}}},
			FlagGroups: []parser.FlagGroup{{Name: "USE", Values: []parser.FlagValue{{RawToken: "flag*", Markers: []string{"*"}}}}},
		}, {
			Package: parser.Package{Category: "cat", Name: "pkg"},
		}},
		Diagnostics: []parser.Diagnostic{{Code: "UNRECOGNIZED_LINE", Message: "partial input", SourceLines: []int{9}}},
	}
	context := Context{LocallyPatchedPackages: []parser.Package{{Category: "cat", Name: "pkg"}}}
	before := cloneResult(plan)
	contextBefore := Context{LocallyPatchedPackages: append([]parser.Package(nil), context.LocallyPatchedPackages...)}
	got := Analyze(plan, context)
	if got.Status != parser.StatusPartial || !reflect.DeepEqual(got.Diagnostics, plan.Diagnostics) {
		t.Fatalf("Parser outcome lost: %+v", got)
	}
	wantRules := []RuleID{RuleLocalPatchedPackage, RuleUseChanged, RuleSlotChanged, RuleLocalPatchedPackage}
	wantEntries := []int{0, 0, 0, 1}
	if len(got.Findings) != len(wantRules) {
		t.Fatalf("findings = %+v, want %d", got.Findings, len(wantRules))
	}
	for index, finding := range got.Findings {
		if finding.RuleID != wantRules[index] || finding.EntryIndex != wantEntries[index] {
			t.Errorf("finding[%d] = %+v, want rule %s entry %d", index, finding, wantRules[index], wantEntries[index])
		}
	}
	if !reflect.DeepEqual(plan, before) {
		t.Fatal("Analyze mutated its Parser input")
	}
	if !reflect.DeepEqual(context, contextBefore) {
		t.Fatal("Analyze mutated its Context input")
	}
	if again := Analyze(plan, context); !reflect.DeepEqual(got, again) {
		t.Fatalf("repeated result differs:\nfirst:  %+v\nsecond: %+v", got, again)
	}

	unsupported := parser.Result{Status: parser.StatusUnsupported, Diagnostics: []parser.Diagnostic{{Code: "EMPTY_INPUT"}}}
	unsupportedResult := Analyze(unsupported, Context{})
	if unsupportedResult.Status != parser.StatusUnsupported || !reflect.DeepEqual(unsupportedResult.Diagnostics, unsupported.Diagnostics) || len(unsupportedResult.Findings) != 0 {
		t.Fatalf("unsupported result = %+v", unsupportedResult)
	}
}

func TestAnalyzeCopiesDiagnostics(t *testing.T) {
	plan := parser.Result{Status: parser.StatusPartial, Diagnostics: []parser.Diagnostic{{Code: "X", SourceLines: []int{3}}}}
	got := Analyze(plan, Context{})
	got.Diagnostics[0].SourceLines[0] = 99
	if plan.Diagnostics[0].SourceLines[0] != 3 {
		t.Fatal("result diagnostics alias Parser input")
	}
}

func cloneResult(result parser.Result) parser.Result {
	clone := result
	clone.Diagnostics = cloneDiagnostics(result.Diagnostics)
	if result.Summary != nil {
		summary := *result.Summary
		clone.Summary = &summary
	}
	if result.Entries != nil {
		clone.Entries = make([]parser.Entry, len(result.Entries))
	}
	for index, entry := range result.Entries {
		clonedEntry := entry
		clonedEntry.Target.Repository = cloneString(entry.Target.Repository)
		clonedEntry.Target.Slot = cloneString(entry.Target.Slot)
		clonedEntry.Target.Subslot = cloneString(entry.Target.Subslot)
		clonedEntry.SourceLines = append([]int(nil), entry.SourceLines...)
		if entry.Download != nil {
			download := *entry.Download
			clonedEntry.Download = &download
		}
		if entry.Installed.Versions != nil {
			clonedEntry.Installed.Versions = make([]parser.InstalledVersion, len(entry.Installed.Versions))
		}
		for versionIndex, version := range entry.Installed.Versions {
			clonedEntry.Installed.Versions[versionIndex] = parser.InstalledVersion{
				Version:    version.Version,
				Repository: cloneString(version.Repository),
				Slot:       cloneString(version.Slot),
				Subslot:    cloneString(version.Subslot),
			}
		}
		if entry.FlagGroups != nil {
			clonedEntry.FlagGroups = make([]parser.FlagGroup, len(entry.FlagGroups))
		}
		for groupIndex, group := range entry.FlagGroups {
			clonedGroup := parser.FlagGroup{Name: group.Name}
			if group.Values != nil {
				clonedGroup.Values = make([]parser.FlagValue, len(group.Values))
			}
			for valueIndex, value := range group.Values {
				clonedGroup.Values[valueIndex] = value
				clonedGroup.Values[valueIndex].Markers = append([]string(nil), value.Markers...)
			}
			clonedEntry.FlagGroups[groupIndex] = clonedGroup
		}
		clone.Entries[index] = clonedEntry
	}
	return clone
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func contains(value, substring string) bool {
	return strings.Contains(value, substring)
}

func withSubslots(entry parser.Entry, target, installed *string) parser.Entry {
	entry.Target.Subslot = target
	entry.Installed.Versions[0].Subslot = installed
	return entry
}
