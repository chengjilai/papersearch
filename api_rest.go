package main

import (
	"sort"
	"strconv"
	"strings"
)

// Crossref REST API. DOI metadata for the whole DOI ecosystem.

type crossrefResp struct {
	Message struct {
		Items []struct {
			Title  []string `json:"title"`
			Author []struct {
				Given  string `json:"given"`
				Family string `json:"family"`
				Name   string `json:"name"`
			} `json:"author"`
			DOI            string   `json:"DOI"`
			ContainerTitle []string `json:"container-title"`
			Published      struct {
				DateParts [][]int `json:"date-parts"`
			} `json:"published"`
			Type string `json:"type"`
		} `json:"items"`
	} `json:"message"`
}

func searchCrossref(q string, n int, since int) ([]Result, error) {
	endpoint := "https://api.crossref.org/works?query=" + qval(q) +
		"&rows=" + itoa(n) + "&mailto=" + contactEmail +
		"&select=DOI,title,author,container-title,published,type"
	if since > 0 {
		endpoint += "&filter=from-pub-date:" + itoa(since) + "-01-01"
	}

	var resp crossrefResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Message.Items))
	for _, it := range resp.Message.Items {
		r := Result{Source: "crossref", DOI: it.DOI, URL: "https://doi.org/" + it.DOI}
		if len(it.Title) > 0 {
			r.Title = it.Title[0]
		}
		if len(it.ContainerTitle) > 0 {
			r.Venue = it.ContainerTitle[0]
		}
		for _, a := range it.Author {
			name := a.Name
			if name == "" {
				name = strings.TrimSpace(a.Given + " " + a.Family)
			}
			if name != "" {
				r.Authors = append(r.Authors, name)
			}
		}
		if len(it.Published.DateParts) > 0 && len(it.Published.DateParts[0]) > 0 {
			r.Year = it.Published.DateParts[0][0]
		}
		results = append(results, r)
	}
	return results, nil
}

// OpenAlex REST API. Largest open scholarly catalog; full-text-aware relevance search.

type openAlexResp struct {
	Results []struct {
		Title           string `json:"display_name"`
		PublicationYear int    `json:"publication_year"`
		DOI             string `json:"doi"`
		CitedByCount    int    `json:"cited_by_count"`
		Authorships     []struct {
			Author struct {
				DisplayName string `json:"display_name"`
			} `json:"author"`
		} `json:"authorships"`
		PrimaryLocation struct {
			LandingPageURL string `json:"landing_page_url"`
			Source         struct {
				DisplayName string `json:"display_name"`
			} `json:"source"`
		} `json:"primary_location"`
		AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
	} `json:"results"`
}

func searchOpenAlex(q string, n int, since int) ([]Result, error) {
	endpoint := "https://api.openalex.org/works?search=" + qval(q) +
		"&per-page=" + itoa(n) + "&mailto=" + contactEmail +
		"&select=id,doi,display_name,publication_year,authorships,cited_by_count,primary_location,abstract_inverted_index"
	if since > 0 {
		endpoint += "&filter=from_publication_date:" + itoa(since) + "-01-01"
	}

	var resp openAlexResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Results))
	for _, w := range resp.Results {
		r := Result{
			Source:    "openalex",
			Title:     w.Title,
			Year:      w.PublicationYear,
			Citations: w.CitedByCount,
			DOI:       strings.TrimPrefix(w.DOI, "https://doi.org/"),
		}
		if w.PrimaryLocation.LandingPageURL != "" {
			r.URL = w.PrimaryLocation.LandingPageURL
		}
		r.Venue = w.PrimaryLocation.Source.DisplayName
		for _, as := range w.Authorships {
			r.Authors = append(r.Authors, as.Author.DisplayName)
		}
		r.Abstract = reconstructInverted(w.AbstractInvertedIndex)
		results = append(results, r)
	}
	return results, nil
}

// reconstructInverted rebuilds an abstract from OpenAlex's word->positions map.
func reconstructInverted(ix map[string][]int) string {
	if len(ix) == 0 {
		return ""
	}
	total := 0
	for _, pos := range ix {
		for _, p := range pos {
			if p >= total {
				total = p + 1
			}
		}
	}
	words := make([]string, total)
	for w, pos := range ix {
		for _, p := range pos {
			if p < total {
				words[p] = w
			}
		}
	}
	joined := strings.Join(words, " ")
	// words can contain underscores from NLP tokenization; leave as-is but collapse whitespace
	return strings.Join(strings.Fields(joined), " ")
}

var _ = sort.Ints // keep sort import balanced if rules change

// DOAJ (Directory of Open Access Journals) API v3. Open-access journal articles.

