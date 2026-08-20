package main

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
			if y, ok := atoiOK(d.Attributes.DateCreated[:4]); ok {
				r.Year = y
			}
		}
		if since > 0 && r.Year < since {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}
