package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

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
	);

	CREATE TABLE IF NOT EXISTS users_library (
		user_id TEXT,
		manga_id TEXT,
		status TEXT,
		current_chapter INTEGER DEFAULT 0,
		added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, manga_id)
	);`

	_, err = db.Exec(schema)
	if err != nil {
		return nil, fmt.Errorf("error creating schema: %w", err)
	}

	return db, nil
}

func LoadData(db *sql.DB, jsonFilePath string) error {
	fileData, err := os.ReadFile(jsonFilePath)
	if err != nil {
		return err
	}

	var mangaList []importManga
	if err := json.Unmarshal(fileData, &mangaList); err != nil {
		return err
	}

	for _, m := range mangaList {
		genresJSON, _ := json.Marshal(m.Genres.Values())
		authorText := strings.Join(m.Author.Values(), ", ")
		if authorText == "" {
			authorText = "Unknown"
		}

		_, err := db.Exec(`
			INSERT OR IGNORE INTO manga (id, title, author, genres, status, total_chapters, description)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, m.ID, m.Title, authorText, string(genresJSON), m.Status, m.TotalChapters, m.Description)
		if err != nil {
			log.Printf("Error inserting manga data %s: %v", m.ID, err)
		}
	}
	return nil
}

// LoadFiles imports multiple manga JSON files in order.
func LoadDataFiles(db *sql.DB, jsonFilePaths ...string) error {
	for _, jsonFilePath := range jsonFilePaths {
		if err := LoadData(db, jsonFilePath); err != nil {
			return fmt.Errorf("failed loading %s: %w", jsonFilePath, err)
		}
	}
	return nil
}

type importManga struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Author        stringOrSlice `json:"author"`
	Genres        stringSlice   `json:"genres"`
	Status        string        `json:"status"`
	TotalChapters int           `json:"total_chapters"`
	Description   string        `json:"description"`
}

type stringOrSlice []string

func (s *stringOrSlice) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = nil
		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		if single == "" {
			*s = nil
			return nil
		}
		*s = []string{single}
		return nil
	}

	var multi []string
	if err := json.Unmarshal(data, &multi); err == nil {
		*s = multi
		return nil
	}

	return fmt.Errorf("invalid author format: %s", string(data))
}

func (s stringOrSlice) Values() []string {
	return []string(s)
}

type stringSlice []string

func (s *stringSlice) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = nil
		return nil
	}

	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return fmt.Errorf("invalid string array format: %s", string(data))
	}

	*s = values
	return nil
}

func (s stringSlice) Values() []string {
	return []string(s)
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
