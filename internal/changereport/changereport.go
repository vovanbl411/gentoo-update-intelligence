// Package changereport binds one parsed plan entry to Gentoo metadata snapshots
// and retains the result of metadatadiff.Compare.
package changereport

import (
	"errors"
	"fmt"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
)

// ErrInvalidEntryIndex identifies an index outside plan.Entries.
var ErrInvalidEntryIndex = errors.New("change report entry index out of range")

// BindingCode identifies why snapshots could not be bound to a plan entry.
type BindingCode string

const (
	CodeOperationUnsupported      BindingCode = "OPERATION_UNSUPPORTED"
	CodeInstalledContextMissing   BindingCode = "INSTALLED_CONTEXT_MISSING"
	CodeInstalledContextAmbiguous BindingCode = "INSTALLED_CONTEXT_AMBIGUOUS"
	CodeSnapshotMissing           BindingCode = "SNAPSHOT_MISSING"
	CodeRepositoryUnknown         BindingCode = "REPOSITORY_UNKNOWN"
	CodePackageMismatch           BindingCode = "PACKAGE_MISMATCH"
	CodeVersionMismatch           BindingCode = "VERSION_MISMATCH"
	CodeRepositoryMismatch        BindingCode = "REPOSITORY_MISMATCH"
	CodeSlotMismatch              BindingCode = "SLOT_MISMATCH"
	CodeSubslotMismatch           BindingCode = "SUBSLOT_MISMATCH"
)

// BindingField identifies the plan or metadata field checked by a diagnostic.
type BindingField string

const (
	BindingFieldOperation  BindingField = "operation"
	BindingFieldInstalled  BindingField = "installed_context"
	BindingFieldSnapshot   BindingField = "snapshot"
	BindingFieldPackage    BindingField = "package"
	BindingFieldVersion    BindingField = "version"
	BindingFieldRepository BindingField = "repository"
	BindingFieldSlot       BindingField = "SLOT"
	BindingFieldSubslot    BindingField = "SUBSLOT"
)

// BindingDiagnostic describes a failed plan-to-snapshot check. Nil optional
// values mean that the corresponding fact does not apply; pointers preserve
// the distinction between an absent value and a known empty value.
type BindingDiagnostic struct {
	Code     BindingCode
	Side     *metadatadiff.Side
	Field    *BindingField
	Expected *string
	Actual   *string
}

// AuxiliarySection is a report section whose comparison is not implemented.
type AuxiliarySection struct {
	Status metadatadiff.Status
	Reason string
}

// Report contains the selected Parser entry and its independent analysis
// sections. Source line numbers retain the Parser's one-based numbering.
type Report struct {
	EntryIndex               int
	SourceLines              []int
	ParserStatus             parser.Status
	ParserDiagnostics        []parser.Diagnostic
	Gentoo                   GentooSection
	EffectiveConfig          AuxiliarySection
	Upstream                 AuxiliarySection
	InterpretationLimitation string
}

// GentooSection contains either the full metadata comparison or binding
// diagnostics explaining why the comparison could not be made.
type GentooSection struct {
	Status      metadatadiff.Status
	Comparison  *metadatadiff.Result
	Diagnostics []BindingDiagnostic
}

const (
	ReasonNotImplemented = "NOT_IMPLEMENTED"
	interpretationLimit  = "Сравниваются выбранные repository metadata; соответствие metadata установленной сборке не подтверждено."
)

