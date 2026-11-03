// Package metadatadiff normalizes prepared Gentoo md5-cache snapshots and
// compares the EAPI, SLOT/SUBSLOT, and IUSE fields.
package metadatadiff

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Source identifies the package and the exact repository input used for a
// snapshot. Repository is the repository name, not SourceURL's mirror address.
type Source struct {
	Category   string
	Name       string
	Version    string
	Repository string
	SourceURL  string
	Commit     string
	CachePath  string
}

type FieldState string

const (
	StateValid   FieldState = "valid"
	StateMissing FieldState = "missing"
	StateInvalid FieldState = "invalid"
)

type EAPI struct {
	State     FieldState
	Value     string
	Supported bool
	Lines     []int
}

type Slot struct {
	State           FieldState
	Raw             string
	Name            string
	Subslot         string
	ExplicitSubslot bool
	Lines           []int
}

type IUSEDefault string

const (
	DefaultUnspecified IUSEDefault = "unspecified"
	DefaultEnabled     IUSEDefault = "enabled"
	DefaultDisabled    IUSEDefault = "disabled"
)

type IUSEDeclaration struct {
	Token string
	Line  int
}

type IUSEFlag struct {
	Name         string
	Default      IUSEDefault
	Declarations []IUSEDeclaration
}

type IUSE struct {
	State FieldState
	Flags []IUSEFlag
	Lines []int
}

type Diagnostic struct {
	Code  string
	Field string
	Lines []int
}

type Snapshot struct {
	Source      Source
	Content     string
	SHA256      string
	EAPI        EAPI
	Slot        Slot
	IUSE        IUSE
	Diagnostics []Diagnostic
}

// Normalize parses only the selected metadata keys. Content and its SHA-256
// are retained exactly as provided.
func Normalize(source Source, content string) Snapshot {
	snapshot := Snapshot{Source: source, Content: content}
	digest := sha256.Sum256([]byte(content))
	snapshot.SHA256 = hex.EncodeToString(digest[:])

	values := map[string][]cacheValue{"EAPI": {}, "SLOT": {}, "IUSE": {}}
	for lineNumber, line := range strings.Split(content, "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		if _, selected := values[key]; selected {
			values[key] = append(values[key], cacheValue{value: value, line: lineNumber + 1})
		}
	}

	snapshot.EAPI, snapshot.Diagnostics = normalizeEAPI(values["EAPI"], snapshot.Diagnostics)
	if snapshot.EAPI.State == StateValid && snapshot.EAPI.Supported {
		snapshot.Slot, snapshot.Diagnostics = normalizeSlot(values["SLOT"], snapshot.Diagnostics)
		snapshot.IUSE, snapshot.Diagnostics = normalizeIUSE(values["IUSE"], snapshot.Diagnostics)
	} else {
		snapshot.Slot, snapshot.Diagnostics = uninterpretableSlot(values["SLOT"], snapshot.Diagnostics)
		snapshot.IUSE, snapshot.Diagnostics = uninterpretableIUSE(values["IUSE"], snapshot.Diagnostics)
	}
	return snapshot
}

type cacheValue struct {
	value string
	line  int
}

func normalizeEAPI(values []cacheValue, diagnostics []Diagnostic) (EAPI, []Diagnostic) {
	if len(values) == 0 {
		return EAPI{State: StateMissing}, append(diagnostics, Diagnostic{Code: "EAPI_MISSING", Field: "EAPI"})
	}
	lines := valueLines(values)
	first := values[0].value
	for _, value := range values[1:] {
		if value.value != first {
			return EAPI{State: StateInvalid, Lines: lines}, append(diagnostics, Diagnostic{Code: "EAPI_CONFLICTING_VALUES", Field: "EAPI", Lines: lines})
		}
	}
	if first == "" || !validEAPIName(first) {
		return EAPI{State: StateInvalid, Value: first, Lines: lines}, append(diagnostics, Diagnostic{Code: "EAPI_INVALID", Field: "EAPI", Lines: lines})
	}
	eapi := EAPI{State: StateValid, Value: first, Supported: first == "8", Lines: lines}
	if !eapi.Supported {
		diagnostics = append(diagnostics, Diagnostic{Code: "EAPI_UNSUPPORTED", Field: "EAPI", Lines: lines})
	}
	return eapi, diagnostics
}

func normalizeSlot(values []cacheValue, diagnostics []Diagnostic) (Slot, []Diagnostic) {
	if len(values) == 0 {
		return Slot{State: StateMissing}, append(diagnostics, Diagnostic{Code: "SLOT_MISSING", Field: "SLOT"})
	}
	lines := valueLines(values)
	first := values[0].value
	for _, value := range values[1:] {
		if value.value != first {
			return Slot{State: StateInvalid, Lines: lines}, append(diagnostics, Diagnostic{Code: "SLOT_CONFLICTING_VALUES", Field: "SLOT", Lines: lines})
		}
	}
	parts := strings.Split(first, "/")
	if len(parts) > 2 || !validSlotName(parts[0]) || (len(parts) == 2 && !validSlotName(parts[1])) {
		return Slot{State: StateInvalid, Raw: first, Lines: lines}, append(diagnostics, Diagnostic{Code: "SLOT_INVALID", Field: "SLOT", Lines: lines})
	}
	slot := Slot{State: StateValid, Raw: first, Name: parts[0], Subslot: parts[0], Lines: lines}
	if len(parts) == 2 {
		slot.Subslot = parts[1]
		slot.ExplicitSubslot = true
	}
	return slot, diagnostics
}

