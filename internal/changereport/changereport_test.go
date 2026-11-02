package changereport

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
)

func syntheticPlan() parser.Result {
	return parser.Parse("[ebuild U  ] dev-libs/libxml2-2.14.6::gentoo [2.13.9::gentoo]\n")
}

func fixtureSnapshots(t *testing.T) (metadatadiff.Snapshot, metadatadiff.Snapshot) {
	t.Helper()
	beforeContent, err := os.ReadFile("../../testdata/metadata/libxml2-2.13.9.md5-cache")
	if err != nil {
		t.Fatal(err)
	}
	afterContent, err := os.ReadFile("../../testdata/metadata/libxml2-2.14.6.md5-cache")
	if err != nil {
		t.Fatal(err)
	}
	beforeSource, afterSource := fixtureSource(), fixtureSource()
	beforeSource.Version = "2.13.9"
	beforeSource.CachePath = "metadata/md5-cache/dev-libs/libxml2-2.13.9"
	afterSource.Version = "2.14.6"
	afterSource.CachePath = "metadata/md5-cache/dev-libs/libxml2-2.14.6"
	return metadatadiff.Normalize(beforeSource, string(beforeContent)), metadatadiff.Normalize(afterSource, string(afterContent))
}

func fixtureSource() metadatadiff.Source {
	return metadatadiff.Source{
		Category: "dev-libs", Name: "libxml2", Repository: "gentoo",
		SourceURL: "https://github.com/gentoo-mirror/gentoo",
		Commit:    "ac2e2890e7702bcfa0dab44338707240894de8c7",
	}
}

func TestBuildPositiveRealMetadataFixtures(t *testing.T) {
	plan := syntheticPlan()
	before, after := fixtureSnapshots(t)
	wantComparison := metadatadiff.Compare(before, after)

	report, err := Build(plan, 0, &before, &after)
	if err != nil {
		t.Fatal(err)
	}
	if report.EntryIndex != 0 || !reflect.DeepEqual(report.SourceLines, []int{1}) {
		t.Fatalf("entry link = index %d, lines %v", report.EntryIndex, report.SourceLines)
	}
	if report.ParserStatus != parser.StatusParsed || len(report.ParserDiagnostics) != 0 {
		t.Fatalf("parser outcome = %q, %#v", report.ParserStatus, report.ParserDiagnostics)
	}
	if report.Gentoo.Status != metadatadiff.StatusAvailable || report.Gentoo.Comparison == nil {
		t.Fatalf("Gentoo section = %#v", report.Gentoo)
	}
	if !reflect.DeepEqual(*report.Gentoo.Comparison, wantComparison) {
		t.Fatalf("comparison differs from Compare result:\n got %#v\nwant %#v", report.Gentoo.Comparison, wantComparison)
	}
	comparison := report.Gentoo.Comparison
	if comparison.Changes.Subslot == nil || comparison.Changes.Subslot.Before != "2" || comparison.Changes.Subslot.After != "16" {
		t.Fatalf("SUBSLOT change = %#v", comparison.Changes.Subslot)
	}
	if !reflect.DeepEqual(comparison.Changes.Subslot.Evidence.Before.Lines, []int{14}) || !reflect.DeepEqual(comparison.Changes.Subslot.Evidence.After.Lines, []int{14}) {
		t.Fatalf("SUBSLOT evidence = %#v", comparison.Changes.Subslot.Evidence)
	}
	if got := removedIUSE(comparison.Changes.IUSE.Removed); !reflect.DeepEqual(got, []string{"examples", "lzma"}) {
		t.Fatalf("removed IUSE = %v", got)
	}
	for _, change := range comparison.Changes.IUSE.Removed {
		if !reflect.DeepEqual(change.Evidence.Before.Lines, []int{8}) || !reflect.DeepEqual(change.Evidence.After.Lines, []int{8}) {
			t.Fatalf("IUSE evidence for %q = %#v", change.Name, change.Evidence)
		}
	}
	if report.EffectiveConfig.Status != metadatadiff.StatusUnavailable || report.EffectiveConfig.Reason != ReasonNotImplemented {
		t.Fatalf("effective config = %#v", report.EffectiveConfig)
	}
	if report.Upstream.Status != metadatadiff.StatusUnavailable || report.Upstream.Reason != ReasonNotImplemented {
		t.Fatalf("upstream = %#v", report.Upstream)
	}
	if report.InterpretationLimitation == "" {
		t.Fatal("missing repository interpretation limitation")
	}
}

