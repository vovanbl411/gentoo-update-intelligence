package metadatadiff

import "sort"

type Side string

const (
	SideBefore Side = "before"
	SideAfter  Side = "after"
)

type Field string

const (
	FieldEAPI Field = "EAPI"
	FieldSlot Field = "SLOT"
	FieldIUSE Field = "IUSE"
)

type Status string

const (
	StatusAvailable   Status = "available"
	StatusPartial     Status = "partial"
	StatusUnavailable Status = "unavailable"
)

type SnapshotLink struct {
	Source Source
	SHA256 string
}

// EvidenceRef points either to source lines for a field or to a whole snapshot
// when the field has no source line (for example, a missing cache key).
type EvidenceRef struct {
	Side          Side
	Field         Field
	Lines         []int
	WholeSnapshot bool
}

type EvidencePair struct {
	Before EvidenceRef
	After  EvidenceRef
}

type ValueChange[T comparable] struct {
	Before   T
	After    T
	Evidence EvidencePair
}

type IUSEFlagChange struct {
	Name     string
	Default  IUSEDefault
	Evidence EvidencePair
}

type IUSEDefaultChange struct {
	Name     string
	Before   IUSEDefault
	After    IUSEDefault
	Evidence EvidencePair
}

type IUSEChanges struct {
	Added          []IUSEFlagChange
	Removed        []IUSEFlagChange
	DefaultChanges []IUSEDefaultChange
}

type Changes struct {
	EAPI    *ValueChange[string]
	Slot    *ValueChange[string]
	Subslot *ValueChange[string]
	IUSE    IUSEChanges
}

type Limitation struct {
	Code     string
	Field    Field
	Side     Side
	Evidence []EvidenceRef
}

type Result struct {
	Status         Status
	ComparedFields []Field
	Changes        Changes
	Limitations    []Limitation
	Before         SnapshotLink
	After          SnapshotLink
}

// Compare compares only fields whose values are valid for EAPI 8 on both
// sides. Its order is stable and independent of map iteration.
func Compare(before, after Snapshot) Result {
	result := Result{
		Status:  StatusUnavailable,
		Changes: Changes{IUSE: IUSEChanges{Added: []IUSEFlagChange{}, Removed: []IUSEFlagChange{}, DefaultChanges: []IUSEDefaultChange{}}},
		Before:  SnapshotLink{Source: before.Source, SHA256: before.SHA256},
		After:   SnapshotLink{Source: after.Source, SHA256: after.SHA256},
	}

	result.Limitations = append(result.Limitations, missingSourceLimitations(before, SideBefore)...)
	result.Limitations = append(result.Limitations, missingSourceLimitations(after, SideAfter)...)
	if len(result.Limitations) != 0 {
		return result
	}
	if before.Source.Category != after.Source.Category || before.Source.Name != after.Source.Name {
		result.Limitations = append(result.Limitations, Limitation{Code: "PACKAGE_MISMATCH", Field: Field("category/name"), Evidence: []EvidenceRef{
			evidenceRef(SideBefore, Field("category/name"), nil), evidenceRef(SideAfter, Field("category/name"), nil),
		}})
	}
	if before.Source.Repository != after.Source.Repository {
		result.Limitations = append(result.Limitations, Limitation{Code: "REPOSITORY_MISMATCH", Field: Field("repository"), Evidence: []EvidenceRef{
			evidenceRef(SideBefore, Field("repository"), nil), evidenceRef(SideAfter, Field("repository"), nil),
		}})
	}
	if len(result.Limitations) != 0 {
		return result
	}

	if !supportedEAPI(before.EAPI) {
		result.Limitations = append(result.Limitations, eapiLimitation(before, SideBefore))
	}
	if !supportedEAPI(after.EAPI) {
		result.Limitations = append(result.Limitations, eapiLimitation(after, SideAfter))
	}
	if len(result.Limitations) != 0 {
		return result
	}

	result.ComparedFields = append(result.ComparedFields, FieldEAPI)
	if before.Slot.State == StateValid && after.Slot.State == StateValid {
		result.ComparedFields = append(result.ComparedFields, FieldSlot)
		result.Changes.Slot = stringChange(before.Slot.Name, after.Slot.Name, evidence(before.Slot.Lines, after.Slot.Lines, FieldSlot))
		result.Changes.Subslot = stringChange(before.Slot.Subslot, after.Slot.Subslot, evidence(before.Slot.Lines, after.Slot.Lines, FieldSlot))
	} else {
		if before.Slot.State != StateValid {
			result.Limitations = append(result.Limitations, slotLimitation(before, SideBefore)...)
		}
		if after.Slot.State != StateValid {
			result.Limitations = append(result.Limitations, slotLimitation(after, SideAfter)...)
		}
	}
	if before.IUSE.State == StateValid && after.IUSE.State == StateValid {
		result.ComparedFields = append(result.ComparedFields, FieldIUSE)
		result.Changes.IUSE = compareIUSE(before.IUSE, after.IUSE)
	} else {
		if before.IUSE.State != StateValid {
			result.Limitations = append(result.Limitations, iuseLimitation(before, SideBefore)...)
		}
		if after.IUSE.State != StateValid {
			result.Limitations = append(result.Limitations, iuseLimitation(after, SideAfter)...)
		}
	}

	switch len(result.ComparedFields) {
	case 3:
		result.Status = StatusAvailable
	case 1, 2:
		result.Status = StatusPartial
	}
	return result
}