// Build binds one entry from plan to the supplied metadata snapshots. A nil
// snapshot means that side is unavailable. The only returned error is
// ErrInvalidEntryIndex; other binding failures are retained in Report.
func Build(
	plan parser.Result,
	entryIndex int,
	before, after *metadatadiff.Snapshot,
) (Report, error) {
	if entryIndex < 0 || entryIndex >= len(plan.Entries) {
		return Report{}, fmt.Errorf("%w: %d (entries: %d)", ErrInvalidEntryIndex, entryIndex, len(plan.Entries))
	}

	entry := plan.Entries[entryIndex]
	report := Report{
		EntryIndex:               entryIndex,
		SourceLines:              append([]int(nil), entry.SourceLines...),
		ParserStatus:             plan.Status,
		ParserDiagnostics:        cloneParserDiagnostics(plan.Diagnostics),
		Gentoo:                   GentooSection{Status: metadatadiff.StatusUnavailable, Diagnostics: make([]BindingDiagnostic, 0)},
		EffectiveConfig:          AuxiliarySection{Status: metadatadiff.StatusUnavailable, Reason: ReasonNotImplemented},
		Upstream:                 AuxiliarySection{Status: metadatadiff.StatusUnavailable, Reason: ReasonNotImplemented},
		InterpretationLimitation: interpretationLimit,
	}

	if diagnostics := checkOperationAndInstalled(entry); len(diagnostics) != 0 {
		report.Gentoo.Diagnostics = diagnostics
		return report, nil
	}

	if diagnostics := checkSnapshotsAndPlanRepositories(entry, before, after); len(diagnostics) != 0 {
		report.Gentoo.Diagnostics = diagnostics
		return report, nil
	}

	installed := entry.Installed.Versions[0]
	if diagnostics := checkSnapshotIdentity(entry, installed, before, after); len(diagnostics) != 0 {
		report.Gentoo.Diagnostics = diagnostics
		return report, nil
	}

	if diagnostics := checkSlots(entry, installed, before, after); len(diagnostics) != 0 {
		report.Gentoo.Diagnostics = diagnostics
		return report, nil
	}

	comparison := metadatadiff.Compare(*before, *after)
	report.Gentoo.Status = comparison.Status
	report.Gentoo.Comparison = &comparison
	return report, nil
}

func checkOperationAndInstalled(entry parser.Entry) []BindingDiagnostic {
	diagnostics := make([]BindingDiagnostic, 0, 2)
	if entry.Operation != parser.OperationUpgrade && entry.Operation != parser.OperationDowngrade {
		diagnostics = append(diagnostics, BindingDiagnostic{
			Code:     CodeOperationUnsupported,
			Field:    field(BindingFieldOperation),
			Expected: value("upgrade or downgrade"),
			Actual:   value(string(entry.Operation)),
		})
	}
	switch len(entry.Installed.Versions) {
	case 0:
		diagnostics = append(diagnostics, BindingDiagnostic{
			Code:  CodeInstalledContextMissing,
			Field: field(BindingFieldInstalled),
		})
	case 1:
	default:
		diagnostics = append(diagnostics, BindingDiagnostic{
			Code:     CodeInstalledContextAmbiguous,
			Field:    field(BindingFieldInstalled),
			Expected: value("exactly one installed version"),
			Actual:   value(fmt.Sprintf("%d installed versions", len(entry.Installed.Versions))),
		})
	}
	return diagnostics
}

func checkSnapshotsAndPlanRepositories(entry parser.Entry, before, after *metadatadiff.Snapshot) []BindingDiagnostic {
	diagnostics := make([]BindingDiagnostic, 0, 4)
	if before == nil {
		diagnostics = append(diagnostics, BindingDiagnostic{Code: CodeSnapshotMissing, Side: side(metadatadiff.SideBefore), Field: field(BindingFieldSnapshot)})
	}
	if after == nil {
		diagnostics = append(diagnostics, BindingDiagnostic{Code: CodeSnapshotMissing, Side: side(metadatadiff.SideAfter), Field: field(BindingFieldSnapshot)})
	}
	if entry.Installed.Versions[0].Repository == nil || *entry.Installed.Versions[0].Repository == "" {
		diagnostics = append(diagnostics, BindingDiagnostic{Code: CodeRepositoryUnknown, Side: side(metadatadiff.SideBefore), Field: field(BindingFieldRepository)})
	}
	if entry.Target.Repository == nil || *entry.Target.Repository == "" {
		diagnostics = append(diagnostics, BindingDiagnostic{Code: CodeRepositoryUnknown, Side: side(metadatadiff.SideAfter), Field: field(BindingFieldRepository)})
	}
	return diagnostics
}

