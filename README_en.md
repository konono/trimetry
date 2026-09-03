# Trimetry

[日本語版 README](README.md)

A benchmarking framework for **quantitatively comparing** model, prompt, and skill changes in LLM agents.

## Why Trimetry?

Model updates and skill changes in LLMs can cause quality degradation as well as improvements. Subjective evaluation lacks reproducibility and provides insufficient evidence to explain whether a change improved or worsened results.

For example, even if an execution succeeds after a skill change, you cannot determine whether the success was due to the change. If the baseline success rate is 80% and failure rate is 20%:

- The pre-change run may have happened to hit the 20% failure
- The post-change run may have happened to hit the 80% success
- The actual success rate may not have changed at all

Trimetry eliminates randomness by running multiple trials under identical conditions and quantitatively measuring the impact of changes.

## What It Measures

Metrics recorded and compared in the initial stage:

| Category | Metrics |
|---|---|
| **Latency** | Overall task, per LLM generation, per tool call, TTFT (Time To First Token) |
| **Tokens** | Input, output, total, reasoning, cache read/write |
| **Cost** | Estimated cost from pricing config, provider-reported cost |
| **Tool Usage** | Call count, tool types used, execution time per tool |
| **Success/Failure** | Completion rate, timeout rate, error details |
| **Variance** | Mean, median, stddev, p90, p95 under identical conditions |
| **Accuracy** | Contains-match against `expected_output` |

This enables you to explain:

- Which operations are slow
- What changed after a model update
- How tool usage and processing flow changed after a skill update
- How much results vary under identical conditions

### Prerequisites

