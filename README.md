# papersearch

Search scholarly literature across **21 APIs** — all free or key-gated (no paid
subscriptions) — from one Go command, using only the standard library.

```
$ papersearch "attention is all you need"
$ papersearch -n 10 -since 2023 -api arxiv,openalex,paper "multi-agent llm"
$ papersearch -json -api openalex "protein folding" | jq '.[0]'
```

## Providers

Counts: 15 free/no-key + 6 key-gated = 21.

### Free, no key (12 — all enabled by default)

| name | label | API endpoint (verified) | covers |
|---|---|---|---|
| `arxiv` | arXiv | https://export.arxiv.org/api/query | preprints (CS, math, physics) |
| `crossref` | Crossref | https://api.crossref.org/works | ~180M works in the DOI registry |
| `openalex` | OpenAlex | https://api.openalex.org/works | ~250M works + citation counts + OA links |
| `scholar` | Semantic Scholar | https://api.semanticscholar.org/graph/v1/paper/search | ~200M papers, AI-ranked |
| `pubmed` | PubMed | E-utilities (esearch→esummary) | ~40M biomedical citations |
| `europepmc` | Europe PMC | https://www.ebi.ac.uk/europepmc/webservices/rest/search | 48M+ life sciences + preprints |
| `dblp` | DBLP | https://dblp.org/search/publ/api | CS bibliography 6M+ |
| `datacite` | DataCite | https://api.datacite.org/dois | DOIs for datasets/software (legacy `/works` endpoint is dead) |
| `doaj` | DOAJ | https://doaj.org/api/v3/search/articles | open-access journals |
| `hal` | HAL | https://api.archives-ouvertes.fr/search | French open archive |
| `inspire` | INSPIRE-HEP | https://inspirehep.net/api/literature | high-energy physics |
| `zenodo` | Zenodo | https://zenodo.org/api/records | CERN research data + preprints |
| `osf` | OSF | https://api.osf.io/v2/nodes | open science projects/preprints (Center for Open Science) |
| `openaire` | OpenAIRE | https://api.openaire.eu/search/publications | EU-funded research (XML) |
| `dryad` | Dryad | https://datadryad.org/api/v2/search | research datasets (Dryad repository) |

### Free but key-gated (5 — enabled via env var)

Set `PAPERSEARCH_<NAME>_KEY` (or `..._TOKEN` for ADS). Requested without a
key, the tool prints `skipped: <name> (set the env key)` and continues.

| name | env var | endpoint |
|---|---|---|
| `core` | `PAPERSEARCH_CORE_KEY` | https://api.core.ac.uk/v3/search/works (free, registered) |
| `ads` | `PAPERSEARCH_ADS_TOKEN` | https://api.adsabs.harvard.edu/v1/search/query (astrophysics) |
| `lens` | `PAPERSEARCH_LENS_KEY` | https://api.lens.org/scholarly/search (scholarly + patents) |
| `springer` | `PAPERSEARCH_SPRINGER_KEY` | https://api.springernature.com/meta/v2/json |
| `ieee` | `PAPERSEARCH_IEEE_KEY` | https://ieeexploreapi.ieee.org/api/v1/search/articles |
| `gscholar` | `PAPERSEARCH_SERPAPI_KEY` | https://serpapi.com/search.json (engine=google_scholar; paid quota, key needed) |

### Not keyword-searchable (documented; free, no key)

- **Google Scholar** — has *no official API* at all; unpaid access is scraping
  only (ToS violation, CAPTCHA-blocked from datacenter IPs). Not implemented.
- **OpenCitations** — citations/metadata lookup *by DOI* (https://api.opencitations.net),
  no keyword search.
- **Unpaywall** — full-text availability lookup *by DOI* (https://api.unpaywall.org/v2/:doi?email=).
- **ORCID** — author records, not article search.

## Usage

```
Usage: papersearch [flags] "search phrase"

  -api string   comma-separated providers, or 'all'
                (default: all 12 free ones)
  -n int        max results per provider (default 5, max 50)
  -since int    only items from this year onward
  -timeout d    per-request timeout (default 20s)
  -json         emit JSON array of records (jq-friendly)
  -dedupe       collapse cross-provider duplicates by DOI / normalized title (default true)
  -q            suppress per-provider error notes
```

Output is grouped by provider: title, authors, year, venue, citation count,
and a URL. Errors from individual providers (e.g. Semantic Scholar's notorious
429 rate limiting from datacenter IPs) never abort the whole run.

## Build

```sh
go build -o papersearch .
# or install: go install .  (GOPATH/bin must be on PATH)
```

## Design notes

- Zero external dependencies; `encoding/json`, `encoding/xml`, `net/http` only.
- Requests are polite: a `User-Agent` identifying the tool, a `mailto` param for
  the Crossref/OpenAlex polite pools, at most 3 concurrent outbound requests,
  one bounded retry (2s backoff) on transient 429/5xx, 20s per-request timeout,
  and a 64 MiB response cap.
- Search phrases are used as-is per provider; each provider's own relevance
  ranking is preserved (results are grouped by source, not merged).
- All JSON shapes were verified against live API responses in Aug 2026; several
  providers return inconsistent types (string-vs-array fields, HTML entities,
  legacy endpoints) and the parsers are hardened against all of them.