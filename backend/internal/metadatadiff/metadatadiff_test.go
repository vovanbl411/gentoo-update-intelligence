package metadatadiff

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func testSource() Source {
	return Source{
		Category: "dev-libs", Name: "example", Version: "1.0", Repository: "gentoo",
		SourceURL: "https://mirror.invalid/gentoo", Commit: "deadbeef", CachePath: "metadata/md5-cache/dev-libs/example-1.0",
	}
}

func normalized(content string) Snapshot { return Normalize(testSource(), content) }

func TestRealFixture(t *testing.T) {
	beforeContent, err := os.ReadFile("../../../testdata/metadata/libxml2-2.13.9.md5-cache")
	if err != nil {
		t.Fatal(err)
	}
	afterContent, err := os.ReadFile("../../../testdata/metadata/libxml2-2.14.6.md5-cache")
	if err != nil {
		t.Fatal(err)
	}
	beforeSource, afterSource := testSource(), testSource()
	for _, source := range []*Source{&beforeSource, &afterSource} {
		source.Category, source.Name, source.Repository = "dev-libs", "libxml2", "gentoo"
		source.SourceURL = "https://github.com/gentoo-mirror/gentoo"
		source.Commit = "ac2e2890e7702bcfa0dab44338707240894de8c7"
	}
	beforeSource.Version = "2.13.9"
	beforeSource.CachePath = "metadata/md5-cache/dev-libs/libxml2-2.13.9"
	afterSource.Version = "2.14.6"
	afterSource.CachePath = "metadata/md5-cache/dev-libs/libxml2-2.14.6"
	before := Normalize(beforeSource, string(beforeContent))
	after := Normalize(afterSource, string(afterContent))
	if before.SHA256 != "2e252a5dd5117202ed45b9ccdda024a61a9c7556eb224bde3321a8fedea3e2f3" {
		t.Fatalf("before SHA-256 = %s", before.SHA256)
	}
	if after.SHA256 != "5345c9d0a4c0e7414a3a5bc41b34f77149dd5da3730f3a736d2189d8619da0b6" {
		t.Fatalf("after SHA-256 = %s", after.SHA256)
	}
	result := Compare(before, after)
	if result.Status != "available" {
		t.Fatalf("status = %s", result.Status)
	}
	if !reflect.DeepEqual(result.ComparedFields, []Field{FieldEAPI, FieldSlot, FieldIUSE}) {
		t.Fatalf("compared fields = %#v", result.ComparedFields)
	}
	if len(result.Limitations) != 0 {
		t.Fatalf("limitations = %#v", result.Limitations)
	}
	if before.EAPI.Value != "8" || after.EAPI.Value != "8" {
		t.Fatalf("EAPI values = %q, %q", before.EAPI.Value, after.EAPI.Value)
	}
	if before.Slot.Name != "2" || before.Slot.Subslot != "2" || before.Slot.ExplicitSubslot {
		t.Fatalf("before SLOT = %#v", before.Slot)
	}
	if after.Slot.Name != "2" || after.Slot.Subslot != "16" || !after.Slot.ExplicitSubslot {
		t.Fatalf("after SLOT = %#v", after.Slot)
	}
	if result.Changes.Slot != nil || result.Changes.EAPI != nil {
		t.Fatalf("unexpected EAPI/SLOT change: %#v", result.Changes)
	}
	if result.Changes.Subslot == nil || result.Changes.Subslot.Before != "2" || result.Changes.Subslot.After != "16" {
		t.Fatalf("SUBSLOT change = %#v", result.Changes.Subslot)
	}
	if got := names(result.Changes.IUSE.Removed); !reflect.DeepEqual(got, []string{"examples", "lzma"}) {
		t.Fatalf("removed = %#v", got)
	}
	if len(result.Changes.IUSE.Added) != 0 || len(result.Changes.IUSE.DefaultChanges) != 0 {
		t.Fatalf("unexpected IUSE changes = %#v", result.Changes.IUSE)
	}
	assertEvidence(t, result.Changes.Subslot.Evidence, []int{14}, []int{14})
	for _, change := range result.Changes.IUSE.Removed {
		assertEvidence(t, change.Evidence, []int{8}, []int{8})
	}
}

