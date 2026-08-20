package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ----------------------------------------------------------------------------
// Provider registry — THE single source of truth for every search backend.
//
// Each API is one row in `allSpecs`. Three shapes are supported:
//
//   json      GET a JSON endpoint -> walk `sel` (dot-path to the record array)
//             -> map each record with `one`               (16 providers)
//   post      same but POST a JSON body (`postBody`)      (Lens)
//   bespoke   custom logic via `search` override          (arXiv/DBLP/PubMed/
//             Semantic Scholar/OpenAIRE — XML, two-step, or quirks)
//
// Credentials (6 rows) are handled declaratively by `auth`: bearer header,
// URL query param, or bearer+POST. `secret` names the credential, resolved by
// keyOf() from $PAPERSEARCH_<NAME>_KEY or /run/secrets.
// ----------------------------------------------------------------------------

type authKind int

const (
	authNone       authKind = iota
	authBearer              // Authorization: Bearer <secret>
	authKeyParam            // append &<keyParam>=<secret> to the GET URL
	authPostBearer          // POST with Authorization: Bearer <secret>
)

type spec struct {
	name, label string
	auth        authKind
	secret      string // keyOf() credential name (== provider name when keyed)
	keyParam    string // query param name for authKeyParam
	keyed       bool   // enabled only when its credential exists

	url      func(q string, n, since int) string            // endpoint (json/post)
	postBody func(q string, n, since int) []byte            // authPostBearer only
	sel      string                                         // dot-path to records
	one      func(raw json.RawMessage) Result               // map one record
	search   func(q string, n, since int) ([]Result, error) // bespoke override
}

// errRateLimited is the canonical message for persistent 429s (Semantic Scholar).
var errRateLimited = fmt.Errorf("rate limited by remote service; retry later or provide an API key")

// Credential lookup: env var then /run/secrets, sharing one mapping with keyOf().
var providerEnvVar = map[string]string{
	"core": "PAPERSEARCH_CORE_KEY", "ads": "PAPERSEARCH_ADS_TOKEN",
	"lens": "PAPERSEARCH_LENS_KEY", "springer": "PAPERSEARCH_SPRINGER_KEY",
	"ieee": "PAPERSEARCH_IEEE_KEY", "gscholar": "PAPERSEARCH_SERPAPI_KEY",
}

var providerSecretFile = map[string]string{
	"core": "papersearch-core-key", "ads": "papersearch-ads-token",
	"lens": "papersearch-lens-key", "springer": "papersearch-springer-key",
	"ieee": "papersearch-ieee-key", "gscholar": "papersearch-serpapi-key",
}

var serpYearRe = regexp.MustCompile(`\b(19|20)\d{2}\b`)

