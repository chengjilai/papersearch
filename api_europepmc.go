package main

import (
	"strconv"
	"strings"
)

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

// splitAuthorList splits "A, B, C, and D" author strings without naive splits
// breaking on initials like "Smith, J" (those come as "Smith J" from EuropePMC).
func splitAuthorList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "and ")
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, strings.TrimRight(part, "."))
		}
	}
	return out
}
