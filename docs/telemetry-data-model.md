# Telemetry Data Model

trimetry のテレメトリデータが Langfuse / MLflow それぞれのプラットフォームにどのようにマッピングされるかを記述する。

## コンセプトマッピング

| trimetry | Langfuse | MLflow |
|---|---|---|
| Benchmark Run | Session (`sessionId`) | Experiment (`{benchmarkName}/{scenarioID}-{shortRunID}`) |
| Benchmark Name | Tag (`benchmark:{name}`) + metadata | Experiment 名のプレフィクス |
| Trial | Trace | Trace（root span = `CHAIN` 型） |
| Scenario | metadata (`scenarioId`) + tag (`scenario:{id}`) | root span attribute (`benchmark.scenario_id`) |
| Model | metadata (`modelName`) + tag (`model:{name}`) | root span attribute (`benchmark.model_name`) |
| Generation Step | Generation observation | Span（`LLM` 型） |
| Tool Step | Span observation | Span（`TOOL` 型） |
| Evaluation | Score | Span（`EVALUATOR` 型） |
| App version | `release` フィールド | `InstrumentationVersion` |

## 用語マッピング早見表

| 概念 | Langfuse の用語 | MLflow の用語 |
|---|---|---|
| エラー表現 | `level` (`ERROR` / `DEFAULT`) | OTEL status code (`codes.Error` / `codes.Ok`) |
| エラー詳細 | `statusMessage` | OTEL status message |
| トレースのグループ化 | `sessionId` | Experiment |
| 階層的グループ化 | Tags でフィルタ | Experiment 名の `/` 区切り |
| トークン使用量 | `usage` body（`input`/`output`/`total`/`unit`） | `mlflow.chat.tokenUsage` attribute（JSON） |
| コスト | metadata の `estimatedCost`。generation の `cost_details` で自動集計も可能 | root span attribute `estimatedCost` |
| ステップ入出力 | `input`/`output` フィールド（ネイティブ JSON） | `mlflow.spanInputs`/`mlflow.spanOutputs` attribute（JSON 文字列） |
| span 種別 | イベント type（`generation-create`/`span-create`） | `mlflow.spanType` attribute（`LLM`/`TOOL`/`CHAIN`/`EVALUATOR`） |

## フィールド別比較表

### Trace / Root Span レベル

| フィールド | Langfuse | MLflow |
|---|---|---|
| Trial ID | `id`（Trace ID） | Span に紐づく trace ID |
| Trace 名 | `name`: `{scenarioID}_trial-{N}` | `mlflow.traceName`: `{scenarioID}_trial-{N}` |
| Benchmark Run ID | `sessionId` | `benchmark.run_id` attribute |
| Benchmark Name | `benchmarkName` metadata + tag | Experiment 名に含む |
| Scenario ID | `scenarioId` metadata + tag | `benchmark.scenario_id` attribute |
| Scenario Version | `scenarioVersion` metadata | `benchmark.scenario_version` attribute |
| Model Name | `modelName` metadata + tag | `benchmark.model_name` attribute |
| Model Provider | `modelProvider` metadata | `benchmark.model_provider` attribute |
| Model Parameters | `modelParameters` metadata | `benchmark.model_parameters` attribute（JSON） |
| Trial Number | `trialNumber` metadata | `benchmark.trial_number` attribute |
| Input | `input` フィールド（ChatML 形式: `[{"role":"user","content":"..."}]`） | `mlflow.spanInputs`（`{"prompt": "..."}` JSON） |
| Output | `output` フィールド（ChatML 形式: `[{"role":"assistant","content":"..."}]`） | `mlflow.spanOutputs`（同内容の JSON） |
| Execution Status | `executionStatus` metadata | `executionStatus` attribute + OTEL status code |
| Wall Time | `wallTimeMs` metadata | `wallTimeMs` attribute |
| Retry Count | `retryCount` metadata | `retryCount` attribute |
| Token Usage Source | `tokenUsageSource` metadata | `tokenUsageSource` attribute |
| Cost Source | `costSource` metadata | `costSource` attribute |
| LLM Latency | `llmLatencyMs` metadata | `llmLatencyMs` attribute |
| Idle Time | `idleMs` metadata | `idleMs` attribute |
| Estimated Cost | `estimatedCost` metadata | `estimatedCost` attribute |
| Accuracy Score | `accuracyScore` metadata | `accuracyScore` attribute |
| Total Tokens | output 内の `totalTokens` | `mlflow.chat.tokenUsage` attribute |
| Host Name | `hostName` metadata | `hostName` attribute |
| Host Arch | `hostArch` metadata | `hostArch` attribute |
| AI Settings | `aiSettings` metadata | `aiSettings` attribute（JSON） |
| App Version | `release` フィールド | `InstrumentationVersion` |

