package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"strings"
)

// arXiv Atom XML API. Returns preprints with abstracts.

type arxivFeed struct {
	Entry []struct {
		ID        string `xml:"id"`
		Title     string `xml:"title"`
		Summary   string `xml:"summary"`
		Published string `xml:"published"`
		Authors   []struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Category []struct {
			Term string `xml:"term,attr"`
		} `xml:"category"`
		Links []struct {
			Href  string `xml:"href,attr"`
			Rel   string `xml:"rel,attr"`
			Title string `xml:"title,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

func searchArxiv(q string, n int, since int) ([]Result, error) {
	query := "all:" + q
	if since > 0 {
		query += fmt.Sprintf(" AND submittedDate:[%04d01010000 TO 999912312359]", since)
	}
	endpoint := "https://export.arxiv.org/api/query?search_query=" +
		qval(query) + "&start=0&max_results=" + itoa(n)

	body, err := getRaw(endpoint)
	if err != nil {
		return nil, err
	}
	var feed arxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(feed.Entry))
	for _, e := range feed.Entry {
		r := Result{
			Source:   "arxiv",
			Title:    strings.TrimSpace(e.Title),
			Abstract: strings.TrimSpace(e.Summary),
		}
		year := ""
		if len(e.Published) >= 4 {
			year = e.Published[:4]
		}
		r.Year = parseYear(year)
		for _, a := range e.Authors {
			r.Authors = append(r.Authors, a.Name)
		}
		// arXiv id from abs URL, e.g. http://arxiv.org/abs/1706.03762v2
		r.Identifier = strings.TrimPrefix(e.ID, "http://arxiv.org/abs/")
		r.Identifier = strings.TrimPrefix(r.Identifier, "https://arxiv.org/abs/")
		for _, l := range e.Links {
			if l.Rel == "alternate" {
				r.URL = l.Href
			}
		}
		if r.URL == "" {
			r.URL = e.ID
		}
		for _, c := range e.Category {
			if r.Venue != "" {
				r.Venue += ", "
			}
			r.Venue += c.Term
		}
		results = append(results, r)
	}
	return results, nil
}

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
		r.Year = parseYear(info.Year)
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
