package models

import "time"

// User represents the top-level user object
type User struct {
	ID           int       `json:"id"` // matches INTEGER PRIMARY KEY AUTOINCREMENT
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // never exposed in JSON
	CreatedAt    time.Time `json:"created_at"`
}

// ReadingLists contains the different categories of manga
type ReadingLists struct {
	Reading    []MangaEntry `json:"reading"`
	Completed  []MangaEntry `json:"completed"`
	PlanToRead []MangaEntry `json:"plan_to_read"`
}

// MangaEntry represents an individual manga within a list
type MangaEntry struct {
	MangaID        string    `json:"manga_id"`
	CurrentChapter int       `json:"current_chapter"`
	Status         string    `json:"status"`
	LastUpdated    time.Time `json:"last_updated"`
}
