package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Key-gated providers. Each reads its key from $PAPERSEARCH_<NAME>_KEY.
// They are registered only when the key is present, so default invocations
// are never blocked by a missing credential.

var allKeyedProviders = []provider{
	{name: "core", label: "CORE (full-text aggregator)", keyed: true, search: keySearch(searchCORE, keyCORE)},
	{name: "ads", label: "NASA ADS / SciX (astrophysics)", keyed: true, search: keySearch(searchADS, keyADS)},
	{name: "lens", label: "Lens.org (scholarly + patents)", keyed: true, search: keySearch(searchLens, keyLens)},
	{name: "springer", label: "Springer Nature", keyed: true, search: keySearch(searchSpringer, keySpringer)},
	{name: "ieee", label: "IEEE Xplore", keyed: true, search: keySearch(searchIEEE, keyIEEE)},
}

// keyedProviders returns the subset of keyed providers whose env key is set.
func keyedProviders() []provider {
	var out []provider
	for _, p := range allKeyedProviders {
		if haveKey(p.name) {
			out = append(out, p)
		}
	}
	return out
}

// keySearch wraps a key-arg search func to read the key from the right env var.
func keySearch(fn func(string, int, int, string) ([]Result, error), getKey func() string) func(string, int, int) ([]Result, error) {
	return func(q string, n int, since int) ([]Result, error) {
		return fn(q, n, since, getKey())
	}
}

func keyCORE() string     { return strings.TrimSpace(os.Getenv("PAPERSEARCH_CORE_KEY")) }
func keyADS() string      { return strings.TrimSpace(os.Getenv("PAPERSEARCH_ADS_TOKEN")) }
func keyLens() string     { return strings.TrimSpace(os.Getenv("PAPERSEARCH_LENS_KEY")) }
func keySpringer() string { return strings.TrimSpace(os.Getenv("PAPERSEARCH_SPRINGER_KEY")) }
func keyIEEE() string     { return strings.TrimSpace(os.Getenv("PAPERSEARCH_IEEE_KEY")) }

var errRateLimited = fmt.Errorf("rate limited by remote service; retry later or provide an API key")

// --- CORE v3 ---
type coreResp struct {
	Results []struct {
		Title   string `json:"title"`
		Authors []struct {
			Name string `json:"name"`
		} `json:"authors"`
		YearPublished int     `json:"yearPublished"`
		DOI           string  `json:"doi"`
		URL           string  `json:"url"`
		Abstract      string  `json:"abstract"`
		Similarity    float64 `json:"similarity"`
	} `json:"results"`
}

