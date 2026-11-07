package reportservice

import (
	"errors"
	"reflect"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/changereport"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
)

func TestReinstallTargetOnlyLookup(t *testing.T) {
	plan := parser.Parse("[ebuild R ~] dev-libs/widget-2.0:2/2::gentoo USE=\"foo*\" 0 KiB")
	for _, tc := range []struct {
		name   string
		answer lookupAnswer
		code   changereport.BindingCode
	}{
		{"found", foundAnswerValue(testSnapshot("2.0", "SLOT=2/2\nIUSE=foo\n")), ""},
		{"not found", lookupAnswer{result: metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}}, changereport.CodeSnapshotMissing},
		{"repository unavailable", lookupAnswer{result: metadataprovider.LookupResult{Status: metadataprovider.StatusRepositoryUnavailable}}, changereport.CodeSnapshotMissing},
		{"provider error", lookupAnswer{err: metadataprovider.ErrRead}, changereport.CodeSnapshotMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &fakeLookup{answers: []lookupAnswer{tc.answer}}
			got, err := Build(plan, 0, provider)
			if err != nil {
				t.Fatal(err)
			}
			queries, err := LookupQueries(plan, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(queries) != 1 || len(provider.calls) != 1 || provider.calls[0].query != queries[0] || queries[0].Version != "2.0" {
				t.Fatalf("queries=%+v calls=%+v", queries, provider.calls)
			}
			if got.BeforeLookup.Query != nil || got.BeforeLookup.Result != nil || got.BeforeLookup.SkipReason != SkipBindingRejected || got.AfterLookup.Query == nil {
				t.Fatalf("invented before: %+v", got)
			}
			if got.Report.Gentoo.Comparison != nil || got.Report.Gentoo.Status != metadatadiff.StatusUnavailable {
				t.Fatalf("%+v", got.Report.Gentoo)
			}
			if !errors.Is(got.AfterLookup.Err, tc.answer.err) {
				t.Fatalf("error lost: %v", got.AfterLookup.Err)
			}
			codes := []changereport.BindingCode{}
			for _, d := range got.Report.Gentoo.Diagnostics {
				codes = append(codes, d.Code)
			}
			want := []changereport.BindingCode{changereport.CodeInstalledContextMissing}
			if tc.code != "" {
				want = append(want, tc.code)
			}
			if !reflect.DeepEqual(codes, want) {
				t.Fatalf("codes=%v want=%v", codes, want)
			}
		})
	}
}

func TestReinstallTargetIdentityValidation(t *testing.T) {
	plan := parser.Parse("[ebuild R] dev-libs/widget-2.0:2/2::gentoo 0 KiB")
	for _, tc := range []struct {
		name string
		edit func(*metadatadiff.Snapshot)
		code changereport.BindingCode
	}{
		{"category", func(s *metadatadiff.Snapshot) { s.Source.Category = "other" }, changereport.CodePackageMismatch},
		{"name", func(s *metadatadiff.Snapshot) { s.Source.Name = "other" }, changereport.CodePackageMismatch},
		{"version", func(s *metadatadiff.Snapshot) { s.Source.Version = "1.0" }, changereport.CodeVersionMismatch},
		{"repository", func(s *metadatadiff.Snapshot) { s.Source.Repository = "other" }, changereport.CodeRepositoryMismatch},
		{"slot", func(s *metadatadiff.Snapshot) { s.Slot.Name = "1" }, changereport.CodeSlotMismatch},
		{"subslot", func(s *metadatadiff.Snapshot) { s.Slot.Subslot = "1" }, changereport.CodeSubslotMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := testSnapshot("2.0", "SLOT=2/2\nIUSE=foo\n")
			tc.edit(&snapshot)
			p := &fakeLookup{answers: []lookupAnswer{{result: metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: &snapshot}}}}
			got, err := Build(plan, 0, p)
			if err != nil {
				t.Fatal(err)
			}
			diagnostics := got.Report.Gentoo.Diagnostics
			if len(diagnostics) != 2 || diagnostics[1].Code != tc.code || diagnostics[1].Side == nil || *diagnostics[1].Side != metadatadiff.SideAfter || got.Report.Gentoo.Comparison != nil {
				t.Fatalf("%+v", got.Report.Gentoo)
			}
		})
	}
}

func TestReinstallExplicitAndRejectedBindings(t *testing.T) {
	for _, tc := range []struct {
		name, row string
		queries   int
		code      changereport.BindingCode
	}{
		{"explicit", "[ebuild R] dev-libs/widget-2.0::gentoo [1.0::gentoo]", 2, ""},
		{"ambiguous", "[ebuild R] dev-libs/widget-2.0::gentoo [1.0::gentoo] [1.1::gentoo]", 0, changereport.CodeInstalledContextAmbiguous},
		{"unknown target repository", "[ebuild R] dev-libs/widget-2.0", 0, changereport.CodeRepositoryUnknown},
		{"empty target repository", "[ebuild R] dev-libs/widget-2.0::", 0, changereport.CodeRepositoryUnknown},
		{"unknown installed repository", "[ebuild R] dev-libs/widget-2.0::gentoo [1.0]", 0, changereport.CodeRepositoryUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := parser.Parse(tc.row)
			p := &fakeLookup{answers: []lookupAnswer{foundAnswer("1.0"), foundAnswer("2.0")}}
			got, err := Build(plan, 0, p)
			if err != nil {
				t.Fatal(err)
			}
			queries, err := LookupQueries(plan, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(queries) != tc.queries || len(p.calls) != tc.queries {
				t.Fatalf("%v %v", queries, p.calls)
			}
			if tc.queries == 2 {
				if got.Report.Gentoo.Comparison == nil || got.BeforeLookup.Query.Version != "1.0" || got.AfterLookup.Query.Version != "2.0" {
					t.Fatalf("%+v", got)
				}
			} else {
				found := false
				for _, d := range got.Report.Gentoo.Diagnostics {
					found = found || d.Code == tc.code
				}
				if !found || got.Report.Gentoo.Comparison != nil {
					t.Fatalf("%+v", got)
				}
			}
		})
	}
}

func TestTargetOnlyLookupRejectsUnknownDiagnostics(t *testing.T) {
	allowed := []changereport.BindingDiagnostic{{Code: changereport.CodeInstalledContextMissing}, {Code: changereport.CodeSnapshotMissing}}
	if !lookupAllowed(allowed, true) || lookupAllowed(allowed, false) {
		t.Fatal("wrong target-only permission")
	}
	allowed = append(allowed, changereport.BindingDiagnostic{Code: "FUTURE_CODE"})
	if lookupAllowed(allowed, true) {
		t.Fatal("unknown binding diagnostic allowed a lookup")
	}
}