- [direnv](https://direnv.net/) (for environment variable management)
- Go 1.25+ (required by OpenTelemetry dependency)
- LLM agent CLI for the adapter you plan to use:

| Adapter | Required CLI | Command Executed | Installation |
|---|---|---|---|
| `opencode` | [opencode](https://github.com/opencode-ai/opencode) | `opencode run --format json <input>` | `npm i -g @anthropic-ai/opencode` |
| `claude` | [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `claude -p --output-format stream-json <input>` | `npm i -g @anthropic-ai/claude-code` |
| `codex` | [Codex](https://github.com/openai/codex) | `codex exec --json <input>` | `npm i -g @openai/codex` |
| `cursor` | [Cursor CLI](https://docs.cursor.com/cli) | `cursor -p --output-format stream-json <input>` | Install CLI from the Cursor app |
| `fake` | None | Built-in mock (no external deps) | — |

## Installation

```bash
go install github.com/konono/trimetry/cmd/trimetry@latest
```

With a specific version:

```bash
go install github.com/konono/trimetry/cmd/trimetry@v0.1.0
```

You can also download pre-built binaries from [GitHub Releases](https://github.com/konono/trimetry/releases).

## Quick Start

### 1. Verify Installation (No External Dependencies)

```bash
# Build from source
make build

# Dry run (runs immediately with the fake adapter)
trimetry run --config benchmarks/dry-run.yaml

# Validate a config file only
trimetry validate --config benchmarks/example.yaml
```

### 2. Create Your Own Benchmark

Write a single YAML file to get started. Minimal config:

```yaml
# benchmarks/my-first.yaml
benchmark:
  name: my-first-benchmark
  trials: 3                # Number of trials per scenario x model
  timeout_seconds: 120

scenarios:
  - id: code-gen
    version: "1"
    name: Code Generation
    input: "Write a Python function that reverses a string"
    expected_output: "reverse"   # Contains-match for accuracy evaluation

models:
  - name: claude-sonnet-4
    provider: anthropic
    parameters:
      temperature: 0

adapter:
  type: opencode             # LLM agent to use (see table above)
  options:
    command: opencode
    working_directory: "."

telemetry:
  enabled: false             # Set to false if not using Langfuse/MLflow

report:
  output_directory: benchmark-results
  formats: [json, markdown]
```

```bash
trimetry run --config benchmarks/my-first.yaml
```

After execution, a results report is generated at `benchmark-results/<runId>/summary.md`.

### 3. Compare Results

Run twice with different models or skills, then compare the difference:

```bash
# Before change
trimetry run --config benchmarks/my-first.yaml
# → benchmark-results/run-aaa/summary.json

# After change (re-run with modified model or config)
trimetry run --config benchmarks/my-first.yaml
# → benchmark-results/run-bbb/summary.json

# Compare
trimetry compare \
  --baseline benchmark-results/run-aaa/summary.json \
  --candidate benchmark-results/run-bbb/summary.json
```

### 4. Enable Telemetry (Langfuse)

#### Using Local Langfuse

```bash
# Start Langfuse v4 locally (podman compose)
make langfuse-up

# Install the official Langfuse plugin for opencode
mise run setup-opencode-plugin

# Configure .envrc for local Langfuse
# Comment out the cloud LANGFUSE_* exports and
# uncomment the "local Langfuse" section
cp .envrc.example .envrc
vi .envrc   # Enable export LANGFUSE_BASEURL=http://localhost:3000 etc.
direnv allow
```

#### Using External Langfuse

```bash
cp .envrc.example .envrc
# Edit .envrc: set LANGFUSE_BASEURL, LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY
direnv allow
```

#### Enable Telemetry in Config

```yaml
telemetry:
  enabled: true
  provider: langfuse
  flush_on_trial_end: true
```

Sample configs are available in `examples/` (`opencode-smoke.yaml`, `mlflow-smoke.yaml`).

## CLI

```bash
./trimetry validate            --config benchmarks/example.yaml
./trimetry validate --dry-run  --config examples/opencode-smoke.yaml  # No env vars needed
./trimetry run                 --config benchmarks/example.yaml
./trimetry run                 --config benchmarks/dry-run.yaml  # fake adapter (no flags needed)
./trimetry run --dry-run       --config examples/opencode-smoke.yaml  # fake adapter + telemetry disabled
./trimetry compare             --baseline <path> --candidate <path>
```

> `--dry-run` works with both `validate` and `run`. It substitutes the fake adapter and disables telemetry, so you can run without Langfuse/MLflow environment variables. `benchmarks/dry-run.yaml` is a CI-friendly YAML file with equivalent settings. Configs in `examples/` contain placeholders (`your-model` / `your-provider`) — replace them with actual model names before use.

## Architecture

### Overview

The runner controls benchmark execution, while telemetry backends (Langfuse / MLflow) serve as the observation, storage, and analysis layer.

```
Benchmark Run
  └─ Scenario × Model × Trial
      └─ Trace (Langfuse)
          ├─ opencode.turn (AGENT)          ← Created by official plugin
          │   ├─ opencode.message.user      ← Plugin
          │   ├─ opencode.generation        ← Plugin (tokens, model, cost)
          │   │   └─ tool (TOOL)            ← Plugin
          │   └─ trimetry.annotate          ← Trimetry (trace-level metadata)
          ├─ Tags: [benchmark, scenario:X, model:Y]
          ├─ Metadata: {scenarioId, trialNumber, wallTimeMs, accuracy, ...}
          └─ Scores: accuracy, completion, non_empty
```

### Telemetry Separation of Concerns

In the opencode adapter, trace structure creation and trimetry benchmark metadata injection are separated.

| Responsibility | Owner | Method |
|---|---|---|
| Trace structure (turn → generation → tool) | Official plugin | OTLP span |
| LLM metrics (tokens, model, cost, TTFT) | Official plugin | OTLP span attributes |
| Input/output recording | Official plugin | OTLP span attributes |
| Benchmark scores (accuracy, etc.) | Trimetry | REST `score-create` |
| Benchmark metadata (scenarioId, trialNumber, etc.) | Trimetry | OTLP `langfuse.trace.*` attributes |
| Aggregation & comparison reports | Trimetry | Local files |

Trimetry looks up the Langfuse traceId from the opencode sessionId and sends `langfuse.trace.tags` / `langfuse.trace.metadata` / `langfuse.trace.input` / `langfuse.trace.output` as OTLP span attributes. This displays benchmark information at the trace top level in the Langfuse UI.

### Components

| Component | Responsibility |
|---|---|
| Config Loader | YAML config loading & validation |
| Application Adapter | Subprocess execution and output parsing for CLI tools (opencode, claude, codex, cursor) |
| Trial Executor | Concurrent trial execution, timeout, and retry control |
| Telemetry Adapter | Langfuse: score-create (REST) + trace annotation (OTLP) / MLflow: OTLP |
| Metrics Builder | Build TrialMetrics from adapter output |
| Evaluator | Automatic evaluation of completion / non_empty / accuracy |
| Aggregator | Statistical aggregation (mean, median, stddev, p90, p95) |
| Comparator | Baseline vs. candidate diff comparison |
| Report Generator | JSON + Markdown report output |

### Project Structure

```
benchmark/
├── cmd/trimetry/          # CLI entry point
├── internal/
│   ├── adapter/           # Application Adapter (opencode, claude, codex, cursor, fake)
│   ├── aggregator/        # Statistical aggregation
│   ├── comparator/        # Baseline vs. candidate comparison
│   ├── config/            # YAML config loading & validation
│   ├── evaluator/         # Automatic evaluation (completion, accuracy)
│   ├── id/                # ID generation
│   ├── metrics/           # TrialMetrics construction & field definitions
│   ├── model/             # Common type definitions
│   ├── report/            # JSON + Markdown report output
│   ├── runner/            # Concurrent trial execution & orchestration
│   ├── telemetry/         # Langfuse / MLflow telemetry
│   └── version/           # Version information
├── benchmarks/            # Sample config files
├── examples/              # Example configs for external tools
└── .github/workflows/     # CI
```

### Design Decisions

#### Separation of Logical IDs and Physical Trace IDs

The benchmark runner generates its own logical IDs (`benchmarkRunId`, `trialId`). OTEL Trace IDs are physical observation units and are not used as logical IDs.

LLM agents (e.g., opencode) generate a new OTEL trace for each `ai.streamText()` call, so a single trial can produce multiple traces. The runner does not rewrite Trace IDs — it aggregates results based on logical IDs.

```
Trial (logical ID: tr_xxx)
  ├─ OTEL Trace 1: ai.streamText (LLM call → tool calls)
  ├─ OTEL Trace 2: ai.streamText (LLM call → final response)
  └─ Benchmark Trace: all spans merged under tr_xxx
```

#### Telemetry Backend Selection

| | Langfuse | MLflow |
|---|---|---|
| Protocol | OTLP (trace annotation) + REST (score-create) | OTLP (protobuf) |
| Authentication | Basic Auth | Bearer Token / None |
| Trace creation | Official plugin (OTLP) | Trimetry (OTLP) |
| Trimetry's role | Score + metadata injection | Creates entire trace |

#### Langfuse v4 Support

In Langfuse v4 (`events_only` mode), the REST ingestion API only accepts `score-create`. Trace/generation/span creation is OTLP-only.

In the opencode adapter, the official plugin (`@langfuse/opencode-observability-plugin`) creates the trace structure via OTLP, and trimetry adds the following after the fact:

1. SessionId → traceId lookup (Langfuse observations API)
2. Trace-level metadata injection via `langfuse.trace.*` attributes (OTLP span)
3. Evaluation score registration (REST `score-create`)

#### Handling Unknown Values

Values that could not be retrieved are treated as `null` (Go pointer-type nil), explicitly distinguished from `0`. The source is recorded in `tokenUsageSource` / `costSource` fields.

## Config File

```yaml
benchmark:
  name: my-benchmark
  trials: 5              # Trials per scenario x model
  concurrency: 1          # Concurrent executions
  timeout_seconds: 300    # Default timeout
  retries: 0              # Retry count
  environment: local

scenarios:
  - id: code-generation
    version: "1"
    name: Code Generation Test
    input: "Write a function to sort a list"
    expected_output: "sort"    # Optional: contains-match for accuracy evaluation
    timeout_seconds: 120       # Per-scenario timeout (optional)

models:
  - name: gpt-4o
    provider: openai
    parameters:
      temperature: 0
    pricing:                   # Optional: for cost estimation
      input_per_m_token: 2.50
      output_per_m_token: 10.00

adapter:
  type: opencode               # opencode | claude | codex | cursor | fake
  # See adapter support table below
  options:
    command: opencode
    working_directory: "."

telemetry:
  enabled: true
  provider: langfuse           # langfuse | mlflow
  flush_on_trial_end: true     # Flush telemetry after each trial
  enrichment_dir: /tmp/trimetry-enrichment  # Enrichment file directory (optional)
  tls_skip_verify: false       # Skip TLS verification for MLflow (for self-signed certs)

report:
  output_directory: benchmark-results
  formats: [json, markdown]
  mask_output: false           # Mask output in saved results
```

## Adapter Support

| Adapter | Steps / LLM Latency | Token / Cost | Notes |
|---|---|---|---|
| **opencode** | Full | Full | Primary target |
| **claude** | Full | Provider cost supported | |
| **codex** | Partial | Partial | No Steps, so llmLatencyMs etc. are null |
| **cursor** | Partial | None | Text and tool count only |
| **fake** | Full | Fixed values | For testing & CI |

## Evaluation

Built-in evaluations:

| Name | Description | Trigger |
|---|---|---|
| `completion` | Whether the trial completed successfully | Automatic (all trials) |
| `non_empty` | Whether the output is non-empty | Automatic (all trials) |
| `accuracy` | Contains-match against `expected_output` | Only when `expected_output` is set |

Future planned quality evaluations (in addition to quantitative metrics):

- LLM-as-a-Judge for response quality assessment
- Verification of expected tool calls / absence of forbidden tool calls
- Rule-based evaluation

In the initial phase, rather than fully implementing quality evaluation, the data structures are designed to allow evaluation logic to be added later.

## Output Files

```
benchmark-results/
  <benchmarkRunId>/
    run-manifest.json    # Reproducibility config (no API keys)
    trials.jsonl         # Detailed results per trial
    summary.json         # Aggregated results
    summary.md           # Markdown report
    errors.jsonl         # Error details
```

`run-manifest.json` contains the following reproducibility information:

- `configHash` — SHA256 of the original YAML file
- `effectiveConfigHash` — SHA256 of the runtime config (after defaults/env/--dry-run applied, credentials excluded)
- `adapter` — Adapter type and options used
- `dryRun` — Whether the `--dry-run` flag was specified

If any trial fails, times out, or is cancelled, the `run` command returns exit code 1. Each scenario summary records a `cancelledCount`.

## Environment Variables

| Variable | Purpose |
|---|---|
| `LANGFUSE_BASEURL` | Langfuse server URL |
| `LANGFUSE_PUBLIC_KEY` | Langfuse public key |
| `LANGFUSE_SECRET_KEY` | Langfuse secret key |
| `MLFLOW_TRACKING_URI` | MLflow Tracking server URL |
| `MLFLOW_EXPERIMENT_ID` | MLflow experiment ID (for opencode plugin) |
| `MLFLOW_TRACKING_TOKEN` | MLflow auth token (optional) |
| `MLFLOW_TRACKING_WORKSPACE` | MLflow workspace name (optional) |
| `TRIMETRY_ENRICHMENT_DIR` | Enrichment file directory (default: `/tmp/trimetry-enrichment`) |

## Setup (mise)

```bash
mise install                           # Install go, node, opencode, podman, docker-compose
cp .envrc.example .envrc               # Copy and edit environment variable template
cp opencode.json.example opencode.json # Copy and edit opencode provider config
direnv allow                           # Activate environment variables
mise run build                         # Build
mise run setup-opencode-plugin         # Set up Langfuse official plugin
mise run setup-mlflow                  # Set up MLflow plugin
```

## opencode Plugin Configuration

When using telemetry with the opencode adapter, the plugin must be installed and enabled in `opencode.json`.

### Langfuse (Official Plugin)

```bash
mise run setup-opencode-plugin   # Install the official plugin
```

```json
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["@langfuse/opencode-observability-plugin"],
  "experimental": { "openTelemetry": true }
}
```

### MLflow

```bash
mise run setup-mlflow     # Install the MLflow plugin
```

```json
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["@konono/opencode-plugin-mlflow"]
}
```

> **Note:** The Langfuse and MLflow plugins are mutually exclusive. Do not include both in the `plugin` array simultaneously.

## Running in an aw Container

You can run trimetry inside a disposable [aw](https://github.com/konono/aw) container. Place the required files in the target repo and launch with `aw bench`.

### Required Files in the Target Repo

```
my-project/                        ← Target repo for benchmarking
├── opencode.json                  ← opencode provider config (models, endpoints)
├── .vllm-token                    ← API key (referenced from opencode.json)
└── benchmarks/
    └── file-count.yaml            ← Trimetry benchmark config
```

**opencode.json** — Place at the root of the target repo. Describes provider settings and model definitions:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "vllm": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "https://your-vllm-endpoint/v1",
        "apiKey": "{file:.vllm-token}"
      },
      "models": {
        "your-model": {
          "name": "Your Model",
          "limit": { "context": 32000, "output": 8000 }
        }
      }
    }
  },
  "model": "vllm/your-model",
  "experimental": { "openTelemetry": true }
}
```

**benchmarks/\*.yaml** — Trimetry benchmark config. Set `working_directory` to `.`:

```yaml
benchmark:
  name: my-benchmark
  trials: 5
  timeout_seconds: 120

scenarios:
  - id: my-scenario
    version: "1"
    input: "Your benchmark prompt here"

models:
  - name: your-model
    provider: vllm

adapter:
  type: opencode
  options:
    working_directory: "."

telemetry:
  enabled: true
  provider: langfuse
  flush_on_trial_end: true

report:
  output_directory: benchmark-results
  formats: [json, markdown]
```

### Building and Running the Image

```bash
# 1. Build image in the trimetry repo (no GITHUB_TOKEN needed)
cd ~/gitrepo/trimetry
make aw-build

# 2. Move to the target repo and run the benchmark
cd ~/gitrepo/my-project
aw bench -- trimetry run --config benchmarks/file-count.yaml
```

### What's Inside the Image

The `docker/Dockerfile` bakes in:

| Component | Purpose |
|---|---|
| Go (via mise) | Build & run trimetry |
| trimetry | Benchmark execution |
| opencode | LLM agent CLI |
| @langfuse/opencode-observability-plugin | Official Langfuse plugin (trace structure creation) |
| Global opencode config | Plugin + OTEL enabled in any directory |

> **Note:** The official plugin is on the npm public registry, so `GITHUB_TOKEN` is not required at build time.

### Exporting to Another Environment

```bash
# Export as tar
make aw-save

# Load on another environment
podman load < trimetry-bench.tar
```

## Known Limitations

- Cost estimation depends on pricing config (provider-reported cost is only partially supported)
- LLM-as-a-Judge is planned for future implementation
- tokensPerGen uses generation count from Steps. For adapters without Steps (codex/cursor), it falls back to `len(ToolCalls) + 1`
- Ctrl+C (SIGINT) cancels queued trials waiting on the semaphore (`executionStatus: "cancelled"`, `errorType: "cancelled"`). Running trials continue until the adapter timeout
- Retries (`retries`) retry until success. Timeouts are also retried, but retries are aborted on context cancellation

## License

MIT