var allSpecs = []spec{
	// ---- free (15) -------------------------------------------------------
	{name: "arxiv", label: "arXiv", search: searchArxiv},
	{name: "scholar", label: "Semantic Scholar", search: searchScholar},
	{name: "pubmed", label: "PubMed", search: searchPubMed},
	{name: "dblp", label: "DBLP", search: searchDblp},
	{name: "openaire", label: "OpenAIRE (EU research)", search: searchOpenAIRE},

	{name: "crossref", label: "Crossref", sel: "message.items",
		url: func(q string, n, since int) string {
			e := "https://api.crossref.org/works?query=" + qval(q) + "&rows=" + itoa(n) +
				"&mailto=" + contactEmail + "&select=DOI,title,author,container-title,published"
			if since > 0 {
				e += "&filter=from-pub-date:" + itoa(since) + "-01-01"
			}
			return e
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "crossref", Title: str(raw, "title"), Venue: str(raw, "container-title"),
				DOI: str(raw, "DOI"), URL: "https://doi.org/" + str(raw, "DOI")}
			each(raw, "author", func(a json.RawMessage) {
				if n := str(a, "name"); n != "" {
					r.Authors = append(r.Authors, n)
				} else if n = strings.TrimSpace(str(a, "given") + " " + str(a, "family")); n != "" {
					r.Authors = append(r.Authors, n)
				}
			})
			r.Year = intv(raw, "published.date-parts.0.0")
			return r
		},
	},
	{name: "openalex", label: "OpenAlex", sel: "results",
		url: func(q string, n, since int) string {
			e := "https://api.openalex.org/works?search=" + qval(q) + "&per-page=" + itoa(n) +
				"&mailto=" + contactEmail + "&select=id,display_name,publication_year,doi,cited_by_count,authorships,primary_location,abstract_inverted_index"
			if since > 0 {
				e += "&filter=from_publication_date:" + itoa(since) + "-01-01"
			}
			return e
		},
		one: func(raw json.RawMessage) Result {
			r := Result{
				Source: "openalex", Title: str(raw, "display_name"), Year: intv(raw, "publication_year"),
				Citations: intv(raw, "cited_by_count"), DOI: strings.TrimPrefix(str(raw, "doi"), "https://doi.org/"),
				URL: str(raw, "primary_location.landing_page_url"), Venue: str(raw, "primary_location.source.display_name"),
			}
			each(raw, "authorships", func(a json.RawMessage) {
				r.Authors = append(r.Authors, str(a, "author.display_name"))
			})
			r.Abstract = abstractInverted(raw)
			return r
		},
	},
	{name: "datacite", label: "DataCite", sel: "data",
		url: func(q string, n, since int) string {
			return "https://api.datacite.org/dois?query=" + qval(q) + "&page[size]=" + itoa(n)
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "datacite", DOI: str(raw, "id"), Title: str(raw, "attributes.titles.0.title"),
				Year: intv(raw, "attributes.publicationYear")}
			if t := str(raw, "attributes.types.resourceType"); t != "" {
				r.Venue = t
			} else {
				r.Venue = str(raw, "attributes.publisher")
			}
			each(raw, "attributes.creators", func(c json.RawMessage) {
				r.Authors = append(r.Authors, str(c, "name"))
			})
			if u := str(raw, "attributes.url"); u != "" {
				r.URL = u
			} else {
				r.URL = "https://doi.org/" + r.DOI
			}
			return r
		},
	},
	{name: "doaj", label: "DOAJ", sel: "results",
		url: func(q string, n, since int) string {
			return "https://doaj.org/api/v3/search/articles/" + qval(q) + "?pageSize=" + itoa(n)
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "doaj", Title: collapseSpace(str(raw, "bibjson.title")), Venue: str(raw, "bibjson.journal.title"),
				Year: intv(raw, "bibjson.year"), Abstract: str(raw, "bibjson.abstract")}
			each(raw, "bibjson.author", func(a json.RawMessage) {
				r.Authors = append(r.Authors, str(a, "name"))
			})
			each(raw, "bibjson.identifier", func(id json.RawMessage) {
				if str(id, "type") == "doi" {
					r.DOI = str(id, "id")
				}
			})
			r.URL = str(raw, "bibjson.link.0.url")
			if r.DOI != "" && r.URL == "" {
				r.URL = "https://doi.org/" + r.DOI
			}
			return r
		},
	},
	{name: "osf", label: "OSF (Open Science Framework)", sel: "data",
		url: func(q string, n, since int) string {
			return "https://api.osf.io/v2/nodes/?search=" + qval(q) + "&page[size]=" + itoa(n)
		},
		one: func(raw json.RawMessage) Result {
			return Result{Source: "osf", Title: str(raw, "attributes.title"), Abstract: str(raw, "attributes.description"),
				Venue: str(raw, "attributes.category"), URL: str(raw, "links.html"),
				Year: parseYear(str(raw, "attributes.date_created"))}
		},
	},
	{name: "dryad", label: "Dryad (research datasets)", sel: "_embedded.stash:datasets",
		url: func(q string, n, since int) string {
			return "https://datadryad.org/api/v2/search?q=" + qval(q) + "&limit=" + itoa(n)
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "dryad", Title: str(raw, "title"), Abstract: stripHTML(str(raw, "abstract"))}
			if d := str(raw, "identifier"); strings.HasPrefix(d, "doi:") {
				r.DOI = strings.TrimPrefix(d, "doi:")
			}
			if r.DOI != "" {
				r.URL = "https://doi.org/" + r.DOI
			}
			each(raw, "authors", func(a json.RawMessage) {
				r.Authors = append(r.Authors, strings.TrimSpace(str(a, "firstName")+" "+str(a, "lastName")))
			})
			r.Year = parseYear(str(raw, "publicationDate"))
			return r
		},
	},
	{name: "europepmc", label: "Europe PMC", sel: "resultList.result",
		url: func(q string, n, since int) string {
			query := q
			if since > 0 {
				query = "(" + q + ") AND PUB_YEAR:[" + itoa(since) + " TO 3000]"
			}
			return "https://www.ebi.ac.uk/europepmc/webservices/rest/search?query=" + qval(query) +
				"&format=json&pageSize=" + itoa(n) + "&resultType=core"
		},
		one: func(raw json.RawMessage) Result {
			return Result{
				Source: "europepmc", Title: str(raw, "title"), DOI: str(raw, "doi"), Venue: str(raw, "journalTitle"),
				Citations: intv(raw, "citedByCount"), Abstract: str(raw, "abstractText"),
				URL:        "https://europepmc.org/article/" + str(raw, "source") + "/" + str(raw, "id"),
				Identifier: str(raw, "source") + ":" + str(raw, "id"),
				Year:       parseYear(str(raw, "pubYear")),
				Authors:    splitAuthorList(str(raw, "authorString")),
			}
		},
	},
	{name: "hal", label: "HAL", sel: "response.docs",
		url: func(q string, n, since int) string {
			return "https://api.archives-ouvertes.fr/search/?q=" + qval(q) + "&wt=json&rows=" + itoa(n) +
				"&fl=title_s,uri_s,producedDate_s,doiId_s,authFullName_s,citedBy_s,journalTitle_s"
		},
		one: func(raw json.RawMessage) Result {
			return Result{Source: "hal", Title: str(raw, "title_s"), URL: str(raw, "uri_s"), DOI: str(raw, "doiId_s"),
				Venue: str(raw, "journalTitle_s"), Citations: intv(raw, "citedBy_s"),
				Authors: strAll(raw, "authFullName_s"), Year: parseYear(str(raw, "producedDate_s"))}
		},
	},
	{name: "inspire", label: "INSPIRE-HEP", sel: "hits.hits",
		url: func(q string, n, since int) string {
			query := q
			if since > 0 {
				query += " and date >= " + itoa(since)
			}
			return "https://inspirehep.net/api/literature?q=" + qval(query) + "&size=" + itoa(n) +
				"&fields=titles,authors,publication_info,dois,arxiv_eprints,id"
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "inspire", Title: str(raw, "metadata.titles.0.title"),
				Year: intv(raw, "metadata.publication_info.0.year"), Venue: str(raw, "metadata.publication_info.0.journal_title"),
				DOI: str(raw, "metadata.dois.0.value")}
			if a := str(raw, "metadata.arxiv_eprints.0.value"); a != "" {
				r.Identifier = "arXiv:" + a
			}
			each(raw, "metadata.authors", func(a json.RawMessage) {
				r.Authors = append(r.Authors, str(a, "full_name"))
			})
			if r.DOI != "" {
				r.URL = "https://doi.org/" + r.DOI
			} else if r.Identifier != "" {
				r.URL = "https://arxiv.org/abs/" + strings.TrimPrefix(r.Identifier, "arXiv:")
			} else if id := str(raw, "id"); id != "" {
				r.URL = "https://inspirehep.net/literature/" + id
			} else if id := intv(raw, "id"); id > 0 {
				r.URL = "https://inspirehep.net/literature/" + itoa(id)
			}
			return r
		},
	},
	{name: "zenodo", label: "Zenodo", sel: "hits.hits",
		url: func(q string, n, since int) string {
			query := q
			if since > 0 {
				query = "publication_date:[" + itoa(since) + "-01-01 TO *] AND (" + q + ")"
			}
			return "https://zenodo.org/api/records?q=" + qval(query) + "&size=" + itoa(n) + "&sort=mostrecent"
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "zenodo", Title: str(raw, "metadata.title"), Venue: str(raw, "metadata.resource_type.title"),
				DOI: str(raw, "doi"), Abstract: stripHTML(str(raw, "metadata.description")),
				Year: parseYear(str(raw, "metadata.publication_date"))}
			each(raw, "metadata.creators", func(c json.RawMessage) {
				r.Authors = append(r.Authors, str(c, "name"))
			})
			if u := str(raw, "links.doi"); u != "" {
				r.URL = u
			} else {
				r.URL = str(raw, "links.self")
			}
			return r
		},
	},

	// ---- keyed (6; enabled when the credential exists) ---------------------
	{name: "core", label: "CORE (full-text aggregator)", keyed: true, auth: authBearer, secret: "core", sel: "results",
		url: func(q string, n, since int) string {
			return "https://api.core.ac.uk/v3/search/works?q=" + qval(q) + "&limit=" + itoa(n)
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "core", Title: str(raw, "title"), Year: intv(raw, "yearPublished"),
				DOI: str(raw, "doi"), URL: str(raw, "url"), Abstract: str(raw, "abstract")}
			each(raw, "authors", func(a json.RawMessage) {
				r.Authors = append(r.Authors, str(a, "name"))
			})
			return r
		},
	},
	{name: "ads", label: "NASA ADS / SciX (astrophysics)", keyed: true, auth: authBearer, secret: "ads", sel: "response.docs",
		url: func(q string, n, since int) string {
			e := "https://api.adsabs.harvard.edu/v1/search/query?q=" + qval(q) +
				"&rows=" + itoa(n) + "&fl=title,author,year,doi,bibcode,citation_count,abstract"
			if since > 0 {
				e += "&fq=" + qval("year:["+itoa(since)+" TO *]") // year is an int field
			}
			return e
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "ads", Identifier: str(raw, "bibcode"), Citations: intv(raw, "citation_count"),
				Title: str(raw, "title"), Authors: strAll(raw, "author"),
				Year: parseYear(str(raw, "year")), DOI: str(raw, "doi"), Abstract: str(raw, "abstract")}
			if b := r.Identifier; b != "" {
				r.URL = "https://ui.adsabs.harvard.edu/abs/" + b + "/abstract"
			}
			return r
		},
	},
	{name: "springer", label: "Springer Nature", keyed: true, auth: authKeyParam, secret: "springer", keyParam: "api_key", sel: "records",
		url: func(q string, n, since int) string {
			e := "https://api.springernature.com/meta/v2/json?q=" + qval(q) + "&p=" + itoa(n)
			if since > 0 {
				e += "&date=from:" + itoa(since) + "-01-01"
			}
			return e
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "springer", Title: str(raw, "title"), Venue: str(raw, "publicationName"),
				DOI: str(raw, "doi"), URL: str(raw, "url"), Abstract: str(raw, "abstract"),
				Year: parseYear(str(raw, "publicationDate"))}
			each(raw, "creators", func(c json.RawMessage) {
				r.Authors = append(r.Authors, str(c, "creator"))
			})
			return r
		},
	},
	{name: "ieee", label: "IEEE Xplore", keyed: true, auth: authKeyParam, secret: "ieee", keyParam: "apikey", sel: "articles",
		url: func(q string, n, since int) string {
			e := "https://ieeexploreapi.ieee.org/api/v1/search/articles?querytext=" + qval(q) +
				"&max_records=" + itoa(n) + "&format=json"
			if since > 0 {
				e += "&start_year=" + itoa(since)
			}
			return e
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "ieee", Title: str(raw, "title"), Year: intv(raw, "publication_year"),
				Venue: str(raw, "publication_title"), DOI: str(raw, "doi"), Abstract: str(raw, "abstract"),
				Citations: intv(raw, "citation_count")}
			each(raw, "authors", func(a json.RawMessage) {
				r.Authors = append(r.Authors, str(a, "full_name"))
			})
			if n := str(raw, "article_number"); n != "" {
				r.URL = "https://ieeexplore.ieee.org/document/" + n
			} else if r.DOI != "" {
				r.URL = "https://doi.org/" + r.DOI
			}
			return r
		},
	},
	{name: "gscholar", label: "Google Scholar (via SerpAPI)", keyed: true, auth: authKeyParam, secret: "gscholar",
		keyParam: "api_key", sel: "organic_results",
		url: func(q string, n, since int) string {
			return "https://serpapi.com/search.json?engine=google_scholar&q=" + qval(q) + "&num=" + itoa(n)
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "gscholar", Title: str(raw, "title"), URL: str(raw, "link"),
				Abstract: str(raw, "snippet"), Identifier: str(raw, "result_id")}
			for _, m := range serpYearRe.FindAllString(str(raw, "publication_info.summary"), -1) {
				if y := parseYear(m); y > 0 {
					r.Year = y
					break
				}
			}
			each(raw, "publication_info.authors", func(a json.RawMessage) {
				r.Authors = append(r.Authors, str(a, "name"))
			})
			parts := strings.Split(str(raw, "publication_info.summary"), " - ")
			if len(parts) > 0 && parts[len(parts)-1] != "" && !serpYearRe.MatchString(parts[len(parts)-1]) {
				r.Venue = parts[len(parts)-1]
			} else {
				for i, seg := range parts {
					if serpYearRe.MatchString(seg) && i > 0 && r.Venue == "" {
						r.Venue = parts[i-1]
					}
				}
			}
			r.Citations = intv(raw, "inline_links.cited_by.total")
			return r
		},
	},
	{name: "lens", label: "Lens.org (scholarly + patents)", keyed: true, auth: authPostBearer, secret: "lens", sel: "data",
		url: func(q string, n, since int) string {
			return "https://api.lens.org/scholarly/search"
		},
		postBody: func(q string, n, since int) []byte {
			payload := map[string]any{
				"query":   map[string]any{"match": map[string]string{"title.en": q}},
				"size":    n,
				"include": []string{"title", "authors", "year_published", "source", "doi", "external_ids", "citation_count", "abstract", "id"},
				"scroll":  "false",
			}
			if since > 0 {
				payload["filter"] = map[string]any{"range": map[string]int{"year_published": since}}
			}
			b, _ := json.Marshal(payload)
			return b
		},
		one: func(raw json.RawMessage) Result {
			r := Result{Source: "lens", Title: str(raw, "title"), Year: intv(raw, "year_published"),
				Venue: str(raw, "source"), DOI: str(raw, "doi"), Citations: intv(raw, "citation_count"), Abstract: str(raw, "abstract")}
			each(raw, "authors", func(a json.RawMessage) {
				if n := strings.TrimSpace(str(a, "first_name") + " " + str(a, "last_name")); n != "" {
					r.Authors = append(r.Authors, n)
				}
			})
			each(raw, "external_ids", func(id json.RawMessage) {
				if str(id, "type") == "doi" {
					r.DOI = str(id, "value")
				}
			})
			if r.DOI != "" {
				r.URL = "https://doi.org/" + r.DOI
			} else if id := str(raw, "id"); id != "" {
				r.URL = "https://www.lens.org/lens/scholar/" + id
			}
			return r
		},
	},
}

