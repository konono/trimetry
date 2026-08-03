package telemetry

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/konono/trimetry/internal/adapter"
	"github.com/konono/trimetry/internal/version"
	mdl "github.com/konono/trimetry/internal/model"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type MLflowAdapter struct {
	trackingURI   string
	token         string
	workspace     string
	tlsSkipVerify bool
	client        *http.Client

	mu            sync.Mutex
	experimentIDs map[string]string // experimentName -> experimentID
	traces        map[string]mlflowTraceState
	tps           map[string]*sdktrace.TracerProvider // experimentID -> TracerProvider
}

type mlflowTraceState struct {
	ExperimentID string
	StartTimeMs  int64
	Input        string
	ModelName    string
	TrialCtx     TrialContext
}

func NewMLflowAdapter(trackingURI, token, workspace string, tlsSkipVerify bool) *MLflowAdapter {
	trackingURI = strings.TrimRight(trackingURI, "/")
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	if tlsSkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return &MLflowAdapter{
		trackingURI:   trackingURI,
		token:         token,
		workspace:     workspace,
		tlsSkipVerify: tlsSkipVerify,
		experimentIDs: make(map[string]string),
		traces:        make(map[string]mlflowTraceState),
		tps:           make(map[string]*sdktrace.TracerProvider),
		client:        client,
	}
}

func (a *MLflowAdapter) StartTrial(ctx TrialContext) {
	shortRunID := ctx.BenchmarkRunID
	if idx := strings.LastIndex(shortRunID, "-"); idx >= 0 && len(shortRunID)-idx > 8 {
		shortRunID = shortRunID[len(shortRunID)-8:]
	} else if len(shortRunID) > 8 {
		shortRunID = shortRunID[len(shortRunID)-8:]
	}
	expName := fmt.Sprintf("%s-%s", ctx.ScenarioID, shortRunID)
	if ctx.BenchmarkName != "" {
		expName = fmt.Sprintf("%s/%s-%s", ctx.BenchmarkName, ctx.ScenarioID, shortRunID)
	}
	experimentID, err := a.ensureExperiment(expName)
	if err != nil {
		log.Printf("WARNING: MLflow ensureExperiment failed: %v", err)
		return
	}

	a.mu.Lock()
	a.traces[ctx.TrialID] = mlflowTraceState{
		ExperimentID: experimentID,
		StartTimeMs:  time.Now().UnixMilli(),
		Input:        ctx.Input,
		ModelName:    ctx.ModelName,
		TrialCtx:     ctx,
	}
	a.mu.Unlock()
}

