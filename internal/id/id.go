package id

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/google/uuid"
)

func NewBenchmarkRunID() string {
	return "br_" + uuid.Must(uuid.NewV7()).String()
}

func NewTrialID() string {
	return "tr_" + uuid.Must(uuid.NewV7()).String()
}

func NewEventID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func NewHexID(byteLen int) string {
	b := make([]byte, byteLen)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
