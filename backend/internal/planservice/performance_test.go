package planservice

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analyzer"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
)

type delayedLookup struct{}

func (delayedLookup) Lookup(metadataprovider.Query) (metadataprovider.LookupResult, error) {
	time.Sleep(5 * time.Millisecond)
	return metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}, nil
}

func BenchmarkLargePlan(b *testing.B) {
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, fmt.Sprintf("[ebuild U] dev-libs/pkg%d-2::gentoo [1::gentoo]", i))
	}
	input := strings.Join(lines, "\n")
	plan := parser.Parse(input)
	for name, analyze := range map[string]func(string, parser.Result, analyzer.Context, reportservice.MetadataLookup, func(int, int)) (Result, error){
		"serial":   AnalyzeParsedWithProgress,
		"parallel": AnalyzeParsedParallelWithProgress,
	} {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := analyze(input, plan, analyzer.Context{}, delayedLookup{}, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type concurrentLookup struct {
	mu           sync.Mutex
	active, peak int
	calls        map[metadataprovider.Query]int
}

func (p *concurrentLookup) Lookup(q metadataprovider.Query) (metadataprovider.LookupResult, error) {
	p.mu.Lock()
	p.calls[q]++
	p.active++
	if p.active > p.peak {
		p.peak = p.active
	}
	p.mu.Unlock()
	time.Sleep(10 * time.Millisecond)
	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	if q.Version == "1" {
		return metadataprovider.LookupResult{}, fmt.Errorf("lookup failed")
	}
	return metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}, nil
}

func TestParallelAnalysisPreservesResultsAndProgress(t *testing.T) {
	input := "[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]\n[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]\n[ebuild U] dev-libs/other-3::gentoo [2::gentoo]"
	plan := parser.Parse(input)
	serial := &concurrentLookup{calls: make(map[metadataprovider.Query]int)}
	want, err := AnalyzeParsedWithProgress(input, plan, analyzer.Context{}, serial, nil)
	if err != nil {
		t.Fatal(err)
	}
	parallel := &concurrentLookup{calls: make(map[metadataprovider.Query]int)}
	var progress []int
	got, err := AnalyzeParsedParallelWithProgress(input, plan, analyzer.Context{}, parallel, func(completed, total int) {
		if total != 4 {
			t.Errorf("total = %d, want 4", total)
		}
		progress = append(progress, completed)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("parallel result differs from serial result")
	}
	if !reflect.DeepEqual(progress, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("progress = %v", progress)
	}
	if parallel.peak != 2 {
		t.Fatalf("peak concurrent lookups = %d", parallel.peak)
	}
	for query, count := range parallel.calls {
		if count != 1 {
			t.Errorf("query %v called %d times", query, count)
		}
	}
}
