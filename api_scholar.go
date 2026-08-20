package main

import "strings"

// Semantic Scholar Graph API. AI-ranked citations; aggressive rate limits.

type s2Resp struct {
	Data []struct {
		PaperID string `json:"paperId"`
		Title   string `json:"title"`
		Authors []struct {
			Name string `json:"name"`
		} `json:"authors"`
		Year          int    `json:"year"`
		Venue         string `json:"venue"`
		CitationCount int    `json:"citationCount"`
		ExternalIDs   struct {
			DOI string `json:"DOI"`
		} `json:"externalIds"`
		URL      string `json:"url"`
		Abstract string `json:"abstract"`
	} `json:"data"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

func searchScholar(q string, n int, since int) ([]Result, error) {
	endpoint := "https://api.semanticscholar.org/graph/v1/paper/search?query=" +
		qval(q) + "&limit=" + itoa(n) +
		"&fields=title,authors,year,venue,citationCount,externalIds,url,abstract"
	if since > 0 {
		endpoint += "&year=" + itoa(since) + "-" + itoa(since+1000)
	}

	var resp s2Resp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}
	if resp.Code == "429" || strings.Contains(resp.Message, "Too Many Requests") {
		return nil, errRateLimited
	}

	results := make([]Result, 0, len(resp.Data))
	for _, p := range resp.Data {
		r := Result{
			Source:    "semanticscholar",
			Title:     p.Title,
			Year:      p.Year,
			Venue:     p.Venue,
			Citations: p.CitationCount,
			DOI:       p.ExternalIDs.DOI,
			URL:       p.URL,
			Abstract:  p.Abstract,
		}
		if r.URL == "" && r.DOI != "" {
			r.URL = "https://doi.org/" + r.DOI
		}
		for _, a := range p.Authors {
			r.Authors = append(r.Authors, a.Name)
		}
		results = append(results, r)
	}
	return results, nil
}
