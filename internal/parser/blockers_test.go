package parser

import (
	"reflect"
	"testing"
)

// All plans in this file are synthetic, based on the pinned Portage formatter.
func TestSyntheticBlockerResolutionAndStrengthAreIndependent(t *testing.T) {
	for _, marker := range []string{"b", "B"} {
		for _, strength := range []string{"soft", "hard"} {
			t.Run(marker+"/"+strength, func(t *testing.T) {
				raw := "  [blocks " + marker + "     ] <dev-libs/old-2:0 (\"<dev-libs/old-2:0\" is " + strength + " blocking app-misc/new-3-r1, app-misc/other-1)"
				got := Parse(raw)
				if got.Status != StatusParsed || len(got.Blockers) != 1 || len(got.Entries) != 0 || len(got.Diagnostics) != 0 {
					t.Fatalf("parse = %+v", got)
				}
				b := got.Blockers[0]
				wantResolution := BlockerUnresolved
				if marker == "b" {
					wantResolution = BlockerResolved
				}
				if b.Atom != "<dev-libs/old-2:0" || b.ConstraintAtom == nil || *b.ConstraintAtom != b.Atom || b.Marker != marker || b.Resolution != wantResolution || b.Strength != BlockerStrength(strength) || b.RawLine != raw || !reflect.DeepEqual(b.SourceLines, []int{1}) || !reflect.DeepEqual(b.Parents, []string{"app-misc/new-3-r1", "app-misc/other-1"}) {
					t.Fatalf("blocker facts = %+v", b)
				}
			})
		}
	}
	got := Parse("[blocks b ] cat/old (is hard blocking cat/new-1)").Blockers[0]
	if got.ConstraintAtom != nil || got.Atom != "cat/old" || got.Strength != BlockerHard {
		t.Fatalf("invented constraint: %+v", got)
	}
	for _, marker := range []string{"b", "B"} {
		b := Parse("[blocks " + marker + "] cat/old").Blockers[0]
		if b.Strength != BlockerStrengthUnknown || b.Parents != nil || b.ConstraintAtom != nil {
			t.Fatalf("invented explanation: %+v", b)
		}
	}
}

func TestSyntheticRemovalsAndCounts(t *testing.T) {
	removal := `[uninstall     ] dev-libs/old-1.2-r3:0/4::gentoo USE="foo -bar"`
	got := Parse("[ebuild UD] cat/pkg-1::repo [2::repo]\n" + removal + "\n[blocks b] dev-libs/old (is soft blocking cat/pkg-1)\nTotal: 1 package (1 downgrade, 1 uninstall), Size of downloads: 0 KiB\nConflict: 1 block (all satisfied)")
	if got.Status != StatusParsed || len(got.Entries) != 1 || len(got.Removals) != 1 || got.Summary == nil || got.Summary.Packages != 1 || got.Summary.Uninstalls != 1 || got.ConflictSummary == nil || got.ConflictSummary.Unresolved != 0 {
		t.Fatalf("parse/counts = %+v", got)
	}
	r := got.Removals[0]
	if r.Package != (Package{Category: "dev-libs", Name: "old"}) || r.Target.Version != "1.2-r3" || r.Target.Repository == nil || *r.Target.Repository != "gentoo" || r.Target.Slot == nil || *r.Target.Slot != "0" || r.Target.Subslot == nil || *r.Target.Subslot != "4" || r.RawLine != removal || !reflect.DeepEqual(r.SourceLines, []int{2}) || !reflect.DeepEqual(groupTokens(r.FlagGroups), []string{"USE:foo -bar"}) {
		t.Fatalf("removal facts = %+v", r)
	}
	only := Parse("[uninstall] cat/pkg-1")
	if only.Status != StatusParsed || len(only.Entries) != 0 || len(only.Removals) != 1 || only.Removals[0].Target.Repository != nil || only.Removals[0].Target.Slot != nil {
		t.Fatalf("removal-only facts = %+v", only)
	}
	plural := Parse("[uninstall] cat/old-1\n[uninstall] cat/other-1\n[ebuild U] cat/pkg-2 [1]\nTotal: 1 package (1 upgrade, 2 uninstalls), Size of downloads: 0 KiB")
	if plural.Status != StatusParsed || plural.Summary.Uninstalls != 2 {
		t.Fatalf("plural = %+v", plural)
	}
	badTotal := "[ebuild U] cat/pkg-2 [1]\n" + removal + "\nTotal: 2 packages (1 upgrade, 1 uninstall), Size of downloads: 0 KiB"
	if got := Parse(badTotal); got.Status != StatusPartial || !hasDiagnostic(got.Diagnostics, "SUMMARY_MISMATCH") {
		t.Fatalf("removal incorrectly counted in Total: %+v", got)
	}
	missingRemoval := Parse("[ebuild U] cat/pkg-2 [1]\nTotal: 1 package (1 upgrade, 1 uninstall), Size of downloads: 0 KiB")
	if !hasDiagnostic(missingRemoval.Diagnostics, "SUMMARY_MISMATCH") {
		t.Fatalf("missing removal not detected: %+v", missingRemoval)
	}
}

