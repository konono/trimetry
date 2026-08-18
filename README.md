# Trimetry

LLM エージェントのモデル・プロンプト・スキル変更を**定量的に比較する**ベンチマーク基盤。

## なぜ必要か

LLM のモデル更新やスキル更新は、品質向上だけでなく品質劣化を引き起こす可能性がある。体感ベースの評価では再現性がなく、変更による改善・悪化を説明する根拠が不足する。

例えば、あるスキル変更後の実行が成功しても、その成功が変更によるものかは判断できない。変更前から成功率 80%・失敗率 20% なら:

- 変更前の実行で、たまたま 20% の失敗を引いた
- 変更後の実行で、たまたま 80% の成功を引いた
- 実際には成功率が変わっていない

Trimetry は同一条件で複数回試行し、偶然性を排除して変更の影響を定量的に測定する。

## 何を計測するか

初期段階で記録・比較できる指標:

| カテゴリ | 指標 |
|---|---|
| **レイテンシー** | タスク全体、LLM Generation ごと、ツール呼び出しごと、TTFT (Time To First Token) |
| **トークン** | 入力、出力、合計、reasoning、cache read/write |
| **コスト** | pricing 設定による推定コスト、provider 報告のコスト |
| **ツール利用** | 呼び出し回数、使用されたツール種類、各ツールの実行時間 |
| **成功・失敗** | 完了率、タイムアウト率、エラー内容 |
| **ばらつき** | 同一条件での mean, median, stddev, p90, p95 |
| **正確性** | `expected_output` に対する contains マッチ |

これにより以下を説明可能にする:

- どの処理が遅いのか
- モデル変更で何が変わったのか
- スキル変更でツール利用や処理フローがどう変化したか
- 同一条件での実行結果がどの程度ばらつくか

### 前提条件

- Go 1.25 以上（opentelemetry 依存による要件）
- ベンチマーク対象の LLM エージェント CLI（使用する adapter に応じてインストール）:

