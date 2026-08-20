package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
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
			switch t.Name.Local {
			case "result":
				cur = &openaireDoc{}
			case "pid":
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

// Semantic Scholar Graph API. AI-ranked citations; aggressive rate limits.

type s2Resp struct {
	Data []struct {
		PaperID string `json:"paperId"`
		Title   string `json:"title"`
		Authors []struct {
			Name string `json:"name"`
		} `json:"authors"`
		Year          int    `json:"year"`
		Venue         string `json:"venue"`
		CitationCount int    `json:"citationCount"`
		ExternalIDs   struct {
			DOI string `json:"DOI"`
		} `json:"externalIds"`
		URL      string `json:"url"`
		Abstract string `json:"abstract"`
	} `json:"data"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

func searchScholar(q string, n int, since int) ([]Result, error) {
	endpoint := "https://api.semanticscholar.org/graph/v1/paper/search?query=" +
		qval(q) + "&limit=" + itoa(n) +
		"&fields=title,authors,year,venue,citationCount,externalIds,url,abstract"
	if since > 0 {
		endpoint += "&year=" + itoa(since) + "-" + itoa(since+1000)
	}

	var resp s2Resp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}
	if resp.Code == "429" || strings.Contains(resp.Message, "Too Many Requests") {
		return nil, errRateLimited
	}

	results := make([]Result, 0, len(resp.Data))
	for _, p := range resp.Data {
		r := Result{
			Source:    "semanticscholar",
			Title:     p.Title,
			Year:      p.Year,
			Venue:     p.Venue,
			Citations: p.CitationCount,
			DOI:       p.ExternalIDs.DOI,
			URL:       p.URL,
			Abstract:  p.Abstract,
		}
		if r.URL == "" && r.DOI != "" {
			r.URL = "https://doi.org/" + r.DOI
		}
		for _, a := range p.Authors {
			r.Authors = append(r.Authors, a.Name)
		}
		results = append(results, r)
	}
	return results, nil
}
