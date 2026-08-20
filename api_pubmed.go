package main

import (
	"encoding/json"
	"strconv"
	"strings"
)

// PubMed E-utilities: esearch (get PMIDs) then esummary (get metadata).

type esearchResp struct {
	Result struct {
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

type esummaryResp struct {
	Result map[string]json.RawMessage `json:"result"`
}

type pubmedDoc struct {
	Title   string `json:"title"`
	Authors []struct {
		Name string `json:"name"`
	} `json:"authors"`
	PubDate     string `json:"pubdate"`
	FullJournal string `json:"fulljournalname"`
	ArticleIDs  []struct {
		IDType string `json:"idtype"`
		Value  string `json:"value"`
	} `json:"articleids"`
}

func searchPubMed(q string, n int, since int) ([]Result, error) {
	term := q
	if since > 0 {
		term += ` AND ("` + itoa(since) + `/01/01"[Date - Publication] : "3000/12/31"[Date - Publication])`
	}
	searchURL := "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi" +
		"?db=pubmed&term=" + qval(term) + "&retmax=" + itoa(n) + "&retmode=json&sort=relevance"

	var sr esearchResp
	if err := getJSON(searchURL, &sr); err != nil {
		return nil, err
	}
	if len(sr.Result.IDList) == 0 {
		return nil, nil
	}

	sumURL := "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esummary.fcgi" +
		"?db=pubmed&id=" + strings.Join(sr.Result.IDList, ",") + "&retmode=json"

	var smr esummaryResp
	if err := getJSON(sumURL, &smr); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(sr.Result.IDList))
	for _, id := range sr.Result.IDList {
		raw, ok := smr.Result[id]
		if !ok {
			continue
		}
		var rec pubmedDoc
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue
		}
		r := Result{
			Source:     "pubmed",
			Title:      rec.Title,
			Venue:      rec.FullJournal,
			Identifier: "PMID:" + id,
			URL:        "https://pubmed.ncbi.nlm.nih.gov/" + id + "/",
		}
		if len(rec.PubDate) >= 4 {
			if y, err := strconv.Atoi(rec.PubDate[:4]); err == nil {
				r.Year = y
			}
		}
		for _, a := range rec.Authors {
			r.Authors = append(r.Authors, a.Name)
		}
		for _, aid := range rec.ArticleIDs {
			if aid.IDType == "doi" {
				r.DOI = aid.Value
			}
		}
		if r.DOI != "" {
			r.URL = "https://doi.org/" + r.DOI
		}
		results = append(results, r)
	}
	return results, nil
}
