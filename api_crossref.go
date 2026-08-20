package main

import "strings"

// Crossref REST API. DOI metadata for the whole DOI ecosystem.

type crossrefResp struct {
	Message struct {
		Items []struct {
			Title  []string `json:"title"`
			Author []struct {
				Given  string `json:"given"`
				Family string `json:"family"`
				Name   string `json:"name"`
			} `json:"author"`
			DOI            string   `json:"DOI"`
			ContainerTitle []string `json:"container-title"`
			Published      struct {
				DateParts [][]int `json:"date-parts"`
			} `json:"published"`
			Type string `json:"type"`
		} `json:"items"`
	} `json:"message"`
}

func searchCrossref(q string, n int, since int) ([]Result, error) {
	endpoint := "https://api.crossref.org/works?query=" + qval(q) +
		"&rows=" + itoa(n) + "&mailto=" + contactEmail +
		"&select=DOI,title,author,container-title,published,type"
	if since > 0 {
		endpoint += "&filter=from-pub-date:" + itoa(since) + "-01-01"
	}

	var resp crossrefResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Message.Items))
	for _, it := range resp.Message.Items {
		r := Result{Source: "crossref", DOI: it.DOI, URL: "https://doi.org/" + it.DOI}
		if len(it.Title) > 0 {
			r.Title = it.Title[0]
		}
		if len(it.ContainerTitle) > 0 {
			r.Venue = it.ContainerTitle[0]
		}
		for _, a := range it.Author {
			name := a.Name
			if name == "" {
				name = strings.TrimSpace(a.Given + " " + a.Family)
			}
			if name != "" {
				r.Authors = append(r.Authors, name)
			}
		}
		if len(it.Published.DateParts) > 0 && len(it.Published.DateParts[0]) > 0 {
			r.Year = it.Published.DateParts[0][0]
		}
		results = append(results, r)
	}
	return results, nil
}