func TestSyntheticNoChangeAndIUSEOrder(t *testing.T) {
	tests := []struct{ name, before, after string }{
		{"same", "EAPI=8\nSLOT=2\nIUSE=foo +bar", "EAPI=8\nSLOT=2\nIUSE=foo +bar"},
		{"reordered", "EAPI=8\nSLOT=2\nIUSE=foo +bar baz", "EAPI=8\nSLOT=2\nIUSE=baz foo +bar"},
		{"implicit explicit equivalent", "EAPI=8\nSLOT=2\nIUSE=", "EAPI=8\nSLOT=2/2\nIUSE="},
	}
	for _, tc := range tests {
		t.Run("synthetic/"+tc.name, func(t *testing.T) {
			result := Compare(normalized(tc.before), normalized(tc.after))
			if result.Status != "available" || len(result.Limitations) != 0 {
				t.Fatalf("result = %#v", result)
			}
			if result.Changes.Slot != nil || result.Changes.Subslot != nil || len(result.Changes.IUSE.Added)+len(result.Changes.IUSE.Removed)+len(result.Changes.IUSE.DefaultChanges) != 0 {
				t.Fatalf("unexpected changes = %#v", result.Changes)
			}
		})
	}
}

func TestSyntheticIUSEChangesAndDefaults(t *testing.T) {
	before := normalized("EAPI=8\nSLOT=0\nIUSE=foo +enabled -disabled -gone")
	after := normalized("EAPI=8\nSLOT=0\nIUSE=+foo -enabled disabled +added")
	result := Compare(before, after)
	if result.Status != "available" {
		t.Fatalf("status = %s", result.Status)
	}
	if got := names(result.Changes.IUSE.Added); !reflect.DeepEqual(got, []string{"added"}) {
		t.Fatalf("added = %#v", got)
	}
	if result.Changes.IUSE.Added[0].Default != DefaultEnabled {
		t.Fatalf("added default = %s", result.Changes.IUSE.Added[0].Default)
	}
	assertEvidence(t, result.Changes.IUSE.Added[0].Evidence, []int{3}, []int{3})
	if got := names(result.Changes.IUSE.Removed); !reflect.DeepEqual(got, []string{"gone"}) {
		t.Fatalf("removed = %#v", got)
	}
	if result.Changes.IUSE.Removed[0].Default != DefaultDisabled {
		t.Fatalf("removed default = %s", result.Changes.IUSE.Removed[0].Default)
	}
	if len(result.Changes.IUSE.DefaultChanges) != 3 || result.Changes.IUSE.DefaultChanges[0].Name != "disabled" || result.Changes.IUSE.DefaultChanges[1].Name != "enabled" || result.Changes.IUSE.DefaultChanges[2].Name != "foo" {
		t.Fatalf("default changes = %#v", result.Changes.IUSE.DefaultChanges)
	}
	if result.Changes.IUSE.DefaultChanges[0].Before != DefaultDisabled || result.Changes.IUSE.DefaultChanges[0].After != DefaultUnspecified {
		t.Fatalf("disabled defaults = %#v", result.Changes.IUSE.DefaultChanges[0])
	}
	if result.Changes.IUSE.DefaultChanges[1].Before != DefaultEnabled || result.Changes.IUSE.DefaultChanges[1].After != DefaultDisabled {
		t.Fatalf("enabled defaults = %#v", result.Changes.IUSE.DefaultChanges[1])
	}
	if result.Changes.IUSE.DefaultChanges[2].Before != DefaultUnspecified || result.Changes.IUSE.DefaultChanges[2].After != DefaultEnabled {
		t.Fatalf("foo defaults = %#v", result.Changes.IUSE.DefaultChanges[2])
	}
	assertEvidence(t, result.Changes.IUSE.DefaultChanges[2].Evidence, []int{3}, []int{3})
}

