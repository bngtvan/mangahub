package manga

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GET /manga — search manga with filters
func GetMangaListHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := c.Query("status")
		genre := c.Query("genre")
		pageStr := c.Query("page")
		limitStr := c.Query("limit")

		page, _ := strconv.Atoi(pageStr)
		limit, _ := strconv.Atoi(limitStr)
		if page <= 0 {
			page = 1
		}
		if limit <= 0 || limit > 50 {
			limit = 10
		}

		offset := (page - 1) * limit

		query := `SELECT id, source_id, title, author, genres, status, total_chapters, description, cover_url, source 
                  FROM manga WHERE 1=1`
		args := []interface{}{}

		if status != "" {
			query += ` AND status = ?`
			args = append(args, status)
		}
		if genre != "" {
			query += ` AND genres LIKE ?`
			args = append(args, "%"+genre+"%")
		}

		query += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)

		rows, err := db.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
			return
		}
		defer rows.Close()

		type Manga struct {
			ID            int    `json:"id"`        // internal sequential ID
			SourceID      string `json:"source_id"` // external AniList/MangaDex/Manual ID
			Title         string `json:"title"`
			Author        string `json:"author"`
			Genres        string `json:"genres"`
			Status        string `json:"status"`
			TotalChapters int    `json:"total_chapters"`
			Description   string `json:"description"`
			CoverURL      string `json:"cover_url"`
			Source        string `json:"source"`
		}

		var mangas []Manga
		for rows.Next() {
			var m Manga
			if err := rows.Scan(&m.ID, &m.SourceID, &m.Title, &m.Author, &m.Genres, &m.Status,
				&m.TotalChapters, &m.Description, &m.CoverURL, &m.Source); err != nil {
				continue
			}
			mangas = append(mangas, m)
		}

		c.JSON(http.StatusOK, gin.H{
			"page":   page,
			"limit":  limit,
			"result": mangas,
		})
	}
}

// GET /manga/:id — get manga details by internal ID or source_id
// GET /manga/:id — get manga details by internal ID or source_id query
func GetMangaByIDHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		sourceID := c.Query("source_id")

		var manga struct {
			ID            int    `json:"id"`
			SourceID      string `json:"source_id"`
			Title         string `json:"title"`
			Author        string `json:"author"`
			Genres        string `json:"genres"`
			Status        string `json:"status"`
			TotalChapters int    `json:"total_chapters"`
			Description   string `json:"description"`
			CoverURL      string `json:"cover_url"`
			Source        string `json:"source"`
		}

		var err error
		if sourceID != "" {
			// Explicit search by external source_id
			err = db.QueryRow(`
                SELECT id, source_id, title, author, genres, status, total_chapters, description, cover_url, source
                FROM manga WHERE source_id = ?`, sourceID).
				Scan(&manga.ID, &manga.SourceID, &manga.Title, &manga.Author, &manga.Genres,
					&manga.Status, &manga.TotalChapters, &manga.Description, &manga.CoverURL, &manga.Source)
		} else {
			// Default search by internal sequential ID
			idInt, convErr := strconv.Atoi(idStr)
			if convErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid manga ID"})
				return
			}
			err = db.QueryRow(`
                SELECT id, source_id, title, author, genres, status, total_chapters, description, cover_url, source
                FROM manga WHERE id = ?`, idInt).
				Scan(&manga.ID, &manga.SourceID, &manga.Title, &manga.Author, &manga.Genres,
					&manga.Status, &manga.TotalChapters, &manga.Description, &manga.CoverURL, &manga.Source)
		}

		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manga not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
			return
		}

		c.JSON(http.StatusOK, manga)
	}
}
