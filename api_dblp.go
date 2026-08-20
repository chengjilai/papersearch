package main

import (
	"encoding/json"
	"html"
)

// DBLP bibliographic API (computer science). JSON format, CC0 data.

type dblpResp struct {
	Result struct {
		Hits struct {
			Hit []struct {
				Info struct {
					Title   string          `json:"title"`
					Venue   string          `json:"venue"`
					Year    string          `json:"year"`
					DOI     string          `json:"doi"`
					URL     string          `json:"url"`
					Key     string          `json:"key"`
					Type    string          `json:"type"`
					Authors json.RawMessage `json:"authors"`
				} `json:"info"`
			} `json:"hit"`
		} `json:"hits"`
	} `json:"result"`
}

func searchDblp(q string, n int, since int) ([]Result, error) {
	endpoint := "https://dblp.org/search/publ/api?q=" + qval(q) + "&format=json&h=" + itoa(n)
	var resp dblpResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Result.Hits.Hit))
	for _, h := range resp.Result.Hits.Hit {
		info := h.Info
		if info.Title == "" {
			continue
		}
		r := Result{
			Source: "dblp",
			Title:  collapseSpace(html.UnescapeString(info.Title)),
			Venue:  html.UnescapeString(info.Venue),
			DOI:    info.DOI,
			URL:    info.URL,
		}
		if info.Key != "" && r.URL == "" {
			r.URL = "https://dblp.org/rec/" + info.Key + ".html"
		}
		if y, err := atoiSafe(info.Year); err == nil {
			r.Year = y
		}
		if since > 0 && r.Year < since {
			continue
		}
		// DBLP authors field is an array of {text} for many authors, a single
		// {text} object for one author, or (rarely) a plain string.
		if len(info.Authors) > 0 {
			r.Authors = parseDblpAuthors(info.Authors)
		}
		results = append(results, r)
	}
	return results, nil
}

func parseDblpAuthors(raw json.RawMessage) []string {
	type author struct {
		Text string `json:"text"`
	}
	// DBLP wraps authors in {"author": ...}; the inner value is an array of
	// objects, a single object, or (rarely) a plain string.
	var wrap struct {
		Author json.RawMessage `json:"author"`
	}
	if json.Unmarshal(raw, &wrap) == nil && len(wrap.Author) > 0 {
		raw = wrap.Author
	}
	var many []author
	if err := json.Unmarshal(raw, &many); err == nil && len(many) > 0 {
		out := make([]string, 0, len(many))
		for _, a := range many {
			if a.Text != "" {
				out = append(out, html.UnescapeString(a.Text))
			}
		}
		return out
	}
	var one author
	if err := json.Unmarshal(raw, &one); err == nil && one.Text != "" {
		return []string{one.Text}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return []string{s}
	}
	return nil
}

func atoiSafe(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
