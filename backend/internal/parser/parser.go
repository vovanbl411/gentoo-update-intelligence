// Package parser extracts facts from the supported, plain-text Portage plan
// format. It deliberately has no dependencies on the filesystem or services.
package parser

import (
	"bufio"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Status describes how much of the input was understood.
type Status string

const (
	StatusParsed      Status = "parsed"
	StatusPartial     Status = "partial"
	StatusUnsupported Status = "unsupported"
)

// Operation is the Portage operation represented by an ebuild row.
type Operation string

const (
	OperationUpgrade   Operation = "upgrade"
	OperationDowngrade Operation = "downgrade"
	OperationInstall   Operation = "install"
	OperationReinstall Operation = "reinstall"
)

// Result is the deterministic, source-linked result of parsing one input.
type Result struct {
	Status          Status
	Entries         []Entry
	Blockers        []Blocker
	Removals        []Removal
	ConflictSummary *ConflictSummary
	Summary         *Summary
	Diagnostics     []Diagnostic
}

// Package identifies a package independently of its version.
type Package struct {
	Category string
	Name     string
}

// Target contains only the target atom facts explicitly shown by Portage.
// Nil optional fields mean that the input did not show that fact.
type Target struct {
	Version    string
	Repository *string
	Slot       *string
	Subslot    *string
}

// InstalledVersion records one version shown in the row's installed context.
type InstalledVersion struct {
	Version    string
	Repository *string
	Slot       *string
	Subslot    *string
}

// InstalledContext contains the versions shown alongside the target atom.
type InstalledContext struct {
	Versions []InstalledVersion
}

// Entry is one supported [ebuild ...] row.
type Entry struct {
	Package        Package
	Operation      Operation
	NewSlot        bool
	UnstableMarker bool
	Target         Target
	Installed      InstalledContext
	FlagGroups     []FlagGroup
	Download       *Size
	SourceLines    []int
}

// FlagGroup retains the name and position of a group, including an empty one.
type FlagGroup struct {
	Name   string
	Values []FlagValue
}

// FlagState describes the enabled or disabled state shown by a flag token.
type FlagState string

const (
	FlagEnabled  FlagState = "enabled"
	FlagDisabled FlagState = "disabled"
)

// FlagValue preserves both the displayed token and its interpreted state.
type FlagValue struct {
	Name     string
	State    FlagState
	RawToken string
	Markers  []string
}

// Size is a displayed size and its original unit; it is not normalized.
type Size struct {
	Value int64
	Unit  string
}

// Summary contains the counts and download size printed by Portage.
type Summary struct {
	Packages   int
	Upgrades   int
	Reinstalls int
	Downgrades int
	Uninstalls int
	NewSlots   int
	Download   Size
	SourceLine int
}

// Diagnostic describes a significant fragment that could not be fully used.
type Diagnostic struct {
	Code        string
	Message     string
	SourceLines []int
}

var (
	rowPrefixRE   = regexp.MustCompile(`^\[(ebuild|binary)\s+([^]]*)\]\s*(.*)$`)
	versionRE     = regexp.MustCompile(`^(.+)-([0-9][A-Za-z0-9._+~-]*)$`)
	summaryRE     = regexp.MustCompile(`^Total:\s*([0-9]+) package(?:s)?\s*\((.*)\),\s*Size of downloads:\s*([0-9,]+)\s+(B|KiB|MiB|GiB)$`)
	summaryItemRE = regexp.MustCompile(`^([0-9]+)\s+(upgrade|upgrades|downgrade|downgrades|in new slot|in new slots|uninstall|uninstalls|reinstall|reinstalls)$`)
	sizeRE        = regexp.MustCompile(`^([0-9,]+)\s+(B|KiB|MiB|GiB)$`)
	sizeIntegerRE = regexp.MustCompile(`^(?:[0-9]+|[0-9]{1,3}(?:,[0-9]{3})+)$`)
	ansiRE        = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
)

// Parse reads the supported subset of a plain-text Portage plan. It never
// performs I/O and reports malformed or unsupported input in the result.
func Parse(input string) Result {
	result := Result{Status: StatusUnsupported}
	if strings.TrimSpace(input) == "" {
		result.Diagnostics = []Diagnostic{{Code: "EMPTY_INPUT", Message: "вход пуст или содержит только пробельные символы"}}
		return result
	}

	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Buffer(make([]byte, 1024), 2*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if ansiRE.MatchString(line) {
			result.Diagnostics = append(result.Diagnostics, diagnostic("UNSUPPORTED_FORMAT", "ANSI-форматирование не поддерживается", lineNumber))
			continue
		}
		if strings.Contains(trimmed, "--tree") || strings.Contains(trimmed, "--columns") {
			result.Diagnostics = append(result.Diagnostics, diagnostic("UNSUPPORTED_FORMAT", "формат --tree или --columns не поддерживается", lineNumber))
			continue
		}

		if strings.HasPrefix(trimmed, "[blocks") {
			blocker, ok := parseBlocker(line, lineNumber)
			if !ok {
				result.Diagnostics = append(result.Diagnostics, diagnostic("MALFORMED_BLOCKER", "повреждён или не поддержан формат blocker", lineNumber))
			} else {
				result.Blockers = append(result.Blockers, blocker)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "[uninstall") {
			removal, ok := parseRemoval(line, lineNumber)
			if !ok {
				result.Diagnostics = append(result.Diagnostics, diagnostic("MALFORMED_REMOVAL", "повреждён или не поддержан формат удаления", lineNumber))
			} else {
				result.Removals = append(result.Removals, removal)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "Conflict:") {
			summary, ok := parseConflictSummary(trimmed, lineNumber)
			if !ok || result.ConflictSummary != nil {
				result.Diagnostics = append(result.Diagnostics, diagnostic("MALFORMED_CONFLICT_SUMMARY", "строка Conflict повреждена или повторяется", lineNumber))
			} else {
				result.ConflictSummary = &summary
			}
			continue
		}

		if match := rowPrefixRE.FindStringSubmatch(trimmed); match != nil {
			if match[1] == "binary" {
				result.Diagnostics = append(result.Diagnostics, diagnostic("UNSUPPORTED_OPERATION", "записи [binary …] не поддерживаются", lineNumber))
				continue
			}
			// Portage places the operation in the bracket marker field. Looking
			// there avoids mistaking letters in a package or flag for an operation.
			entry, code, message := parseEbuildRow(match[2], match[3], lineNumber)
			if code != "" {
				result.Diagnostics = append(result.Diagnostics, diagnostic(code, message, lineNumber))
				continue
			}
			result.Entries = append(result.Entries, entry)
			continue
		}

		if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "ebuild") {
			result.Diagnostics = append(result.Diagnostics, diagnostic("MALFORMED_ENTRY", "повреждён заголовок записи ebuild", lineNumber))
			continue
		}
		if strings.HasPrefix(trimmed, "Total:") {
			if result.Summary != nil {
				result.Diagnostics = append(result.Diagnostics, diagnostic("MALFORMED_SUMMARY", "обнаружено более одной сводки", lineNumber))
				continue
			}
			if match := summaryRE.FindStringSubmatch(trimmed); match != nil {
				summary, ok := parseSummary(match, lineNumber)
				if ok {
					result.Summary = &summary
				} else {
					result.Diagnostics = append(result.Diagnostics, diagnostic("MALFORMED_SUMMARY", "сводка содержит неподдерживаемый или неоднозначный счётчик", lineNumber))
				}
			} else {
				result.Diagnostics = append(result.Diagnostics, diagnostic("MALFORMED_SUMMARY", "не удалось разобрать строку сводки", lineNumber))
			}
			continue
		}
		if isServiceText(trimmed) {
			continue
		}
		result.Diagnostics = append(result.Diagnostics, diagnostic("UNRECOGNIZED_LINE", "значимая строка не распознана", lineNumber))
	}

	// Scanner errors are unusual for strings, but an oversized line must not
	// silently yield a successful partial input.
	if scanner.Err() != nil {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "MALFORMED_ENTRY", Message: "строка входа превышает допустимый размер"})
	}
	if result.Summary != nil && (result.Summary.Packages != len(result.Entries) || result.Summary.Uninstalls != len(result.Removals)) {
		// The total record count is mandatory; displayed rounded KiB totals are
		// preserved as facts and are not compared against a sum of row sizes.
		result.Diagnostics = append(result.Diagnostics, diagnostic("SUMMARY_MISMATCH", "количество записей не совпадает со сводкой", result.Summary.SourceLine))
	}
	if summary := result.ConflictSummary; summary != nil {
		unresolved := 0
		for _, blocker := range result.Blockers {
			if blocker.Resolution == BlockerUnresolved {
				unresolved++
			}
		}
		if summary.Blockers != len(result.Blockers) || summary.Unresolved != unresolved {
			result.Diagnostics = append(result.Diagnostics, diagnostic("CONFLICT_SUMMARY_MISMATCH", "число blockers не совпадает со строкой Conflict", summary.SourceLine))
		}
	}
	// A summary consistency check happens after scanning, so restore source
	// order before returning diagnostics to callers.
	sort.SliceStable(result.Diagnostics, func(i, j int) bool {
		left, right := result.Diagnostics[i].SourceLines, result.Diagnostics[j].SourceLines
		if len(left) == 0 {
			return false
		}
		if len(right) == 0 {
			return true
		}
		return left[0] < right[0]
	})

	if len(result.Entries) == 0 && len(result.Blockers) == 0 && len(result.Removals) == 0 {
		result.Status = StatusUnsupported
	} else if len(result.Diagnostics) != 0 {
		result.Status = StatusPartial
	} else {
		result.Status = StatusParsed
	}
	return result
}

