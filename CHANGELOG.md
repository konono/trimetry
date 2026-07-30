# Changelog

## [0.1.0] - 2026-07-30

### Added
- Benchmark runner with parallel trial execution, retry, and timeout support
- Application adapters: opencode, claude, codex, cursor, fake
- Telemetry integration: Langfuse (REST API) and MLflow (OTLP)
- Enrichment: reasoning/finishReason/aiSettings capture via file-based OTEL span processing
- Evaluators: completion, non_empty, accuracy (contains match)
- Metrics builder with token usage, latency, cost estimation, TTFT
- Statistical aggregation (mean, median, stddev, p90, p95)
- Baseline vs candidate comparison with version mismatch warnings
- JSON + Markdown report generation
- CLI: validate, run, compare, diagnose-tracing, version
- CI with race detector and dry-run validation
- Configurable enrichment directory via `TRIMETRY_ENRICHMENT_DIR`
- `--dry-run` flag disables telemetry (no external credentials required)
- Declarative metrics field registry (`metrics.Fields`) for single-source-of-truth aggregation, comparison, and reporting
- JSONL parser shared via generic `scanJSONLines[T]` function
- Unit tests for adapter parsers, evaluator, metrics, and comparator
- Version injection via `-ldflags` at build time
- `run-manifest.json` records adapter type, effectiveConfigHash, and dryRun flag
- Trial failure exits with code 1 for CI integration
- Secret detection for Langfuse keys (`sk-lf-` prefix)
- Go 1.25 requirement (opentelemetry dependency)
- `validate --dry-run` for credential-free config validation
- `config.ApplyDryRun()` centralizes adapter=fake + telemetry=false
- `ErrCancelled` error type for context-cancelled trials
- Comparison report includes baseline/candidate run IDs
- `buildContexts()` consolidates TrialContext/ExecutionContext construction
- `model.HasFailedTrials()` / `model.CountTrialStatuses()` helpers
- `ExecStatusCancelled` and `CancelledCount` in summary, comparison, and Markdown reports
- `effectiveConfigHash` excludes authentication fields (safe for shared reports)
- CLI error handling unified — all commands return errors to main
- `parseConfigFlags` helper for consistent --config / --dry-run parsing
- `run-manifest.json` redacts secret-like values in adapter options
- Secret prefix list shared between validation, EffectiveHash, and manifest output