func (a *MLflowAdapter) FinishTrial(result TrialResult) {
	a.mu.Lock()
	state, ok := a.traces[result.TrialID]
	delete(a.traces, result.TrialID)
	a.mu.Unlock()

	if !ok {
		log.Printf("WARNING: MLflow trace not found for trial %s", result.TrialID)
		return
	}

	executionMs := time.Now().UnixMilli() - state.StartTimeMs
	if result.Metrics != nil && result.Metrics.WallTimeMs > 0 {
		executionMs = result.Metrics.WallTimeMs
	}

	ctx := context.Background()

	tp, err := a.getOrCreateTracerProvider(ctx, state.ExperimentID)
	if err != nil {
		log.Printf("WARNING: MLflow tracer provider failed: %v", err)
		return
	}

	tracer := tp.Tracer("benchmark", trace.WithInstrumentationVersion(version.Version))

	startTime := time.UnixMilli(state.StartTimeMs)
	endTime := startTime.Add(time.Duration(executionMs) * time.Millisecond)

	rootInputJSON, _ := json.Marshal(map[string]any{"prompt": state.Input})
	rootOutputJSON, _ := json.Marshal(buildTrialOutput(result))

	rootAttrs := []attribute.KeyValue{
		attribute.String("mlflow.spanType", "CHAIN"),
		attribute.String("mlflow.spanInputs", string(rootInputJSON)),
		attribute.String("mlflow.spanOutputs", string(rootOutputJSON)),
		attribute.String("mlflow.traceName", fmt.Sprintf("%s_trial-%d", state.TrialCtx.ScenarioID, state.TrialCtx.TrialNumber)),
		attribute.String("benchmark.run_id", state.TrialCtx.BenchmarkRunID),
		attribute.String("benchmark.scenario_id", state.TrialCtx.ScenarioID),
		attribute.String("benchmark.scenario_version", state.TrialCtx.ScenarioVersion),
		attribute.String("benchmark.model_name", state.TrialCtx.ModelName),
		attribute.String("benchmark.model_provider", state.TrialCtx.ModelProvider),
		attribute.String("benchmark.trial_number", strconv.Itoa(state.TrialCtx.TrialNumber)),
		attribute.String("executionStatus", string(result.ExecutionStatus)),
	}

	if len(state.TrialCtx.ModelParameters) > 0 {
		mpJSON, _ := json.Marshal(state.TrialCtx.ModelParameters)
		rootAttrs = append(rootAttrs, attribute.String("benchmark.model_parameters", string(mpJSON)))
	}

	if result.Metrics != nil {
		rootAttrs = append(rootAttrs,
			attribute.Int64("wallTimeMs", result.Metrics.WallTimeMs),
			attribute.Int64("retryCount", int64(result.Metrics.RetryCount)),
			attribute.String("tokenUsageSource", result.Metrics.TokenUsageSource),
			attribute.String("costSource", result.Metrics.CostSource),
		)
		if result.Metrics.TotalTokens != nil {
			tokenJSON, _ := json.Marshal(map[string]any{
				"input_tokens":  ptrVal(result.Metrics.InputTokens),
				"output_tokens": ptrVal(result.Metrics.OutputTokens),
				"total_tokens":  *result.Metrics.TotalTokens,
			})
			rootAttrs = append(rootAttrs, attribute.String("mlflow.chat.tokenUsage", string(tokenJSON)))
		}
		if result.Metrics.LLMLatencyMs != nil {
			rootAttrs = append(rootAttrs, attribute.Int64("llmLatencyMs", *result.Metrics.LLMLatencyMs))
		}
		if result.Metrics.IdleMs != nil {
			rootAttrs = append(rootAttrs, attribute.Int64("idleMs", *result.Metrics.IdleMs))
		}
		if result.Metrics.EstimatedCost != nil {
			rootAttrs = append(rootAttrs, attribute.Float64("estimatedCost", *result.Metrics.EstimatedCost))
		}
		if result.Metrics.AccuracyScore != nil {
			rootAttrs = append(rootAttrs, attribute.Float64("accuracyScore", *result.Metrics.AccuracyScore))
		}
	}

	_, rootSpan := tracer.Start(ctx, fmt.Sprintf("%s_trial-%d", state.TrialCtx.ScenarioID, state.TrialCtx.TrialNumber),
		trace.WithTimestamp(startTime),
		trace.WithAttributes(rootAttrs...),
	)

	rootSC := rootSpan.SpanContext()
	rootCtx := trace.ContextWithSpanContext(ctx, rootSC)

	steps, enrichment := prepareEnrichedSteps(result)
	if enrichment.Environment != nil {
		env := enrichment.Environment
		if env.HostName != "" {
			rootSpan.SetAttributes(attribute.String("hostName", env.HostName))
		}
		if env.HostArch != "" {
			rootSpan.SetAttributes(attribute.String("hostArch", env.HostArch))
		}
		if len(env.AISettings) > 0 {
			aiJSON, _ := json.Marshal(env.AISettings)
			rootSpan.SetAttributes(attribute.String("aiSettings", string(aiJSON)))
		}
	}
	for i, step := range steps {
		a.emitStepSpan(rootCtx, tracer, i, step, result.ModelName)
	}

	for _, eval := range result.Evaluations {
		if eval.Score != nil {
			a.emitEvalSpan(rootCtx, tracer, eval, endTime)
		}
	}

	if result.ExecutionStatus == mdl.ExecStatusFailed || result.ExecutionStatus == mdl.ExecStatusTimeout {
		rootSpan.SetStatus(codes.Error, string(result.ExecutionStatus))
	} else {
		rootSpan.SetStatus(codes.Ok, "")
	}
	rootSpan.End(trace.WithTimestamp(endTime))

	tp.ForceFlush(ctx)
}