func TestBuildRejectsSnapshotsForRealQEMUEntry(t *testing.T) {
	input, err := os.ReadFile("../../testdata/parser/ebuild-updates.txt")
	if err != nil {
		t.Fatal(err)
	}
	plan := parser.Parse(string(input))
	before, after := fixtureSnapshots(t)
	report, err := Build(plan, 2, &before, &after)
	if err != nil {
		t.Fatal(err)
	}
	if report.Gentoo.Status != metadatadiff.StatusUnavailable || report.Gentoo.Comparison != nil {
		t.Fatalf("Gentoo section = %#v", report.Gentoo)
	}
	if report.EntryIndex != 2 || !reflect.DeepEqual(report.SourceLines, []int{8}) {
		t.Fatalf("QEMU plan link = index %d, lines %v", report.EntryIndex, report.SourceLines)
	}
	if len(report.Gentoo.Diagnostics) != 4 {
		t.Fatalf("diagnostics = %#v", report.Gentoo.Diagnostics)
	}
	wantCodes := []BindingCode{CodePackageMismatch, CodeVersionMismatch, CodePackageMismatch, CodeVersionMismatch}
	wantSides := []metadatadiff.Side{metadatadiff.SideBefore, metadatadiff.SideBefore, metadatadiff.SideAfter, metadatadiff.SideAfter}
	for i, diagnostic := range report.Gentoo.Diagnostics {
		if diagnostic.Code != wantCodes[i] || diagnostic.Side == nil || *diagnostic.Side != wantSides[i] {
			t.Fatalf("diagnostic %d = %#v", i, diagnostic)
		}
	}
}

func TestBuildDoesNotReorderSnapshots(t *testing.T) {
	plan := syntheticPlan()
	before, after := fixtureSnapshots(t)
	report, err := Build(plan, 0, &after, &before)
	if err != nil {
		t.Fatal(err)
	}
	wantExpected := []string{"2.13.9", "2.14.6"}
	wantActual := []string{"2.14.6", "2.13.9"}
	if len(report.Gentoo.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v", report.Gentoo.Diagnostics)
	}
	for i, diagnostic := range report.Gentoo.Diagnostics {
		if diagnostic.Code != CodeVersionMismatch || diagnostic.Side == nil || *diagnostic.Side != []metadatadiff.Side{metadatadiff.SideBefore, metadatadiff.SideAfter}[i] {
			t.Fatalf("diagnostic %d = %#v", i, diagnostic)
		}
		if diagnostic.Expected == nil || *diagnostic.Expected != wantExpected[i] || diagnostic.Actual == nil || *diagnostic.Actual != wantActual[i] {
			t.Fatalf("diagnostic %d expected/actual = %#v/%#v", i, diagnostic.Expected, diagnostic.Actual)
		}
	}
}

func TestBuildBindingDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*parser.Entry, *metadatadiff.Snapshot, *metadatadiff.Snapshot)
		codes  []BindingCode
		sides  []metadatadiff.Side
	}{
		{name: "before version", mutate: func(_ *parser.Entry, before, _ *metadatadiff.Snapshot) { before.Source.Version = "2.13.9-r1" }, codes: []BindingCode{CodeVersionMismatch}, sides: []metadatadiff.Side{metadatadiff.SideBefore}},
		{name: "after version", mutate: func(_ *parser.Entry, _, after *metadatadiff.Snapshot) { after.Source.Version = "2.14.6-r1" }, codes: []BindingCode{CodeVersionMismatch}, sides: []metadatadiff.Side{metadatadiff.SideAfter}},
		{name: "before repository", mutate: func(plan *parser.Entry, _, _ *metadatadiff.Snapshot) {
			*plan.Installed.Versions[0].Repository = "other"
		}, codes: []BindingCode{CodeRepositoryMismatch}, sides: []metadatadiff.Side{metadatadiff.SideBefore}},
		{name: "after repository", mutate: func(plan *parser.Entry, _, _ *metadatadiff.Snapshot) { *plan.Target.Repository = "other" }, codes: []BindingCode{CodeRepositoryMismatch}, sides: []metadatadiff.Side{metadatadiff.SideAfter}},
		{name: "package", mutate: func(_ *parser.Entry, before, _ *metadatadiff.Snapshot) { before.Source.Name = "other" }, codes: []BindingCode{CodePackageMismatch}, sides: []metadatadiff.Side{metadatadiff.SideBefore}},
		{name: "slots", mutate: func(plan *parser.Entry, before, after *metadatadiff.Snapshot) {
			plan.Installed.Versions[0].Slot = stringPointer("9")
			plan.Installed.Versions[0].Subslot = stringPointer("9")
			plan.Target.Slot = stringPointer("9")
			plan.Target.Subslot = stringPointer("9")
		}, codes: []BindingCode{CodeSlotMismatch, CodeSubslotMismatch, CodeSlotMismatch, CodeSubslotMismatch}, sides: []metadatadiff.Side{metadatadiff.SideBefore, metadatadiff.SideBefore, metadatadiff.SideAfter, metadatadiff.SideAfter}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := syntheticPlan()
			before, after := fixtureSnapshots(t)
			test.mutate(&plan.Entries[0], &before, &after)
			report, err := Build(plan, 0, &before, &after)
			if err != nil {
				t.Fatal(err)
			}
			if report.Gentoo.Comparison != nil || report.Gentoo.Status != metadatadiff.StatusUnavailable {
				t.Fatalf("Gentoo section = %#v", report.Gentoo)
			}
			if len(report.Gentoo.Diagnostics) != len(test.codes) {
				t.Fatalf("diagnostics = %#v", report.Gentoo.Diagnostics)
			}
			for index, diagnostic := range report.Gentoo.Diagnostics {
				if diagnostic.Code != test.codes[index] || diagnostic.Side == nil || *diagnostic.Side != test.sides[index] {
					t.Fatalf("diagnostic %d = %#v", index, diagnostic)
				}
				if diagnostic.Expected == nil || diagnostic.Actual == nil || diagnostic.Field == nil {
					t.Fatalf("mismatch diagnostic lacks typed values: %#v", diagnostic)
				}
			}
		})
	}
}

func TestBuildRepositoryBindingRequiresKnownNonemptyPlanValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*parser.Entry)
		side   metadatadiff.Side
	}{
		{"before unknown", func(entry *parser.Entry) { entry.Installed.Versions[0].Repository = nil }, metadatadiff.SideBefore},
		{"before empty", func(entry *parser.Entry) { entry.Installed.Versions[0].Repository = stringPointer("") }, metadatadiff.SideBefore},
		{"after unknown", func(entry *parser.Entry) { entry.Target.Repository = nil }, metadatadiff.SideAfter},
		{"after empty", func(entry *parser.Entry) { entry.Target.Repository = stringPointer("") }, metadatadiff.SideAfter},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := syntheticPlan()
			test.mutate(&plan.Entries[0])
			before, after := fixtureSnapshots(t)
			report, err := Build(plan, 0, &before, &after)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Gentoo.Diagnostics) != 1 || report.Gentoo.Diagnostics[0].Code != CodeRepositoryUnknown {
				t.Fatalf("diagnostics = %#v", report.Gentoo.Diagnostics)
			}
			if report.Gentoo.Diagnostics[0].Side == nil || *report.Gentoo.Diagnostics[0].Side != test.side {
				t.Fatalf("side = %#v, want %q", report.Gentoo.Diagnostics[0].Side, test.side)
			}
		})
	}
}

