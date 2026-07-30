package adapter

import (
	"bufio"
	"encoding/json"
	"log"
	"strings"
)

func scanJSONLines[T any](raw string, handle func(T)) {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var event T
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		handle(event)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("WARNING: JSONL scanner error: %v", err)
	}
}