func parseEbuildRow(markerField, body string, line int) (Entry, string, string) {
	markerParts := strings.Fields(markerField)
	if len(markerParts) == 0 {
		return Entry{}, "MALFORMED_ENTRY", "в записи отсутствует поле операции Portage"
	}
	unstableMarker := false
	for _, marker := range markerParts[1:] {
		if marker != "~" || unstableMarker {
			return Entry{}, "UNSUPPORTED_OPERATION", "сочетание маркеров Portage не поддерживается"
		}
		unstableMarker = true
	}
	var operation Operation
	switch markerParts[0] {
	case "U":
		operation = OperationUpgrade
	case "UD":
		operation = OperationDowngrade
	case "R":
		operation = OperationReinstall
	case "NS":
		operation = OperationInstall
	default:
		return Entry{}, "UNSUPPORTED_OPERATION", "операция Portage не поддерживается"
	}

	tokens, ok := splitTokens(body)
	if !ok || len(tokens) == 0 {
		return Entry{}, "MALFORMED_ENTRY", "повреждена строка записи или группа флагов"
	}
	target, pkg, err := parseTarget(tokens[0])
	if err != nil {
		return Entry{}, "MALFORMED_ENTRY", "не удалось разобрать пакет и целевую версию"
	}
	entry := Entry{
		Package: pkg, Operation: operation, NewSlot: operation == OperationInstall,
		UnstableMarker: unstableMarker, Target: target,
		SourceLines: []int{line},
	}

	for index := 1; index < len(tokens); index++ {
		token := tokens[index]
		if strings.HasPrefix(token, "[") {
			if !strings.HasSuffix(token, "]") || strings.Contains(token[1:len(token)-1], "[") {
				return Entry{}, "MALFORMED_ENTRY", "повреждён установленный контекст"
			}
			installed, ok := parseInstalled(token[1 : len(token)-1])
			if !ok {
				return Entry{}, "MALFORMED_ENTRY", "не удалось разобрать установленный контекст"
			}
			entry.Installed.Versions = append(entry.Installed.Versions, installed)
			continue
		}
		if strings.Contains(token, "=") {
			group, ok := parseFlagGroup(token)
			if !ok {
				return Entry{}, "MALFORMED_ENTRY", "повреждена именованная группа флагов"
			}
			entry.FlagGroups = append(entry.FlagGroups, group)
			continue
		}
		if index+1 < len(tokens) {
			if match := sizeRE.FindStringSubmatch(token + " " + tokens[index+1]); match != nil {
				size, err := parseSize(match[1], match[2])
				if err != nil {
					return Entry{}, "MALFORMED_ENTRY", "некорректный целочисленный размер"
				}
				entry.Download = &size
				index++
				continue
			}
		}
		if match := sizeRE.FindStringSubmatch(token); match != nil {
			size, err := parseSize(match[1], match[2])
			if err != nil {
				return Entry{}, "MALFORMED_ENTRY", "некорректный целочисленный размер"
			}
			entry.Download = &size
			continue
		}
		return Entry{}, "MALFORMED_ENTRY", "неожиданный токен в записи ebuild"
	}
	return entry, "", ""
}