func TestBuildMissingSnapshotsAndCollectsCurrentStageDiagnostics(t *testing.T) {
	plan := syntheticPlan()
	plan.Entries[0].Installed.Versions[0].Repository = nil
	report, err := Build(plan, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantCodes := []BindingCode{CodeSnapshotMissing, CodeSnapshotMissing, CodeRepositoryUnknown}
	if len(report.Gentoo.Diagnostics) != len(wantCodes) {
		t.Fatalf("diagnostics = %#v", report.Gentoo.Diagnostics)
	}
	for i, code := range wantCodes {
		if report.Gentoo.Diagnostics[i].Code != code {
			t.Fatalf("diagnostic %d = %#v, want code %s", i, report.Gentoo.Diagnostics[i], code)
		}
	}
}

func TestBuildReportsEachMissingSnapshotSide(t *testing.T) {
	plan := syntheticPlan()
	before, after := fixtureSnapshots(t)
	for _, test := range []struct {
		name   string
		before *metadatadiff.Snapshot
		after  *metadatadiff.Snapshot
		want   metadatadiff.Side
	}{
		{"before", nil, &after, metadatadiff.SideBefore},
		{"after", &before, nil, metadatadiff.SideAfter},
	} {
		t.Run(test.name, func(t *testing.T) {
			report, err := Build(plan, 0, test.before, test.after)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Gentoo.Diagnostics) != 1 || report.Gentoo.Diagnostics[0].Code != CodeSnapshotMissing || report.Gentoo.Diagnostics[0].Side == nil || *report.Gentoo.Diagnostics[0].Side != test.want {
				t.Fatalf("diagnostics = %#v", report.Gentoo.Diagnostics)
			}
		})
	}
}

func TestBuildUnsupportedOperationAndInstalledContext(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*parser.Entry)
		codes  []BindingCode
	}{
		{"install", func(entry *parser.Entry) { entry.Operation = parser.OperationInstall }, []BindingCode{CodeOperationUnsupported}},
		{"no installed version", func(entry *parser.Entry) { entry.Installed.Versions = nil }, []BindingCode{CodeInstalledContextMissing}},
		{"multiple installed versions", func(entry *parser.Entry) {
			entry.Installed.Versions = append(entry.Installed.Versions, entry.Installed.Versions[0])
		}, []BindingCode{CodeInstalledContextAmbiguous}},
		{"operation and context", func(entry *parser.Entry) { entry.Operation = parser.OperationInstall; entry.Installed.Versions = nil }, []BindingCode{CodeOperationUnsupported, CodeInstalledContextMissing}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := syntheticPlan()
			test.mutate(&plan.Entries[0])
			before, after := fixtureSnapshots(t)
			report, err := Build(plan, 0, &before, &after)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Gentoo.Diagnostics) != len(test.codes) {
				t.Fatalf("diagnostics = %#v", report.Gentoo.Diagnostics)
			}
			for i, code := range test.codes {
				if report.Gentoo.Diagnostics[i].Code != code {
					t.Fatalf("diagnostic %d = %#v, want %s", i, report.Gentoo.Diagnostics[i], code)
				}
			}
		})
	}
}

func TestBuildRejectsParsedNSInstallEntry(t *testing.T) {
	plan := parser.Parse("[ebuild NS  ] dev-libs/libxml2-2.14.6:2::gentoo [2.13.9:2::gentoo]\n")
	if len(plan.Entries) != 1 || plan.Entries[0].Operation != parser.OperationInstall {
		t.Fatalf("synthetic NS parse = %#v", plan)
	}
	before, after := fixtureSnapshots(t)
	report, err := Build(plan, 0, &before, &after)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Gentoo.Diagnostics) == 0 || report.Gentoo.Diagnostics[0].Code != CodeOperationUnsupported || report.Gentoo.Comparison != nil {
		t.Fatalf("Gentoo section = %#v", report.Gentoo)
	}
}

