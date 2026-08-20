package main

import "strconv"

// DOAJ (Directory of Open Access Journals) API v3. Open-access journal articles.

type doajResp struct {
	Total   int `json:"total"`
	Results []struct {
		BibJSON struct {
			Title    string   `json:"title"`
			TitleAlt []string `json:"-"`
			Year     string   `json:"year"`
			Journal  struct {
				Title string `json:"title"`
			} `json:"journal"`
			Authors []struct {
				Name string `json:"name"`
			} `json:"author"`
			Identifier []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"identifier"`
			Abstract string `json:"abstract"`
			Link     []struct {
				URL  string `json:"url"`
				Type string `json:"type"`
			} `json:"link"`
		} `json:"bibjson"`
	} `json:"results"`
}

func searchDOAJ(q string, n int, since int) ([]Result, error) {
	endpoint := "https://doaj.org/api/v3/search/articles/" + qval(q) + "?pageSize=" + itoa(n)
	var resp doajResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Results))
	for _, item := range resp.Results {
		b := item.BibJSON
		r := Result{
			Source:   "doaj",
			Title:    collapseSpace(b.Title),
			Venue:    b.Journal.Title,
			Abstract: b.Abstract,
		}
		if y, err := strconv.Atoi(b.Year); err == nil {
			r.Year = y
		}
		for _, a := range b.Authors {
			r.Authors = append(r.Authors, a.Name)
		}
		for _, id := range b.Identifier {
			if id.Type == "doi" {
				r.DOI = id.ID
			}
		}
		for _, l := range b.Link {
			if r.URL == "" {
				r.URL = l.URL
			}
		}
		if r.DOI != "" && r.URL == "" {
			r.URL = "https://doi.org/" + r.DOI
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}
