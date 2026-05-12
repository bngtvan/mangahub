package auth

import (
	"database/sql"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Secret key for signing JWTs (move to config/env in production)
var jwtSecret = []byte("super-secret-key-for-jwt")

// LoginHandler - authenticates user and returns JWT
func LoginHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		// Check credentials in DB (simplified example)
		var storedPassword string
		var userID string
		err := db.QueryRow("SELECT id, password FROM users WHERE username = ?", req.Username).Scan(&userID, &storedPassword)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		// Compare hashed password
		if bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(req.Password)) != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
			return
		}

		// Create JWT claims
		claims := jwt.MapClaims{
			"user_id":  userID,
			"username": req.Username,
			"exp":      time.Now().Add(time.Hour * 24).Unix(), // expires in 24h
		}

		// Generate token
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, err := token.SignedString(jwtSecret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}

		// Return token
		c.JSON(http.StatusOK, gin.H{"token": tokenString})
	}
}

// RegisterHandler - creates a new user with validation
func RegisterHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		// Generate random UUID for user.id
		userID := uuid.New().String()

		// Validate email format
		emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
		if !emailRegex.MatchString(req.Email) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email format"})
			return
		}

		// Validate password strength
		if len(req.Password) < 8 ||
			!regexp.MustCompile(`[A-Z]`).MatchString(req.Password) ||
			!regexp.MustCompile(`[0-9]`).MatchString(req.Password) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 8 characters, include a number and an uppercase letter"})
			return
		}

		// Check for duplicate username
		var usernameExists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", req.Username).Scan(&usernameExists)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		if usernameExists {
			c.JSON(http.StatusConflict, gin.H{"error": "Username already taken"})
			return
		}

		// Check for duplicate email
		var emailExists bool
		err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)", req.Email).Scan(&emailExists)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		if emailExists {
			c.JSON(http.StatusConflict, gin.H{"error": "Email already registered"})
			return
		}

		// Hash password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}

		// Insert new user
		_, err = db.Exec(`
            INSERT INTO users (id, username, email, password, created_at)
            VALUES (?, ?, ?, ?, ?)`,
			userID, req.Username, req.Email, hashedPassword, time.Now())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "User registered successfully",
			"user_id": userID,
		})
	}
}
