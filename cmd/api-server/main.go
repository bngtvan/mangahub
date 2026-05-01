package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"mangahub/pkg/database"
	"mangahub/pkg/models"
)

type APIServer struct {
	Router    *gin.Engine
	Database  *sql.DB
	JWTSecret string
}

// setupRoutes định nghĩa tất cả các endpoint cần thiết theo đặc tả dự án [2]
func (s *APIServer) setupRoutes() {
	// Authentication API group
	auth := s.Router.Group("/auth")
	{
		auth.POST("/register", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "POST /auth/register - Register an account (no logic yet)"})
		})
		auth.POST("/login", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "POST /auth/login - Log in (no logic yet)"})
		})
	}

	manga := s.Router.Group("/manga")
	{
		// 1. GET /manga?query=one - Search or list manga
		manga.GET("", func(c *gin.Context) {
			queryParam := c.Query("query")
			var rows *sql.Rows
			var err error

			// UC-003: Use a LIKE pattern for search [4]
			if queryParam != "" {
				searchPattern := "%" + queryParam + "%"
				rows, err = s.Database.Query(`
					SELECT id, title, author, genres, status, total_chapters, description 
					FROM manga WHERE title LIKE ?`, searchPattern)
			} else {
				rows, err = s.Database.Query(`
					SELECT id, title, author, genres, status, total_chapters, description 
					FROM manga`)
			}

			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
				return
			}
			defer rows.Close()

			var results []models.Manga
			for rows.Next() {
				var m models.Manga
				var genresText string

				if err := rows.Scan(&m.ID, &m.Title, &m.Author, &genresText, &m.Status, &m.TotalChapters, &m.Description); err != nil {
					continue
				}
				// Parse the JSON genres array stored as text [3]
				json.Unmarshal([]byte(genresText), &m.Genres)
				results = append(results, m)
			}

			c.JSON(http.StatusOK, gin.H{"data": results})
		})

		// 2. GET /manga/{id} - Get manga details
		manga.GET("/:id", func(c *gin.Context) {
			id := c.Param("id")
			var m models.Manga
			var genresText string

			// UC-004: Query detailed information for one manga [5, 6]
			err := s.Database.QueryRow(`
				SELECT id, title, author, genres, status, total_chapters, description 
				FROM manga WHERE id = ?`, id).
				Scan(&m.ID, &m.Title, &m.Author, &genresText, &m.Status, &m.TotalChapters, &m.Description)

			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Manga not found"})
				return
			} else if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
				return
			}

			json.Unmarshal([]byte(genresText), &m.Genres)
			c.JSON(http.StatusOK, gin.H{"data": m})
		})
	}

	users := s.Router.Group("/users")
	{
		// 3. PUT /users/progress - Update reading progress
		users.PUT("/progress", func(c *gin.Context) {
			// Define the payload struct received from the client
			var req struct {
				UserID  string `json:"user_id"` // Temporary body field. In Phase 2, this will come from the JWT token [1, 2]
				MangaID string `json:"manga_id"`
				Chapter int    `json:"chapter"`
				Status  string `json:"status"`
			}

			// Validate the JSON body
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			// UC-006: Insert or update progress in the local database [7, 8]
			// SQLite supports ON CONFLICT (UPSERT)
			updateQuery := `
			INSERT INTO user_progress (user_id, manga_id, current_chapter, status, updated_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(user_id, manga_id) DO UPDATE SET 
				current_chapter = excluded.current_chapter,
				status = excluded.status,
				updated_at = CURRENT_TIMESTAMP;
			`

			_, err := s.Database.Exec(updateQuery, req.UserID, req.MangaID, req.Chapter, req.Status)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update progress"})
				return
			}

			// IMPORTANT: TCP broadcast hook [8]
			// TODO: In Week 4 (Phase 2), you will call a function or channel to forward this data
			// to the TCP server so it can broadcast a JSON progress message to connected clients [9-11].
			// Example: tcpServer.Broadcast <- ProgressUpdate{...}

			c.JSON(http.StatusOK, gin.H{
				"message": "Progress updated successfully",
				"data":    req,
			})
		})
	}
}

func main() {
	// 1. Initialize the database
	projectRoot, err := database.ResolveProjectPath("go.mod")
	if err != nil {
		log.Fatal("Error finding project root: ", err)
	}
	dbDir := filepath.Join(filepath.Dir(projectRoot), "database")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		log.Fatal("Error creating database directory: ", err)
	}
	db, err := database.InitDB(filepath.Join(dbDir, "data.db"))
	if err != nil {
		log.Fatal("Error initializing database: ", err)
	}
	defer db.Close()
	fmt.Println("Database schema initialized successfully!")

	// 2. Load dummy data
	dummyPath, err := database.ResolveProjectPath("data/dummy.json")
	if err != nil {
		log.Fatal("Error finding dummy data file: ", err)
	}
	err = database.LoadDummyData(db, dummyPath)
	if err != nil {
		log.Printf("Dummy data load error (it may already be loaded): %v\n", err)
	}

	// 3. Initialize the HTTP API server [1]
	server := &APIServer{
		Router:    gin.Default(), // Initialize the Gin router with the default middleware (logger, recovery)
		Database:  db,
		JWTSecret: "super-secret-key-for-jwt", // Temporarily hard-coded secret key
	}

	// 4. Register routes on the router
	server.setupRoutes()

	// 5. Start the server on the default port 8080 (or change as needed) [3]
	fmt.Println("Starting HTTP API server at http://localhost:8080 ...")
	if err := server.Router.Run(":8080"); err != nil {
		log.Fatal("Error running server: ", err)
	}
}
