package planservice

import (
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analyzer"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
)

type rebuildLookup struct {
	mu        sync.Mutex
	calls     map[metadataprovider.Query]int
	snapshots map[metadataprovider.Query]metadatadiff.Snapshot
}

func (p *rebuildLookup) Lookup(q metadataprovider.Query) (metadataprovider.LookupResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls[q]++
	snapshot, ok := p.snapshots[q]
	if !ok {
		return metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}, nil
	}
	return metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: &snapshot}, nil
}
func rebuildProvider(plan parser.Result) *rebuildLookup {
	p := &rebuildLookup{calls: map[metadataprovider.Query]int{}, snapshots: map[metadataprovider.Query]metadatadiff.Snapshot{}}
	for _, e := range plan.Entries {
		add := func(version string, repo, slot, sub *string) {
			q := metadataprovider.Query{Repository: *repo, Category: e.Package.Category, Name: e.Package.Name, Version: version}
			s := "0"
			if slot != nil {
				s = *slot
			}
			if sub != nil {
				s += "/" + *sub
			}
			p.snapshots[q] = metadatadiff.Normalize(metadatadiff.Source{Repository: q.Repository, Category: q.Category, Name: q.Name, Version: q.Version}, "EAPI=8\nSLOT="+s+"\nIUSE=foo\n")
		}
		add(e.Target.Version, e.Target.Repository, e.Target.Slot, e.Target.Subslot)
		for _, v := range e.Installed.Versions {
			add(v.Version, v.Repository, v.Slot, v.Subslot)
		}
	}
	return p
}

func TestRebuildFixtureServicesAndProgress(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/parser/ebuild-updates-3.txt")
	if err != nil {
		t.Fatal(err)
	}
	input := string(data)
	plan := parser.Parse(input)
	if plan.Status != parser.StatusParsed || len(plan.Entries) != 43 {
		t.Fatal(plan)
	}
	count, err := LookupUnitCount(plan)
	if err != nil || count != 79 {
		t.Fatalf("units=%d err=%v", count, err)
	}
	var reference Result
	for _, mode := range []string{"serial", "parallel", "progressive", "duplicates"} {
		t.Run(mode, func(t *testing.T) {
			pplan := plan
			if mode == "duplicates" {
				pplan.Entries = append(append([]parser.Entry{}, plan.Entries...), plan.Entries[10], plan.Entries[0])
			}
			provider := rebuildProvider(pplan)
			var progress []int
			onProgress := func(c, total int) {
				if total != 79 {
					t.Errorf("total=%d", total)
				}
				progress = append(progress, c)
			}
			var got Result
			var err error
			switch mode {
			case "serial":
				got, err = AnalyzeParsedWithProgress(input, pplan, analyzer.Context{}, provider, onProgress)
			case "parallel":
				got, err = AnalyzeParsedParallelWithProgress(input, pplan, analyzer.Context{}, provider, onProgress)
			default:
				last := -1
				got, err = AnalyzeParsedProgressive(input, pplan, analyzer.Context{}, provider, 2, func(c, total int, partial *Result) {
					if c != last {
						onProgress(c, total)
						last = c
					}
					if partial != nil {
						previous := -1
						for _, r := range partial.Reports {
							if r.Report.EntryIndex <= previous {
								t.Error("partial reports out of order")
							}
							previous = r.Report.EntryIndex
							if r.AfterLookup.Result == nil {
								t.Error("published before target lookup")
							}
						}
					}
				})
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Analysis.Findings) != 0 {
				t.Fatalf("unexpected findings: %+v", got.Analysis.Findings)
			}
			if len(got.Reports) != len(pplan.Entries) || len(provider.calls) != 79 {
				t.Fatalf("reports=%d calls=%d", len(got.Reports), len(provider.calls))
			}
			for q, n := range provider.calls {
				if n != 1 {
					t.Errorf("query %v called %d", q, n)
				}
			}
			if len(progress) != 80 {
				t.Fatalf("progress=%v", progress)
			}
			for i, c := range progress {
				if c != i {
					t.Errorf("progress[%d]=%d", i, c)
				}
			}
			comparisons := 0
			for i, r := range got.Reports {
				if r.Report.EntryIndex != i || r.AfterLookup.Query == nil || r.AfterLookup.Query.Version != pplan.Entries[i].Target.Version {
					t.Errorf("report %d: %+v", i, r)
				}
				if pplan.Entries[i].Operation == parser.OperationReinstall {
					if r.BeforeLookup.Query != nil || r.BeforeLookup.Result != nil || r.Report.Gentoo.Comparison != nil || r.BeforeLookup.SkipReason == "" {
						t.Fatalf("invented before %d: %+v", i, r)
					}
				} else {
					if r.Report.Gentoo.Comparison == nil {
						t.Errorf("missing comparison %d: %+v", i, r.Report.Gentoo)
					}
					comparisons++
				}
			}
			expected := 36
			if mode == "duplicates" {
				expected++
			}
			if comparisons != expected {
				t.Errorf("comparisons=%d", comparisons)
			}
			if mode == "serial" {
				reference = got
			} else if mode != "duplicates" && !reflect.DeepEqual(got, reference) {
				t.Error("serial and parallel results differ")
			}
		})
	}
}