func TestBuildUnknownPlanSlotsDoNotRejectSnapshots(t *testing.T) {
	plan := syntheticPlan()
	before, after := fixtureSnapshots(t)
	report, err := Build(plan, 0, &before, &after)
	if err != nil {
		t.Fatal(err)
	}
	if report.Gentoo.Status != metadatadiff.StatusAvailable || report.Gentoo.Comparison == nil || len(report.Gentoo.Diagnostics) != 0 {
		t.Fatalf("Gentoo section = %#v", report.Gentoo)
	}
}

func TestBuildChecksExplicitNormalizedSlotsAndSubslots(t *testing.T) {
	plan := syntheticPlan()
	entry := &plan.Entries[0]
	entry.Installed.Versions[0].Slot = stringPointer("2")
	entry.Installed.Versions[0].Subslot = stringPointer("2")
	entry.Target.Slot = stringPointer("2")
	entry.Target.Subslot = stringPointer("16")
	before, after := fixtureSnapshots(t)
	report, err := Build(plan, 0, &before, &after)
	if err != nil {
		t.Fatal(err)
	}
	if report.Gentoo.Status != metadatadiff.StatusAvailable || len(report.Gentoo.Diagnostics) != 0 {
		t.Fatalf("explicit matching SLOT/SUBSLOT rejected: %#v", report.Gentoo)
	}

	entry.Target.Subslot = stringPointer("2")
	report, err = Build(plan, 0, &before, &after)
	if err != nil {
		t.Fatal(err)
	}
	if report.Gentoo.Comparison != nil || len(report.Gentoo.Diagnostics) != 1 || report.Gentoo.Diagnostics[0].Code != CodeSubslotMismatch {
		t.Fatalf("explicit SUBSLOT mismatch = %#v", report.Gentoo)
	}
}

func TestBuildDoesNotInferUnknownSubslotFromImplicitMetadata(t *testing.T) {
	plan := syntheticPlan()
	plan.Entries[0].Installed.Versions[0].Slot = stringPointer("2")
	before, after := fixtureSnapshots(t)
	report, err := Build(plan, 0, &before, &after)
	if err != nil {
		t.Fatal(err)
	}
	if report.Gentoo.Status != metadatadiff.StatusAvailable || len(report.Gentoo.Diagnostics) != 0 {
		t.Fatalf("unknown plan SUBSLOT was inferred: %#v", report.Gentoo)
	}
}

func TestBuildDoesNotAssertMismatchForInvalidOrMissingMetadataSlot(t *testing.T) {
	for _, state := range []metadatadiff.FieldState{metadatadiff.StateInvalid, metadatadiff.StateMissing} {
		t.Run(string(state), func(t *testing.T) {
			plan := syntheticPlan()
			plan.Entries[0].Installed.Versions[0].Slot = stringPointer("plan-value")
			before, after := fixtureSnapshots(t)
			before.Slot = metadatadiff.Slot{State: state}
			report, err := Build(plan, 0, &before, &after)
			if err != nil {
				t.Fatal(err)
			}
			if report.Gentoo.Comparison == nil || report.Gentoo.Status != metadatadiff.StatusPartial {
				t.Fatalf("Compare result not preserved: %#v", report.Gentoo)
			}
			if len(report.Gentoo.Diagnostics) != 0 {
				t.Fatalf("binding diagnostics = %#v", report.Gentoo.Diagnostics)
			}
		})
	}
}

