# papersearch

Search 21 academic APIs (15 free, 6 key-gated) from one command; stdlib only.

```sh
$ papersearch "attention is all you need"
$ papersearch -n 10 -since 2023 -json -api arxiv,openalex "multi-agent llm" | jq
$ papersearch -api all "gravitational waves"
```

## Providers

### Free (15 — on by default)

| name | endpoint | covers |
|---|---|---|
| `arxiv` | https://export.arxiv.org/api/query | preprints (CS, math, physics) |
| `crossref` | https://api.crossref.org/works | ~180M works, DOI registry |
| `openalex` | https://api.openalex.org/works | ~250M works, citations, OA links |
| `scholar` | https://api.semanticscholar.org/graph/v1/paper/search | ~200M papers; aggressive rate limits |
| `pubmed` | E-utilities (esearch→esummary) | ~40M biomedical citations |
| `europepmc` | https://www.ebi.ac.uk/europepmc/webservices/rest/search | 48M+ life sciences + preprints |
| `dblp` | https://dblp.org/search/publ/api | CS bibliography, 6M+ |
| `datacite` | https://api.datacite.org/dois | dataset/software DOIs (legacy `/works` is dead) |
| `doaj` | https://doaj.org/api/v3/search/articles | open-access journals |
| `hal` | https://api.archives-ouvertes.fr/search | French open archive |
| `inspire` | https://inspirehep.net/api/literature | high-energy physics |
| `zenodo` | https://zenodo.org/api/records | CERN research data + preprints |
| `osf` | https://api.osf.io/v2/nodes | open-science projects/preprints |
| `openaire` | https://api.openaire.eu/search/publications | EU-funded research (XML) |
| `dryad` | https://datadryad.org/api/v2/search | research datasets |

### Keyed (6 — enabled when the env var is set)

Env var `PAPERSEARCH_<NAME>_KEY` (ADS uses `..._TOKEN`); without it the provider
is skipped (`skipped: <name> (set the env key)`). Keys also fall back to
`/run/secrets/papersearch-<name>` (NixOS credentials).

| name | env var | endpoint |
|---|---|---|
| `core` | `PAPERSEARCH_CORE_KEY` | https://api.core.ac.uk/v3/search/works |
| `ads` | `PAPERSEARCH_ADS_TOKEN` | https://api.adsabs.harvard.edu/v1/search/query (astrophysics) |
| `lens` | `PAPERSEARCH_LENS_KEY` | https://api.lens.org/scholarly/search |
| `springer` | `PAPERSEARCH_SPRINGER_KEY` | https://api.springernature.com/meta/v2/json |
| `ieee` | `PAPERSEARCH_IEEE_KEY` | https://ieeexploreapi.ieee.org/api/v1/search/articles |
| `gscholar` | `PAPERSEARCH_SERPAPI_KEY` | https://serpapi.com/search.json (engine=google_scholar) — Google Scholar, since it has no API; paid quota |

### Not keyword-searchable (documented only)

Google Scholar (no official API), OpenCitations + Unpaywall (DOI lookup only),
ORCID (author records).

## Usage

```
papersearch [flags] "query"

  -api string   comma-separated providers or 'all' (default: all 15 free)
  -n int        results per provider (default 5, max 50)
  -since int    only records from this year onward
  -json         emit a JSON array (jq-friendly)
  -dedupe       drop cross-provider duplicates by DOI / title (default true)
  -timeout d    per-request timeout (default 20s)
  -q            suppress per-provider error notes
```

Output is grouped per provider: title, authors, year, venue, citation count, URL.
A failing provider (e.g. Semantic Scholar rate limits) prints a warning and is
skipped — the rest still run.

## Build

```sh
go build -o papersearch .
```

## Design

- 4 files: `main.go` (CLI), `providers.go` (one table row per API: auth kind +
  endpoint + record path + field extractor), `bespoke.go` (the 5 irregular
  providers: arXiv/OpenAIRE XML, PubMed two-step, DBLP quirks), `support.go`
  (HTTP + helpers).
- Requests: 3 concurrent, one bounded retry (2s) on 429/5xx, 20s timeout,
  64 MiB cap, `mailto` for the Crossref/OpenAlex polite pools.
- Duplicates across providers are collapsed by DOI (or title) for `-json`.
- Parsers are hardened against inconsistent field types seen live (string-vs-
  array, HTML entities, legacy endpoints).
