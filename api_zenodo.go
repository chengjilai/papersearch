package main

import "strings"

// Zenodo API. CERN-hosted research data repository (datasets, software, preprints).

type zenodoResp struct {
	Hits struct {
		Hits []struct {
			ID    int    `json:"id"`
			DOI   string `json:"doi"`
			Links struct {
				Self string `json:"self"`
				DOI  string `json:"doi"`
			} `json:"links"`
			Metadata struct {
				Title           string `json:"title"`
				PublicationDate string `json:"publication_date"`
				Creators        []struct {
					Name string `json:"name"`
				} `json:"creators"`
				Description  string `json:"description"`
				ResourceType struct {
					Title string `json:"title"`
				} `json:"resource_type"`
			} `json:"metadata"`
		} `json:"hits"`
	} `json:"hits"`
}

func searchZenodo(q string, n int, since int) ([]Result, error) {
	query := q
	if since > 0 {
		query = "publication_date:[" + itoa(since) + "-01-01 TO *] AND (" + q + ")"
	}
	endpoint := "https://zenodo.org/api/records?q=" + qval(query) + "&size=" + itoa(n) +
		"&sort=mostrecent"

	var resp zenodoResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Hits.Hits))
	for _, h := range resp.Hits.Hits {
		r := Result{
			Source:   "zenodo",
			Title:    h.Metadata.Title,
			DOI:      h.DOI,
			Venue:    h.Metadata.ResourceType.Title,
			Abstract: stripHTML(h.Metadata.Description),
		}
		if len(h.Metadata.PublicationDate) >= 4 {
			if y, ok := atoiOK(h.Metadata.PublicationDate[:4]); ok {
				r.Year = y
			}
		}
		for _, c := range h.Metadata.Creators {
			r.Authors = append(r.Authors, c.Name)
		}
		if h.Links.DOI != "" {
			r.URL = h.Links.DOI
		} else {
			r.URL = h.Links.Self
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func atoiOK(s string) (int, bool) {
	var n int
	if s == "" {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return n, true
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// stripHTML removes HTML tags and trims whitespace (for Zenodo descriptions).
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, c := range s {
		switch {
		case c == 0x3c:
			inTag = true
		case c == 0x3e:
			inTag = false
		case !inTag:
			b.WriteRune(c)
		}
	}
	return strings.TrimSpace(b.String())
}
