package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"mangahub/pkg/models"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "./mangahub.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Read JSON file
	data, err := ioutil.ReadFile("data/mangadex.json")
	if err != nil {
		log.Fatal(err)
	}

	var mangas []models.MangaDetails
	if err := json.Unmarshal(data, &mangas); err != nil {
		log.Fatal(err)
	}

	// Insert into DB
	for _, m := range mangas {
		genresJSON, _ := json.Marshal(m.Genres)
		_, err := db.Exec(`INSERT OR REPLACE INTO manga 
            (id, title, author, genres, status, total_chapters, description, cover_url, source) 
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.Title, m.Authors, string(genresJSON), m.Status, m.TotalChapters, m.Description, m.CoverURL, m.Source)
		if err != nil {
			fmt.Println("Failed to insert:", m.Title, err)
		}
	}
	fmt.Println("Manga data loaded successfully!")
}
