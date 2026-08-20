package main

import (
	"encoding/json"
	"strings"
)

// INSPIRE-HEP API. High-energy physics literature.

type inspireResp struct {
	Hits struct {
		Total int `json:"total"`
		Hits  []struct {
			ID       json.RawMessage `json:"id"`
			Metadata struct {
				Titles []struct {
					Title string `json:"title"`
				} `json:"titles"`
				Authors []struct {
					FullName string `json:"full_name"`
				} `json:"authors"`
				PublicationInfo []struct {
					Year         int    `json:"year"`
					JournalTitle string `json:"journal_title"`
				} `json:"publication_info"`
				DOIs []struct {
					Value string `json:"value"`
				} `json:"dois"`
				ArxivEprints []struct {
					Value string `json:"value"`
				} `json:"arxiv_eprints"`
			} `json:"metadata"`
		} `json:"hits"`
	} `json:"hits"`
}

func searchInspire(q string, n int, since int) ([]Result, error) {
	query := q
	if since > 0 {
		query += " and date >= " + itoa(since)
	}
	endpoint := "https://inspirehep.net/api/literature?q=" + qval(query) + "&size=" + itoa(n) +
		"&fields=titles,authors,publication_info,dois,arxiv_eprints"

	var resp inspireResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Hits.Hits))
	for _, h := range resp.Hits.Hits {
		m := h.Metadata
		r := Result{
			Source: "inspire",
		}
		if len(m.Titles) > 0 {
			r.Title = m.Titles[0].Title
		}
		for _, a := range m.Authors {
			r.Authors = append(r.Authors, a.FullName)
		}
		if len(m.PublicationInfo) > 0 {
			r.Year = m.PublicationInfo[0].Year
			r.Venue = m.PublicationInfo[0].JournalTitle
		}
		if len(m.DOIs) > 0 {
			r.DOI = m.DOIs[0].Value
		}
		if len(m.ArxivEprints) > 0 {
			r.Identifier = "arXiv:" + m.ArxivEprints[0].Value
		}
		if r.Year < since {
			continue
		}
		if r.DOI != "" {
			r.URL = "https://doi.org/" + r.DOI
		} else if r.Identifier != "" {
			r.URL = "https://arxiv.org/abs/" + strings.TrimPrefix(r.Identifier, "arXiv:")
		} else {
			r.URL = "https://inspirehep.net/literature/" + inspireIDToString(h.ID)
		}
		results = append(results, r)
	}
	return results, nil
}

func inspireIDToString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return itoa(n)
	}
	return string(raw)
}
