package main

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