| Adapter | 必要な CLI | 実行されるコマンド | インストール |
|---|---|---|---|
| `opencode` | [opencode](https://github.com/opencode-ai/opencode) | `opencode run --format json <input>` | `npm i -g @anthropic-ai/opencode` |
| `claude` | [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `claude -p --output-format stream-json <input>` | `npm i -g @anthropic-ai/claude-code` |
| `codex` | [Codex](https://github.com/openai/codex) | `codex exec --json <input>` | `npm i -g @openai/codex` |
| `cursor` | [Cursor CLI](https://docs.cursor.com/cli) | `cursor -p --output-format stream-json <input>` | Cursor アプリから CLI をインストール |
| `fake` | なし | 内蔵モック（外部依存なし） | — |

## インストール

```bash
go install github.com/konono/trimetry/cmd/trimetry@latest
```

バージョン指定:

```bash
go install github.com/konono/trimetry/cmd/trimetry@v0.1.0
```

または [GitHub Releases](https://github.com/konono/trimetry/releases) からビルド済みバイナリをダウンロードできます。

## Quick Start

### 1. 動作確認（外部依存なし）

```bash
# ソースからビルドする場合
make build

# ドライラン（fake adapter で即実行できる）
trimetry run --config benchmarks/dry-run.yaml

# 設定ファイルのバリデーションだけ試す
trimetry validate --config benchmarks/example.yaml
```

### 2. 自分のベンチマークを作る

YAML ファイルを 1 つ書くだけで始められる。最小構成:

```yaml
# benchmarks/my-first.yaml
benchmark:
  name: my-first-benchmark
  trials: 3                # 同一条件で何回試行するか
  timeout_seconds: 120

scenarios:
  - id: code-gen
    version: "1"
    name: Code Generation
    input: "Write a Python function that reverses a string"
    expected_output: "reverse"   # 出力にこの文字列が含まれるか（accuracy 評価）

models:
  - name: claude-sonnet-4
    provider: anthropic
    parameters:
      temperature: 0

adapter:
  type: opencode             # 使う LLM エージェント（下表参照）
  options:
    command: opencode
    working_directory: "."

telemetry:
  enabled: false             # Langfuse/MLflow を使わないならまず false で

report:
  output_directory: benchmark-results
  formats: [json, markdown]
```

```bash
trimetry run --config benchmarks/my-first.yaml
```

実行後、`benchmark-results/<runId>/summary.md` に結果レポートが生成される。

### 3. 結果を比較する

モデルやスキルを変更して 2 回実行し、差分を見る:

```bash
# 変更前
trimetry run --config benchmarks/my-first.yaml
# → benchmark-results/run-aaa/summary.json

# 変更後（モデルや設定を変えて再実行）
trimetry run --config benchmarks/my-first.yaml
# → benchmark-results/run-bbb/summary.json

# 比較
trimetry compare \
  --baseline benchmark-results/run-aaa/summary.json \
  --candidate benchmark-results/run-bbb/summary.json
```

### 4. テレメトリを有効にする（Langfuse）

#### ローカル Langfuse を使う場合

```bash
# Langfuse v4 をローカルで起動（podman compose）
make langfuse-up

# opencode に公式 Langfuse プラグインをインストール
mise run setup-langfuse

# .env を編集してローカル Langfuse を指す
cp .env.langfuse.example .env.langfuse
# LANGFUSE_BASEURL=http://localhost:3000
# LANGFUSE_PUBLIC_KEY=pk-lf-trimetry-local
# LANGFUSE_SECRET_KEY=sk-lf-trimetry-local
```

#### 外部 Langfuse を使う場合

```bash
cp .env.example .env
# .env を編集: LANGFUSE_BASEURL, LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY を設定
```

#### 設定ファイルでテレメトリを有効化

```yaml
telemetry:
  enabled: true
  provider: langfuse
  flush_on_trial_end: true
```

`examples/` にサンプル設定があります（`opencode-smoke.yaml`, `mlflow-smoke.yaml`）。

## CLI

```bash
./trimetry validate            --config benchmarks/example.yaml
./trimetry validate --dry-run  --config examples/opencode-smoke.yaml  # 環境変数不要
./trimetry run                 --config benchmarks/example.yaml
./trimetry run                 --config benchmarks/dry-run.yaml  # fake adapter (フラグ不要)
./trimetry run --dry-run       --config examples/opencode-smoke.yaml  # fake adapter + telemetry 無効化
./trimetry compare             --baseline <path> --candidate <path>
./trimetry diagnose-tracing    --config <path>
```

> `--dry-run` は `validate` と `run` の両方で使えます。adapter を fake に差し替え、telemetry を無効化するため、Langfuse/MLflow の環境変数なしで実行できます。`benchmarks/dry-run.yaml` は YAML で同等の設定を定義した CI 用ファイルです。`examples/` の設定はプレースホルダー（`your-model` / `your-provider`）を含むため、実際のモデル名に書き換えて使用してください。

## アーキテクチャ

### 全体構造

ベンチマークの実行制御はランナーが担い、テレメトリバックエンド (Langfuse / MLflow) は観測・保存・分析のレイヤーとして使用する。

```
Benchmark Run
  └─ Scenario × Model × Trial
      └─ Trace (Langfuse)
          ├─ opencode.turn (AGENT)          ← 公式プラグインが作成
          │   ├─ opencode.message.user      ← プラグイン
          │   ├─ opencode.generation        ← プラグイン (tokens, model, cost)
          │   │   └─ tool (TOOL)            ← プラグイン
          │   └─ trimetry.annotate          ← trimetry (trace-level metadata)
          ├─ Tags: [benchmark, scenario:X, model:Y]
          ├─ Metadata: {scenarioId, trialNumber, wallTimeMs, accuracy, ...}
          └─ Scores: accuracy, completion, non_empty
```

### テレメトリの責務分離

opencode adapter では、トレース構造の作成と trimetry のベンチマークメタデータ注入を分離している。

| 責務 | 担当 | 方法 |
|---|---|---|
| トレース構造 (turn → generation → tool) | 公式プラグイン | OTLP span |
| LLM メトリクス (tokens, model, cost, TTFT) | 公式プラグイン | OTLP span attributes |
| input/output 記録 | 公式プラグイン | OTLP span attributes |
| ベンチマーク Score (accuracy 等) | trimetry | REST `score-create` |
| ベンチマーク metadata (scenarioId, trialNumber 等) | trimetry | OTLP `langfuse.trace.*` 属性 |
| 集計・比較レポート | trimetry | ローカルファイル |

trimetry は opencode の sessionId から Langfuse の traceId を検索し、`langfuse.trace.tags` / `langfuse.trace.metadata` / `langfuse.trace.input` / `langfuse.trace.output` を OTLP span 属性として送信する。これにより Langfuse UI の trace トップレベルにベンチマーク情報が表示される。

### コンポーネント

| コンポーネント | 責務 |
|---|---|
| Config Loader | YAML 設定読み込み・バリデーション |
| Application Adapter | CLI ツール (opencode, claude, codex, cursor) のサブプロセス実行と出力パース |
| Trial Executor | Trial の並行実行、タイムアウト、リトライ制御 |
| Telemetry Adapter | Langfuse: score-create (REST) + trace annotation (OTLP) / MLflow: OTLP |
| Metrics Builder | adapter 出力から TrialMetrics を構築 |
| Evaluator | completion / non_empty / accuracy の自動評価 |
| Aggregator | 統計集計 (mean, median, stddev, p90, p95) |
| Comparator | Baseline vs Candidate の差分比較 |
| Report Generator | JSON + Markdown レポート出力 |

### プロジェクト構成

```
benchmark/
├── cmd/trimetry/          # CLI エントリポイント
├── internal/
│   ├── adapter/           # Application Adapter (opencode, claude, codex, cursor, fake)
│   ├── aggregator/        # 統計集計
│   ├── comparator/        # Baseline vs Candidate 比較
│   ├── config/            # YAML 設定読み込み・バリデーション
│   ├── diagnostics/       # トレーシング診断
│   ├── evaluator/         # 自動評価 (completion, accuracy)
│   ├── id/                # ID 生成
│   ├── metrics/           # TrialMetrics 構築・フィールド定義
│   ├── model/             # 共通型定義
│   ├── report/            # JSON + Markdown レポート出力
│   ├── runner/            # Trial 並行実行・オーケストレーション
│   ├── telemetry/         # Langfuse / MLflow テレメトリ
│   └── version/           # バージョン情報
├── benchmarks/            # サンプル設定ファイル
├── examples/              # 外部ツール用設定例
└── .github/workflows/     # CI
```

### 設計判断

#### 論理 ID と物理 Trace ID の分離

ベンチマークランナーは独自の論理 ID (`benchmarkRunId`, `trialId`) を生成する。OTEL Trace ID は物理的な観測単位であり、論理 ID として使用しない。

LLM エージェント (opencode 等) は `ai.streamText()` 呼び出しごとに新しい OTEL trace を生成するため、1 回の trial で複数の Trace が作られる。ランナーは Trace ID を書き換えず、論理 ID ベースで結果を集約する。

```
Trial (logical ID: tr_xxx)
  ├─ OTEL Trace 1: ai.streamText (LLM call → tool calls)
  ├─ OTEL Trace 2: ai.streamText (LLM call → final response)
  └─ Benchmark Trace: all spans merged under tr_xxx
```

#### テレメトリバックエンドの選択

| | Langfuse | MLflow |
|---|---|---|
| プロトコル | OTLP (trace annotation) + REST (score-create) | OTLP (protobuf) |
| 認証 | Basic Auth | Bearer Token / なし |
| トレース作成 | 公式プラグイン (OTLP) | trimetry (OTLP) |
| trimetry の役割 | score + metadata 注入 | trace 全体を作成 |

#### Langfuse v4 対応

Langfuse v4 (`events_only` モード) では REST ingestion API が `score-create` のみ受け付ける。trace/generation/span の作成は OTLP 経由のみ。

opencode adapter では公式プラグイン (`@langfuse/opencode-observability-plugin`) がトレース構造を OTLP で作成し、trimetry は以下を後付けする:

1. sessionId → traceId の検索（Langfuse observations API）
2. `langfuse.trace.*` 属性による trace レベルのメタデータ注入（OTLP span）
3. evaluation score の登録（REST `score-create`）

#### 不明値の扱い

取得できなかった値は `null` (Go のポインタ型 nil) として扱い、`0` とは明示的に区別する。取得元は `tokenUsageSource` / `costSource` フィールドで記録する。

## 設定ファイル

```yaml
benchmark:
  name: my-benchmark
  trials: 5              # シナリオ×モデルあたりの試行回数
  concurrency: 1          # 同時実行数
  timeout_seconds: 300    # デフォルトタイムアウト
  retries: 0              # リトライ回数
  environment: local

scenarios:
  - id: code-generation
    version: "1"
    name: Code Generation Test
    input: "Write a function to sort a list"
    expected_output: "sort"    # 任意: contains マッチで accuracy 評価
    timeout_seconds: 120       # シナリオ別タイムアウト (任意)

models:
  - name: gpt-4o
    provider: openai
    parameters:
      temperature: 0
    pricing:                   # 任意: コスト推定用
      input_per_m_token: 2.50
      output_per_m_token: 10.00

adapter:
  type: opencode               # opencode | claude | codex | cursor | fake
  # Adapter 対応状況は下表参照
  options:
    command: opencode
    working_directory: "."

telemetry:
  enabled: true
  provider: langfuse           # langfuse | mlflow
  flush_on_trial_end: true     # trial ごとにテレメトリをフラッシュ
  enrichment_dir: /tmp/trimetry-enrichment  # enrichment ファイルの読み取りディレクトリ (任意)
  tls_skip_verify: false       # MLflow の TLS 検証をスキップ (self-signed cert 用)

report:
  output_directory: benchmark-results
  formats: [json, markdown]
  mask_output: false           # 出力をマスクして保存
```

## Adapter 対応状況

| Adapter | Steps / LLM latency | Token / Cost | 備考 |
|---|---|---|---|
| **opencode** | 完全 | 完全 | メイン想定 |
| **claude** | 完全 | provider cost 対応 | |
| **codex** | 部分的 | 部分的 | Steps 未生成のため llmLatencyMs 等は null |
| **cursor** | 部分的 | なし | テキストと tool カウントのみ |
| **fake** | 完全 | 固定値 | テスト・CI 用 |

## 評価

現在の組み込み評価:

| 名前 | 内容 | 実行条件 |
|---|---|---|
| `completion` | Trial が正常完了したか | 自動 (全 trial) |
| `non_empty` | 出力が空でないか | 自動 (全 trial) |
| `accuracy` | `expected_output` に対する contains マッチ | `expected_output` 設定時のみ |

将来的には、定量指標とは別に以下の品質評価を追加予定:

- LLM-as-a-Judge による回答品質評価
- 期待するツールが呼ばれたか / 禁止ツールが呼ばれていないか
- ルールベース評価

初期フェーズでは品質評価を完全に実装するのではなく、後から評価ロジックを追加できるデータ構造を準備している。

## 出力ファイル

```
benchmark-results/
  <benchmarkRunId>/
    run-manifest.json    # 再現用設定情報（API キーなし）
    trials.jsonl         # Trial ごとの詳細結果
    summary.json         # 集計結果
    summary.md           # Markdown レポート
    errors.jsonl         # エラー詳細
```

`run-manifest.json` には以下の再現性情報が含まれます:

- `configHash` — 元の YAML ファイルの SHA256
- `effectiveConfigHash` — 実行時設定（defaults/env/--dry-run 適用後）の SHA256（認証情報は除外）
- `adapter` — 使用した adapter の type と options
- `dryRun` — `--dry-run` フラグが指定されたかどうか

trial が 1 つでも失敗・タイムアウト・キャンセルした場合、`run` コマンドは exit code 1 を返します。各シナリオサマリーに `cancelledCount` が記録されます。

## 環境変数

| 変数 | 用途 |
|---|---|
| `LANGFUSE_BASEURL` | Langfuse サーバー URL |
| `LANGFUSE_PUBLIC_KEY` | Langfuse 公開キー |
| `LANGFUSE_SECRET_KEY` | Langfuse 秘密キー |
| `MLFLOW_TRACKING_URI` | MLflow Tracking サーバー URL |
| `MLFLOW_EXPERIMENT_ID` | MLflow の experiment ID (opencode プラグイン用) |
| `MLFLOW_TRACKING_TOKEN` | MLflow 認証トークン (任意) |
| `MLFLOW_TRACKING_WORKSPACE` | MLflow ワークスペース名 (任意) |
| `TRIMETRY_ENRICHMENT_DIR` | enrichment ファイルの読み取りディレクトリ (デフォルト: `/tmp/trimetry-enrichment`) |

## セットアップ (mise)

```bash
mise install                    # go, node, opencode, podman, docker-compose をインストール
mise run build                  # ビルド
mise run setup-langfuse         # Langfuse 公式プラグイン設定
mise run setup-mlflow           # MLflow プラグイン設定
```

## opencode プラグインの設定

opencode adapter でテレメトリを使う場合、プラグインのインストールと `opencode.json` での有効化が必要。

### Langfuse（公式プラグイン）

```bash
mise run setup-langfuse   # 公式プラグインをインストール
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
mise run setup-mlflow     # MLflow プラグインをインストール
```

```json
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["@konono/opencode-plugin-mlflow"]
}
```

> **注意:** Langfuse と MLflow のプラグインは排他的に使用する。両方を同時に `plugin` 配列に入れないこと。

## aw コンテナでの実行

[aw](https://github.com/konono/aw) の使い捨てコンテナ内で trimetry を実行できる。ベンチマーク対象リポに必要なファイルを配置し、`aw bench` で起動する。

### 対象リポに必要なファイル

```
my-project/                        ← ベンチマーク対象リポ
├── opencode.json                  ← opencode のプロバイダ設定（モデル・エンドポイント）
├── .vllm-token                    ← API キー（opencode.json から参照）
└── benchmarks/
    └── file-count.yaml            ← trimetry のベンチマーク設定
```

**opencode.json** — ベンチマーク対象リポのルートに配置。プロバイダ設定とモデル定義を記述する:

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

**benchmarks/\*.yaml** — trimetry のベンチマーク設定。`working_directory` は `.` にする:

```yaml
benchmark:
  name: my-benchmark
  trials: 5
  timeout_seconds: 120

scenarios:
  - id: my-scenario
    version: "1"
    input: "ベンチマーク用のプロンプト"

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

### イメージのビルドと実行

```bash
# 1. trimetry リポでイメージビルド（GITHUB_TOKEN 不要）
cd ~/gitrepo/trimetry
make aw-build

# 2. 対象リポに移動してベンチマーク実行
cd ~/gitrepo/my-project
aw bench -- trimetry run --config benchmarks/file-count.yaml
```

### イメージの中身

`docker/Dockerfile` で以下をベイクしている:

| コンポーネント | 用途 |
|---|---|
| Go (mise 経由) | trimetry のビルド・実行 |
| trimetry | ベンチマーク実行 |
| opencode | LLM エージェント CLI |
| @langfuse/opencode-observability-plugin | Langfuse 公式プラグイン（トレース構造作成） |
| グローバル opencode 設定 | どのディレクトリでもプラグイン + OTEL が有効 |

> **Note:** 公式プラグインは npm public registry にあるため、ビルド時に `GITHUB_TOKEN` は不要。

### 別環境への持ち出し

```bash
# tar に書き出し
make aw-save

# 別環境でロード
podman load < trimetry-bench.tar
```

## 既知の制約

- コスト推定は pricing 設定に依存 (provider 報告のコストは一部のみサポート)
- LLM-as-a-Judge は将来対応予定
- tokensPerGen は Steps から数えた generation 数を使用。Steps がない adapter (codex/cursor) では `len(ToolCalls) + 1` にフォールバック
- Ctrl+C (SIGINT) はセマフォ待ちの未開始 trial をキャンセルする（`executionStatus: "cancelled"`, `errorType: "cancelled"`）。実行中の trial はアダプターのタイムアウトまで走り続ける
- リトライ (`retries`) は成功するまで再試行する。タイムアウトもリトライ対象だが、コンテキストキャンセル時はリトライを中断する

## License

MIT
