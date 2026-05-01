package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	// Import models from your project (replace "mangahub" with your module name if needed)
	"mangahub/pkg/models"

	_ "github.com/mattn/go-sqlite3"
)

func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE,
		password_hash TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS manga (
		id TEXT PRIMARY KEY,
		title TEXT,
		author TEXT,
		genres TEXT,
		status TEXT,
		total_chapters INTEGER,
		description TEXT
	);

	CREATE TABLE IF NOT EXISTS user_progress (
		user_id TEXT,
		manga_id TEXT,
		current_chapter INTEGER,
		status TEXT,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, manga_id)
	);`

	_, err = db.Exec(schema)
	if err != nil {
		return nil, fmt.Errorf("error creating schema: %w", err)
	}

	return db, nil
}

func LoadDummyData(db *sql.DB, jsonFilePath string) error {
	fileData, err := os.ReadFile(jsonFilePath)
	if err != nil {
		return err
	}

	// Use the Manga struct from the models package
	var mangaList []models.Manga
	if err := json.Unmarshal(fileData, &mangaList); err != nil {
		return err
	}

	for _, m := range mangaList {
		genresJSON, _ := json.Marshal(m.Genres)
		_, err := db.Exec(`
			INSERT OR IGNORE INTO manga (id, title, author, genres, status, total_chapters, description)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, m.ID, m.Title, m.Author, string(genresJSON), m.Status, m.TotalChapters, m.Description)
		if err != nil {
			log.Printf("Error inserting manga data %s: %v", m.ID, err)
		}
	}
	return nil
}

// ResolveProjectPath searches upward from the current directory until it finds the requested path.
func ResolveProjectPath(relativePath string) (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("unable to get current directory: %w", err)
	}

	for {
		candidate := filepath.Join(currentDir, relativePath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			break
		}
		currentDir = parentDir
	}

	return "", fmt.Errorf("could not find %s from the current directory", relativePath)
}
