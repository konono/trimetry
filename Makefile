.PHONY: build test clean validate run dry-run compare

BINARY := trimetry
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/konono/trimetry/internal/version.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/trimetry/

test:
	go test ./... -count=1

clean:
	rm -f $(BINARY)
	rm -rf benchmark-results/

validate:
	./$(BINARY) validate --config benchmarks/example.yaml

dry-run: build
	./$(BINARY) run --dry-run --config benchmarks/dry-run.yaml

run: build
	./$(BINARY) run --config benchmarks/example.yaml

compare:
	@echo "Usage: make compare BASELINE=<path> CANDIDATE=<path>"
	./$(BINARY) compare --baseline $(BASELINE) --candidate $(CANDIDATE)