func TestSyntheticAllIUSEDefaultTransitions(t *testing.T) {
	transitions := []struct {
		before, after         string
		wantBefore, wantAfter IUSEDefault
	}{
		{"flag", "+flag", DefaultUnspecified, DefaultEnabled},
		{"+flag", "flag", DefaultEnabled, DefaultUnspecified},
		{"flag", "-flag", DefaultUnspecified, DefaultDisabled},
		{"-flag", "flag", DefaultDisabled, DefaultUnspecified},
		{"+flag", "-flag", DefaultEnabled, DefaultDisabled},
		{"-flag", "+flag", DefaultDisabled, DefaultEnabled},
	}
	for _, tc := range transitions {
		t.Run("synthetic/"+tc.before+"_to_"+tc.after, func(t *testing.T) {
			before := normalized("EAPI=8\nSLOT=0\nIUSE=" + tc.before)
			after := normalized("EAPI=8\nSLOT=0\nIUSE=" + tc.after)
			result := Compare(before, after)
			if result.Status != StatusAvailable || len(result.Changes.IUSE.DefaultChanges) != 1 {
				t.Fatalf("result = %#v", result)
			}
			change := result.Changes.IUSE.DefaultChanges[0]
			if change.Name != "flag" || change.Before != tc.wantBefore || change.After != tc.wantAfter {
				t.Fatalf("default change = %#v", change)
			}
		})
	}
}

func TestSyntheticMissingEmptyAndInvalidIUSE(t *testing.T) {
	missing := normalized("EAPI=8\nSLOT=0\n")
	empty := normalized("EAPI=8\nSLOT=0\nIUSE=\n")
	result := Compare(missing, empty)
	if result.Status != "partial" || !reflect.DeepEqual(result.ComparedFields, []Field{FieldEAPI, FieldSlot}) {
		t.Fatalf("missing versus empty result = %#v", result)
	}
	if len(result.Changes.IUSE.Removed) != 0 || len(result.Changes.IUSE.Added) != 0 {
		t.Fatalf("missing IUSE treated as change: %#v", result.Changes.IUSE)
	}
	if !reflect.DeepEqual(result.Limitations, []Limitation{{Code: "IUSE_MISSING", Field: FieldIUSE, Side: SideBefore, Evidence: []EvidenceRef{{Side: SideBefore, Field: FieldIUSE, WholeSnapshot: true}}}}) {
		t.Fatalf("limitations = %#v", result.Limitations)
	}
	ref := evidenceRef(SideBefore, FieldIUSE, nil)
	if !ref.WholeSnapshot || len(ref.Lines) != 0 {
		t.Fatalf("missing-key evidence = %#v", ref)
	}

	for _, tc := range []struct{ name, iuse string }{
		{"bad name", "IUSE=foo/bar"},
		{"conflicting defaults", "IUSE=foo +foo"},
		{"conflicting repeated keys", "IUSE=foo\nIUSE=bar"},
	} {
		t.Run("synthetic/"+tc.name, func(t *testing.T) {
			snapshot := normalized("EAPI=8\nSLOT=0\n" + tc.iuse)
			if snapshot.IUSE.State != StateInvalid || len(snapshot.IUSE.Flags) != 0 {
				t.Fatalf("IUSE = %#v", snapshot.IUSE)
			}
			compared := Compare(snapshot, empty)
			if compared.Status != "partial" || !reflect.DeepEqual(compared.ComparedFields, []Field{FieldEAPI, FieldSlot}) {
				t.Fatalf("result = %#v", compared)
			}
		})
	}
	conflicting := normalized("EAPI=8\nSLOT=0\nIUSE=foo +foo")
	if len(conflicting.Diagnostics) != 1 || conflicting.Diagnostics[0].Code != "IUSE_CONFLICTING_DEFAULTS" || !reflect.DeepEqual(conflicting.Diagnostics[0].Lines, []int{3}) {
		t.Fatalf("conflicting-default diagnostic = %#v", conflicting.Diagnostics)
	}
}

