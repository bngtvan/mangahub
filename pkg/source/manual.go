package source

import (
	"encoding/json"
	"log"
	"mangahub/pkg/models"
	"os"
)

// Manual loads manga data from manual_input.json
func Manual() ([]models.MangaDetails, error) {
	var mangas []models.MangaDetails

	file, err := os.Open("./data/manual_input.json")
	if err != nil {
		log.Printf("Manual input file error: %v", err)
		return nil, err
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&mangas); err != nil {
		log.Printf("Manual input decode error: %v", err)
		return nil, err
	}

	// Ensure Source is set
	for i := range mangas {
		mangas[i].Source = "manual"
	}

	return mangas, nil
}
