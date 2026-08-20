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

// Europe PMC REST API. Life sciences + preprints (arXiv, bioRxiv, medRxiv).

type europePMCResp struct {
	ResultList struct {
		Result []struct {
			ID           string `json:"id"`
			Source       string `json:"source"`
			DOI          string `json:"doi"`
			Title        string `json:"title"`
			AuthorString string `json:"authorString"`
			JournalTitle string `json:"journalTitle"`
			PubYear      string `json:"pubYear"`
			CitedByCount int    `json:"citedByCount"`
			AbstractText string `json:"abstractText"`
		} `json:"result"`
	} `json:"resultList"`
}

func searchEuropePMC(q string, n int, since int) ([]Result, error) {
	query := q
	if since > 0 {
		query = "(" + q + ") AND PUB_YEAR:[" + itoa(since) + " TO 3000]"
	}
	endpoint := "https://www.ebi.ac.uk/europepmc/webservices/rest/search" +
		"?query=" + qval(query) + "&format=json&pageSize=" + itoa(n) +
		"&resultType=core"

	var resp europePMCResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.ResultList.Result))
	for _, r := range resp.ResultList.Result {
		res := Result{
			Source:     "europepmc",
			Title:      r.Title,
			DOI:        r.DOI,
			Venue:      r.JournalTitle,
			Citations:  r.CitedByCount,
			Abstract:   r.AbstractText,
			URL:        "https://europepmc.org/article/" + r.Source + "/" + r.ID,
			Identifier: r.Source + ":" + r.ID,
		}
		if y, err := strconv.Atoi(r.PubYear); err == nil {
			res.Year = y
		}
		res.Authors = splitAuthorList(r.AuthorString)
		results = append(results, res)
	}
	return results, nil
}
