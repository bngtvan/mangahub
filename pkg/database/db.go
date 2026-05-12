package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"mangahub/pkg/models"

	_ "github.com/mattn/go-sqlite3"
)

// InitDB initializes SQLite and creates schema with foreign keys and random user IDs
func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Enable foreign key enforcement
	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		return nil, err
	}

	// Define schema statements separately
	statements := []string{
		`CREATE TABLE IF NOT EXISTS manga (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            source_id TEXT,
            title TEXT,
            author TEXT,
            genres TEXT,
            status TEXT,
            total_chapters INTEGER,
            description TEXT,
            cover_url TEXT,
            source TEXT
        );`,
		`CREATE TABLE IF NOT EXISTS users (
            id TEXT PRIMARY KEY,
            username TEXT UNIQUE,
            email TEXT UNIQUE,
            password TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );`,
		`CREATE TABLE IF NOT EXISTS user_progress (
            user_id TEXT,
            manga_id INTEGER,
            current_chapter INTEGER,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (user_id, manga_id),
            FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
            FOREIGN KEY (manga_id) REFERENCES manga(id) ON DELETE CASCADE
        );`,
		`CREATE TABLE IF NOT EXISTS user_library (
            user_id TEXT,
            manga_id INTEGER,
            PRIMARY KEY (user_id, manga_id),
            FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
            FOREIGN KEY (manga_id) REFERENCES manga(id) ON DELETE CASCADE
        );`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("failed to execute schema statement: %w", err)
		}
	}

	log.Println("✅ Database initialized successfully with all tables.")
	return db, nil
}

// ResolveProjectPath finds a file relative to project root
func ResolveProjectPath(target string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(cwd, target)); err == nil {
			return filepath.Join(cwd, target), nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return "", errors.New("project root not found")
}

// LoadMangaData reads a JSON file and inserts its contents into the manga table.
// If the source is AniList or manual_input, we skip overwriting MangaDex entries.
func LoadMangaData(db *sql.DB, jsonPath string) error {
	data, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", jsonPath, err)
	}

	var mangas []models.MangaDetails
	if err := json.Unmarshal(data, &mangas); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	source := filepath.Base(jsonPath) // e.g. "mangadex.json"

	for _, m := range mangas {
		// Check if manga already exists by source_id
		var existingSource string
		err := db.QueryRow("SELECT source FROM manga WHERE source_id = ?", m.ID).Scan(&existingSource)

		if err == nil {
			if existingSource == "mangadex" && source != "mangadex.json" {
				log.Printf("Skipping overwrite for %s (MangaDex preferred)\n", m.Title)
				continue
			}
		}

		authorsJSON, _ := json.Marshal(m.Authors)
		genresJSON, _ := json.Marshal(m.Genres)

		_, err = db.Exec(`
            INSERT OR REPLACE INTO manga 
            (source_id, title, author, genres, status, total_chapters, description, cover_url, source)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.Title, string(authorsJSON), string(genresJSON),
			m.Status, m.TotalChapters, m.Description, m.CoverURL, detectSource(source))
		if err != nil {
			log.Printf("Failed to insert manga %s: %v\n", m.Title, err)
		}
	}

	log.Printf("Loaded %d manga entries from %s\n", len(mangas), filepath.Base(jsonPath))
	return nil
}

// LoadAllSources loads mangadex, anilist, and manual_input JSON files
func LoadAllSources(db *sql.DB, basePath string) error {
	sources := []string{"mangadex.json", "anilist.json", "manual_input.json"}

	for _, file := range sources {
		path := filepath.Join(basePath, file)
		if _, err := os.Stat(path); err != nil {
			log.Printf("Skipping missing file: %s\n", file)
			continue
		}
		if err := LoadMangaData(db, path); err != nil {
			log.Printf("Error loading %s: %v\n", file, err)
		}
	}
	return nil
}

// detectSource maps filename to source string
func detectSource(filename string) string {
	switch filename {
	case "mangadex.json":
		return "mangadex"
	case "anilist.json":
		return "anilist"
	case "manual_input.json":
		return "manual"
	default:
		return "unknown"
	}
}
