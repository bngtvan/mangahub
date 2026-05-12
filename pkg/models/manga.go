package models

type Manga struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes struct {
		Title       map[string]string   `json:"title"`
		AltTitles   []map[string]string `json:"altTitles"`
		Description map[string]string   `json:"description"`
		Status      string              `json:"status"`
		Tags        []struct {
			Atttributes struct {
				Name map[string]string `json:"name"`
			} `json:"attributes"`
		} `json:"tags"`
	} `json:"attributes"`
	Relationships []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			FileName string `json:"fileName"`
		} `json:"attributes"`
	} `json:"relationships"`
}

type MangaDetails struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Authors       []string `json:"author"`
	Genres        []string `json:"genres"`
	Status        string   `json:"status"`
	TotalChapters int      `json:"total_chapters"`
	Description   string   `json:"description"`
	CoverURL      string   `json:"cover_url"`
	Source        string   `json:"source"`
}