func missingSourceLimitations(snapshot Snapshot, side Side) []Limitation {
	source := snapshot.Source
	fields := []string{"category", "name", "version", "repository", "source_url", "commit", "cache_path"}
	values := []string{source.Category, source.Name, source.Version, source.Repository, source.SourceURL, source.Commit, source.CachePath}
	limitations := make([]Limitation, 0)
	for i, value := range values {
		if value == "" {
			field := Field(fields[i])
			limitations = append(limitations, Limitation{Code: "SOURCE_MISSING", Field: field, Side: side, Evidence: []EvidenceRef{evidenceRef(side, field, nil)}})
		}
	}
	return limitations
}

func eapiLimitation(snapshot Snapshot, side Side) Limitation {
	eapi := snapshot.EAPI
	code := "EAPI_UNSUPPORTED"
	if eapi.State == StateMissing {
		code = "EAPI_MISSING"
	} else if eapi.State == StateInvalid {
		code = "EAPI_INVALID"
	}
	return Limitation{Code: code, Field: FieldEAPI, Side: side, Evidence: []EvidenceRef{evidenceRef(side, FieldEAPI, eapi.Lines)}}
}

func supportedEAPI(eapi EAPI) bool {
	return eapi.State == StateValid && eapi.Supported && eapi.Value == "8"
}

func slotLimitation(snapshot Snapshot, side Side) []Limitation {
	slot := snapshot.Slot
	code := "SLOT_INVALID"
	if slot.State == StateMissing {
		code = "SLOT_MISSING"
	}
	return []Limitation{{Code: code, Field: FieldSlot, Side: side, Evidence: []EvidenceRef{evidenceRef(side, FieldSlot, slot.Lines)}}}
}

func iuseLimitation(snapshot Snapshot, side Side) []Limitation {
	iuse := snapshot.IUSE
	code := "IUSE_INVALID"
	if iuse.State == StateMissing {
		code = "IUSE_MISSING"
	}
	return []Limitation{{Code: code, Field: FieldIUSE, Side: side, Evidence: []EvidenceRef{evidenceRef(side, FieldIUSE, iuse.Lines)}}}
}

func evidence(beforeLines, afterLines []int, field Field) EvidencePair {
	return EvidencePair{
		Before: evidenceRef(SideBefore, field, beforeLines),
		After:  evidenceRef(SideAfter, field, afterLines),
	}
}

func evidenceRef(side Side, field Field, lines []int) EvidenceRef {
	if len(lines) == 0 {
		return EvidenceRef{Side: side, Field: field, WholeSnapshot: true}
	}
	return EvidenceRef{Side: side, Field: field, Lines: append([]int(nil), lines...)}
}

func stringChange(before, after string, refs EvidencePair) *ValueChange[string] {
	if before == after {
		return nil
	}
	return &ValueChange[string]{Before: before, After: after, Evidence: refs}
}

func compareIUSE(before, after IUSE) IUSEChanges {
	beforeByName := make(map[string]IUSEFlag, len(before.Flags))
	afterByName := make(map[string]IUSEFlag, len(after.Flags))
	for _, flag := range before.Flags {
		beforeByName[flag.Name] = flag
	}
	for _, flag := range after.Flags {
		afterByName[flag.Name] = flag
	}
	changes := IUSEChanges{Added: []IUSEFlagChange{}, Removed: []IUSEFlagChange{}, DefaultChanges: []IUSEDefaultChange{}}
	for _, flag := range after.Flags {
		old, exists := beforeByName[flag.Name]
		if !exists {
			changes.Added = append(changes.Added, IUSEFlagChange{Name: flag.Name, Default: flag.Default, Evidence: evidence(before.Lines, declarationLines(flag.Declarations), FieldIUSE)})
			continue
		}
		if old.Default != flag.Default {
			changes.DefaultChanges = append(changes.DefaultChanges, IUSEDefaultChange{
				Name: flag.Name, Before: old.Default, After: flag.Default,
				Evidence: evidence(declarationLines(old.Declarations), declarationLines(flag.Declarations), FieldIUSE),
			})
		}
	}
	for _, flag := range before.Flags {
		if _, exists := afterByName[flag.Name]; !exists {
			changes.Removed = append(changes.Removed, IUSEFlagChange{Name: flag.Name, Default: flag.Default, Evidence: evidence(declarationLines(flag.Declarations), after.Lines, FieldIUSE)})
		}
	}
	sort.Slice(changes.Added, func(i, j int) bool { return changes.Added[i].Name < changes.Added[j].Name })
	sort.Slice(changes.Removed, func(i, j int) bool { return changes.Removed[i].Name < changes.Removed[j].Name })
	sort.Slice(changes.DefaultChanges, func(i, j int) bool { return changes.DefaultChanges[i].Name < changes.DefaultChanges[j].Name })
	return changes
}