func TestSyntheticConflictSummaryAndMismatch(t *testing.T) {
	rows := "[blocks b] cat/old\n[blocks B] cat/other\n"
	got := Parse(rows + "Conflict: 2 blocks (1 unsatisfied)")
	if got.Status != StatusParsed || got.ConflictSummary == nil || *got.ConflictSummary != (ConflictSummary{Blockers: 2, Unresolved: 1, SourceLine: 3}) {
		t.Fatalf("summary = %+v", got)
	}
	for _, summary := range []string{"Conflict: 3 blocks (1 unsatisfied)", "Conflict: 2 blocks (all satisfied)"} {
		got := Parse(rows + summary)
		if got.Status != StatusPartial || !hasDiagnostic(got.Diagnostics, "CONFLICT_SUMMARY_MISMATCH") {
			t.Fatalf("mismatch missed: %+v", got)
		}
	}
	got = Parse("Conflict: 1 block (1 unsatisfied)")
	if got.Status != StatusUnsupported || len(got.Blockers) != 0 || !hasDiagnostic(got.Diagnostics, "CONFLICT_SUMMARY_MISMATCH") {
		t.Fatalf("summary invented blocker: %+v", got)
	}
	got = Parse(rows + "Conflict: 2 blocks (1 unsatisfied)\nConflict: 2 blocks (1 unsatisfied)")
	if !hasDiagnostic(got.Diagnostics, "MALFORMED_CONFLICT_SUMMARY") {
		t.Fatalf("duplicate summary = %+v", got)
	}
}

func TestSyntheticMalformedBlockerAndRemovalStayVisible(t *testing.T) {
	tests := []struct{ raw, code string }{
		{"[blocks x] cat/pkg", "MALFORMED_BLOCKER"},
		{"[blocks B cat/pkg", "MALFORMED_BLOCKER"},
		{"[blocks B]", "MALFORMED_BLOCKER"},
		{"[blocks b] cat/pkg ()", "MALFORMED_BLOCKER"},
		{"[blocks b] cat/pkg (is unknown blocking cat/other-1)", "MALFORMED_BLOCKER"},
		{"[blocks b] cat/pkg (is hard blocking cat/other-1) unexpected", "MALFORMED_BLOCKER"},
		{"[blocks b] cat/pkg (is soft blocking cat/other-1, )", "MALFORMED_BLOCKER"},
		{"[uninstall] cat/pkg", "MALFORMED_REMOVAL"},
		{"[uninstall] cat/pkg-1 [2]", "MALFORMED_REMOVAL"},
		{"[uninstall] cat/pkg-1 garbage", "MALFORMED_REMOVAL"},
		{"Conflict: 1 block (2 unsatisfied)", "MALFORMED_CONFLICT_SUMMARY"},
	}
	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			got := Parse("[ebuild U] cat/pkg-2 [1]\n" + test.raw)
			if got.Status != StatusPartial || !hasDiagnostic(got.Diagnostics, test.code) || len(got.Blockers) != 0 || len(got.Removals) != 0 {
				t.Fatalf("malformed input hidden: %+v", got)
			}
			if !reflect.DeepEqual(got.Diagnostics[0].SourceLines, []int{2}) {
				t.Fatalf("source = %+v", got.Diagnostics)
			}
		})
	}
	skipped := Parse("[ebuild U] cat/pkg-2 [1]\nWARNING: One or more updates/rebuilds have been skipped due to a dependency conflict:")
	if skipped.Status != StatusPartial || !hasDiagnostic(skipped.Diagnostics, "UNRECOGNIZED_LINE") {
		t.Fatalf("skipped-update scope widened: %+v", skipped)
	}
}
