package id

import (
	"strings"
	"testing"
)

func TestNewBenchmarkRunID(t *testing.T) {
	id := NewBenchmarkRunID()
	if !strings.HasPrefix(id, "br_") {
		t.Errorf("expected prefix br_, got %s", id)
	}
	if len(id) < 10 {
		t.Errorf("id too short: %s", id)
	}
}

func TestNewTrialID(t *testing.T) {
	id := NewTrialID()
	if !strings.HasPrefix(id, "tr_") {
		t.Errorf("expected prefix tr_, got %s", id)
	}
}

func TestIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewTrialID()
		if seen[id] {
			t.Fatalf("duplicate id at iteration %d: %s", i, id)
		}
		seen[id] = true
	}
}
