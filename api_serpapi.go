package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Google Scholar via SerpAPI. The only way to query Scholar programmatically
// without violating Google's ToS by scraping (SerpAPI runs the proxies for you).

type serpResp struct {
	OrganicResults []struct {
		Title           string `json:"title"`
		Link            string `json:"link"`
		Snippet         string `json:"snippet"`
		ResultID        string `json:"result_id"`
		PublicationInfo struct {
			Summary string `json:"summary"`
			Authors []struct {
				Name string `json:"name"`
			} `json:"authors"`
		} `json:"publication_info"`
		InlineLinks struct {
			CitedBy struct {
				Total int `json:"total"`
			} `json:"cited_by"`
		} `json:"inline_links"`
		Resources []struct {
			Title string `json:"title"`
			Link  string `json:"link"`
		} `json:"resources"`
	} `json:"organic_results"`
	Error string `json:"error"`
}

var serpYearRe = regexp.MustCompile(`\b(19|20)\d{2}\b`)

func searchGoogleScholar(q string, n int, since int, key string) ([]Result, error) {
	endpoint := "https://serpapi.com/search.json?engine=google_scholar&q=" +
		qval(q) + "&num=" + itoa(n) + "&api_key=" + qval(key)

	var resp serpResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("serpapi: %s", resp.Error)
	}

	results := make([]Result, 0, len(resp.OrganicResults))
	for _, r := range resp.OrganicResults {
		res := Result{
			Source:     "scholar",
			Title:      r.Title,
			Abstract:   r.Snippet,
			URL:        r.Link,
			Identifier: r.ResultID,
		}
		// Year usually embedded in publication_info.summary, e.g.
		// "A Vaswani, N Shazeer, N Parmar… - Advances in neural …, 2017 - proceedings.neurips.cc"
		for _, m := range serpYearRe.FindAllString(r.PublicationInfo.Summary, -1) {
			if y, err := atoiSafe(m); err == nil {
				res.Year = y
				break
			}
		}
		for _, a := range r.PublicationInfo.Authors {
			res.Authors = append(res.Authors, a.Name)
		}
		// Venue is the summary's last segment (the source website, e.g.
		// "proceedings.neurips.cc"), or the segment just before the year.
		parts := strings.Split(r.PublicationInfo.Summary, " - ")
		if n := len(parts); n > 0 && parts[n-1] != "" && !serpYearRe.MatchString(parts[n-1]) {
			res.Venue = parts[n-1]
		} else {
			for i, seg := range parts {
				if serpYearRe.MatchString(seg) && i > 0 && res.Venue == "" {
					res.Venue = parts[i-1]
				}
			}
		}
		res.Citations = r.InlineLinks.CitedBy.Total
		if since > 0 && res.Year != 0 && res.Year < since {
			continue
		}
		results = append(results, res)
	}
	return results, nil
}
