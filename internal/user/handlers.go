package user

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// POST /users/library — Add manga to user’s library
func AddToLibraryHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Query("user_id")
		mangaID := c.Query("manga_id")

		if userID == "" || mangaID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing user_id or manga_id"})
			return
		}

		// Convert mangaID to integer
		mangaInt, err := strconv.Atoi(mangaID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid manga_id"})
			return
		}

		// Ensure manga exists
		var exists bool
		err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM manga WHERE id = ?)", mangaInt).Scan(&exists)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manga not found"})
			return
		}

		_, err = db.Exec(`INSERT OR IGNORE INTO user_library (user_id, manga_id) VALUES (?, ?)`, userID, mangaInt)
		if err != nil {
			log.Println("Insert error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database insert failed"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Manga added to library"})
	}
}

// GET /users/library — Get user’s library
func GetLibraryHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Query("user_id")
		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing user_id"})
			return
		}

		rows, err := db.Query(`
            SELECT m.id, m.title, m.cover_url, m.status, m.source
            FROM manga m
            JOIN user_library ul ON m.id = ul.manga_id
            WHERE ul.user_id = ?`, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
			return
		}
		defer rows.Close()

		type LibraryItem struct {
			ID       int    `json:"id"`
			Title    string `json:"title"`
			CoverURL string `json:"cover_url"`
			Status   string `json:"status"`
			Source   string `json:"source"`
		}

		var library []LibraryItem
		for rows.Next() {
			var item LibraryItem
			if err := rows.Scan(&item.ID, &item.Title, &item.CoverURL, &item.Status, &item.Source); err != nil {
				continue
			}
			library = append(library, item)
		}

		c.JSON(http.StatusOK, gin.H{"library": library})
	}
}

// PUT /users/progress — Update reading progress with timestamp
func UpdateProgressHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Query("user_id")
		mangaID := c.Query("manga_id")
		chapterStr := c.Query("chapter")

		chapter, _ := strconv.Atoi(chapterStr)
		if userID == "" || mangaID == "" || chapter <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing or invalid parameters"})
			return
		}

		mangaInt, err := strconv.Atoi(mangaID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid manga_id"})
			return
		}

		timestamp := time.Now().Unix()

		_, err = db.Exec(`
            INSERT INTO user_progress (user_id, manga_id, current_chapter, updated_at)
            VALUES (?, ?, ?, ?)
            ON CONFLICT(user_id, manga_id) DO UPDATE 
            SET current_chapter = excluded.current_chapter, updated_at = excluded.updated_at`,
			userID, mangaInt, chapter, timestamp)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database update failed"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":    "Progress updated",
			"chapter":    chapter,
			"updated_at": timestamp,
		})
	}
}

// DELETE /users/library — remove manga from user’s library
func RemoveFromLibraryHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Query("user_id")
		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing user_id"})
			return
		}

		var req struct {
			MangaID string `json:"manga_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
			return
		}

		mangaInt, err := strconv.Atoi(req.MangaID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid manga_id"})
			return
		}

		result, err := db.Exec(`DELETE FROM user_library WHERE user_id = ? AND manga_id = ?`, userID, mangaInt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove manga"})
			return
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"message": "Manga not found in library"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Manga removed from library"})
	}
}
