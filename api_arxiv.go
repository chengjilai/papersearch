package main

import (
	"encoding/xml"
	"fmt"
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
		fmt.Sscanf(year, "%d", &r.Year)
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