func checkSnapshotIdentity(entry parser.Entry, installed parser.InstalledVersion, before, after *metadatadiff.Snapshot) []BindingDiagnostic {
	diagnostics := make([]BindingDiagnostic, 0, 6)
	type sideSnapshot struct {
		side       metadatadiff.Side
		snapshot   *metadatadiff.Snapshot
		version    string
		repository string
	}
	sides := []sideSnapshot{
		{side: metadatadiff.SideBefore, snapshot: before, version: installed.Version, repository: *installed.Repository},
		{side: metadatadiff.SideAfter, snapshot: after, version: entry.Target.Version, repository: *entry.Target.Repository},
	}
	for _, current := range sides {
		source := current.snapshot.Source
		if source.Category != entry.Package.Category || source.Name != entry.Package.Name {
			diagnostics = append(diagnostics, mismatch(CodePackageMismatch, current.side, BindingFieldPackage,
				packageName(entry.Package), packageName(parser.Package{Category: source.Category, Name: source.Name})))
		}
		if source.Version != current.version {
			diagnostics = append(diagnostics, mismatch(CodeVersionMismatch, current.side, BindingFieldVersion, current.version, source.Version))
		}
		if source.Repository != current.repository {
			diagnostics = append(diagnostics, mismatch(CodeRepositoryMismatch, current.side, BindingFieldRepository, current.repository, source.Repository))
		}
	}
	return diagnostics
}

func checkSlots(entry parser.Entry, installed parser.InstalledVersion, before, after *metadatadiff.Snapshot) []BindingDiagnostic {
	diagnostics := make([]BindingDiagnostic, 0, 4)
	type sideSlot struct {
		side        metadatadiff.Side
		planSlot    *string
		planSubslot *string
		snapshot    *metadatadiff.Snapshot
	}
	sides := []sideSlot{
		{side: metadatadiff.SideBefore, planSlot: installed.Slot, planSubslot: installed.Subslot, snapshot: before},
		{side: metadatadiff.SideAfter, planSlot: entry.Target.Slot, planSubslot: entry.Target.Subslot, snapshot: after},
	}
	for _, current := range sides {
		if current.snapshot.Slot.State != metadatadiff.StateValid {
			continue
		}
		slot := current.snapshot.Slot
		if current.planSlot != nil && *current.planSlot != slot.Name {
			diagnostics = append(diagnostics, mismatch(CodeSlotMismatch, current.side, BindingFieldSlot, *current.planSlot, slot.Name))
		}
		if current.planSubslot != nil && *current.planSubslot != slot.Subslot {
			diagnostics = append(diagnostics, mismatch(CodeSubslotMismatch, current.side, BindingFieldSubslot, *current.planSubslot, slot.Subslot))
		}
	}
	return diagnostics
}

func mismatch(code BindingCode, diagnosticSide metadatadiff.Side, diagnosticField BindingField, expected, actual string) BindingDiagnostic {
	return BindingDiagnostic{
		Code:     code,
		Side:     side(diagnosticSide),
		Field:    field(diagnosticField),
		Expected: value(expected),
		Actual:   value(actual),
	}
}

func packageName(pkg parser.Package) string { return pkg.Category + "/" + pkg.Name }

func side(value metadatadiff.Side) *metadatadiff.Side { return &value }
func field(value BindingField) *BindingField          { return &value }
func value(value string) *string                      { return &value }

func cloneParserDiagnostics(diagnostics []parser.Diagnostic) []parser.Diagnostic {
	if diagnostics == nil {
		return nil
	}
	cloned := make([]parser.Diagnostic, len(diagnostics))
	for index, diagnostic := range diagnostics {
		cloned[index] = diagnostic
		cloned[index].SourceLines = append([]int(nil), diagnostic.SourceLines...)
	}
	return cloned
}