func TestBindingDiagnosticDistinguishesKnownEmptyFromMissingValues(t *testing.T) {
	plan := syntheticPlan()
	plan.Entries[0].Target.Slot = stringPointer("")
	before, after := fixtureSnapshots(t)
	report, err := Build(plan, 0, &before, &after)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Gentoo.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v", report.Gentoo.Diagnostics)
	}
	diagnostic := report.Gentoo.Diagnostics[0]
	if diagnostic.Side == nil || diagnostic.Field == nil || diagnostic.Expected == nil || diagnostic.Actual == nil {
		t.Fatalf("diagnostic omitted applicable values: %#v", diagnostic)
	}
	if *diagnostic.Expected != "" || *diagnostic.Actual != "2" {
		t.Fatalf("expected/actual = %q/%q", *diagnostic.Expected, *diagnostic.Actual)
	}
	if diagnostic.Code != CodeSlotMismatch || *diagnostic.Side != metadatadiff.SideAfter {
		t.Fatalf("diagnostic identity = %#v", diagnostic)
	}
}

func TestBuildPreservesPartialAndUnavailableCompareResults(t *testing.T) {
	t.Run("partial missing IUSE", func(t *testing.T) {
		plan := syntheticPlan()
		before, after := fixtureSnapshots(t)
		before.IUSE = metadatadiff.IUSE{State: metadatadiff.StateMissing}
		comparison := metadatadiff.Compare(before, after)
		report, err := Build(plan, 0, &before, &after)
		if err != nil {
			t.Fatal(err)
		}
		if comparison.Status != metadatadiff.StatusPartial || report.Gentoo.Status != comparison.Status || !reflect.DeepEqual(report.Gentoo.Comparison, &comparison) {
			t.Fatalf("partial comparison not preserved: %#v", report.Gentoo)
		}
	})
	t.Run("unavailable unsupported EAPI", func(t *testing.T) {
		plan := syntheticPlan()
		before, after := fixtureSnapshots(t)
		before.EAPI = metadatadiff.EAPI{State: metadatadiff.StateValid, Value: "7", Supported: false, Lines: []int{1}}
		comparison := metadatadiff.Compare(before, after)
		report, err := Build(plan, 0, &before, &after)
		if err != nil {
			t.Fatal(err)
		}
		if comparison.Status != metadatadiff.StatusUnavailable || report.Gentoo.Status != comparison.Status || !reflect.DeepEqual(report.Gentoo.Comparison, &comparison) {
			t.Fatalf("unavailable comparison not preserved: %#v", report.Gentoo)
		}
	})
}

func TestBuildPreservesPartialParserOutcomeAndDiagnostics(t *testing.T) {
	plan := syntheticPlan()
	plan.Status = parser.StatusPartial
	plan.Diagnostics = []parser.Diagnostic{{Code: "UNRECOGNIZED_LINE", Message: "partial input", SourceLines: []int{2, 4}}}
	before, after := fixtureSnapshots(t)
	report, err := Build(plan, 0, &before, &after)
	if err != nil {
		t.Fatal(err)
	}
	if report.ParserStatus != parser.StatusPartial || !reflect.DeepEqual(report.ParserDiagnostics, plan.Diagnostics) {
		t.Fatalf("Parser outcome = %q, %#v", report.ParserStatus, report.ParserDiagnostics)
	}
	if report.Gentoo.Status != metadatadiff.StatusAvailable {
		t.Fatalf("Gentoo status = %q", report.Gentoo.Status)
	}
	report.ParserDiagnostics[0].SourceLines[0] = 99
	if plan.Diagnostics[0].SourceLines[0] != 2 {
		t.Fatal("report diagnostics share nested source-line storage with Parser input")
	}
}

func TestBuildInvalidIndexesReturnRecognizableErrorAndZeroReport(t *testing.T) {
	for _, test := range []struct {
		name  string
		plan  parser.Result
		index int
	}{
		{"negative", syntheticPlan(), -1},
		{"equal length", syntheticPlan(), 1},
		{"empty entries", parser.Result{Status: parser.StatusUnsupported}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			report, err := Build(test.plan, test.index, nil, nil)
			if !errors.Is(err, ErrInvalidEntryIndex) {
				t.Fatalf("error = %v, want ErrInvalidEntryIndex", err)
			}
			if !reflect.DeepEqual(report, Report{}) {
				t.Fatalf("invalid index report = %#v, want zero value", report)
			}
		})
	}
}