// buildRegistration turns the spec table into provider registries.
func buildRegistration() (free, keyed []provider) {
	for _, s := range allSpecs {
		p := buildProvider(s)
		if s.keyed {
			keyed = append(keyed, p)
		} else {
			free = append(free, p)
		}
	}
	return free, keyed
}

func buildProvider(s spec) provider {
	p := provider{name: s.name, label: s.label, keyed: s.keyed}
	if s.search != nil {
		p.search = s.search
		return p
	}
	p.search = func(q string, n, since int) ([]Result, error) { return fetchList(s, q, n, since) }
	return p
}

// fetchList runs the generic json/post provider: build endpoint, authenticate,
// fetch, walk the record array, map each record, apply the since filter.
func fetchList(s spec, q string, n, since int) ([]Result, error) {
	var body []byte
	if s.postBody != nil {
		body = s.postBody(q, n, since)
	}
	raw, err := callAPI(s, s.url(q, n, since), body)
	if err != nil {
		return nil, err
	}
	var root json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	arr := at(root, s.sel)
	var records []json.RawMessage
	if len(arr) > 0 && json.Unmarshal(arr, &records) != nil {
		return nil, fmt.Errorf("%s: record selector %q is not an array", s.label, s.sel)
	}
	results := make([]Result, 0, len(records))
	for _, rec := range records {
		r := s.one(rec)
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// callAPI applies the spec's auth and performs the request.
func callAPI(s spec, endpoint string, body []byte) ([]byte, error) {
	key := ""
	if s.keyed {
		key = keyOf(s.secret)
	}
	switch s.auth {
	case authBearer:
		return getRawWithBearer(endpoint, key)
	case authKeyParam:
		if key != "" {
			sep := "?"
			if strings.Contains(endpoint, "?") {
				sep = "&"
			}
			endpoint += sep + qval(s.keyParam) + "=" + qval(key)
		}
		return getRaw(endpoint)
	case authPostBearer:
		req, err := newPost(endpoint, body, key)
		if err != nil {
			return nil, err
		}
		return doReq(req)
	default:
		return getRaw(endpoint)
	}
}

// ----------------------------------------------------------------------------
// Raw-accessor helpers (shared by every `one` extractor).
// ----------------------------------------------------------------------------

// at resolves a dot-path within raw JSON (nil for missing). Numeric steps
// index into arrays: "a.0.b" = b inside the first element of a. Object keys
// containing dots are not supported (none of the APIs use them).
func at(raw json.RawMessage, p string) json.RawMessage {
	cur := raw
	for _, step := range strings.Split(p, ".") {
		if len(cur) == 0 {
			return nil
		}
		if i, err := strconv.Atoi(step); err == nil {
			var arr []json.RawMessage
			if json.Unmarshal(cur, &arr) != nil || i >= len(arr) {
				return nil
			}
			cur = arr[i]
			continue
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(cur, &m) != nil {
			return nil
		}
		next, ok := m[step]
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

// str/intv/each/strAll: typed accessors over a record.
func str(raw json.RawMessage, p string) string { return jstr(at(raw, p)) }
func intv(raw json.RawMessage, p string) int   { return jint(at(raw, p)) }
func each(raw json.RawMessage, p string, fn func(json.RawMessage)) {
	arr := at(raw, p)
	var items []json.RawMessage
	if json.Unmarshal(arr, &items) == nil {
		for _, it := range items {
			fn(it)
		}
	}
}
func strAll(raw json.RawMessage, p string) []string {
	v := at(raw, p)
	var a []string
	if len(v) > 0 && json.Unmarshal(v, &a) == nil {
		return a
	}
	var s string
	if len(v) > 0 && json.Unmarshal(v, &s) == nil && s != "" {
		return []string{s}
	}
	return nil
}

// jstr accepts string, [string] or missing; jint accepts number or "NNNN" string.
func jstr(v json.RawMessage) string {
	if len(v) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	var arr []string
	if json.Unmarshal(v, &arr) == nil && len(arr) > 0 {
		return arr[0]
	}
	return ""
}

func jint(v json.RawMessage) int {
	var i int
	if json.Unmarshal(v, &i) == nil {
		return i
	}
	return parseYear(jstr(v))
}

// abstractInverted reconstructs an abstract from OpenAlex's word->position map.
func abstractInverted(raw json.RawMessage) string {
	v := at(raw, "abstract_inverted_index")
	var ix map[string][]int
	if json.Unmarshal(v, &ix) != nil || len(ix) == 0 {
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
	return strings.Join(strings.Fields(strings.Join(words, " ")), " ")
}
