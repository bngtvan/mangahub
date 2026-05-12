package source

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mangahub/pkg/models"
	"net/http"
	"strings"
)

const anilistQuery = `
query ($page: Int, $perPage: Int) {
  Page(page: $page, perPage: $perPage) {
    pageInfo {
      total
      currentPage
      lastPage
      hasNextPage
      perPage
    }
    media(type: MANGA, sort: POPULARITY_DESC) {
      id
      title { romaji english }
      genres
      status
      chapters
      description(asHtml: false)
      coverImage { large }
      staff(perPage: 1) {
        nodes { name { full } }
      }
    }
  }
}`

func AniList() ([]models.MangaDetails, error) {
	var all []models.MangaDetails

	page := 1
	for len(all) < 100 {
		// Build request body with current page
		body, _ := json.Marshal(map[string]any{
			"query": anilistQuery,
			"variables": map[string]any{
				"page":    page,
				"perPage": 50, // AniList max is 50
			},
		})

		resp, err := http.Post("https://graphql.anilist.co", "application/json", bytes.NewBuffer(body))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var result struct {
			Data struct {
				Page struct {
					PageInfo struct {
						Total       int  `json:"total"`
						CurrentPage int  `json:"currentPage"`
						LastPage    int  `json:"lastPage"`
						HasNextPage bool `json:"hasNextPage"`
						PerPage     int  `json:"perPage"`
					} `json:"pageInfo"`
					Media []struct {
						ID    int `json:"id"`
						Title struct {
							Romaji  string `json:"romaji"`
							English string `json:"english"`
						} `json:"title"`
						Genres      []string `json:"genres"`
						Status      string   `json:"status"`
						Chapters    int      `json:"chapters"`
						Description string   `json:"description"`
						CoverImage  struct {
							Large string `json:"large"`
						} `json:"coverImage"`
						Staff struct {
							Nodes []struct {
								Name struct {
									Full string `json:"full"`
								} `json:"name"`
							} `json:"nodes"`
						} `json:"staff"`
					} `json:"media"`
				} `json:"Page"`
			} `json:"data"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		// Collect results
		for _, m := range result.Data.Page.Media {

			// Fetch for English title
			title := m.Title.English
			if title == "" {
				title = m.Title.Romaji
			}

			// Fetch author names (can be multiple)
			var authors []string
			for _, node := range m.Staff.Nodes {
				name := strings.TrimSpace(node.Name.Full)
				if name != "" {
					authors = append(authors, name)
				}
			}

			// Change status for database synchronize
			status := ""
			if strings.ToLower(m.Status) == "finished" {
				status = "completed"
			} else if strings.ToLower(m.Status) == "releasing" {
				status = "ongoing"
			} else {
				status = strings.ToLower(m.Status)
			}

			all = append(all, models.MangaDetails{
				ID:            fmt.Sprintf("%d", m.ID),
				Title:         title,
				Authors:       authors,
				Genres:        m.Genres,
				Status:        status,
				TotalChapters: m.Chapters,
				Description:   m.Description,
				CoverURL:      m.CoverImage.Large,
				Source:        "anilist",
			})
		}

		// Stop if no more pages
		if !result.Data.Page.PageInfo.HasNextPage {
			break
		}
		page++
	}

	return all, nil
}