### Generation Step レベル

| フィールド | Langfuse | MLflow |
|---|---|---|
| Step Index | `stepIndex` metadata | `stepIndex` attribute |
| Status | `status` metadata | `status` attribute |
| Level/Error | `level` (`ERROR`/`DEFAULT`) + `statusMessage` | (root span の OTEL status に集約) |
| Reason | `reason` metadata | `finishReason` attribute |
| Duration | `durationMs` metadata | `durationMs` attribute |
| LLM Inference | `llmInferenceMs` metadata | `llmInferenceMs` attribute |
| Tool Time | `toolTimeMs` metadata | `toolTimeMs` attribute |
| Model | `model` フィールド（ネイティブ） | `model` attribute |
| Tokens | `usage` body | `mlflow.chat.tokenUsage` + 個別 attribute |
| TTFT | `ttftMs` metadata | `ttftMs` attribute |
| Tools Called | `toolsCalled` metadata | `toolsCalled` attribute（JSON） |
| Reasoning Tokens | `reasoningTokens` metadata | `reasoningTokens` attribute |
| Cache Read/Write | `cacheReadTokens`/`cacheWriteTokens` metadata | `cacheReadTokens`/`cacheWriteTokens` attribute |
| Reasoning Text | output 内（enrichment 経由） | `reasoning` attribute（enrichment 経由） |
| Finish Reason | output 内（enrichment 経由） | `finishReason` attribute（enrichment 経由） |
| AI Settings | metadata `aiSettings`（enrichment 経由） | `aiSettings` attribute（enrichment 経由） |
| Start/End Time | `startTime`/`endTime`（未設定なら `endTime` = null） | span timestamp（未設定なら `endTime` = `startTime`） |

### Tool Step レベル

| フィールド | Langfuse | MLflow |
|---|---|---|
| Step Index | `stepIndex` metadata | `stepIndex` attribute |
| Status | `status` metadata | `status` attribute |
| Level/Error | `level` + `statusMessage` | (root span の OTEL status に集約) |
| Duration | `durationMs` metadata | `durationMs` attribute |
| Call ID | `callID` metadata | `callID` attribute |
| Title | `title` metadata | `title` attribute |
| Display | `display` metadata | `display` attribute（JSON） |

### Evaluation

| フィールド | Langfuse | MLflow |
|---|---|---|
| Evaluator Name | Score `name` | Span 名 `eval:{name}` + `evaluatorName` attribute |
| Score | Score `value` | `score` attribute |
| Reason | Score `comment` | `reason` attribute |
| Passed | ― | `passed` in `mlflow.spanOutputs` |
| Data Type | `NUMERIC` | ― |
| Score Config | `configId`（自動作成） | ― |
| Span Type | ―（Score は observation ではない） | `EVALUATOR` |

## 設計判断

### Langfuse generation の `version` フィールドは使用しない

Langfuse の `version` は observation レベルのフィールドで、プロンプトテンプレートのバージョン管理を目的としている。trimetry はプロンプト管理を行わないため、semantics が合わない。代わりに:

- **App version** → `release` フィールド（trace レベル）
- **Scenario version** → `scenarioVersion` metadata（trace レベル）

### ステップの end time の扱いが異なる

- **Langfuse**: `endTime` が不明な場合は `null`（JSON で省略）。Langfuse API はこれを「進行中または不明」として扱う。
- **MLflow**: OTEL span は具体的な `time.Time` 値が必要。不明な場合は `startTime` と同値にする（zero duration）。

これはプラットフォームの制約の違いであり、意図的に異なる実装を維持している。

### BenchmarkName の表現が異なる

- **MLflow**: Experiment 名に含める（`{BenchmarkName}/{ScenarioID}-{shortRunID}`）。MLflow の Experiment はトレースの主要なグループ化メカニズム。
- **Langfuse**: Tag（`benchmark:{name}`）と metadata に記録。Langfuse では `sessionId` が主要なグループ化で、Tag によるフィルタリングが補助的に機能する。

### estimatedCost の記録場所

- **Langfuse**: trace metadata に記録。Langfuse の `cost_details` は generation レベルのフィールドで、個々の LLM 呼び出しのコストを記録するもの。`estimatedCost` はトライアル全体の集計値であり、trace metadata が適切。
- **MLflow**: root span attribute に記録。

### Token 使用量の記録形式

- **Langfuse**: generation observation の `usage` body（ネイティブフィールド）。Langfuse はこれを自動集計してダッシュボードに表示する。
- **MLflow**: `mlflow.chat.tokenUsage` attribute（JSON 文字列）。MLflow はこの規約名でトークン集計を行う。
