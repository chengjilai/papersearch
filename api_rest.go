package main

import (
	"encoding/json"
	"strings"
)

// The simple "GET a JSON list of records" providers, expressed with the
// docProvider runner in support.go — no per-provider response structs.
var docProviders = []docProvider{
	{
		name: "crossref", label: "Crossref", sel: "message.items",
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
	{
		name: "openalex", label: "OpenAlex", sel: "results",
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
	{
		name: "datacite", label: "DataCite", sel: "data",
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
	{
		name: "doaj", label: "DOAJ", sel: "results",
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
	{
		name: "osf", label: "OSF (Open Science Framework)", sel: "data",
		url: func(q string, n, since int) string {
			return "https://api.osf.io/v2/nodes/?search=" + qval(q) + "&page[size]=" + itoa(n)
		},
		one: func(raw json.RawMessage) Result {
			return Result{Source: "osf", Title: str(raw, "attributes.title"), Abstract: str(raw, "attributes.description"),
				Venue: str(raw, "attributes.category"), URL: str(raw, "links.html"),
				Year: parseYear(str(raw, "attributes.date_created"))}
		},
	},
	{
		name: "dryad", label: "Dryad (research datasets)", sel: "_embedded.stash:datasets",
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
	{
		name: "europepmc", label: "Europe PMC", sel: "resultList.result",
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
	{
		name: "hal", label: "HAL", sel: "response.docs",
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
	{
		name: "inspire", label: "INSPIRE-HEP", sel: "hits.hits",
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
	{
		name: "zenodo", label: "Zenodo", sel: "hits.hits",
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
}

// str/intv/each/strAll: raw-typed convenience wrappers over at().
func str(raw json.RawMessage, p string) string { return jstr(at(raw, p)) }
func intv(raw json.RawMessage, p string) int   { return jint(at(raw, p)) }
func each(raw json.RawMessage, p string, fn func(json.RawMessage)) {
	arr := at(raw, p)
	var items []json.RawMessage
	if json.Unmarshal(arr, &items) == nil || len(arr) == 0 {
		for _, it := range items {
			fn(it)
		}
	}
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
