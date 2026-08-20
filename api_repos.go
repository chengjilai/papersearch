package main

import (
	"encoding/xml"
	"io"
	"strings"
)

// OpenAIRE API — EU-funded research publications, free, no key. Returns XML
// with the DRI ontology; elements are bare tags (no prefix) but nested under
// optional dri: wrappers, so we walk tokens rather than model the tree.

type openaireDoc struct {
	Title  string
	Author []string
	Date   string
	DOI    string
	Venue  string
}

func searchOpenAIRE(q string, n int, since int) ([]Result, error) {
	endpoint := "https://api.openaire.eu/search/publications?keywords=" +
		qval(q) + "&size=" + itoa(n)
	body, err := getRaw(endpoint)
	if err != nil {
		return nil, err
	}

	docs := parseOpenAIREDocs(body, n)
	results := make([]Result, 0, len(docs))
	for _, d := range docs {
		r := Result{
			Source:  "openaire",
			Title:   d.Title,
			Venue:   d.Venue,
			DOI:     d.DOI,
			Authors: d.Author,
		}
		if len(d.Date) >= 4 {
			r.Year = parseYear(d.Date[:4])
		}
		if r.DOI != "" {
			r.URL = "https://doi.org/" + r.DOI
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func parseOpenAIREDocs(body []byte, max int) []openaireDoc {
	var docs []openaireDoc
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	var cur *openaireDoc
	lastTag := ""
	pidClass := ""

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			lastTag = t.Name.Local
			if t.Name.Local == "result" {
				cur = &openaireDoc{}
			} else if t.Name.Local == "pid" {
				pidClass = ""
				for _, a := range t.Attr {
					if a.Name.Local == "classid" || a.Name.Local == "classname" {
						pidClass += strings.ToLower(a.Value) + " "
					}
				}
			}
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text == "" || cur == nil {
				continue
			}
			switch lastTag {
			case "title":
				if cur.Title == "" {
					cur.Title = collapseSpace(text)
				}
			case "creator":
				cur.Author = append(cur.Author, collapseSpace(text))
			case "dateofacceptance":
				cur.Date = text
			case "pid":
				if strings.Contains(pidClass, "doi") {
					cur.DOI = text
				}
			case "publisher":
				if cur.Venue == "" {
					cur.Venue = text
				}
			}
		case xml.EndElement:
			if t.Name.Local == "result" {
				if cur != nil && (cur.Title != "" || cur.DOI != "") {
					docs = append(docs, *cur)
					if len(docs) >= max {
						return docs
					}
				}
				cur = nil
			}
		}
	}
	return docs
}
