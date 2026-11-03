package parser

import (
	"regexp"
	"strings"
)

type BlockerResolution string

const (
	BlockerResolved   BlockerResolution = "resolved"
	BlockerUnresolved BlockerResolution = "unresolved"
)

type BlockerStrength string

const (
	BlockerSoft            BlockerStrength = "soft"
	BlockerHard            BlockerStrength = "hard"
	BlockerStrengthUnknown BlockerStrength = "unknown"
)

// Blocker retains the displayed constraint independently of package actions.
// Parents are displayed CPVs, not inferred links to Entries.
type Blocker struct {
	Atom           string
	ConstraintAtom *string
	Marker         string
	Resolution     BlockerResolution
	Strength       BlockerStrength
	Parents        []string
	RawLine        string
	SourceLines    []int
}

// Removal is an explicitly displayed uninstall, not a ChangeReport entry.
type Removal struct {
	Package     Package
	Target      Target
	FlagGroups  []FlagGroup
	RawLine     string
	SourceLines []int
}

type ConflictSummary struct {
	Blockers   int
	Unresolved int
	SourceLine int
}

var (
	blockerRE            = regexp.MustCompile(`^\[blocks\s+([bB])\s*\]\s+(\S+)(?:\s+\((.*)\))?$`)
	blockerExplanationRE = regexp.MustCompile(`^(?:"([^"\s]+)" )?is (soft|hard) blocking (.+)$`)
	removalRE            = regexp.MustCompile(`^\[uninstall\s*\]\s+(.+)$`)
	conflictSummaryRE    = regexp.MustCompile(`^Conflict:\s*([0-9]+) blocks? \((all satisfied|[0-9]+ unsatisfied)\)$`)
	// This is a lexical boundary check, not a complete dependency atom parser.
	displayedAtomRE = regexp.MustCompile(`^[!<>=~]*[A-Za-z0-9_+.-]+/[A-Za-z0-9_+.*:/=\[\],!?()@~<>-]+$`)
)

func parseBlocker(raw string, line int) (Blocker, bool) {
	match := blockerRE.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil || !displayedAtomRE.MatchString(match[2]) {
		return Blocker{}, false
	}
	b := Blocker{Atom: match[2], Marker: match[1], Resolution: BlockerUnresolved,
		Strength: BlockerStrengthUnknown, RawLine: raw, SourceLines: []int{line}}
	if b.Marker == "b" {
		b.Resolution = BlockerResolved
	}
	if match[3] == "" {
		// An explicit empty explanation is malformed, not an unknown type.
		if strings.HasSuffix(strings.TrimSpace(raw), "()") {
			return Blocker{}, false
		}
		return b, true
	}
	explanation := blockerExplanationRE.FindStringSubmatch(match[3])
	if explanation == nil {
		return Blocker{}, false
	}
	if explanation[1] != "" {
		if !displayedAtomRE.MatchString(explanation[1]) {
			return Blocker{}, false
		}
		b.ConstraintAtom = &explanation[1]
	}
	b.Strength = BlockerStrength(explanation[2])
	for _, parent := range strings.Split(explanation[3], ", ") {
		// The formatter prints parent CPVs, without dependency operators.
		target, _, err := parseTarget(parent)
		if err != nil || strings.ContainsAny(parent, " \t,\"()!<>=~[]") || target.Repository != nil || target.Slot != nil {
			return Blocker{}, false
		}
		b.Parents = append(b.Parents, parent)
	}
	return b, true
}

func parseRemoval(raw string, line int) (Removal, bool) {
	match := removalRE.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return Removal{}, false
	}
	// Reuse the established atom/flag parser without treating removal as an
	// upgrade in the public result. Installed context and size are not allowed.
	entry, code, _ := parseEbuildRow("U", match[1], line)
	if code != "" || len(entry.Installed.Versions) != 0 || entry.Download != nil {
		return Removal{}, false
	}
	return Removal{Package: entry.Package, Target: entry.Target, FlagGroups: entry.FlagGroups,
		RawLine: raw, SourceLines: []int{line}}, true
}

func parseConflictSummary(raw string, line int) (ConflictSummary, bool) {
	match := conflictSummaryRE.FindStringSubmatch(raw)
	if match == nil {
		return ConflictSummary{}, false
	}
	summary := ConflictSummary{Blockers: integer(match[1]), SourceLine: line}
	if match[2] != "all satisfied" {
		summary.Unresolved = integer(strings.Fields(match[2])[0])
	}
	if summary.Unresolved > summary.Blockers {
		return ConflictSummary{}, false
	}
	return summary, true
}
