package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/konono/trimetry/internal/adapter"
	"github.com/konono/trimetry/internal/id"
	mdl "github.com/konono/trimetry/internal/model"
)

type LangfuseAdapter struct {
	baseURL   string
	publicKey string
	secretKey string
	client    *http.Client

	mu           sync.Mutex
	batch        []ingestionEvent
	scoreConfigs map[string]string // name -> configId
}

func NewLangfuseAdapter(baseURL, publicKey, secretKey string) *LangfuseAdapter {
	a := &LangfuseAdapter{
		baseURL:      baseURL,
		publicKey:    publicKey,
		secretKey:    secretKey,
		scoreConfigs: make(map[string]string),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	a.loadScoreConfigs()
	return a
}

type ingestionEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Body      any       `json:"body"`
}

type traceCreateBody struct {
	ID        string         `json:"id"`
	Name      string         `json:"name,omitempty"`
	SessionID string         `json:"sessionId,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	Timestamp time.Time      `json:"timestamp,omitempty"`
	Input     any            `json:"input,omitempty"`
	Output    any            `json:"output,omitempty"`
}

type generationCreateBody struct {
	ID              string         `json:"id"`
	TraceID         string         `json:"traceId"`
	Name            string         `json:"name"`
	Model           string         `json:"model,omitempty"`
	ModelParameters map[string]any `json:"modelParameters,omitempty"`
	StartTime       time.Time      `json:"startTime"`
	EndTime         *time.Time     `json:"endTime,omitempty"`
	Input           any            `json:"input,omitempty"`
	Output          any            `json:"output,omitempty"`
	Usage           *usageBody     `json:"usage,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type spanCreateBody struct {
	ID        string         `json:"id"`
	TraceID   string         `json:"traceId"`
	Name      string         `json:"name"`
	StartTime time.Time      `json:"startTime"`
	EndTime   *time.Time     `json:"endTime,omitempty"`
	Input     any            `json:"input,omitempty"`
	Output    any            `json:"output,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type usageBody struct {
	Input  *int64 `json:"input,omitempty"`
	Output *int64 `json:"output,omitempty"`
	Total  *int64 `json:"total,omitempty"`
	Unit   string `json:"unit,omitempty"`
}

type scoreCreateBody struct {
	ID            string  `json:"id"`
	TraceID       string  `json:"traceId"`
	Name          string  `json:"name"`
	Value         float64 `json:"value"`
	Comment       string  `json:"comment,omitempty"`
	DataType      string  `json:"dataType,omitempty"`
	ConfigID      string  `json:"configId,omitempty"`
	ObservationID string  `json:"observationId,omitempty"`
}

func (a *LangfuseAdapter) StartTrial(ctx TrialContext) {
	metadata := map[string]any{
		"benchmarkRunId":  ctx.BenchmarkRunID,
		"scenarioId":      ctx.ScenarioID,
		"scenarioVersion": ctx.ScenarioVersion,
		"trialId":         ctx.TrialID,
		"trialNumber":     ctx.TrialNumber,
		"modelName":       ctx.ModelName,
		"modelProvider":   ctx.ModelProvider,
	}
	if len(ctx.ModelParameters) > 0 {
		metadata["modelParameters"] = ctx.ModelParameters
	}

	tags := []string{
		"benchmark",
		fmt.Sprintf("scenario:%s", ctx.ScenarioID),
		fmt.Sprintf("model:%s", ctx.ModelName),
	}

	event := ingestionEvent{
		ID:        id.NewEventID(),
		Type:      "trace-create",
		Timestamp: time.Now(),
		Body: traceCreateBody{
			ID:        ctx.TrialID,
			Name:      fmt.Sprintf("%s_trial-%d", ctx.ScenarioID, ctx.TrialNumber),
			SessionID: ctx.BenchmarkRunID,
			Metadata:  metadata,
			Tags:      tags,
			Timestamp: time.Now(),
			Input:     ctx.Input,
		},
	}

	a.addEvent(event)
}

func (a *LangfuseAdapter) FinishTrial(result TrialResult) {
	output := buildTrialOutput(result)

	metadata := map[string]any{
		"executionStatus": result.ExecutionStatus,
	}
	if result.Metrics != nil {
		metadata["wallTimeMs"] = result.Metrics.WallTimeMs
		metadata["retryCount"] = result.Metrics.RetryCount
		metadata["tokenUsageSource"] = result.Metrics.TokenUsageSource
		metadata["costSource"] = result.Metrics.CostSource
		if result.Metrics.LLMLatencyMs != nil {
			metadata["llmLatencyMs"] = *result.Metrics.LLMLatencyMs
		}
		if result.Metrics.IdleMs != nil {
			metadata["idleMs"] = *result.Metrics.IdleMs
		}
	}

	steps, enrichment := prepareEnrichedSteps(result)

	if enrichment.Environment != nil {
		metadata["hostName"] = enrichment.Environment.HostName
		metadata["hostArch"] = enrichment.Environment.HostArch
		if len(enrichment.Environment.AISettings) > 0 {
			metadata["aiSettings"] = enrichment.Environment.AISettings
		}
	}

	a.addEvent(ingestionEvent{
		ID:        id.NewEventID(),
		Type:      "trace-create",
		Timestamp: time.Now(),
		Body: traceCreateBody{
			ID:       result.TrialID,
			Output:   output,
			Metadata: metadata,
		},
	})
	for i, step := range steps {
		switch step.Type {
		case adapter.StepTypeGeneration:
			a.addGenerationObservation(result.TrialID, i, step)
		case adapter.StepTypeTool:
			a.addToolObservation(result.TrialID, i, step)
		}
	}

	for _, eval := range result.Evaluations {
		if eval.Score != nil {
			a.addScore(result.TrialID, eval)
		}
	}
}

func (a *LangfuseAdapter) addGenerationObservation(traceID string, idx int, step adapter.StepDetail) {
	startTime := time.UnixMilli(step.StartMs)
	endTime := time.UnixMilli(step.EndMs)

	meta := map[string]any{
		"stepIndex":      idx,
		"status":         step.Status,
		"reason":         step.Reason,
		"durationMs":     step.DurationMs,
		"llmInferenceMs": step.LLMInferenceMs,
		"toolTimeMs":     step.ToolTimeMs,
	}
	if len(step.ToolsCalled) > 0 {
		meta["toolsCalled"] = step.ToolsCalled
	}
	if step.TTFTMs != nil {
		meta["ttftMs"] = *step.TTFTMs
	}
	if step.Tokens != nil {
		meta["reasoningTokens"] = step.Tokens.Reasoning
		meta["cacheReadTokens"] = step.Tokens.CacheRead
		meta["cacheWriteTokens"] = step.Tokens.CacheWrite
	}
	for k, v := range step.Metadata {
		meta[k] = v
	}

	body := generationCreateBody{
		ID:        id.NewEventID(),
		TraceID:   traceID,
		Name:      step.Name,
		StartTime: startTime,
		EndTime:   &endTime,
		Input:     step.Input,
		Output:    step.Output,
		Metadata:  meta,
	}

	if step.Model != "" {
		body.Model = step.Model
	}
	if step.Tokens != nil {
		body.Usage = &usageBody{
			Input:  &step.Tokens.Input,
			Output: &step.Tokens.Output,
			Total:  &step.Tokens.Total,
			Unit:   "TOKENS",
		}
	}

	a.addEvent(ingestionEvent{
		ID:        id.NewEventID(),
		Type:      "generation-create",
		Timestamp: time.Now(),
		Body:      body,
	})
}

func (a *LangfuseAdapter) addToolObservation(traceID string, idx int, step adapter.StepDetail) {
	startTime := time.UnixMilli(step.StartMs)
	endTime := time.UnixMilli(step.EndMs)

	meta := map[string]any{
		"stepIndex":  idx,
		"status":     step.Status,
		"durationMs": step.DurationMs,
	}
	if step.CallID != "" {
		meta["callID"] = step.CallID
	}
	if step.Title != "" {
		meta["title"] = step.Title
	}
	if step.Display != nil {
		meta["display"] = step.Display
	}
	for k, v := range step.Metadata {
		if k == "display" {
			continue
		}
		meta[k] = v
	}

	body := spanCreateBody{
		ID:        id.NewEventID(),
		TraceID:   traceID,
		Name:      step.Name,
		StartTime: startTime,
		EndTime:   &endTime,
		Input:     step.Input,
		Output:    step.Output,
		Metadata:  meta,
	}

	a.addEvent(ingestionEvent{
		ID:        id.NewEventID(),
		Type:      "span-create",
		Timestamp: time.Now(),
		Body:      body,
	})
}

func (a *LangfuseAdapter) addScore(traceID string, eval mdl.EvaluationResult) {
	score := 0.0
	if eval.Score != nil {
		score = *eval.Score
	}

	configID := a.ensureScoreConfig(eval.EvaluatorName)

	body := scoreCreateBody{
		ID:       id.NewEventID(),
		TraceID:  traceID,
		Name:     eval.EvaluatorName,
		Value:    score,
		Comment:  eval.Reason,
		DataType: "NUMERIC",
		ConfigID: configID,
	}

	a.addEvent(ingestionEvent{
		ID:        id.NewEventID(),
		Type:      "score-create",
		Timestamp: time.Now(),
		Body:      body,
	})
}

func (a *LangfuseAdapter) loadScoreConfigs() {
	u := fmt.Sprintf("%s/api/public/score-configs?limit=50", a.baseURL)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return
	}
	req.SetBasicAuth(a.publicKey, a.secretKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return
	}

	var result struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}

	for _, cfg := range result.Data {
		a.scoreConfigs[cfg.Name] = cfg.ID
	}
}

func (a *LangfuseAdapter) ensureScoreConfig(name string) string {
	a.mu.Lock()
	if cid, ok := a.scoreConfigs[name]; ok {
		a.mu.Unlock()
		return cid
	}
	a.mu.Unlock()

	body, _ := json.Marshal(map[string]any{
		"name":        name,
		"dataType":    "NUMERIC",
		"minValue":    0,
		"maxValue":    1,
		"description": fmt.Sprintf("Benchmark evaluator: %s", name),
	})

	req, err := http.NewRequest("POST", a.baseURL+"/api/public/score-configs", bytes.NewReader(body))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(a.publicKey, a.secretKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	a.mu.Lock()
	a.scoreConfigs[name] = result.ID
	a.mu.Unlock()
	return result.ID
}

func (a *LangfuseAdapter) addEvent(event ingestionEvent) {
	a.mu.Lock()
	a.batch = append(a.batch, event)
	a.mu.Unlock()
}

func (a *LangfuseAdapter) Flush() {
	a.mu.Lock()
	events := make([]ingestionEvent, len(a.batch))
	copy(events, a.batch)
	a.batch = a.batch[:0]
	a.mu.Unlock()

	if len(events) == 0 {
		return
	}

	if err := a.sendBatch(events); err != nil {
		log.Printf("WARNING: Langfuse flush failed: %v", err)
	}
}

func (a *LangfuseAdapter) sendBatch(events []ingestionEvent) error {
	payload := map[string]any{
		"batch": events,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	req, err := http.NewRequest("POST", a.baseURL+"/api/public/ingestion", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(a.publicKey, a.secretKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var respBody bytes.Buffer
		respBody.ReadFrom(resp.Body)
		return fmt.Errorf("langfuse returned %d: %s", resp.StatusCode, respBody.String())
	}

	log.Printf("  Langfuse: sent %d events", len(events))
	return nil
}

func (a *LangfuseAdapter) FetchTracesBySession(sessionID string) ([]LangfuseTrace, error) {
	u := fmt.Sprintf("%s/api/public/traces?sessionId=%s", a.baseURL, url.QueryEscape(sessionID))
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(a.publicKey, a.secretKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("traces API returned %d", resp.StatusCode)
	}

	var result struct {
		Data []LangfuseTrace `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

type LangfuseTrace struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	SessionID string         `json:"sessionId"`
	Metadata  map[string]any `json:"metadata"`
	Tags      []string       `json:"tags"`
	Timestamp string         `json:"timestamp"`
}