type doajResp struct {
	Total   int `json:"total"`
	Results []struct {
		BibJSON struct {
			Title    string   `json:"title"`
			TitleAlt []string `json:"-"`
			Year     string   `json:"year"`
			Journal  struct {
				Title string `json:"title"`
			} `json:"journal"`
			Authors []struct {
				Name string `json:"name"`
			} `json:"author"`
			Identifier []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"identifier"`
			Abstract string `json:"abstract"`
			Link     []struct {
				URL  string `json:"url"`
				Type string `json:"type"`
			} `json:"link"`
		} `json:"bibjson"`
	} `json:"results"`
}

func searchDOAJ(q string, n int, since int) ([]Result, error) {
	endpoint := "https://doaj.org/api/v3/search/articles/" + qval(q) + "?pageSize=" + itoa(n)
	var resp doajResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Results))
	for _, item := range resp.Results {
		b := item.BibJSON
		r := Result{
			Source:   "doaj",
			Title:    collapseSpace(b.Title),
			Venue:    b.Journal.Title,
			Abstract: b.Abstract,
		}
		if y, err := strconv.Atoi(b.Year); err == nil {
			r.Year = y
		}
		for _, a := range b.Authors {
			r.Authors = append(r.Authors, a.Name)
		}
		for _, id := range b.Identifier {
			if id.Type == "doi" {
				r.DOI = id.ID
			}
		}
		for _, l := range b.Link {
			if r.URL == "" {
				r.URL = l.URL
			}
		}
		if r.DOI != "" && r.URL == "" {
			r.URL = "https://doi.org/" + r.DOI
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// DataCite REST API v2. DOIs for datasets, software, and scholarly artifacts.
// Note: the legacy api.datacite.org/works endpoint is deprecated (410).

type dataciteResp struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			DOI    string `json:"doi"`
			Titles []struct {
				Title string `json:"title"`
			} `json:"titles"`
			Creators []struct {
				Name string `json:"name"`
			} `json:"creators"`
			PublicationYear int    `json:"publicationYear"`
			Publisher       string `json:"publisher"`
			URL             string `json:"url"`
			Types           struct {
				ResourceType        string `json:"resourceType"`
				ResourceTypeGeneral string `json:"resourceTypeGeneral"`
			} `json:"types"`
		} `json:"attributes"`
	} `json:"data"`
}

func searchDataCite(q string, n int, since int) ([]Result, error) {
	endpoint := "https://api.datacite.org/dois?query=" + qval(q) + "&page[size]=" + itoa(n)
	var resp dataciteResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Data))
	for _, d := range resp.Data {
		a := d.Attributes
		r := Result{
			Source: "datacite",
			DOI:    a.DOI,
			Year:   a.PublicationYear,
			Venue:  a.Publisher,
		}
		if len(a.Titles) > 0 {
			r.Title = a.Titles[0].Title
		}
		for _, c := range a.Creators {
			r.Authors = append(r.Authors, c.Name)
		}
		if a.Types.ResourceType != "" {
			r.Venue = a.Types.ResourceType
		} else {
			r.Venue = a.Publisher
		}
		if a.URL != "" {
			r.URL = a.URL
		} else {
			r.URL = "https://doi.org/" + a.DOI
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// OSF (Open Science Framework) API v2 — free research projects/preprints/datasets.
// Documented at https://developer.osf.io (IETF Basic Auth optional; anonymous read works).

type osfResp struct {
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
	Data []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			DateCreated string `json:"date_created"`
			Category    string `json:"category"`
		} `json:"attributes"`
		Links struct {
			HTML string `json:"html"`
		} `json:"links"`
		Relationships struct {
			Contributors struct {
				Links struct {
					Related struct {
						Href string `json:"href"`
					} `json:"related"`
				} `json:"links"`
			} `json:"contributors"`
		} `json:"relationships"`
	} `json:"data"`
}

func searchOSF(q string, n int, since int) ([]Result, error) {
	endpoint := "https://api.osf.io/v2/nodes/?search=" + qval(q) +
		"&page[size]=" + itoa(n)
	var resp osfResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Data))
	for _, d := range resp.Data {
		r := Result{
			Source:   "osf",
			Title:    d.Attributes.Title,
			Abstract: d.Attributes.Description,
			Venue:    d.Attributes.Category,
			URL:      d.Links.HTML,
		}
		if len(d.Attributes.DateCreated) >= 4 {
			r.Year = parseYear(d.Attributes.DateCreated[:4])
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// Dryad API v2 — data repository for research datasets (free, no key).

type dryadResp struct {
	Total    int `json:"total"`
	Embedded struct {
		Datasets []struct {
			Title           string `json:"title"`
			Abstract        string `json:"abstract"`
			PublicationDate string `json:"publicationDate"`
			Authors         []struct {
				FirstName string `json:"firstName"`
				LastName  string `json:"lastName"`
			} `json:"authors"`
			Identifier string `json:"identifier"`
			Metrics    struct {
				Downloads int `json:"downloads"`
			} `json:"metrics"`
			Links struct {
				Self struct {
					Href string `json:"href"`
				} `json:"self"`
			} `json:"_links"`
		} `json:"stash:datasets"`
	} `json:"_embedded"`
}

func searchDryad(q string, n int, since int) ([]Result, error) {
	endpoint := "https://datadryad.org/api/v2/search?q=" + qval(q) + "&limit=" + itoa(n)
	var resp dryadResp
	if err := getJSON(endpoint, &resp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(resp.Embedded.Datasets))
	for _, d := range resp.Embedded.Datasets {
		r := Result{
			Source:   "dryad",
			Title:    d.Title,
			Abstract: stripHTML(d.Abstract),
		}
		if strings.HasPrefix(d.Identifier, "doi:") {
			r.DOI = strings.TrimPrefix(d.Identifier, "doi:")
		}
		if r.DOI != "" {
			r.URL = "https://doi.org/" + r.DOI
		} else if d.Links.Self.Href != "" {
			r.URL = "https://datadryad.org" + d.Links.Self.Href
		}
		for _, a := range d.Authors {
			name := strings.TrimSpace(a.FirstName + " " + a.LastName)
			if name != "" {
				r.Authors = append(r.Authors, name)
			}
		}
		if len(d.PublicationDate) >= 4 {
			r.Year = parseYear(d.PublicationDate[:4])
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}
