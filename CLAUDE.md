# CLAUDE.md

## Trimetry とは

LLM エージェント（opencode, Claude Code, Codex, Cursor）のベンチマーク基盤。同一条件で複数回試行し、モデル変更やスキル変更の影響を定量的に比較する。

## コマンド一覧

```bash
trimetry run --config <path>              # ベンチマーク実行
trimetry run --config <path> --dry-run    # ドライラン（fake adapter、テレメトリ無効）
trimetry run --config <path> --verbose    # TTY で trial ごとの詳細行を表示
trimetry validate --config <path>         # 設定ファイルのバリデーションのみ
trimetry compare --baseline <path> --candidate <path>  # 2 つの実行結果を比較
trimetry diagnostics --config <path>      # テレメトリ接続の診断
trimetry version                          # バージョン表示
```

## ビルドとテスト

```bash
make build    # バイナリビルド（./trimetry が生成される）
make test     # テスト実行
make dry-run  # ドライラン（外部依存なし、CI でも使える）
```

## ベンチマーク設定ファイルの作り方

YAML で記述する。最小構成は以下の 4 セクション。

### 最小構成（外部依存なし、すぐ試せる）

```yaml
benchmark:
  name: my-benchmark
  trials: 3                # シナリオ×モデルあたりの試行回数
  concurrency: 1           # 同時実行数
  timeout_seconds: 60      # デフォルトタイムアウト（秒）

scenarios:
  - id: hello
    version: "1"
    name: Greeting Test
    input: "Hello, what model are you?"

models:
  - name: fake-model
    provider: fake

adapter:
  type: fake               # fake adapter = 外部ツール不要

telemetry:
  enabled: false

report:
  output_directory: benchmark-results
  formats: [json, markdown]
```

これを `benchmarks/my-test.yaml` に保存して `trimetry run --config benchmarks/my-test.yaml` で実行できる。

### 実際の LLM エージェントを使う構成

```yaml
benchmark:
  name: opencode-eval
  trials: 5
  concurrency: 1
  timeout_seconds: 300
  retries: 1               # 失敗時のリトライ回数

scenarios:
  - id: code-generation
    version: "1"
    name: Code Generation
    input: "Write a function to sort a list in Python"
    expected_output: "sort"   # contains マッチで accuracy 評価
    timeout_seconds: 120      # シナリオ別タイムアウト（省略時は benchmark.timeout_seconds）

  - id: file-listing
    version: "1"
    name: File Listing
    input: "List all .go files in this directory"

models:
  - name: claude-sonnet-4
    provider: anthropic
    parameters:
      temperature: 0
    pricing:                  # コスト推定用（省略可）
      input_per_m_token: 3.00
      output_per_m_token: 15.00

  - name: gpt-4o
    provider: openai
    parameters:
      temperature: 0
    pricing:
      input_per_m_token: 2.50
      output_per_m_token: 10.00

adapter:
  type: opencode              # opencode | claude | codex | cursor | fake
  options:
    command: opencode
    working_directory: "."

telemetry:
  enabled: true
  provider: langfuse          # langfuse | mlflow
  flush_on_trial_end: true

report:
  output_directory: benchmark-results
  formats: [json, markdown]
  mask_output: false          # true にするとレポート内の出力をマスク
```

### 設定フィールドのリファレンス

#### benchmark

| フィールド | 型 | デフォルト | 説明 |
|---|---|---|---|
| `name` | string | 必須 | ベンチマーク名 |
| `trials` | int | 5 | シナリオ×モデルあたりの試行回数 |
| `concurrency` | int | 1 | 同時実行数 |
| `timeout_seconds` | int | 300 | デフォルトタイムアウト（秒） |
| `retries` | int | 0 | 失敗時のリトライ回数 |
| `environment` | string | `"local"` | 実行環境ラベル |

#### scenarios[]

| フィールド | 型 | 説明 |
|---|---|---|
| `id` | string | 必須。一意な識別子 |
| `version` | string | 必須。シナリオのバージョン |
| `name` | string | 表示名 |
| `input` | string | 必須。エージェントへの入力プロンプト |
| `expected_output` | string | 省略可。出力に対する contains マッチで accuracy 評価 |
| `timeout_seconds` | int | 省略時は `benchmark.timeout_seconds` を使用 |
| `metadata` | map | 任意のメタデータ |

#### models[]

| フィールド | 型 | 説明 |
|---|---|---|
| `name` | string | 必須。モデル名 |
| `provider` | string | 必須。プロバイダー名 |
| `parameters` | map | モデルパラメータ（temperature 等） |
| `pricing.input_per_m_token` | float | 入力 100 万トークンあたりのコスト（USD） |
| `pricing.output_per_m_token` | float | 出力 100 万トークンあたりのコスト（USD） |

