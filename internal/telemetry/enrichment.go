package telemetry

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

func collectFileEnrichment(enrichmentDir string, trialID string) enrichmentResult {
	filePath := fmt.Sprintf("%s/%s.jsonl", enrichmentDir, trialID)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return enrichmentResult{}
	}

	var out enrichmentResult
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry struct {
			TrialID      string         `json:"trialId"`
			Timestamp    string         `json:"timestamp"`
			Reasoning    string         `json:"reasoning"`
			FinishReason string         `json:"finishReason"`
			AISettings   map[string]any `json:"aiSettings"`
			HostName     string         `json:"hostName"`
			HostArch     string         `json:"hostArch"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		out.Reasonings = append(out.Reasonings, reasoningInfo{
			Reasoning:    entry.Reasoning,
			FinishReason: entry.FinishReason,
			Timestamp:    entry.Timestamp,
			AISettings:   entry.AISettings,
		})
		if out.Environment == nil && (entry.HostName != "" || entry.HostArch != "") {
			out.Environment = &EnvironmentInfo{
				HostName:   entry.HostName,
				HostArch:   entry.HostArch,
				AISettings: entry.AISettings,
			}
		}
	}

	if len(out.Reasonings) > 0 {
		log.Printf("  Enrichment: %d entries from file for trial %s", len(out.Reasonings), trialID)
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		log.Printf("  Enrichment: failed to remove %s: %v", filePath, err)
	}
	return out
}
