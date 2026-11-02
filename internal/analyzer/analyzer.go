// Package analyzer applies deterministic review rules to parsed Portage plans.
package analyzer

import (
	"strings"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
)

// RuleID identifies one Analyzer v0.1 rule.
type RuleID string

const (
	RuleUnresolvedBlocker   RuleID = "UNRESOLVED_BLOCKER"
	RuleDowngradePlanned    RuleID = "DOWNGRADE_PLANNED"
	RuleLocalPatchedPackage RuleID = "LOCAL_PATCHED_PACKAGE"
	RuleUseChanged          RuleID = "USE_CHANGED"
	RuleSlotChanged         RuleID = "SLOT_CHANGED"
)

// Category is the attention category assigned to a finding.
type Category string

const (
	CategoryReview  Category = "REVIEW"
	CategoryBlocker Category = "BLOCKER"
)

// Context contains exact package names that the user says have local patches.
type Context struct {
	LocallyPatchedPackages []parser.Package
}

// Result preserves the Parser outcome and adds ordered review findings.
type Result struct {
	Status      parser.Status
	Diagnostics []parser.Diagnostic
	Findings    []Finding
}

// Finding refers to a package entry or, through evidence, a blocker.
type Finding struct {
	RuleID     RuleID
	Category   Category
	Message    string
	EntryIndex *int
	Evidence   Evidence
}

// Evidence is one of the typed, rule-specific evidence values below.
type Evidence interface {
	evidenceKind()
}

// LocalPatchedPackageEvidence points to the first matching user context item.
type LocalPatchedPackageEvidence struct {
	ContextIndex int
}

func (LocalPatchedPackageEvidence) evidenceKind() {}

// UseTokenRef points to one value in one entry flag group.
type UseTokenRef struct {
	GroupIndex int
	ValueIndex int
}

// UseChangedEvidence lists affected USE tokens in source order.
type UseChangedEvidence struct {
	Tokens []UseTokenRef
}

func (UseChangedEvidence) evidenceKind() {}

// SlotChangedEvidence points to the sole installed version used for comparison.
type SlotChangedEvidence struct {
	InstalledVersionIndex int
}

func (SlotChangedEvidence) evidenceKind() {}

// DowngradePlannedEvidence points to the displayed operation and versions on
// one Parser plan row. InstalledVersions preserves ambiguity without choosing.
type DowngradePlannedEvidence struct {
	EntryIndex        int
	Operation         parser.Operation
	InstalledVersions []string
	TargetVersion     string
}

func (DowngradePlannedEvidence) evidenceKind() {}

// UnresolvedBlockerEvidence points to a blocker, independently of Entries.
type UnresolvedBlockerEvidence struct{ BlockerIndex int }

func (UnresolvedBlockerEvidence) evidenceKind() {}

