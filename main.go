package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Free (no-key) providers: the 9 explicit ones plus the 6 data-driven
// docProviders registered in init() from api_rest.go.
var freeProviders = []provider{
	{name: "arxiv", label: "arXiv", search: searchArxiv},
	{name: "scholar", label: "Semantic Scholar", search: searchScholar},
	{name: "pubmed", label: "PubMed", search: searchPubMed},
	{name: "dblp", label: "DBLP", search: searchDblp},
	{name: "openaire", label: "OpenAIRE (EU research)", search: searchOpenAIRE},
}

func init() {
	for _, d := range docProviders {
		freeProviders = append(freeProviders, provider{name: d.name, label: d.label, search: docSearch(d)})
	}
}

func main() {
	var (
		apiList = flag.String("api", "arxiv,crossref,openalex,scholar,pubmed,europepmc,dblp,datacite,doaj,hal,inspire,zenodo,osf,openaire,dryad",
			"comma-separated providers, or 'all'. Names: "+providerNames())
		n       = flag.Int("n", 5, "max results per provider")
		since   = flag.Int("since", 0, "only papers from this year onward")
		timeout = flag.Duration("timeout", 20*time.Second, "per-request timeout")
		jsonOut = flag.Bool("json", false, "emit machine-readable JSON")
		quiet   = flag.Bool("q", false, "suppress per-provider error notes")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "papersearch — search scholarly literature across many public APIs.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: papersearch [flags] \"search phrase\"\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nProviders:\n")
		for _, p := range freeProviders {
			fmt.Fprintf(os.Stderr, "  %-10s %s\n", p.name, p.label)
		}
		for _, p := range allKeyedProviders {
			fmt.Fprintf(os.Stderr, "  %-10s %s (keyed: PAPERSEARCH_<NAME>_KEY)\n", p.name, p.label)
		}
	}
	flag.Parse()

	httpClient.Timeout = *timeout

	query := strings.Join(flag.Args(), " ")
	if query == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *n < 1 || *n > 50 {
		fmt.Fprintln(os.Stderr, "papersearch: -n must be between 1 and 50")
		os.Exit(2)
	}

	provs, skipped := selectProviders(*apiList)

	var wg sync.WaitGroup
	results := make([][]Result, len(provs))
	errors := make([]error, len(provs))
	sem := make(chan struct{}, 3) // max 3 concurrent outbound requests
	for i, p := range provs {
		wg.Add(1)
		go func(i int, p provider) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i], errors[i] = p.search(query, *n, *since)
		}(i, p)
	}
	wg.Wait()

	var all []Result
	totalErr := 0
	for i, p := range provs {
		if errors[i] != nil {
			totalErr++
			if !*quiet {
				fmt.Fprintf(os.Stderr, "warning: %s: %v\n", p.label, errors[i])
			}
			continue
		}
		all = append(all, results[i]...)
	}

	for _, s := range skipped {
		fmt.Fprintf(os.Stderr, "skipped: %s (set the env key)\n", s)
	}

	if len(all) == 0 {
		if len(provs) == totalErr {
			fmt.Fprintln(os.Stderr, "papersearch: all providers failed (see warnings above)")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "no results found")
		return
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(all)
		return
	}

	printTable(provs, results, errors)
}

func selectProviders(req string) (selected []provider, skipped []string) {
	all := make(map[string]provider)
	for _, p := range freeProviders {
		all[p.name] = p
	}
	for _, p := range allKeyedProviders { // registered regardless of env presence
		if _, exists := all[p.name]; !exists {
			all[p.name] = p
		}
	}

	if req == "all" {
		for _, name := range providerNameList() {
			if p, ok := all[name]; ok && (!p.keyed || haveKey(name)) {
				selected = append(selected, p)
			}
		}
		return selected, nil
	}

	seen := map[string]bool{}
	for _, name := range strings.Split(req, ",") {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if p, ok := all[name]; ok {
			if p.keyed {
				if haveKey(name) {
					selected = append(selected, p)
				} else {
					skipped = append(skipped, name)
				}
			} else {
				selected = append(selected, p)
			}
		} else {
			fmt.Fprintf(os.Stderr, "unknown provider %q (see -h)\n", name)
		}
	}
	return selected, skipped
}

func haveKey(name string) bool {
	switch name {
	case "core":
		return keyCORE() != ""
	case "ads":
		return keyADS() != ""
	case "lens":
		return keyLens() != ""
	case "springer":
		return keySpringer() != ""
	case "ieee":
		return keyIEEE() != ""
	case "gscholar":
		return keySerpAPI() != ""
	}
	return false
}

// providerNameList derives the sorted provider names from the registries
// (single source of truth — no hardcoded duplicate list to keep in sync).
func providerNameList() []string {
	names := make([]string, 0, len(freeProviders)+len(allKeyedProviders))
	for _, p := range freeProviders {
		names = append(names, p.name)
	}
	for _, p := range allKeyedProviders {
		names = append(names, p.name)
	}
	sort.Strings(names)
	return names
}

func providerNames() string {
	return strings.Join(providerNameList(), ",")
}

func printTable(provs []provider, results [][]Result, errors []error) {
	for i, p := range provs {
		if errors[i] != nil || len(results[i]) == 0 {
			continue
		}
		fmt.Printf("\n=== %s (%d) ===\n", p.label, len(results[i]))
		for j, r := range results[i] {
			fmt.Printf("  %2d. %s\n", j+1, r.Title)
			meta := ""
			if len(r.Authors) > 0 {
				meta += fmtAuthors(r.Authors, 6)
			}
			if r.Year != 0 {
				meta += fmt.Sprintf(" · %s", r.yearOrNA())
			}
			if r.Venue != "" {
				meta += fmt.Sprintf(" · %s", truncate(r.Venue, 60))
			}
			if r.Citations > 0 {
				meta += fmt.Sprintf(" · %d cites", r.Citations)
			}
			if meta != "" {
				fmt.Printf("      %s\n", truncate(meta, 120))
			}
			if r.URL != "" {
				fmt.Printf("      %s\n", r.URL)
			} else if r.DOI != "" {
				fmt.Printf("      https://doi.org/%s\n", r.DOI)
			}
		}
	}
}
