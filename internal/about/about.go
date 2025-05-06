package about

type Datum struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description,omitempty"`
	License      string   `json:"license"`
	Contributors []string `json:"contributors,omitempty"`
	Homepage     string   `json:"homepage,omitempty"`
	Repository   string   `json:"repository,omitempty"`
}

func About() Datum {
	return Datum{
		Name:        "SyncopateDB",
		Description: "A flexible, lightweight data store with advanced query capabilities",
		Version:     "0.3.0",
		License:     "MIT",
		Contributors: []string{
			"The Phillarmonic Software Team <the PhillarMonkeys>",
		},
	}
}
