package metadatacache

import (
	"strings"
	"testing"
)

func sample() Inputs {
	return Inputs{Repository: "guru", SourceURL: "https://example.invalid/guru", Category: "dev-libs", Name: "widget", Version: "1.0",
		ReferenceCommit: strings.Repeat("a", 40), SourceCommit: strings.Repeat("b", 40), EbuildBlob: strings.Repeat("c", 40),
		Masters: []Master{{Name: "gentoo", Commit: strings.Repeat("d", 40)}, {Name: "other", Commit: strings.Repeat("e", 40)}},
		Profile: "gentoo:profiles/test", GeneratorVersion: "v1", PortageVersion: "3.0"}
}

func TestKeyDistinguishesReproducibilityInputs(t *testing.T) {
	base := sample()
	want, err := Key(base)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(*Inputs)
	}{
		{"repository", func(v *Inputs) { v.Repository = "other-repo" }},
		{"source", func(v *Inputs) { v.SourceURL += "/other" }},
		{"category", func(v *Inputs) { v.Category = "app-misc" }},
		{"name", func(v *Inputs) { v.Name = "other" }},
		{"version", func(v *Inputs) { v.Version = "1.1" }},
		{"reference", func(v *Inputs) { v.ReferenceCommit = strings.Repeat("f", 40) }},
		{"selected", func(v *Inputs) { v.SourceCommit = strings.Repeat("f", 40) }},
		{"blob", func(v *Inputs) { v.EbuildBlob = strings.Repeat("f", 40) }},
		{"master", func(v *Inputs) { v.Masters[0].Commit = strings.Repeat("f", 40) }},
		{"profile", func(v *Inputs) { v.Profile += "/other" }},
		{"generator", func(v *Inputs) { v.GeneratorVersion = "v2" }},
		{"portage", func(v *Inputs) { v.PortageVersion = "3.1" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := sample()
			tc.change(&input)
			got, err := Key(input)
			if err != nil || got == want {
				t.Fatalf("key = %q, %v", got, err)
			}
		})
	}
	reordered := sample()
	reordered.Masters[0], reordered.Masters[1] = reordered.Masters[1], reordered.Masters[0]
	if got, err := Key(reordered); err != nil || got == want {
		t.Fatalf("reordered key = %q, %v", got, err)
	}
	if base.Masters[0].Name != "gentoo" {
		t.Fatal("Key mutated input")
	}
}

func TestKeyRejectsIncompleteAndDuplicateMasters(t *testing.T) {
	input := sample()
	input.PortageVersion = ""
	if _, err := Key(input); err == nil {
		t.Fatal("missing version accepted")
	}
	input = sample()
	input.Masters = append(input.Masters, input.Masters[0])
	if _, err := Key(input); err == nil {
		t.Fatal("duplicate master accepted")
	}
	input = sample()
	input.EbuildBlob = "short"
	if _, err := Key(input); err == nil {
		t.Fatal("short blob accepted")
	}
}
