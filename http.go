package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const contactEmail = "chengjilai@sjtu.edu.cn" // polite-pool identifier for Crossref/OpenAlex

var httpClient = &http.Client{Timeout: 20 * time.Second}

// getRaw issues GET and returns the body. Non-2xx becomes an error.
func getRaw(endpoint string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "papersearch/0.1 (mailto:"+contactEmail+")")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20)) // 64 MiB cap
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, endpoint, msg)
	}
	return body, nil
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// collapseSpace replaces runs of whitespace (incl. newlines) with a single space.
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

// fmtAuthors renders authors "A, B, C, …" with "+N more" overflow.
func fmtAuthors(a []string, max int) string {
	if len(a) == 0 {
		return ""
	}
	var b strings.Builder
	for i, name := range a {
		if i > 0 {
			b.WriteString(", ")
		}
		if i == max {
			b.WriteString(fmt.Sprintf("+%d more", len(a)-max))
			break
		}
		b.WriteString(name)
	}
	return b.String()
}
