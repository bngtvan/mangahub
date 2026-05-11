package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"

	"mangahub/internal/tcp"
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

// validateEmail checks if email format is valid
func validateEmail(email string) bool {
	email = strings.TrimSpace(email)
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	re := regexp.MustCompile(pattern)
	return re.MatchString(email)
}

// validatePassword checks password strength:
// - At least 8 characters
// - At least one uppercase letter
// - At least one lowercase letter
// - At least one digit
// - At least one special character (!@#$%^&*)
func validatePassword(password string) (bool, string) {
	if len(password) < 8 {
		return false, "Password must be at least 8 characters long"
	}

	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	if !hasUpper {
		return false, "Password must contain at least one uppercase letter"
	}

	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	if !hasLower {
		return false, "Password must contain at least one lowercase letter"
	}

	hasDigit := regexp.MustCompile(`\d`).MatchString(password)
	if !hasDigit {
		return false, "Password must contain at least one digit"
	}

	hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};:'",.<>?/\\|` + "`" + `]`).MatchString(password)
	if !hasSpecial {
		return false, "Password must contain at least one special character (!@#$%^&*)"
	}

	return true, ""
}

// setupRoutes registers all HTTP endpoints exposed by the API server.
func (s *APIServer) setupRoutes() {
	// GET /health
	// Public health check endpoint.
	s.Router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// Authentication endpoints.
	auth := s.Router.Group("/auth")
	{
		// POST /auth/register
		// Public endpoint to register a new account.
		// Body: {"username":"string", "email":"string", "password":"string"}
		auth.POST("/register", func(c *gin.Context) {
			var req struct {
				Username string `json:"username"`
				Email    string `json:"email"`
				Password string `json:"password"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			// Validate and trim inputs
			req.Username = strings.TrimSpace(req.Username)
			req.Email = strings.TrimSpace(req.Email)

			// Validate username
			if req.Username == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Username is required"})
				return
			}

			if len(req.Username) < 3 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Username must be at least 3 characters long"})
				return
			}

			// Validate email format
			if req.Email == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
				return
			}

			if !validateEmail(req.Email) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email format"})
				return
			}

			// Validate password strength
			isValid, errMsg := validatePassword(req.Password)
			if !isValid {
				c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
				return
			}

			// Hash password
			passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
				return
			}

			// Create user
			userID := fmt.Sprintf("user-%d", time.Now().UnixNano())
			_, err = s.Database.Exec(`
				INSERT INTO users (id, username, email, password_hash) VALUES (?, ?, ?, ?)
			`, userID, req.Username, req.Email, string(passwordHash))
			if err != nil {
				// Check which field caused the conflict
				if strings.Contains(err.Error(), "UNIQUE constraint failed: users.username") {
					c.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
				} else if strings.Contains(err.Error(), "UNIQUE constraint failed: users.email") {
					c.JSON(http.StatusConflict, gin.H{"error": "Email already exists"})
				} else {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
				}
				return
			}

			c.JSON(http.StatusCreated, gin.H{
				"message": "User registered successfully",
				"data": gin.H{
					"user_id":  userID,
					"username": req.Username,
					"email":    req.Email,
				},
			})
		})

		// POST /auth/login
		// Public endpoint to authenticate and issue JWT.
		// Body: {"username":"string", "password":"string"}
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

	// Manga endpoints.
	manga := s.Router.Group("/manga")
	{
		// GET /manga
		// Public endpoint to search manga by title/author with optional status/genre filters.
		// Supports pagination via page and limit query parameters.
		manga.GET("", func(c *gin.Context) {
			queryParam := c.Query("query")
			statusParam := c.Query("status")
			genreParam := c.Query("genre")
			pageParam := c.DefaultQuery("page", "1")
			limitParam := c.DefaultQuery("limit", "10")

			page, err := strconv.Atoi(pageParam)
			if err != nil || page < 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "page must be a positive integer"})
				return
			}

			limit, err := strconv.Atoi(limitParam)
			if err != nil || limit < 1 || limit > 100 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 100"})
				return
			}

			offset := (page - 1) * limit

			baseQuery := `
				SELECT id, title, author, genres, status, total_chapters, description
				FROM manga WHERE 1=1
			`
			var args []interface{}

			if queryParam != "" {
				baseQuery += " AND (title LIKE ? OR author LIKE ?)"
				args = append(args, "%"+queryParam+"%", "%"+queryParam+"%")
			}
			if statusParam != "" {
				baseQuery += " AND status = ?"
				args = append(args, statusParam)
			}
			if genreParam != "" {
				// genres is stored as JSON text, so LIKE is used for simple filtering.
				baseQuery += " AND genres LIKE ?"
				args = append(args, "%"+genreParam+"%")
			}

			countQuery := "SELECT COUNT(*) FROM manga WHERE 1=1"
			countArgs := make([]interface{}, 0, len(args))
			countArgs = append(countArgs, args...)
			if queryParam != "" {
				countQuery += " AND (title LIKE ? OR author LIKE ?)"
			}
			if statusParam != "" {
				countQuery += " AND status = ?"
			}
			if genreParam != "" {
				countQuery += " AND genres LIKE ?"
			}

			var total int
			if err := s.Database.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
				return
			}

			baseQuery += " ORDER BY title ASC LIMIT ? OFFSET ?"
			args = append(args, limit, offset)

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
				// Parse stored JSON string into a []string response field.
				json.Unmarshal([]byte(genresText), &m.Genres)
				results = append(results, m)
			}

			c.JSON(http.StatusOK, gin.H{
				"data": results,
				"pagination": gin.H{
					"page":       page,
					"limit":      limit,
					"total":      total,
					"total_page": (total + limit - 1) / limit,
				},
			})
		})

		// GET /manga/:id
		// Public endpoint for manga detail.
		// If a valid Bearer token is provided, includes current_progress for that user.
		manga.GET("/:id", func(c *gin.Context) {
			id := c.Param("id")
			var m models.Manga
			var genresText string
			var detail gin.H

			// Load the base manga record.
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
			detail = gin.H{"manga": m}

			if userID, ok := s.optionalUserFromRequest(c); ok {
				var currentChapter sql.NullInt64
				var progressStatus sql.NullString
				var updatedAt sql.NullString
				err = s.Database.QueryRow(`
					SELECT current_chapter, status, updated_at
					FROM user_progress
					WHERE user_id = ? AND manga_id = ?
				`, userID, id).Scan(&currentChapter, &progressStatus, &updatedAt)
				if err == nil {
					detail["current_progress"] = gin.H{
						"current_chapter": currentChapter.Int64,
						"status":          progressStatus.String,
						"updated_at":      updatedAt.String,
					}
				} else if !errors.Is(err, sql.ErrNoRows) {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
					return
				}
			}

			c.JSON(http.StatusOK, gin.H{"data": detail})
		})
	}

	// Authenticated user endpoints.
	users := s.Router.Group("/users")
	users.Use(s.authMiddleware())
	{
		// POST /users/library
		// Auth required. Adds a manga to the user's library or updates it if already present.
		// Body: {"manga_id":"string", "status":"string", "current_chapter":number}
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

		// GET /users/library
		// Auth required. Returns the authenticated user's library entries.
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

		// PUT /users/progress
		// Auth required. Updates a user's reading progress and sync status.
		// Body: {"manga_id":"string", "chapter":number, "status":"string"}
		users.PUT("/progress", func(c *gin.Context) {
			// Request payload.
			var req struct {
				MangaID string `json:"manga_id"`
				Chapter int    `json:"chapter"`
				Status  string `json:"status"`
			}

			// Validate JSON body.
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			userID, ok := c.Get("user_id")
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				return
			}

			// Validate chapter against manga total_chapters.
			var totalChapters int
			err := s.Database.QueryRow(`SELECT total_chapters FROM manga WHERE id = ?`, req.MangaID).Scan(&totalChapters)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Manga not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
				return
			}

			if req.Chapter < 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter must be at least 1"})
				return
			}
			if totalChapters > 0 && req.Chapter > totalChapters {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter exceeds total chapters"})
				return
			}

			// Upsert progress in local SQLite.
			updateQuery := `
			INSERT INTO user_progress (user_id, manga_id, current_chapter, status, updated_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(user_id, manga_id) DO UPDATE SET 
				current_chapter = excluded.current_chapter,
				status = excluded.status,
				updated_at = CURRENT_TIMESTAMP;
			`

			_, err = s.Database.Exec(updateQuery, userID.(string), req.MangaID, req.Chapter, req.Status)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update progress"})
				return
			}

			// Keep library status/chapter synchronized with progress.
			_, _ = s.Database.Exec(`
				INSERT INTO users_library (user_id, manga_id, status, current_chapter, updated_at)
				VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
				ON CONFLICT(user_id, manga_id) DO UPDATE SET
					status = excluded.status,
					current_chapter = excluded.current_chapter,
					updated_at = CURRENT_TIMESTAMP
			`, userID.(string), req.MangaID, req.Status, req.Chapter)

			// Build TCP progress payload.
			update := tcp.ProgressUpdate{
				UserID:    userID.(string),
				MangaID:   req.MangaID,
				Chapter:   req.Chapter,
				Timestamp: time.Now().Unix(),
			}

			outboundStatus := "sent"
			if err := sendProgressToTCP(update); err != nil {
				// Queue payload for background retry when TCP is unavailable.
				payload, _ := json.Marshal(update)
				_ = queueOutbox(s.Database, payload)
				outboundStatus = "queued"
			}

			c.JSON(http.StatusOK, gin.H{
				"message":  "Progress updated successfully",
				"outbound": outboundStatus,
				"data":     req,
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

// optionalUserFromRequest returns the authenticated user ID when a valid Bearer token is present.
func (s *APIServer) optionalUserFromRequest(c *gin.Context) (string, bool) {
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", false
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	token, err := jwt.ParseWithClaims(tokenString, &authClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return "", false
	}

	claims, ok := token.Claims.(*authClaims)
	if !ok || claims.UserID == "" {
		return "", false
	}

	return claims.UserID, true
}

// sendProgressToTCP attempts to send the progress update to the TCP sync server.
func sendProgressToTCP(update tcp.ProgressUpdate) error {
	conn, err := net.DialTimeout("tcp", "localhost:9090", 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	payload, err := json.Marshal(update)
	if err != nil {
		return err
	}

	// write payload followed by newline (server expects lines)
	payload = append(payload, '\n')
	_, err = conn.Write(payload)
	return err
}

// queueOutbox saves the JSON payload in tcp_outbox for later delivery.
func queueOutbox(db *sql.DB, payload []byte) error {
	_, err := db.Exec(`INSERT INTO tcp_outbox (payload) VALUES (?)`, string(payload))
	return err
}

// startOutboxFlusher runs a background loop to flush queued TCP messages.
func (s *APIServer) startOutboxFlusher(stopCh <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			// Fetch up to 10 queued items
			rows, err := s.Database.Query(`SELECT id, payload FROM tcp_outbox ORDER BY id LIMIT 10`)
			if err != nil {
				continue
			}
			var idsToDelete []int64
			for rows.Next() {
				var id int64
				var payloadText string
				if err := rows.Scan(&id, &payloadText); err != nil {
					continue
				}
				var update tcp.ProgressUpdate
				if err := json.Unmarshal([]byte(payloadText), &update); err != nil {
					// malformed payload -> delete
					idsToDelete = append(idsToDelete, id)
					continue
				}
				if err := sendProgressToTCP(update); err == nil {
					idsToDelete = append(idsToDelete, id)
				}
			}
			rows.Close()

			// delete sent items
			for _, id := range idsToDelete {
				_, _ = s.Database.Exec(`DELETE FROM tcp_outbox WHERE id = ?`, id)
			}
		}
	}
}

func main() {
	// Initialize database.
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

	// Seed manga data files.
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

	// Create API server.
	server := &APIServer{
		Router:    gin.Default(), // Includes logger and recovery middleware.
		Database:  db,
		JWTSecret: "super-secret-key-for-jwt", // TODO: move to environment variable.
	}

	// Register routes.
	server.setupRoutes()
	server.setupChatRoutes()

	// Start outbox flusher for queued TCP messages.
	stopCh := make(chan struct{})
	go server.startOutboxFlusher(stopCh)

	// Start HTTP server.
	fmt.Println("Starting HTTP API server at http://localhost:8080 ...")
	if err := server.Router.Run(":8080"); err != nil {
		log.Fatal("Error running server: ", err)
	}
}