func TestSyntheticDuplicateAndMalformedSelectedKeys(t *testing.T) {
	same := normalized("EAPI=8\nEAPI=8\nSLOT=2/16\nSLOT=2/16\nIUSE=foo\nIUSE=foo\n")
	if same.EAPI.State != StateValid || same.Slot.State != StateValid || same.IUSE.State != StateValid || len(same.Diagnostics) != 0 {
		t.Fatalf("same duplicates = %#v", same)
	}
	if !reflect.DeepEqual(same.EAPI.Lines, []int{1, 2}) || !reflect.DeepEqual(same.IUSE.Flags[0].Declarations, []IUSEDeclaration{{Token: "foo", Line: 5}, {Token: "foo", Line: 6}}) {
		t.Fatalf("duplicate evidence lost: %#v", same)
	}
	conflict := normalized("EAPI=8\nEAPI=7\nSLOT=2\nSLOT=3\nIUSE=foo")
	if conflict.EAPI.State != StateInvalid || conflict.Slot.State != StateInvalid {
		t.Fatalf("conflicts not invalid: %#v", conflict)
	}
	if !reflect.DeepEqual(conflict.EAPI.Lines, []int{1, 2}) || !reflect.DeepEqual(conflict.Slot.Lines, []int{3, 4}) {
		t.Fatalf("conflict lines = %#v / %#v", conflict.EAPI.Lines, conflict.Slot.Lines)
	}
	for _, raw := range []string{"", "/2", "2/", "2/3/4", ".2", "-2", "+2", "2 3", "2\t"} {
		t.Run("synthetic/SLOT="+raw, func(t *testing.T) {
			got := normalized("EAPI=8\nSLOT=" + raw + "\nIUSE=")
			if got.Slot.State != StateInvalid {
				t.Fatalf("SLOT state = %s", got.Slot.State)
			}
		})
	}
}

func TestSyntheticEAPIUnavailableBlocksAllFields(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        FieldState
		code        string
	}{
		{"missing", "", StateMissing, "EAPI_MISSING"},
		{"invalid", "8/", StateInvalid, "EAPI_INVALID"},
		{"unsupported", "9", StateValid, "EAPI_UNSUPPORTED"},
	} {
		t.Run("synthetic/"+tc.name, func(t *testing.T) {
			content := "SLOT=bad\nIUSE=bad/token\n"
			if tc.name != "missing" {
				content = "EAPI=" + tc.value + "\n" + content
			}
			result := Compare(normalized(content), normalized("EAPI=8\nSLOT=0\nIUSE=foo"))
			if result.Status != "unavailable" || len(result.ComparedFields) != 0 || result.Changes.Slot != nil || result.Changes.Subslot != nil || result.Changes.EAPI != nil || len(result.Changes.IUSE.Added)+len(result.Changes.IUSE.Removed)+len(result.Changes.IUSE.DefaultChanges) != 0 {
				t.Fatalf("result = %#v", result)
			}
			if len(result.Limitations) != 1 || result.Limitations[0].Code != tc.code {
				t.Fatalf("limitations = %#v", result.Limitations)
			}
			if got := normalized(content).EAPI.State; got != tc.want {
				t.Fatalf("EAPI state = %s", got)
			}
			unsupported := normalized(content)
			if unsupported.Slot.State == StateValid || unsupported.IUSE.State == StateValid {
				t.Fatalf("non-EAPI-8 fields became trusted: SLOT=%#v IUSE=%#v", unsupported.Slot, unsupported.IUSE)
			}
		})
	}
}

func TestSyntheticPartialAndOnlyEAPI(t *testing.T) {
	valid := normalized("EAPI=8\nSLOT=0\nIUSE=foo")
	badSlot := normalized("EAPI=8\nSLOT=/0\nIUSE=bar")
	partial := Compare(badSlot, valid)
	if partial.Status != "partial" || !reflect.DeepEqual(partial.ComparedFields, []Field{FieldEAPI, FieldIUSE}) {
		t.Fatalf("partial = %#v", partial)
	}
	if len(partial.Limitations) != 1 || partial.Limitations[0].Side != SideBefore {
		t.Fatalf("partial limitations = %#v", partial.Limitations)
	}
	onlyEAPI := Compare(normalized("EAPI=8\nSLOT=/0\n"), normalized("EAPI=8\nSLOT=\n"))
	if onlyEAPI.Status != "partial" || !reflect.DeepEqual(onlyEAPI.ComparedFields, []Field{FieldEAPI}) {
		t.Fatalf("only EAPI = %#v", onlyEAPI)
	}
}

