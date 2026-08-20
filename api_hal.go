package main

import (
	"encoding/json"
	"strconv"
)

// HAL (Hyper Articles en Ligne) Solr API. French multidisciplinary open archive.

type halResp struct {
	Response struct {
		Docs []struct {
			Title        []string        `json:"title_s"`
			URI          string          `json:"uri_s"`
			ProducedDate string          `json:"producedDate_s"`
			DoiID        json.RawMessage `json:"doiId_s"`
			Authors      []string        `json:"authFullName_s"`
			CitedBy      int             `json:"citedBy_s"`
			JournalTitle json.RawMessage `json:"journalTitle_s"`
		} `json:"docs"`
	} `json:"response"`
}

func searchHAL(q string, n int, since int) ([]Result, error) {
	endpoint := "https://api.archives-ouvertes.fr/search/?q=" + qval(q) +
		"&wt=json&rows=" + itoa(n) +
		"&fl=title_s,uri_s,producedDate_s,doiId_s,authFullName_s,citedBy_s,journalTitle_s"

	var resp halResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Response.Docs))
	for _, d := range resp.Response.Docs {
		r := Result{
			Source:    "hal",
			URL:       d.URI,
			Citations: d.CitedBy,
		}
		if len(d.Title) > 0 {
			r.Title = d.Title[0]
		}
		if len(d.DoiID) > 0 {
			var ids []string
			if err := json.Unmarshal(d.DoiID, &ids); err == nil && len(ids) > 0 {
				r.DOI = ids[0]
			} else {
				var one string
				if err := json.Unmarshal(d.DoiID, &one); err == nil {
					r.DOI = one
				}
			}
		}
		if len(d.JournalTitle) > 0 {
			var names []string
			if err := json.Unmarshal(d.JournalTitle, &names); err == nil && len(names) > 0 {
				r.Venue = names[0]
			} else {
				var one string
				if err := json.Unmarshal(d.JournalTitle, &one); err == nil {
					r.Venue = one
				}
			}
		}
		r.Authors = d.Authors
		if len(d.ProducedDate) >= 4 {
			if y, err := strconv.Atoi(d.ProducedDate[:4]); err == nil {
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