func searchCORE(q string, n int, since int, key string) ([]Result, error) {
	endpoint := "https://api.core.ac.uk/v3/search/works?q=" + qval(q) + "&limit=" + itoa(n)
	body, err := getRawWithBearer(endpoint, key)
	if err != nil {
		return nil, err
	}
	var resp coreResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(resp.Results))
	for _, w := range resp.Results {
		r := Result{
			Source:   "core",
			Title:    w.Title,
			Year:     w.YearPublished,
			DOI:      w.DOI,
			URL:      w.URL,
			Abstract: w.Abstract,
		}
		for _, a := range w.Authors {
			r.Authors = append(r.Authors, a.Name)
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// --- NASA ADS (SciX) ---
type adsResp struct {
	Response struct {
		Docs []struct {
			Title         []string `json:"title"`
			Author        []string `json:"author"`
			Year          []int    `json:"year"`
			DOI           []string `json:"doi"`
			Bibcode       string   `json:"bibcode"`
			CitationCount int      `json:"citation_count"`
			Abstract      []string `json:"abstract"`
		} `json:"docs"`
	} `json:"response"`
}

func searchADS(q string, n int, since int, key string) ([]Result, error) {
	endpoint := "https://api.adsabs.harvard.edu/v1/search/query?q=" + qval(q) +
		"&rows=" + itoa(n) + "&fl=title,author,year,doi,bibcode,citation_count,abstract"
	if since > 0 {
		endpoint += "&fq=year:[" + itoa(since) + "-01-01 TO *]"
	}
	body, err := getRawWithBearer(endpoint, key)
	if err != nil {
		return nil, err
	}
	var resp adsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(resp.Response.Docs))
	for _, d := range resp.Response.Docs {
		r := Result{
			Source:     "ads",
			Identifier: d.Bibcode,
			Citations:  d.CitationCount,
		}
		if len(d.Title) > 0 {
			r.Title = d.Title[0]
		}
		if len(d.Author) > 0 {
			r.Authors = d.Author
		}
		if len(d.Year) > 0 {
			r.Year = d.Year[0]
		}
		if len(d.DOI) > 0 {
			r.DOI = d.DOI[0]
		}
		if len(d.Abstract) > 0 {
			r.Abstract = d.Abstract[0]
		}
		if d.Bibcode != "" {
			r.URL = "https://ui.adsabs.harvard.edu/abs/" + d.Bibcode + "/abstract"
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// --- Lens.org ---
type lensResp struct {
	Data []struct {
		Title   string `json:"title"`
		Authors []struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"authors"`
		YearPublished int    `json:"year_published"`
		Source        string `json:"source"`
		DOI           string `json:"doi"`
		ExternalIDs   []struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"external_ids"`
		CitationCount int    `json:"citation_count"`
		Abstract      string `json:"abstract"`
		ID            string `json:"id"`
	} `json:"data"`
}

func searchLens(q string, n int, since int, key string) ([]Result, error) {
	payload := map[string]any{
		"query": map[string]any{
			"match": map[string]string{"title.en": q},
		},
		"size":    n,
		"include": []string{"title", "authors", "year_published", "source", "doi", "external_ids", "citation_count", "abstract", "id"},
		"scroll":  "false",
	}
	if since > 0 {
		payload["filter"] = map[string]any{
			"range": map[string]int{"year_published": since},
		}
	}
	body, _ := json.Marshal(payload)
	req, err := newPost("https://api.lens.org/scholarly/search", body, key)
	if err != nil {
		return nil, err
	}
	respBody, err := doReq(req)
	if err != nil {
		return nil, err
	}
	var resp lensResp
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(resp.Data))
	for _, d := range resp.Data {
		r := Result{
			Source:    "lens",
			Title:     d.Title,
			Year:      d.YearPublished,
			Venue:     d.Source,
			DOI:       d.DOI,
			Citations: d.CitationCount,
			Abstract:  d.Abstract,
		}
		for _, a := range d.Authors {
			name := strings.TrimSpace(a.FirstName + " " + a.LastName)
			if name != "" {
				r.Authors = append(r.Authors, name)
			}
		}
		for _, id := range d.ExternalIDs {
			if id.Type == "doi" {
				r.DOI = id.Value
			}
		}
		if r.DOI != "" {
			r.URL = "https://doi.org/" + r.DOI
		} else if d.ID != "" {
			r.URL = "https://www.lens.org/lens/scholar/" + d.ID
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// --- Springer Nature ---
type springerResp struct {
	Records []struct {
		Title    string `json:"title"`
		Creators []struct {
			Creator string `json:"creator"`
		} `json:"creators"`
		PublicationName string `json:"publicationName"`
		PublicationDate string `json:"publicationDate"`
		DOI             string `json:"doi"`
		URL             string `json:"url"`
		Abstract        string `json:"abstract"`
	} `json:"records"`
}

func searchSpringer(q string, n int, since int, key string) ([]Result, error) {
	endpoint := "https://api.springernature.com/meta/v2/json?q=" + qval(q) +
		"&api_key=" + key + "&p=" + itoa(n)
	if since > 0 {
		endpoint += "&date=from:" + itoa(since) + "-01-01"
	}
	var resp springerResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(resp.Records))
	for _, r := range resp.Records {
		res := Result{
			Source:   "springer",
			Title:    r.Title,
			Venue:    r.PublicationName,
			DOI:      r.DOI,
			URL:      r.URL,
			Abstract: r.Abstract,
		}
		if len(r.PublicationDate) >= 4 {
			if y, err := atoiSafe(r.PublicationDate[:4]); err == nil {
				res.Year = y
			}
		}
		for _, c := range r.Creators {
			res.Authors = append(res.Authors, c.Creator)
		}
		if since > 0 && res.Year < since {
			continue
		}
		results = append(results, res)
	}
	return results, nil
}

// --- IEEE Xplore ---
type ieeeResp struct {
	Articles []struct {
		Title   string `json:"title"`
		Authors []struct {
			FullName string `json:"full_name"`
		} `json:"authors"`
		PublicationTitle string `json:"publication_title"`
		PublicationYear  int    `json:"publication_year"`
		DOI              string `json:"doi"`
		Abstract         string `json:"abstract"`
		ArticleNumber    string `json:"article_number"`
		CitationCount    int    `json:"citation_count"`
		PDFURL           string `json:"pdf_url"`
	} `json:"articles"`
}

func searchIEEE(q string, n int, since int, key string) ([]Result, error) {
	endpoint := "https://ieeexploreapi.ieee.org/api/v1/search/articles?querytext=" + qval(q) +
		"&apikey=" + key + "&max_records=" + itoa(n) + "&format=json"
	if since > 0 {
		endpoint += "&start_year=" + itoa(since)
	}
	var resp ieeeResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(resp.Articles))
	for _, a := range resp.Articles {
		r := Result{
			Source:    "ieee",
			Title:     a.Title,
			Year:      a.PublicationYear,
			Venue:     a.PublicationTitle,
			DOI:       a.DOI,
			Abstract:  a.Abstract,
			Citations: a.CitationCount,
		}
		for _, au := range a.Authors {
			r.Authors = append(r.Authors, au.FullName)
		}
		if a.ArticleNumber != "" {
			r.URL = "https://ieeexplore.ieee.org/document/" + a.ArticleNumber
		} else if a.DOI != "" {
			r.URL = "https://doi.org/" + a.DOI
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// --- helpers ---
func getRawWithBearer(endpoint, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "papersearch/0.1 (mailto:"+contactEmail+")")
	req.Header.Set("Authorization", "Bearer "+token)
	return doReq(req)
}

func newPost(endpoint string, body []byte, token string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "papersearch/0.1 (mailto:"+contactEmail+")")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func doReq(req *http.Request) ([]byte, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(io.LimitReader(resp.Body, 64<<20)); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		n := buf.Len()
		if n > 300 {
			n = 300
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, buf.String()[:n])
	}
	return buf.Bytes(), nil
}