func (a *MLflowAdapter) emitStepSpan(parentCtx context.Context, tracer trace.Tracer, idx int, step adapter.StepDetail, defaultModel string) {
	stepStart, stepEnd, hasEnd := stepTimeRange(step.StartMs, step.EndMs)
	if !hasEnd {
		stepEnd = stepStart
	}

	inputJSON, _ := json.Marshal(step.Input)
	outputJSON, _ := json.Marshal(step.Output)

	spanType := "UNKNOWN"
	switch step.Type {
	case adapter.StepTypeGeneration:
		spanType = "LLM"
	case adapter.StepTypeTool:
		spanType = "TOOL"
	}

	attrs := []attribute.KeyValue{
		attribute.String("mlflow.spanType", spanType),
		attribute.String("mlflow.spanInputs", string(inputJSON)),
		attribute.String("mlflow.spanOutputs", string(outputJSON)),
		attribute.Int64("stepIndex", int64(idx)),
		attribute.String("status", step.Status),
		attribute.Int64("durationMs", step.DurationMs),
	}

	if step.Type == adapter.StepTypeGeneration {
		model := step.Model
		if model == "" {
			model = defaultModel
		}
		if model != "" {
			attrs = append(attrs, attribute.String("model", model))
		}
		attrs = append(attrs, attribute.Int64("llmInferenceMs", step.LLMInferenceMs))
		if step.ToolTimeMs > 0 {
			attrs = append(attrs, attribute.Int64("toolTimeMs", step.ToolTimeMs))
		}
		if step.Reason != "" {
			attrs = append(attrs, attribute.String("finishReason", step.Reason))
		}
		if outputMap, ok := step.Output.(map[string]any); ok {
			if r, ok := outputMap["reasoning"].(string); ok && r != "" {
				attrs = append(attrs, attribute.String("reasoning", r))
			}
			if fr, ok := outputMap["finishReason"].(string); ok && fr != "" {
				attrs = append(attrs, attribute.String("finishReason", fr))
			}
		}
		if step.Metadata != nil {
			if aiSettings, ok := step.Metadata["aiSettings"]; ok {
				aiJSON, _ := json.Marshal(aiSettings)
				attrs = append(attrs, attribute.String("aiSettings", string(aiJSON)))
			}
		}
		if step.Tokens != nil {
			tokenJSON, _ := json.Marshal(map[string]any{
				"input_tokens":  step.Tokens.Input,
				"output_tokens": step.Tokens.Output,
				"total_tokens":  step.Tokens.Total,
			})
			attrs = append(attrs, attribute.String("mlflow.chat.tokenUsage", string(tokenJSON)))
			attrs = append(attrs,
				attribute.Int64("inputTokens", step.Tokens.Input),
				attribute.Int64("outputTokens", step.Tokens.Output),
				attribute.Int64("reasoningTokens", step.Tokens.Reasoning),
				attribute.Int64("cacheReadTokens", step.Tokens.CacheRead),
				attribute.Int64("cacheWriteTokens", step.Tokens.CacheWrite),
			)
		}
		if step.TTFTMs != nil {
			attrs = append(attrs, attribute.Int64("ttftMs", *step.TTFTMs))
		}
		if len(step.ToolsCalled) > 0 {
			toolsJSON, _ := json.Marshal(step.ToolsCalled)
			attrs = append(attrs, attribute.String("toolsCalled", string(toolsJSON)))
		}
	}

	if step.Type == adapter.StepTypeTool {
		if step.CallID != "" {
			attrs = append(attrs, attribute.String("callID", step.CallID))
		}
		if step.Title != "" {
			attrs = append(attrs, attribute.String("title", step.Title))
		}
		if step.Display != nil {
			displayJSON, _ := json.Marshal(step.Display)
			attrs = append(attrs, attribute.String("display", string(displayJSON)))
		}
	}

	_, span := tracer.Start(parentCtx, step.Name,
		trace.WithTimestamp(stepStart),
		trace.WithAttributes(attrs...),
	)
	span.SetStatus(codes.Ok, "")
	span.End(trace.WithTimestamp(stepEnd))
}

func (a *MLflowAdapter) emitEvalSpan(parentCtx context.Context, tracer trace.Tracer, eval mdl.EvaluationResult, baseTime time.Time) {
	scoreJSON, _ := json.Marshal(map[string]any{
		"evaluatorName": eval.EvaluatorName,
		"score":         eval.Score,
		"passed":        eval.Passed,
		"reason":        eval.Reason,
	})

	attrs := []attribute.KeyValue{
		attribute.String("mlflow.spanType", "EVALUATOR"),
		attribute.String("mlflow.spanOutputs", string(scoreJSON)),
		attribute.String("evaluatorName", eval.EvaluatorName),
		attribute.Float64("score", *eval.Score),
		attribute.String("reason", eval.Reason),
	}

	_, span := tracer.Start(parentCtx, "eval:"+eval.EvaluatorName,
		trace.WithTimestamp(baseTime),
		trace.WithAttributes(attrs...),
	)
	span.SetStatus(codes.Ok, "")
	span.End(trace.WithTimestamp(baseTime))
}

