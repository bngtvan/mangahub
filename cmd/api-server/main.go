package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"

	"mangahub/pkg/database"
	"mangahub/pkg/models"
)

type APIServer struct {
	Router    *gin.Engine
	Database  *sql.DB
	JWTSecret string
}

type authClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// setupRoutes định nghĩa tất cả các endpoint cần thiết theo đặc tả dự án [2]
func (s *APIServer) setupRoutes() {
	// Authentication API group
	auth := s.Router.Group("/auth")
	{
		auth.POST("/register", func(c *gin.Context) {
			var req struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			req.Username = strings.TrimSpace(req.Username)
			if req.Username == "" || len(req.Password) < 6 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Username is required and password must be at least 6 characters"})
				return
			}

			passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
				return
			}

			userID := fmt.Sprintf("user-%d", time.Now().UnixNano())
			_, err = s.Database.Exec(`
				INSERT INTO users (id, username, password_hash) VALUES (?, ?, ?)
			`, userID, req.Username, string(passwordHash))
			if err != nil {
				c.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
				return
			}

			c.JSON(http.StatusCreated, gin.H{
				"message": "User registered successfully",
				"data": gin.H{
					"user_id":  userID,
					"username": req.Username,
				},
			})
		})
		auth.POST("/login", func(c *gin.Context) {
			var req struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			var userID, passwordHash string
			err := s.Database.QueryRow(`
				SELECT id, password_hash FROM users WHERE username = ?
			`, strings.TrimSpace(req.Username)).Scan(&userID, &passwordHash)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
				return
			}

			if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
				return
			}

			token, err := s.generateJWT(userID, req.Username)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"message": "Login successful",
				"data": gin.H{
					"token": token,
				},
			})
		})
	}

	manga := s.Router.Group("/manga")
	{
		// 1. GET /manga?query=one - Search or list manga
		manga.GET("", func(c *gin.Context) {
			queryParam := c.Query("query")
			statusParam := c.Query("status")
			genreParam := c.Query("genre")

			baseQuery := `
				SELECT id, title, author, genres, status, total_chapters, description
				FROM manga WHERE 1=1
			`
			var args []interface{}

			if queryParam != "" {
				baseQuery += " AND title LIKE ?"
				args = append(args, "%"+queryParam+"%")
			}
			if statusParam != "" {
				baseQuery += " AND status = ?"
				args = append(args, statusParam)
			}
			if genreParam != "" {
				// genres is stored as JSON text, use LIKE for basic filtering.
				baseQuery += " AND genres LIKE ?"
				args = append(args, "%"+genreParam+"%")
			}

			rows, err := s.Database.Query(baseQuery, args...)

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
	users.Use(s.authMiddleware())
	{
		// 3. POST /users/library - Add manga to library
		users.POST("/library", func(c *gin.Context) {
			var req struct {
				MangaID        string `json:"manga_id"`
				Status         string `json:"status"`
				CurrentChapter int    `json:"current_chapter"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			userID, ok := c.Get("user_id")
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				return
			}

			req.MangaID = strings.TrimSpace(req.MangaID)
			if req.MangaID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id is required"})
				return
			}

			if req.Status == "" {
				req.Status = "plan_to_read"
			}

			_, err := s.Database.Exec(`
				INSERT INTO users_library (user_id, manga_id, status, current_chapter, updated_at)
				VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
				ON CONFLICT(user_id, manga_id) DO UPDATE SET
					status = excluded.status,
					current_chapter = excluded.current_chapter,
					updated_at = CURRENT_TIMESTAMP
			`, userID.(string), req.MangaID, req.Status, req.CurrentChapter)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add manga to library"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "Manga added to library"})
		})

		// 4. GET /users/library - Get user's library
		users.GET("/library", func(c *gin.Context) {
			userID, ok := c.Get("user_id")
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				return
			}

			rows, err := s.Database.Query(`
				SELECT ul.manga_id, ul.status, ul.current_chapter, ul.updated_at, m.title
				FROM users_library ul
				LEFT JOIN manga m ON m.id = ul.manga_id
				WHERE ul.user_id = ?
				ORDER BY ul.updated_at DESC
			`, userID.(string))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
				return
			}
			defer rows.Close()

			var library []gin.H
			for rows.Next() {
				var mangaID, status, updatedAt string
				var title sql.NullString
				var currentChapter int

				if err := rows.Scan(&mangaID, &status, &currentChapter, &updatedAt, &title); err != nil {
					continue
				}
				library = append(library, gin.H{
					"manga_id":        mangaID,
					"title":           title.String,
					"status":          status,
					"current_chapter": currentChapter,
					"last_updated":    updatedAt,
				})
			}

			c.JSON(http.StatusOK, gin.H{"data": library})
		})

		// 3. PUT /users/progress - Update reading progress
		users.PUT("/progress", func(c *gin.Context) {
			// Define the payload struct received from the client
			var req struct {
				MangaID string `json:"manga_id"`
				Chapter int    `json:"chapter"`
				Status  string `json:"status"`
			}

			// Validate the JSON body
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			userID, ok := c.Get("user_id")
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
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

			_, err := s.Database.Exec(updateQuery, userID.(string), req.MangaID, req.Chapter, req.Status)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update progress"})
				return
			}

			// Keep users_library status synchronized with progress updates.
			_, _ = s.Database.Exec(`
				INSERT INTO users_library (user_id, manga_id, status, current_chapter, updated_at)
				VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
				ON CONFLICT(user_id, manga_id) DO UPDATE SET
					status = excluded.status,
					current_chapter = excluded.current_chapter,
					updated_at = CURRENT_TIMESTAMP
			`, userID.(string), req.MangaID, req.Status, req.Chapter)

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

func (s *APIServer) generateJWT(userID, username string) (string, error) {
	claims := authClaims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.JWTSecret))
}

func (s *APIServer) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing bearer token"})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.ParseWithClaims(tokenString, &authClaims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(s.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		claims, ok := token.Claims.(*authClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
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

	// 2. Load data
	dataFiles := []string{
		"data/manual_input.json",
		"data/mangadex.json",
		"data/anilist.json",
	}

	resolvedFiles := make([]string, 0, len(dataFiles))
	for _, dataFile := range dataFiles {
		resolvedPath, resolveErr := database.ResolveProjectPath(dataFile)
		if resolveErr != nil {
			log.Fatal("Error finding seed data file: ", resolveErr)
		}
		resolvedFiles = append(resolvedFiles, resolvedPath)
	}

	err = database.LoadDataFiles(db, resolvedFiles...)
	if err != nil {
		log.Printf("Data load error (it may already be loaded): %v\n", err)
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