func parseSummary(match []string, line int) (Summary, bool) {
	packages, err := strconv.Atoi(match[1])
	if err != nil {
		return Summary{}, false
	}
	download, err := parseSize(match[3], match[4])
	if err != nil {
		return Summary{}, false
	}
	summary := Summary{Packages: packages, Download: download, SourceLine: line}
	if strings.TrimSpace(match[2]) == "" {
		return summary, true
	}
	seen := map[string]bool{}
	for _, item := range strings.Split(match[2], ",") {
		parts := summaryItemRE.FindStringSubmatch(strings.TrimSpace(item))
		if parts == nil {
			return Summary{}, false
		}
		count, err := strconv.Atoi(parts[1])
		if err != nil {
			return Summary{}, false
		}
		kind := parts[2]
		key := ""
		switch kind {
		case "reinstall", "reinstalls":
			key, summary.Reinstalls = "reinstall", count
		case "upgrade", "upgrades":
			key, summary.Upgrades = "upgrade", count
		case "downgrade", "downgrades":
			key, summary.Downgrades = "downgrade", count
		case "in new slot", "in new slots":
			key, summary.NewSlots = "new slots", count
		case "uninstall", "uninstalls":
			key, summary.Uninstalls = "uninstall", count
		}
		if seen[key] {
			return Summary{}, false
		}
		seen[key] = true
	}
	return summary, true
}