func (a *MLflowAdapter) Flush() {
	a.mu.Lock()
	providers := make([]*sdktrace.TracerProvider, 0, len(a.tps))
	for _, tp := range a.tps {
		providers = append(providers, tp)
	}
	a.mu.Unlock()

	for _, tp := range providers {
		tp.ForceFlush(context.Background())
	}
	log.Printf("  MLflow: all traces finalized")
}

func (a *MLflowAdapter) getOrCreateTracerProvider(ctx context.Context, experimentID string) (*sdktrace.TracerProvider, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if tp, ok := a.tps[experimentID]; ok {
		return tp, nil
	}
	tp, err := a.createTracerProvider(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	a.tps[experimentID] = tp
	return tp, nil
}

func (a *MLflowAdapter) createTracerProvider(ctx context.Context, experimentID string) (*sdktrace.TracerProvider, error) {
	parsedURL, err := url.Parse(a.trackingURI)
	if err != nil {
		return nil, fmt.Errorf("bad tracking URI: %w", err)
	}

	otlpEndpoint := parsedURL.Host
	otlpPath := strings.TrimRight(parsedURL.Path, "/") + "/v1/traces"

	headers := map[string]string{
		"x-mlflow-experiment-id": experimentID,
	}
	if a.workspace != "" {
		headers["X-MLflow-Workspace"] = a.workspace
	}
	if a.token != "" {
		headers["Authorization"] = "Bearer " + a.token
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(otlpEndpoint),
		otlptracehttp.WithURLPath(otlpPath),
		otlptracehttp.WithHeaders(headers),
	}
	if parsedURL.Scheme == "https" && a.tlsSkipVerify {
		opts = append(opts, otlptracehttp.WithTLSClientConfig(&tls.Config{InsecureSkipVerify: true}))
	} else if parsedURL.Scheme != "https" {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}

	res := resource.NewSchemaless(
		attribute.String("service.name", "benchmark"),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(1*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	return tp, nil
}

func ptrVal(p *int64) int64 {
	if p != nil {
		return *p
	}
	return 0
}

// Experiment management

func (a *MLflowAdapter) ensureExperiment(experimentName string) (string, error) {
	a.mu.Lock()
	if eid, ok := a.experimentIDs[experimentName]; ok {
		a.mu.Unlock()
		return eid, nil
	}
	a.mu.Unlock()

	createBody := map[string]any{"name": experimentName}
	resp, err := a.apiPost("/api/2.0/mlflow/experiments/create", createBody)
	if err != nil {
		if strings.Contains(err.Error(), "RESOURCE_ALREADY_EXISTS") {
			return a.getExperimentByName(experimentName)
		}
		return "", fmt.Errorf("create experiment: %w", err)
	}

	var createResult struct {
		ExperimentID string `json:"experiment_id"`
	}
	if err := json.Unmarshal(resp, &createResult); err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	a.mu.Lock()
	a.experimentIDs[experimentName] = createResult.ExperimentID
	a.mu.Unlock()

	log.Printf("  MLflow: created experiment %q (id=%s)", experimentName, createResult.ExperimentID)
	return createResult.ExperimentID, nil
}

func (a *MLflowAdapter) getExperimentByName(name string) (string, error) {
	resp, err := a.apiPost("/api/2.0/mlflow/experiments/search", map[string]any{
		"max_results": 100,
		"filter":      fmt.Sprintf("name = '%s'", name),
	})
	if err != nil {
		return "", err
	}

	var result struct {
		Experiments []struct {
			ExperimentID string `json:"experiment_id"`
		} `json:"experiments"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", err
	}
	if len(result.Experiments) > 0 {
		eid := result.Experiments[0].ExperimentID
		a.mu.Lock()
		a.experimentIDs[name] = eid
		a.mu.Unlock()
		return eid, nil
	}
	return "", fmt.Errorf("experiment %q not found", name)
}

func (a *MLflowAdapter) apiPost(path string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", a.trackingURI+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	if a.workspace != "" {
		req.Header.Set("X-MLflow-Workspace", a.workspace)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var respBody bytes.Buffer
	respBody.ReadFrom(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mlflow returned %d: %s", resp.StatusCode, respBody.String())
	}
	return respBody.Bytes(), nil
}

func (a *MLflowAdapter) TestConnectivity() error {
	_, err := a.apiPost("/api/2.0/mlflow/experiments/search", map[string]any{"max_results": 1})
	return err
}