func TestSyntheticIdentityAndProvenanceGate(t *testing.T) {
	base := normalized("EAPI=8\nSLOT=0\nIUSE=")
	other := normalized("EAPI=8\nSLOT=1\nIUSE=foo")
	other.Source.Name = "different"
	for _, tc := range []struct {
		name          string
		before, after Snapshot
		code          string
	}{
		{"package", base, other, "PACKAGE_MISMATCH"},
		{"repository", base, func() Snapshot {
			s := other
			s.Source.Name = base.Source.Name
			s.Source.Repository = "overlay"
			return s
		}(), "REPOSITORY_MISMATCH"},
	} {
		t.Run("synthetic/"+tc.name, func(t *testing.T) {
			result := Compare(tc.before, tc.after)
			if result.Status != "unavailable" || len(result.ComparedFields) != 0 || result.Changes.Slot != nil || len(result.Changes.IUSE.Added) != 0 {
				t.Fatalf("result = %#v", result)
			}
			if len(result.Limitations) != 1 || result.Limitations[0].Code != tc.code {
				t.Fatalf("limitations = %#v", result.Limitations)
			}
		})
	}
	for _, field := range []string{"Category", "Name", "Version", "Repository", "SourceURL", "Commit", "CachePath"} {
		missing := base
		switch field {
		case "Category":
			missing.Source.Category = ""
		case "Name":
			missing.Source.Name = ""
		case "Version":
			missing.Source.Version = ""
		case "Repository":
			missing.Source.Repository = ""
		case "SourceURL":
			missing.Source.SourceURL = ""
		case "Commit":
			missing.Source.Commit = ""
		case "CachePath":
			missing.Source.CachePath = ""
		}
		result := Compare(missing, base)
		if result.Status != StatusUnavailable || len(result.ComparedFields) != 0 || len(result.Limitations) != 1 || result.Limitations[0].Code != "SOURCE_MISSING" || !result.Limitations[0].Evidence[0].WholeSnapshot {
			t.Fatalf("missing %s source = %#v", field, result)
		}
	}
	newCommit := base
	newCommit.Source.Commit = "different-commit"
	if got := Compare(base, newCommit); got.Status != "available" {
		t.Fatalf("different commit status = %s", got.Status)
	}
}

func TestSyntheticInputsUnchangedAndResultDeterministic(t *testing.T) {
	content := "IUSE=zed +alpha\tminus\nEAPI=8\r\nSLOT=2\nOTHER=a=b\n"
	source := testSource()
	contentCopy, sourceCopy := content, source
	first := Normalize(source, content)
	second := Normalize(source, content)
	if content != contentCopy || source != sourceCopy {
		t.Fatal("Normalize changed an input")
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Normalize is not deterministic:\n%#v\n%#v", first, second)
	}
	if first.Content != content || first.SHA256 == "" {
		t.Fatal("content was altered or hash missing")
	}
	if first.EAPI.State != StateInvalid {
		t.Fatal("CR was stripped from EAPI value")
	}
	left, right := normalized("EAPI=8\nSLOT=1\nIUSE=z a"), normalized("EAPI=8\nSLOT=2\nIUSE=b a")
	leftCopy, rightCopy := left, right
	got1, got2 := Compare(left, right), Compare(left, right)
	if !reflect.DeepEqual(got1, got2) {
		t.Fatal("Compare is not deterministic")
	}
	if !reflect.DeepEqual(left, leftCopy) || !reflect.DeepEqual(right, rightCopy) {
		t.Fatal("Compare changed an input")
	}
}

func TestSyntheticCacheSplitsOnFirstEquals(t *testing.T) {
	snapshot := normalized("OTHER=a=b\nEAPI=8=tail\nSLOT=0\nIUSE=foo\n")
	if snapshot.Content != "OTHER=a=b\nEAPI=8=tail\nSLOT=0\nIUSE=foo\n" || snapshot.EAPI.State != StateInvalid {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if !strings.Contains(snapshot.Content, "OTHER=a=b") {
		t.Fatal("unselected source row was lost")
	}
}

func names(changes []IUSEFlagChange) []string {
	out := make([]string, len(changes))
	for i, change := range changes {
		out[i] = change.Name
	}
	return out
}

func assertEvidence(t *testing.T, got EvidencePair, beforeLines, afterLines []int) {
	t.Helper()
	if got.Before.Side != SideBefore || got.After.Side != SideAfter {
		t.Fatalf("evidence sides = %#v", got)
	}
	if !reflect.DeepEqual(got.Before.Lines, beforeLines) || !reflect.DeepEqual(got.After.Lines, afterLines) {
		t.Fatalf("evidence lines = %#v, want %v / %v", got, beforeLines, afterLines)
	}
	if got.Before.WholeSnapshot != (len(beforeLines) == 0) || got.After.WholeSnapshot != (len(afterLines) == 0) {
		t.Fatalf("whole snapshot evidence = %#v", got)
	}
}
