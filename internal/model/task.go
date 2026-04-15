package model

import "time"

type Task struct {
	ID               int64
	Title            string
	Priority         int
	Source           string
	StartAt          *time.Time
	TargetAt         *time.Time
	UpdatedAt        time.Time
	MetaJSON         string
	Completed        bool
	NotesJSON        string
	GoogleTaskListID string
	GoogleTaskID     string
	LastSyncedAt     *time.Time
}

type Note struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`
}
