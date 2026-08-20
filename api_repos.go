package main

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
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

// Zenodo API. CERN-hosted research data repository (datasets, software, preprints).

type zenodoResp struct {
	Hits struct {
		Hits []struct {
			ID    int    `json:"id"`
			DOI   string `json:"doi"`
			Links struct {
				Self string `json:"self"`
				DOI  string `json:"doi"`
			} `json:"links"`
			Metadata struct {
				Title           string `json:"title"`
				PublicationDate string `json:"publication_date"`
				Creators        []struct {
					Name string `json:"name"`
				} `json:"creators"`
				Description  string `json:"description"`
				ResourceType struct {
					Title string `json:"title"`
				} `json:"resource_type"`
			} `json:"metadata"`
		} `json:"hits"`
	} `json:"hits"`
}

func searchZenodo(q string, n int, since int) ([]Result, error) {
	query := q
	if since > 0 {
		query = "publication_date:[" + itoa(since) + "-01-01 TO *] AND (" + q + ")"
	}
	endpoint := "https://zenodo.org/api/records?q=" + qval(query) + "&size=" + itoa(n) +
		"&sort=mostrecent"

	var resp zenodoResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Hits.Hits))
	for _, h := range resp.Hits.Hits {
		r := Result{
			Source:   "zenodo",
			Title:    h.Metadata.Title,
			DOI:      h.DOI,
			Venue:    h.Metadata.ResourceType.Title,
			Abstract: stripHTML(h.Metadata.Description),
		}
		if len(h.Metadata.PublicationDate) >= 4 {
			r.Year = parseYear(h.Metadata.PublicationDate[:4])
		}
		for _, c := range h.Metadata.Creators {
			r.Authors = append(r.Authors, c.Name)
		}
		if h.Links.DOI != "" {
			r.URL = h.Links.DOI
		} else {
			r.URL = h.Links.Self
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

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
