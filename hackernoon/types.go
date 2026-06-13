package hackernoon

// Story is the record emitted for each Hackernoon article.
type Story struct {
	Rank    int    `json:"rank"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Summary string `json:"summary"`
	Tags    string `json:"tags"`
	URL     string `json:"url"`
}