func parseTarget(atom string) (Target, Package, error) {
	// Remove repository and slot suffixes before splitting the final version
	// from the package atom; package names themselves may contain hyphens.
	base, repository := splitRepository(atom)
	base, slot, subslot := splitSlot(base)
	match := versionRE.FindStringSubmatch(base)
	if match == nil {
		return Target{}, Package{}, strconv.ErrSyntax
	}
	category, name, ok := splitPackage(match[1])
	if !ok {
		return Target{}, Package{}, strconv.ErrSyntax
	}
	return Target{Version: match[2], Repository: repository, Slot: slot, Subslot: subslot}, Package{Category: category, Name: name}, nil
}

func parseInstalled(atom string) (InstalledVersion, bool) {
	base, repository := splitRepository(atom)
	base, slot, subslot := splitSlot(base)
	if base == "" || strings.Contains(base, ":") {
		return InstalledVersion{}, false
	}
	return InstalledVersion{Version: base, Repository: repository, Slot: slot, Subslot: subslot}, true
}

func splitRepository(atom string) (string, *string) {
	if index := strings.LastIndex(atom, "::"); index >= 0 {
		value := atom[index+2:]
		if value != "" {
			return atom[:index], &value
		}
		return atom[:index], nil
	}
	return atom, nil
}