// Analyze applies Analyzer v0.1 rules without changing the Parser result.
func Analyze(plan parser.Result, context Context) Result {
	result := Result{
		Status:      plan.Status,
		Diagnostics: cloneDiagnostics(plan.Diagnostics),
		Findings:    make([]Finding, 0),
	}

	for entryIndex := range plan.Entries {
		entry := plan.Entries[entryIndex]
		if entry.Operation == parser.OperationDowngrade {
			var installedVersions []string
			if len(entry.Installed.Versions) > 0 {
				installedVersions = make([]string, len(entry.Installed.Versions))
				for index, installed := range entry.Installed.Versions {
					installedVersions[index] = installed.Version
				}
			}
			message := "Portage планирует понижение версии. Проверьте, соответствует ли это ожидаемому изменению."
			if len(installedVersions) == 1 && installedVersions[0] != "" && entry.Target.Version != "" {
				message = "Portage планирует понижение версии: " + installedVersions[0] + " → " + entry.Target.Version + ". Проверьте, соответствует ли это ожидаемому изменению."
			}
			result.Findings = append(result.Findings, Finding{
				RuleID:     RuleDowngradePlanned,
				Category:   CategoryReview,
				Message:    message,
				EntryIndex: &entryIndex,
				Evidence: DowngradePlannedEvidence{
					EntryIndex: entryIndex, Operation: entry.Operation,
					InstalledVersions: installedVersions, TargetVersion: entry.Target.Version,
				},
			})
		}

		if contextIndex, ok := firstPackageMatch(entry.Package, context.LocallyPatchedPackages); ok {
			result.Findings = append(result.Findings, Finding{
				RuleID:     RuleLocalPatchedPackage,
				Category:   CategoryReview,
				Message:    "Проверьте применимость локальных патчей для " + packageName(entry.Package) + ".",
				EntryIndex: &entryIndex,
				Evidence:   LocalPatchedPackageEvidence{ContextIndex: contextIndex},
			})
		}

		if evidence, rawTokens := changedUseTokens(entry.FlagGroups); len(evidence.Tokens) > 0 {
			result.Findings = append(result.Findings, Finding{
				RuleID:     RuleUseChanged,
				Category:   CategoryReview,
				Message:    "План показывает изменения USE: " + joinTokens(rawTokens) + ".",
				EntryIndex: &entryIndex,
				Evidence:   evidence,
			})
		}

		if evidence, installedSlot, targetSlot, ok := changedSlot(entry); ok {
			result.Findings = append(result.Findings, Finding{
				RuleID:     RuleSlotChanged,
				Category:   CategoryReview,
				Message:    "SLOT пакета " + packageName(entry.Package) + " изменяется с " + installedSlot + " на " + targetSlot + ".",
				EntryIndex: &entryIndex,
				Evidence:   evidence,
			})
		}
	}

	for blockerIndex, blocker := range plan.Blockers {
		if blocker.Resolution != parser.BlockerUnresolved {
			continue
		}
		result.Findings = append(result.Findings, Finding{
			RuleID: RuleUnresolvedBlocker, Category: CategoryBlocker,
			Message:  "Portage показывает неразрешённую блокировку: " + blocker.Atom + ". В представленном плане это ограничение не разрешено.",
			Evidence: UnresolvedBlockerEvidence{BlockerIndex: blockerIndex},
		})
	}
	return result
}

func firstPackageMatch(pkg parser.Package, packages []parser.Package) (int, bool) {
	for index, candidate := range packages {
		if pkg.Category == candidate.Category && pkg.Name == candidate.Name {
			return index, true
		}
	}
	return 0, false
}

func changedUseTokens(groups []parser.FlagGroup) (UseChangedEvidence, []string) {
	evidence := UseChangedEvidence{Tokens: make([]UseTokenRef, 0)}
	rawTokens := make([]string, 0)
	for groupIndex, group := range groups {
		if group.Name != "USE" {
			continue
		}
		for valueIndex, value := range group.Values {
			if hasMarker(value.Markers, "*") || hasMarker(value.Markers, "%") {
				evidence.Tokens = append(evidence.Tokens, UseTokenRef{GroupIndex: groupIndex, ValueIndex: valueIndex})
				rawTokens = append(rawTokens, value.RawToken)
			}
		}
	}
	return evidence, rawTokens
}

func hasMarker(markers []string, wanted string) bool {
	for _, marker := range markers {
		if marker == wanted {
			return true
		}
	}
	return false
}

func changedSlot(entry parser.Entry) (SlotChangedEvidence, string, string, bool) {
	if (entry.Operation != parser.OperationUpgrade && entry.Operation != parser.OperationDowngrade) || len(entry.Installed.Versions) != 1 || entry.Target.Slot == nil {
		return SlotChangedEvidence{}, "", "", false
	}
	installed := entry.Installed.Versions[0]
	if installed.Slot == nil || *installed.Slot == *entry.Target.Slot {
		return SlotChangedEvidence{}, "", "", false
	}
	return SlotChangedEvidence{InstalledVersionIndex: 0}, *installed.Slot, *entry.Target.Slot, true
}

func packageName(pkg parser.Package) string { return pkg.Category + "/" + pkg.Name }

func cloneDiagnostics(diagnostics []parser.Diagnostic) []parser.Diagnostic {
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

func joinTokens(tokens []string) string {
	return strings.Join(tokens, ", ")
}
