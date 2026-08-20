package main

// Result is a single search hit, normalized across all providers.
type Result struct {
	Source     string   `json:"source"`
	Title      string   `json:"title"`
	Authors    []string `json:"authors,omitempty"`
	Year       int      `json:"year,omitempty"`
	Venue      string   `json:"venue,omitempty"` // journal / conference / repo name
	DOI        string   `json:"doi,omitempty"`
	URL        string   `json:"url,omitempty"`
	Citations  int      `json:"citations,omitempty"`
	Abstract   string   `json:"abstract,omitempty"`
	Identifier string   `json:"identifier,omitempty"` // arXiv id, PMID, bibcode, ...
}

// provider describes one search backend.
type provider struct {
	name   string // flag value, e.g. "arxiv"
	label  string // display name, e.g. "arXiv"
	search func(q string, n int, since int) ([]Result, error)
	keyed  bool // needs env var (CORE/ADS/Lens/Springer/IEEE)
}

func (r Result) yearOrNA() string {
	if r.Year == 0 {
		return "n.d."
	}
	return itoa(r.Year)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
