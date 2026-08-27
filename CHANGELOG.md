# Changelog

## [0.6.1](https://github.com/konono/trimetry/compare/v0.6.0...v0.6.1) (2026-08-27)


### Bug Fixes

* trimetry.annotate の observation type を EVENT に変更 ([#19](https://github.com/konono/trimetry/issues/19)) ([1536524](https://github.com/konono/trimetry/commit/15365246451b8069a19a3dc652425d8587a879e9))

## [0.6.0](https://github.com/konono/trimetry/compare/v0.5.1...v0.6.0) (2026-08-26)


### Features

* Claude adapter の Langfuse テレメトリ対応とモデル名 CLI 渡し ([#17](https://github.com/konono/trimetry/issues/17)) ([483ba56](https://github.com/konono/trimetry/commit/483ba562dcf8d90566a833486221608f4ee10d8f))

## [0.5.1](https://github.com/konono/trimetry/compare/v0.5.0...v0.5.1) (2026-08-21)


### Bug Fixes

* Langfuse healthcheck の IPv6 問題を修正し langfuse-down のデータ保持をデフォルトに ([4338950](https://github.com/konono/trimetry/commit/4338950dbdf5698024e27f3dbcb51373a2fbb51d))
* Langfuse healthcheck の IPv6 問題を修正し langfuse-down のデータ保持をデフォルトに ([8734666](https://github.com/konono/trimetry/commit/87346660a6239332345c204460b406a6e9f7129c))

## [0.5.0](https://github.com/konono/trimetry/compare/v0.4.1...v0.5.0) (2026-08-21)


### Features

* Langfuse Flush の耐障害性改善 ([fb7188d](https://github.com/konono/trimetry/commit/fb7188d86fa594fc1bca1b20ac84d94b5ceaf0a2))
* Langfuse Flush の耐障害性改善（リトライ・バッチ分割・再キュー） ([59f9fe9](https://github.com/konono/trimetry/commit/59f9fe9166c15f2f4b5d50f15886325fed536797))
* Langfuse v4 対応 + 公式プラグインへの移行 ([c322d28](https://github.com/konono/trimetry/commit/c322d2899c7a9694c37105b0b237f406e9f27334))
* Langfuse v4 対応 + 公式プラグインへの移行 + ローカル Langfuse 環境 ([14bb787](https://github.com/konono/trimetry/commit/14bb78737a24f17e4a238c08afde140e0ddb7561))


### Bug Fixes

* langfuse-up の healthcheck タイムアウトを 300 秒に延長 ([cbbf690](https://github.com/konono/trimetry/commit/cbbf690c26b2233b4b1e6eacd4c7b203a12b266c))
* PR レビュー指摘への対応 ([f7e446c](https://github.com/konono/trimetry/commit/f7e446cd52d87b0803be7b724bba40bf8ae6ff04))
* PR レビュー指摘への対応 ([fd53660](https://github.com/konono/trimetry/commit/fd53660163743d8897992470d577de809f37a725))
* レビュー指摘への対応（opencode.json.example / ドキュメント整合性） ([a02da10](https://github.com/konono/trimetry/commit/a02da1074a3c103d507929acaecd0470ec6672ca))
* 環境変数を .envrc に一本化し .env ファイルを廃止 ([fa2402f](https://github.com/konono/trimetry/commit/fa2402f61bf126706a6e71cca1a9b997313da132))
* 環境変数を .envrc に一本化し .env ファイルを廃止 ([ed3953d](https://github.com/konono/trimetry/commit/ed3953d9c962ddf01545991a067aa0af65c0b5ab))

## [0.4.1](https://github.com/konono/trimetry/compare/v0.4.0...v0.4.1) (2026-08-05)


### Bug Fixes

* Dockerfile の GOBIN PATH 追加と version.go の release-please 対応 ([5d16b2d](https://github.com/konono/trimetry/commit/5d16b2d272c68796b180a6ec3516408905a22b91))
* Dockerfile の GOBIN PATH 追加と version.go の release-please 対応 ([7caedfa](https://github.com/konono/trimetry/commit/7caedfab34ac241efc92c95afdecc764fb7c3d57))

## [0.4.0](https://github.com/konono/trimetry/compare/v0.3.0...v0.4.0) (2026-08-05)


### Features

* aw コンテナでのベンチマーク実行をサポート ([8fb5380](https://github.com/konono/trimetry/commit/8fb5380863a1b72949e6a1d69e64e71728746850))
* aw コンテナでのベンチマーク実行をサポート ([68f2260](https://github.com/konono/trimetry/commit/68f2260f75b1dbeb810129a746b836889c7cad4b))
* リッチな実行時表示と summary.md レポート改善 ([4ebcb35](https://github.com/konono/trimetry/commit/4ebcb3522ce4b618926b945af1dbe38ba358c34e))
* リッチな実行時表示と summary.md レポート改善 ([577e73c](https://github.com/konono/trimetry/commit/577e73c776135a0cec0c5af396a0198e93e943ec))

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
