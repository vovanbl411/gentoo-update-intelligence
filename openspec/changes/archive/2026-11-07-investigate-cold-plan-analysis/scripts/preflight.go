package main

import (
	"encoding/json"
	"fmt"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/ebuildresolver"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
	"os"
)

func main() {
	if len(os.Args) == 8 && os.Args[1] == "resolve" {
		r, e := ebuildresolver.Open(os.Args[2])
		if e != nil {
			panic(e)
		}
		v, e := r.Resolve(ebuildresolver.Input{Reference: os.Args[3], Repository: os.Args[4], Category: os.Args[5], Name: os.Args[6], Version: os.Args[7]})
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		json.NewEncoder(os.Stdout).Encode(v)
		return
	}
	b, e := os.ReadFile(os.Args[1])
	if e != nil {
		panic(e)
	}
	p := parser.Parse(string(b))
	qs := []metadataprovider.Query{}
	seen := map[metadataprovider.Query]bool{}
	rebuilds := 0
	for i, e := range p.Entries {
		if e.Operation == parser.OperationReinstall && len(e.Installed.Versions) == 0 {
			rebuilds++
		}
		q, err := reportservice.LookupQueries(p, i)
		if err != nil {
			panic(err)
		}
		for _, v := range q {
			if !seen[v] {
				qs = append(qs, v)
				seen[v] = true
			}
		}
	}
	json.NewEncoder(os.Stdout).Encode(struct {
		Status             parser.Status
		Entries, AfterOnly int
		Queries            []metadataprovider.Query
	}{p.Status, len(p.Entries), rebuilds, qs})
}