func splitSlot(atom string) (string, *string, *string) {
	index := strings.LastIndex(atom, ":")
	if index < 0 {
		return atom, nil, nil
	}
	value := atom[index+1:]
	if value == "" {
		return atom[:index], nil, nil
	}
	parts := strings.SplitN(value, "/", 2)
	if len(parts) == 2 {
		return atom[:index], &parts[0], &parts[1]
	}
	return atom[:index], &parts[0], nil
}

// splitPackage separates the category at its slash and keeps hyphens in the
// package name. Version extraction has already removed the final version.
func splitPackage(value string) (string, string, bool) {
	index := strings.IndexByte(value, '/')
	if index <= 0 || index == len(value)-1 || strings.Contains(value[index+1:], "/") {
		return "", "", false
	}
	return value[:index], value[index+1:], true
}

// splitTokens keeps quoted flag groups together so spaces inside a group do
// not turn into independent row fields.
func splitTokens(value string) ([]string, bool) {
	var tokens []string
	var token strings.Builder
	inQuote := false
	started := false
	for _, r := range value {
		if r == '"' {
			inQuote = !inQuote
			token.WriteRune(r)
			started = true
			continue
		}
		if !inQuote && (r == ' ' || r == '\t') {
			if started {
				tokens = append(tokens, token.String())
				token.Reset()
				started = false
			}
			continue
		}
		token.WriteRune(r)
		started = true
	}
	if inQuote {
		return nil, false
	}
	if started {
		tokens = append(tokens, token.String())
	}
	return tokens, true
}

func parseFlagGroup(token string) (FlagGroup, bool) {
	index := strings.Index(token, `="`)
	if index <= 0 || !strings.HasSuffix(token, `"`) {
		return FlagGroup{}, false
	}
	name := token[:index]
	contents := token[index+2 : len(token)-1]
	group := FlagGroup{Name: name, Values: []FlagValue{}}
	for _, raw := range strings.Fields(contents) {
		group.Values = append(group.Values, parseFlagValue(raw))
	}
	return group, true
}

func parseFlagValue(raw string) FlagValue {
	value := FlagValue{RawToken: raw}
	value.State = FlagEnabled
	trimmed := raw
	parenthesized := strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")")
	if parenthesized {
		value.Markers = append(value.Markers, "(", ")")
		trimmed = trimmed[1 : len(trimmed)-1]
	}
	if strings.HasPrefix(trimmed, "-") {
		value.Markers = append(value.Markers, "-")
		trimmed = trimmed[1:]
		value.State = FlagDisabled
	}
	if strings.HasPrefix(trimmed, "~") {
		value.Markers = append(value.Markers, "~")
		trimmed = trimmed[1:]
	}
	suffixStart := len(trimmed)
	for suffixStart > 0 && (trimmed[suffixStart-1] == '*' || trimmed[suffixStart-1] == '%') {
		suffixStart--
	}
	for _, marker := range trimmed[suffixStart:] {
		value.Markers = append(value.Markers, string(marker))
	}
	trimmed = trimmed[:suffixStart]
	value.Name = trimmed
	return value
}

func isServiceText(line string) bool {
	return line == "These are the packages that would be merged, in order:" ||
		strings.HasPrefix(line, "Calculating dependencies") ||
		strings.HasPrefix(line, "Dependency resolution took ") ||
		strings.HasPrefix(line, "Would you like to merge these packages?")
}

func diagnostic(code, message string, line int) Diagnostic {
	return Diagnostic{Code: code, Message: message, SourceLines: []int{line}}
}

func integer(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

// parseSize validates grouping before removing separators and rejects overflow.
func parseSize(number, unit string) (Size, error) {
	if !sizeIntegerRE.MatchString(number) {
		return Size{}, strconv.ErrSyntax
	}
	value, err := strconv.ParseInt(strings.ReplaceAll(number, ",", ""), 10, 64)
	if err != nil {
		return Size{}, err
	}
	return Size{Value: value, Unit: unit}, nil
}
