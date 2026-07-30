package id

import "github.com/google/uuid"

func NewBenchmarkRunID() string {
	return "br_" + uuid.Must(uuid.NewV7()).String()
}

func NewTrialID() string {
	return "tr_" + uuid.Must(uuid.NewV7()).String()
}

func NewEventID() string {
	return uuid.Must(uuid.NewV7()).String()
}
