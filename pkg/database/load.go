package database

import (
	"database/sql"
	"log"
	"mangahub/pkg/source"
	"strings"
)

func LoadAllMangaData(db *sql.DB) {
	log.Println("🔄 Loading manga data from sources...")

	anilistData, _ := source.AniList()
	mangadexData, _ := source.MangaDex()
	manualData, _ := source.Manual()

	allData := append(anilistData, mangadexData...)
	allData = append(allData, manualData...)

	// Ensure table exists with autoincrement ID
	_, err := db.Exec(`
		DROP TABLE IF EXISTS manga;
        CREATE TABLE IF NOT EXISTS manga (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            title TEXT,
            author TEXT,
            genres TEXT,
            status TEXT,
            total_chapters INTEGER,
            description TEXT,
            cover_url TEXT,
            source TEXT,
            source_id TEXT
        );
        DELETE FROM manga;
    `)
	if err != nil {
		log.Fatalf("Failed to prepare manga table: %v", err)
	}

	// Insert data without touching the id column
	for _, m := range allData {
		_, err := db.Exec(`
            INSERT INTO manga 
            (title, author, genres, status, total_chapters, description, cover_url, source, source_id)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.Title,
			strings.Join(m.Authors, ", "),
			strings.Join(m.Genres, ", "),
			m.Status,
			m.TotalChapters,
			m.Description,
			m.CoverURL,
			m.Source,
			m.ID, // keep original external ID
		)
		if err != nil {
			log.Printf("Failed to insert manga %s: %v", m.Title, err)
		}
	}

	log.Println("✅ Manga data loaded with sequential IDs.")
}
