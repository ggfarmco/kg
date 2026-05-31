package main

import (
	"strconv"
)

func disambiguateDecls(decls []Decl) {
	counts := make(map[string]int, len(decls))
	for i := range decls {
		counts[decls[i].NameSlug]++
	}
	used := make(map[string]struct{}, len(decls))
	for slug, n := range counts {
		if n == 1 {
			used[slug] = struct{}{}
		}
	}
	for i := range decls {
		base := decls[i].NameSlug
		if counts[base] < 2 {
			continue
		}
		cand := base
		if kind, _ := decls[i].Properties["kind"].(string); kind != "" {
			cand = base + "-" + kind
		}
		final := cand
		for n := 2; ; n++ {
			if _, taken := used[final]; !taken {
				break
			}
			final = cand + "-" + strconv.Itoa(n)
		}
		used[final] = struct{}{}
		decls[i].NameSlug = final
	}
}