func TestBuildDoesNotMutateInputsAndIsDeterministic(t *testing.T) {
	plan := syntheticPlan()
	plan.Status = parser.StatusPartial
	plan.Diagnostics = []parser.Diagnostic{{Code: "SYNTHETIC", SourceLines: []int{3, 4}}}
	plan.Entries[0].FlagGroups = []parser.FlagGroup{{Name: "USE", Values: []parser.FlagValue{{Name: "foo", RawToken: "foo", Markers: []string{"*"}}}}}
	before, after := fixtureSnapshots(t)
	before.Slot.Lines = append(before.Slot.Lines, 20)
	before.IUSE.Flags[0].Declarations = append(before.IUSE.Flags[0].Declarations, metadatadiff.IUSEDeclaration{Token: "synthetic", Line: 21})
	before.Diagnostics = append(before.Diagnostics, metadatadiff.Diagnostic{Code: "SYNTHETIC", Lines: []int{22}})

	planBefore := cloneParserResultForTest(plan)
	beforeCopy, afterCopy := cloneSnapshotForTest(before), cloneSnapshotForTest(after)
	first, err := Build(plan, 0, &before, &after)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(plan, 0, &before, &after)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan, planBefore) || !reflect.DeepEqual(before, beforeCopy) || !reflect.DeepEqual(after, afterCopy) {
		t.Fatal("Build mutated an input, including nested slices")
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Build is not deterministic:\nfirst %#v\nsecond %#v", first, second)
	}
}

func removedIUSE(flags []metadatadiff.IUSEFlagChange) []string {
	names := make([]string, len(flags))
	for i, flag := range flags {
		names[i] = flag.Name
	}
	return names
}

func stringPointer(value string) *string { return &value }

func cloneParserResultForTest(result parser.Result) parser.Result {
	cloned := result
	cloned.Diagnostics = append([]parser.Diagnostic(nil), result.Diagnostics...)
	for i := range cloned.Diagnostics {
		cloned.Diagnostics[i].SourceLines = append([]int(nil), result.Diagnostics[i].SourceLines...)
	}
	cloned.Entries = append([]parser.Entry(nil), result.Entries...)
	for i := range cloned.Entries {
		entry := &cloned.Entries[i]
		original := result.Entries[i]
		entry.SourceLines = append([]int(nil), original.SourceLines...)
		entry.Installed.Versions = append([]parser.InstalledVersion(nil), original.Installed.Versions...)
		for j := range entry.Installed.Versions {
			entry.Installed.Versions[j].Repository = cloneStringPointer(original.Installed.Versions[j].Repository)
			entry.Installed.Versions[j].Slot = cloneStringPointer(original.Installed.Versions[j].Slot)
			entry.Installed.Versions[j].Subslot = cloneStringPointer(original.Installed.Versions[j].Subslot)
		}
		entry.Target.Repository = cloneStringPointer(original.Target.Repository)
		entry.Target.Slot = cloneStringPointer(original.Target.Slot)
		entry.Target.Subslot = cloneStringPointer(original.Target.Subslot)
		entry.FlagGroups = append([]parser.FlagGroup(nil), original.FlagGroups...)
		for j := range entry.FlagGroups {
			entry.FlagGroups[j].Values = append([]parser.FlagValue(nil), original.FlagGroups[j].Values...)
			for k := range entry.FlagGroups[j].Values {
				entry.FlagGroups[j].Values[k].Markers = append([]string(nil), original.FlagGroups[j].Values[k].Markers...)
			}
		}
	}
	if result.Summary != nil {
		summary := *result.Summary
		cloned.Summary = &summary
	}
	return cloned
}

func cloneSnapshotForTest(snapshot metadatadiff.Snapshot) metadatadiff.Snapshot {
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

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
