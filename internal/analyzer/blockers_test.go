package analyzer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
)

func TestSyntheticBlockersHaveIndependentEvidenceWithoutEntries(t *testing.T) {
	plan := parser.Parse("[blocks b] cat/old (is hard blocking cat/new-1)\n[blocks B] <cat/other-2 (is soft blocking cat/new-1)\n[blocks B] cat/third")
	before := parser.Parse("[blocks b] cat/old (is hard blocking cat/new-1)\n[blocks B] <cat/other-2 (is soft blocking cat/new-1)\n[blocks B] cat/third")
	result := Analyze(plan, Context{})
	if result.Status != parser.StatusParsed || len(result.Findings) != 2 {
		t.Fatalf("result = %+v", result)
	}
	for index, f := range result.Findings {
		evidence, ok := f.Evidence.(UnresolvedBlockerEvidence)
		if !ok || evidence.BlockerIndex != index+1 || f.EntryIndex != nil || f.RuleID != RuleUnresolvedBlocker || f.Category != CategoryBlocker || !strings.Contains(f.Message, plan.Blockers[index+1].Atom) {
			t.Fatalf("finding = %+v, evidence = %#v", f, f.Evidence)
		}
	}
	if !reflect.DeepEqual(plan, before) || !reflect.DeepEqual(result, Analyze(plan, Context{})) {
		t.Fatal("mutation or nondeterministic result")
	}
	plan.Blockers[1].Parents[0] = "changed"
	if !reflect.DeepEqual(result, Analyze(before, Context{})) {
		t.Fatal("result depends on mutated input slices")
	}
	unknown := Analyze(parser.Result{Blockers: []parser.Blocker{{Resolution: "unknown"}}}, Context{})
	if len(unknown.Findings) != 0 {
		t.Fatal("unknown resolution treated as confirmed blocker")
	}
	first := Analyze(parser.Parse("[blocks B] cat/only"), Context{})
	ref := first.Findings[0].Evidence.(UnresolvedBlockerEvidence)
	if ref.BlockerIndex != 0 || first.Findings[0].EntryIndex != nil {
		t.Fatal("index zero confused with absent entry")
	}
}

func TestSyntheticMixedPartialPlanPreservesFindingsAndDiagnostics(t *testing.T) {
	plan := parser.Parse("[blocks B] cat/old\n[ebuild UD] cat/pkg-1 [2] USE=\"flag*\"\n[uninstall] cat/old-1\nunknown detail")
	result := Analyze(plan, Context{LocallyPatchedPackages: []parser.Package{{Category: "cat", Name: "pkg"}}})
	want := []RuleID{RuleDowngradePlanned, RuleLocalPatchedPackage, RuleUseChanged, RuleUnresolvedBlocker}
	if result.Status != parser.StatusPartial || !reflect.DeepEqual(result.Diagnostics, plan.Diagnostics) || len(result.Findings) != len(want) {
		t.Fatalf("result = %+v", result)
	}
	for i, rule := range want {
		if result.Findings[i].RuleID != rule {
			t.Fatalf("order = %+v", result.Findings)
		}
		if i < 3 && (result.Findings[i].EntryIndex == nil || *result.Findings[i].EntryIndex != 0) {
			t.Fatal("package entry reference lost")
		}
	}
	result.Diagnostics[0].SourceLines[0] = 999
	if plan.Diagnostics[0].SourceLines[0] == 999 {
		t.Fatal("diagnostic source slices alias input")
	}
}
