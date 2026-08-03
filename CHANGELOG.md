# Changelog

## [0.3.0](https://github.com/konono/trimetry/compare/v0.2.0...v0.3.0) (2026-08-03)


### Features

* MLflow/Langfuse adapter 間のテレメトリ記録を統一 ([6865e05](https://github.com/konono/trimetry/commit/6865e0502bd198a852fa76591d8f61b38861ed82))
* MLflow/Langfuse adapter 間のテレメトリ記録を統一 ([0bef7db](https://github.com/konono/trimetry/commit/0bef7db072b7f3ca2da85633a2119e8978ae6685))


### Bug Fixes

* Claude adapter の Langfuse latency が epoch 起点で ~495,966 時間になるバグを修正 ([62b1346](https://github.com/konono/trimetry/commit/62b13463b4c097fefcd15ca193b3ab6ef717c895))

## [0.2.0](https://github.com/konono/trimetry/compare/v0.1.0...v0.2.0) (2026-07-30)


### Features

* trimetry - LLM agent benchmarking framework ([2893d5d](https://github.com/konono/trimetry/commit/2893d5d90a1ef64930706504a9052c46fc9b454b))

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