func normalizeIUSE(values []cacheValue, diagnostics []Diagnostic) (IUSE, []Diagnostic) {
	if len(values) == 0 {
		return IUSE{State: StateMissing}, append(diagnostics, Diagnostic{Code: "IUSE_MISSING", Field: "IUSE"})
	}
	lines := valueLines(values)
	for _, value := range values[1:] {
		if value.value != values[0].value {
			return IUSE{State: StateInvalid, Lines: lines}, append(diagnostics, Diagnostic{Code: "IUSE_CONFLICTING_VALUES", Field: "IUSE", Lines: lines})
		}
	}
	byName := make(map[string]IUSEFlag)
	valid := true
	for _, value := range values {
		for _, token := range strings.Fields(value.value) {
			name, defaultValue, ok := parseIUSEToken(token)
			if !ok {
				valid = false
				diagnostics = append(diagnostics, Diagnostic{Code: "IUSE_INVALID_TOKEN", Field: "IUSE", Lines: []int{value.line}})
				continue
			}
			flag, exists := byName[name]
			if exists && flag.Default != defaultValue {
				valid = false
				conflictLines := declarationLines(flag.Declarations)
				if len(conflictLines) == 0 || conflictLines[len(conflictLines)-1] != value.line {
					conflictLines = append(conflictLines, value.line)
				}
				diagnostics = append(diagnostics, Diagnostic{Code: "IUSE_CONFLICTING_DEFAULTS", Field: "IUSE", Lines: conflictLines})
			}
			if !exists {
				flag = IUSEFlag{Name: name, Default: defaultValue}
			}
			flag.Declarations = append(flag.Declarations, IUSEDeclaration{Token: token, Line: value.line})
			byName[name] = flag
		}
	}
	if !valid {
		return IUSE{State: StateInvalid, Lines: lines}, diagnostics
	}
	flags := make([]IUSEFlag, 0, len(byName))
	for _, flag := range byName {
		flags = append(flags, flag)
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })
	return IUSE{State: StateValid, Flags: flags, Lines: lines}, diagnostics
}

func uninterpretableSlot(values []cacheValue, diagnostics []Diagnostic) (Slot, []Diagnostic) {
	if len(values) == 0 {
		return Slot{State: StateMissing}, append(diagnostics, Diagnostic{Code: "SLOT_MISSING", Field: "SLOT"})
	}
	lines := valueLines(values)
	return Slot{State: StateInvalid, Raw: values[0].value, Lines: lines}, append(diagnostics, Diagnostic{Code: "SLOT_EAPI_UNAVAILABLE", Field: "SLOT", Lines: lines})
}

func uninterpretableIUSE(values []cacheValue, diagnostics []Diagnostic) (IUSE, []Diagnostic) {
	if len(values) == 0 {
		return IUSE{State: StateMissing}, append(diagnostics, Diagnostic{Code: "IUSE_MISSING", Field: "IUSE"})
	}
	lines := valueLines(values)
	return IUSE{State: StateInvalid, Lines: lines}, append(diagnostics, Diagnostic{Code: "IUSE_EAPI_UNAVAILABLE", Field: "IUSE", Lines: lines})
}

func parseIUSEToken(token string) (string, IUSEDefault, bool) {
	name := token
	defaultValue := DefaultUnspecified
	if strings.HasPrefix(token, "+") {
		name, defaultValue = token[1:], DefaultEnabled
	} else if strings.HasPrefix(token, "-") {
		name, defaultValue = token[1:], DefaultDisabled
	}
	if !validUSEFlagName(name) {
		return "", "", false
	}
	return name, defaultValue, true
}

func validEAPIName(value string) bool {
	return validGentooName(value, "-.+")
}

func validSlotName(value string) bool {
	return validGentooName(value, "-.+")
}

func validUSEFlagName(value string) bool {
	if value == "" || !isASCIIAlnum(value[0]) {
		return false
	}
	for i := 1; i < len(value); i++ {
		c := value[i]
		if !isASCIIAlnum(c) && c != '+' && c != '@' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func validGentooName(value, forbiddenPrefix string) bool {
	if value == "" || strings.ContainsRune(forbiddenPrefix, rune(value[0])) {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !isASCIIAlnum(c) && c != '+' && c != '_' && c != '.' && c != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func valueLines(values []cacheValue) []int {
	lines := make([]int, 0, len(values))
	for _, value := range values {
		lines = append(lines, value.line)
	}
	return lines
}

func declarationLines(declarations []IUSEDeclaration) []int {
	lines := make([]int, 0, len(declarations))
	for _, declaration := range declarations {
		if len(lines) == 0 || lines[len(lines)-1] != declaration.Line {
			lines = append(lines, declaration.Line)
		}
	}
	return lines
}
