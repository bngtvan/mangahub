package source

import (
	"encoding/json"
	"fmt"
	"mangahub/pkg/models"
	"net/http"
)

// Fetch for Manga Title
func resolveTitle(m models.Manga) string {
	// Try English first
	if val, ok := m.Attributes.Title["en"]; ok && val != "" {
		return val
	}
	// Check for alTitle for English
	for _, alt := range m.Attributes.AltTitles {
		if en, ok := alt["en"]; ok && en != "" {
			return en
		}
	}

	// If not available English title, take main title in any language
	for _, lang := range []string{"ja-ro", "ja", "fr", "es"} {
		if val, ok := m.Attributes.Title[lang]; ok && val != "" {
			return val
		}
	}

	return "Unknown Title"
}

func getTotalChapters(mangaID string) int {
	url := fmt.Sprintf("https://api.mangadex.org/chapter?manga=%s&limit=1", mangaID)
	resp, err := http.Get(url)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	var result struct {
		Total int `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0
	}
	return result.Total
}

// Fetch author names from MangaDex Author endpoint
func getAuthorName(authorID string) string {
	url := fmt.Sprintf("https://api.mangadex.org/author/%s", authorID)
	resp, err := http.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}
	return result.Data.Attributes.Name
}

func MangaDex() ([]models.MangaDetails, error) {
	url := "https://api.mangadex.org/manga?limit=100&includes[]=cover_art"

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []models.Manga `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var all []models.MangaDetails

	for _, m := range result.Data {
		title := resolveTitle(m)

		// Collect genres from tags
		var genres []string
		for _, tag := range m.Attributes.Tags {
			if en, ok := tag.Atttributes.Name["en"]; ok && en != "" {
				genres = append(genres, en)
			}
		}

		// Cover URL from relationships
		coverURL := ""
		for _, rel := range m.Relationships {
			if rel.Type == "cover_art" {
				coverURL = fmt.Sprintf("https://uploads.mangadex.org/covers/%s/%s", m.ID, rel.Attributes.FileName)
			}
		}

		// Collect author names from relationships
		var authors []string
		for _, rel := range m.Relationships {
			if rel.Type == "author" {
				name := getAuthorName(rel.ID)
				if name != "" {
					authors = append(authors, name)
				}
			}
		}

		// Get chapter count
		totalChapters := getTotalChapters(m.ID)

		all = append(all, models.MangaDetails{
			ID:            m.ID,
			Title:         title,
			Authors:       authors,
			Genres:        genres,
			Status:        m.Attributes.Status,
			TotalChapters: totalChapters,
			Description:   m.Attributes.Description["en"],
			CoverURL:      coverURL,
			Source:        "mangadex",
		})
	}

	return all, nil
}
