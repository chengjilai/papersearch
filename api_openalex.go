package main

import (
	"sort"
	"strings"
)

// OpenAlex REST API. Largest open scholarly catalog; full-text-aware relevance search.

type openAlexResp struct {
	Results []struct {
		Title           string `json:"display_name"`
		PublicationYear int    `json:"publication_year"`
		DOI             string `json:"doi"`
		CitedByCount    int    `json:"cited_by_count"`
		Authorships     []struct {
			Author struct {
				DisplayName string `json:"display_name"`
			} `json:"author"`
		} `json:"authorships"`
		PrimaryLocation struct {
			LandingPageURL string `json:"landing_page_url"`
			Source         struct {
				DisplayName string `json:"display_name"`
			} `json:"source"`
		} `json:"primary_location"`
		AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
	} `json:"results"`
}

func searchOpenAlex(q string, n int, since int) ([]Result, error) {
	endpoint := "https://api.openalex.org/works?search=" + qval(q) +
		"&per-page=" + itoa(n) + "&mailto=" + contactEmail +
		"&select=id,doi,display_name,publication_year,authorships,cited_by_count,primary_location,abstract_inverted_index"
	if since > 0 {
		endpoint += "&filter=from_publication_date:" + itoa(since) + "-01-01"
	}

	var resp openAlexResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Results))
	for _, w := range resp.Results {
		r := Result{
			Source:    "openalex",
			Title:     w.Title,
			Year:      w.PublicationYear,
			Citations: w.CitedByCount,
			DOI:       strings.TrimPrefix(w.DOI, "https://doi.org/"),
		}
		if w.PrimaryLocation.LandingPageURL != "" {
			r.URL = w.PrimaryLocation.LandingPageURL
		}
		r.Venue = w.PrimaryLocation.Source.DisplayName
		for _, as := range w.Authorships {
			r.Authors = append(r.Authors, as.Author.DisplayName)
		}
		r.Abstract = reconstructInverted(w.AbstractInvertedIndex)
		results = append(results, r)
	}
	return results, nil
}

// reconstructInverted rebuilds an abstract from OpenAlex's word->positions map.
func reconstructInverted(ix map[string][]int) string {
	if len(ix) == 0 {
		return ""
	}
	total := 0
	for _, pos := range ix {
		for _, p := range pos {
			if p >= total {
				total = p + 1
			}
		}
	}
	words := make([]string, total)
	for w, pos := range ix {
		for _, p := range pos {
			if p < total {
				words[p] = w
			}
		}
	}
	joined := strings.Join(words, " ")
	// words can contain underscores from NLP tokenization; leave as-is but collapse whitespace
	return strings.Join(strings.Fields(joined), " ")
}

var _ = sort.Ints // keep sort import balanced if rules change
