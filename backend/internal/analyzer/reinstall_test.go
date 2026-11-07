package analyzer

import (
	"reflect"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
)

func TestReinstallUsesOnlyShownFacts(t *testing.T) {
	for _, row := range []string{
		"[ebuild R] dev-test/pkg-1::gentoo",
		"[ebuild R ~] dev-test/pkg-1::gentoo LLVM_SLOT=\"22 (-18%)\"",
	} {
		got := Analyze(parser.Parse(row), Context{})
		if len(got.Findings) != 0 {
			t.Fatalf("rebuild alone generated findings: %+v", got)
		}
	}
	plan := parser.Parse("[ebuild R] dev-test/pkg-1::gentoo USE=\"foo* -bar% baz\"")
	got := Analyze(plan, Context{LocallyPatchedPackages: []parser.Package{{Category: "dev-test", Name: "pkg"}}})
	rules := []RuleID{}
	for _, f := range got.Findings {
		rules = append(rules, f.RuleID)
	}
	if !reflect.DeepEqual(rules, []RuleID{RuleLocalPatchedPackage, RuleUseChanged}) {
		t.Fatalf("%+v", got)
	}
	ev := got.Findings[1].Evidence.(UseChangedEvidence)
	if !reflect.DeepEqual(ev.Tokens, []UseTokenRef{{GroupIndex: 0, ValueIndex: 0}, {GroupIndex: 0, ValueIndex: 1}}) {
		t.Fatalf("%+v", ev)
	}
	if len(plan.Entries[0].Installed.Versions) != 0 {
		t.Fatal("invented installed context")
	}
}
