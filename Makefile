.PHONY: build test clean validate run dry-run compare docker-build aw-build aw-save langfuse-up langfuse-down langfuse-smoke

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

CONTAINER_RUNTIME ?= $(shell command -v podman 2>/dev/null || echo docker)
COMPOSE := $(CONTAINER_RUNTIME) compose -f docker-compose.langfuse.yml
DOCKER_IMAGE := trimetry-bench:latest

docker-build:
	$(CONTAINER_RUNTIME) build -t $(DOCKER_IMAGE) docker/

aw-build:
	aw build bench

aw-save:
	aw build bench --save trimetry-bench.tar

# --- Langfuse local ---
langfuse-up:
	$(COMPOSE) up -d
	@echo "Waiting for Langfuse to become healthy..."
	@timeout=120; while [ $$timeout -gt 0 ]; do \
		if $(COMPOSE) ps langfuse-web --format json 2>/dev/null | grep -q '"healthy"'; then \
			echo "Langfuse is ready at http://localhost:3000"; \
			break; \
		fi; \
		sleep 3; \
		timeout=$$((timeout - 3)); \
	done; \
	if [ $$timeout -le 0 ]; then echo "Timeout waiting for Langfuse"; exit 1; fi

langfuse-down:
	$(COMPOSE) down -v

langfuse-smoke: build langfuse-up
	LANGFUSE_BASEURL=http://localhost:3000 \
	LANGFUSE_PUBLIC_KEY=pk-lf-trimetry-local \
	LANGFUSE_SECRET_KEY=sk-lf-trimetry-local \
	./$(BINARY) run --config benchmarks/langfuse-smoke.yaml
	@echo "Langfuse smoke test passed. Traces visible at http://localhost:3000"
