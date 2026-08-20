package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// contactEmail identifies the tool to Crossref/OpenAlex polite pools.
const contactEmail = "chengjilai@sjtu.edu.cn"

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
	label  string // display name for output
	search func(q string, n int, since int) ([]Result, error)
	keyed  bool // enabled only when its env key / /run/secrets file is present
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// retryCodes are transient failures worth one bounded retry (429 trad, 5xx overloads).
func retryCode(code int) bool {
	return code == 429 || code == 500 || code == 502 || code == 503 || code == 504
}

// doHTTP sends req, retrying up to retries times (with backoff) on transient
// failures. Replays the request body only via Clone (nil body for GETs; the
// POST callers pass retries=0 since their body is consumed once).
func doHTTP(req *http.Request, retries int) ([]byte, error) {
	backoff := []time.Duration{2 * time.Second, 4 * time.Second}
	for attempt := 0; ; attempt++ {
		r := req
		if attempt > 0 {
			r = req.Clone(req.Context())
		}
		resp, err := httpClient.Do(r)
		if err != nil && attempt < retries {
			time.Sleep(backoff[attempt])
			continue
		}
		if err == nil {
			body, rerr := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
			_ = resp.Body.Close()
			if rerr != nil {
				return nil, rerr
			}
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return body, nil
			}
			if attempt < retries && retryCode(resp.StatusCode) {
				time.Sleep(backoff[attempt])
				continue
			}
			msg := strings.TrimSpace(string(body))
			if len(msg) > 300 {
				msg = msg[:300]
			}
			return nil, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, req.URL, msg)
		}
		return nil, err
	}
}

func getRaw(endpoint string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "papersearch/0.1 (mailto:"+contactEmail+")")
	return doHTTP(req, 1)
}

func getRawWithBearer(endpoint, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "papersearch/0.1 (mailto:"+contactEmail+")")
	req.Header.Set("Authorization", "Bearer "+token)
	return doHTTP(req, 1)
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
	return doHTTP(req, 0) // POST bodies aren't replayable — no retry
}

func getJSON(endpoint string, out any) error {
	body, err := getRaw(endpoint)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// qval URL-escapes a search phrase.
func qval(q string) string { return url.QueryEscape(q) }

// itoa is a dependency-light int->string (avoiding fmt in hot paths).
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

func (r Result) yearOrNA() string {
	if r.Year == 0 {
		return "n.d."
	}
	return itoa(r.Year)
}

// parseYear extracts the leading 4+ digit year from a date string; 0 if none.
func parseYear(s string) int {
	y := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		y = y*10 + int(c-'0')
	}
	return y
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// collapseSpace replaces runs of whitespace (incl. newlines) with one space.
func collapseSpace(s string) string {
	run := false
	var b strings.Builder
	for _, c := range s {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if !run {
				b.WriteByte(' ')
				run = true
			}
		} else {
			b.WriteRune(c)
			run = false
		}
	}
	return strings.TrimSpace(b.String())
}

// stripHTML removes HTML tags and trims whitespace (Zenodo/Dryad descriptions).
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, c := range s {
		switch {
		case c == '<':
			inTag = true
		case c == '>':
			inTag = false
		case !inTag:
			b.WriteRune(c)
		}
	}
	return strings.TrimSpace(b.String())
}

// splitAuthorList splits "A, B, C, and D" author strings (Europe PMC style).
func splitAuthorList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "and ")
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, strings.TrimRight(part, "."))
		}
	}
	return out
}

// fmtAuthors renders authors "A, B, C, …" with "+N more" overflow.
func fmtAuthors(a []string, max int) string {
	if len(a) == 0 {
		return ""
	}
	var b strings.Builder
	for i, name := range a {
		if i == max {
			fmt.Fprintf(&b, "… +%d more", len(a)-max)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(name)
	}
	return b.String()
}

// keyOf reads a keyed provider's credential: env var wins, then /run/secrets/<file>.
func keyOf(name string) string {
	if v := strings.TrimSpace(os.Getenv(providerEnvVar[name])); v != "" {
		return v
	}
	if f, ok := providerSecretFile[name]; ok {
		if b, err := os.ReadFile("/run/secrets/" + f); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}