#### adapter

| フィールド | 型 | デフォルト | 説明 |
|---|---|---|---|
| `type` | string | `"opencode"` | `opencode` / `claude` / `codex` / `cursor` / `fake` |
| `options` | map | — | `command`: 実行コマンド、`working_directory`: 作業ディレクトリ |

#### telemetry

| フィールド | 型 | デフォルト | 説明 |
|---|---|---|---|
| `enabled` | bool | false | テレメトリ送信の有効/無効 |
| `provider` | string | `"langfuse"` | `langfuse` / `mlflow` |
| `flush_on_trial_end` | bool | false | trial ごとにフラッシュ |
| `enrichment_dir` | string | `/tmp/trimetry-enrichment` | enrichment ファイルディレクトリ |
| `tls_skip_verify` | bool | false | TLS 検証スキップ（self-signed cert 用） |

Langfuse 使用時は環境変数 `LANGFUSE_BASEURL`, `LANGFUSE_PUBLIC_KEY`, `LANGFUSE_SECRET_KEY` を設定。
MLflow 使用時は `MLFLOW_TRACKING_URI`（必須）、`MLFLOW_TRACKING_TOKEN`（任意）を設定。

## 結果の比較

```bash
# 2 つのベンチマーク実行結果を比較
trimetry compare \
  --baseline benchmark-results/<run-a>/summary.json \
  --candidate benchmark-results/<run-b>/summary.json
```

## 出力ファイル

`trimetry run` の実行後、`benchmark-results/<runId>/` に以下が生成される:

| ファイル | 内容 |
|---|---|
| `run-manifest.json` | 再現用設定情報（API キーなし） |
| `trials.jsonl` | trial ごとの詳細結果 |
| `summary.json` | 統計集計結果（compare の入力にも使う） |
| `summary.md` | Markdown レポート |
| `errors.jsonl` | エラー詳細 |

## Adapter 対応状況

使用する adapter に応じて、対象の LLM エージェント CLI が `$PATH` に必要。

| Adapter | 必要な CLI | 実行コマンド | メトリクス |
|---|---|---|---|
| `opencode` | `opencode` (`npm i -g @anthropic-ai/opencode`) | `opencode run --format json <input>` | 完全（Steps, Token, Cost） |
| `claude` | `claude` (`npm i -g @anthropic-ai/claude-code`) | `claude -p --output-format stream-json <input>` | 完全（Steps, Token, provider cost） |
| `codex` | `codex` (`npm i -g @openai/codex`) | `codex exec --json <input>` | 部分的（Steps 未生成で一部 null） |
| `cursor` | `cursor`（Cursor アプリから CLI インストール） | `cursor -p --output-format stream-json <input>` | 部分的（テキストと tool カウントのみ） |
| `fake` | なし | 内蔵モック | テスト・CI・ドライラン用 |

## プロジェクト構成

| ディレクトリ | 内容 |
|---|---|
| `cmd/trimetry/` | CLI エントリポイント |
| `internal/adapter/` | アプリケーションアダプター（opencode, claude, codex, cursor, fake） |
| `internal/runner/` | trial の並行実行・オーケストレーション |
| `internal/config/` | YAML 設定読み込み・バリデーション |
| `internal/comparator/` | baseline vs candidate 比較 |
| `internal/report/` | JSON + Markdown レポート出力 |
| `internal/telemetry/` | Langfuse / MLflow テレメトリ |
| `internal/ui/` | TTY / 非 TTY の実行時表示 |
| `internal/version/` | バージョン情報（リリース時に自動更新） |
| `benchmarks/` | ベンチマーク設定ファイル |
| `examples/` | 外部ツール用サンプル設定 |

## コミットメッセージ

[Conventional Commits](https://www.conventionalcommits.org/) に従う。release-please がコミットメッセージからリリースノートとバージョンを自動生成するため必須。

```
<type>: <description>
```

| type | 用途 | バージョン影響 |
|---|---|---|
| `feat` | 新機能 | minor |
| `fix` | バグ修正 | patch |
| `docs` | ドキュメントのみ | なし |
| `chore` | ビルド・CI・依存等 | なし |
| `refactor` | リファクタリング | なし |
| `test` | テスト追加・修正 | なし |
| `perf` | パフォーマンス改善 | patch |

Breaking change は `feat!:` または `fix!:` で表記（major バージョンアップ）。
