package main

import (
	"database/sql"
	"log"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"

	"mangahub/internal/auth"
	"mangahub/internal/manga"
	"mangahub/internal/user"
	"mangahub/pkg/database"
)

type APIServer struct {
	Router    *gin.Engine
	Database  *sql.DB
	JWTSecret string
}

func main() {
	// Initialize database with schema
	db, err := database.InitDB("./data/data.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Load manga data
	database.LoadAllSources(db, "./data") // 👈 loads mangadex/anilist/manual_input

	// Initialize router
	r := gin.Default()

	// Create server instance
	server := &APIServer{
		Router:    r,
		Database:  db,
		JWTSecret: "super-secret-key-for-jwt",
	}

	// Register routes
	server.registerRoutes()

	// Start server
	log.Println("Server running on http://localhost:8080")
	r.Run(":8080")
}

func (s *APIServer) registerRoutes() {
	// Auth routes
	authGroup := s.Router.Group("/auth")
	{
		authGroup.POST("/register", auth.RegisterHandler(s.Database))
		authGroup.POST("/login", auth.LoginHandler(s.Database))
	}

	// User routes (protected)
	users := s.Router.Group("/users")
	users.Use(auth.AuthMiddleware())
	{
		users.POST("/library", user.AddToLibraryHandler(s.Database))
		users.GET("/library", user.GetLibraryHandler(s.Database))
		users.PUT("/progress", user.UpdateProgressHandler(s.Database))
		users.DELETE("/library", user.RemoveFromLibraryHandler(s.Database))
	}

	// Manga routes (public)
	mangaGroup := s.Router.Group("/manga")
	{
		mangaGroup.GET("", manga.GetMangaListHandler(s.Database))
		mangaGroup.GET("/:id", manga.GetMangaByIDHandler(s.Database))
	}
}
