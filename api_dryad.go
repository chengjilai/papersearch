package main

import "strings"

// Dryad API v2 — data repository for research datasets (free, no key).

type dryadResp struct {
	Total    int `json:"total"`
	Embedded struct {
		Datasets []struct {
			Title           string `json:"title"`
			Abstract        string `json:"abstract"`
			PublicationDate string `json:"publicationDate"`
			Authors         []struct {
				FirstName string `json:"firstName"`
				LastName  string `json:"lastName"`
			} `json:"authors"`
			Identifier string `json:"identifier"`
			Metrics    struct {
				Downloads int `json:"downloads"`
			} `json:"metrics"`
			Links struct {
				Self struct {
					Href string `json:"href"`
				} `json:"self"`
			} `json:"_links"`
		} `json:"stash:datasets"`
	} `json:"_embedded"`
}

func searchDryad(q string, n int, since int) ([]Result, error) {
	endpoint := "https://datadryad.org/api/v2/search?q=" + qval(q) + "&limit=" + itoa(n)
	var resp dryadResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Embedded.Datasets))
	for _, d := range resp.Embedded.Datasets {
		r := Result{
			Source:   "dryad",
			Title:    d.Title,
			Abstract: stripHTML(d.Abstract),
		}
		if strings.HasPrefix(d.Identifier, "doi:") {
			r.DOI = strings.TrimPrefix(d.Identifier, "doi:")
		}
		if r.DOI != "" {
			r.URL = "https://doi.org/" + r.DOI
		} else if d.Links.Self.Href != "" {
			r.URL = "https://datadryad.org" + d.Links.Self.Href
		}
		for _, a := range d.Authors {
			name := strings.TrimSpace(a.FirstName + " " + a.LastName)
			if name != "" {
				r.Authors = append(r.Authors, name)
			}
		}
		if len(d.PublicationDate) >= 4 {
			if y, ok := atoiOK(d.PublicationDate[:4]); ok {
				r.Year = y
			}
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}
