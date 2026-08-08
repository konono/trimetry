package telemetry

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/konono/trimetry/internal/adapter"
	"github.com/konono/trimetry/internal/id"
	mdl "github.com/konono/trimetry/internal/model"
	"github.com/konono/trimetry/internal/version"
)

type LangfuseAdapter struct {
	baseURL   string
	publicKey string
	secretKey string
	client    *http.Client

	mu           sync.Mutex
	batch        []ingestionEvent
	scoreConfigs map[string]string // name -> configId

	batchChunkSize int
	maxRetries     int
	maxBatchQueue  int
}

type LangfuseOptions struct {
	BaseURL        string
	PublicKey      string
	SecretKey      string
	TLSSkipVerify  bool
	BatchChunkSize int
	MaxRetries     int
	MaxBatchQueue  int
}

func NewLangfuseAdapter(opts LangfuseOptions) *LangfuseAdapter {
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	if opts.TLSSkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	batchChunkSize := opts.BatchChunkSize
	if batchChunkSize <= 0 {
		batchChunkSize = 50
	}
	maxRetries := opts.MaxRetries
	if maxRetries < 0 {
		maxRetries = 3
	}
	maxBatchQueue := opts.MaxBatchQueue
	if maxBatchQueue <= 0 {
		maxBatchQueue = 10000
	}
	a := &LangfuseAdapter{
		baseURL:        baseURL,
		publicKey:      opts.PublicKey,
		secretKey:      opts.SecretKey,
		scoreConfigs:   make(map[string]string),
		client:         client,
		batchChunkSize: batchChunkSize,
		maxRetries:     maxRetries,
		maxBatchQueue:  maxBatchQueue,
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
	Release   string         `json:"release,omitempty"`
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
	Level           string         `json:"level,omitempty"`
	StatusMessage   string         `json:"statusMessage,omitempty"`
}

type spanCreateBody struct {
	ID            string         `json:"id"`
	TraceID       string         `json:"traceId"`
	Name          string         `json:"name"`
	StartTime     time.Time      `json:"startTime"`
	EndTime       *time.Time     `json:"endTime,omitempty"`
	Input         any            `json:"input,omitempty"`
	Output        any            `json:"output,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	Level         string         `json:"level,omitempty"`
	StatusMessage string         `json:"statusMessage,omitempty"`
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
	if ctx.BenchmarkName != "" {
		metadata["benchmarkName"] = ctx.BenchmarkName
	}

	tags := []string{
		"benchmark",
		fmt.Sprintf("scenario:%s", ctx.ScenarioID),
		fmt.Sprintf("model:%s", ctx.ModelName),
	}
	if ctx.BenchmarkName != "" {
		tags = append(tags, fmt.Sprintf("benchmark:%s", ctx.BenchmarkName))
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
			Release:   version.Version,
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
		if result.Metrics.EstimatedCost != nil {
			metadata["estimatedCost"] = *result.Metrics.EstimatedCost
		}
		if result.Metrics.AccuracyScore != nil {
			metadata["accuracyScore"] = *result.Metrics.AccuracyScore
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
	startTime, endTime, hasEnd := stepTimeRange(step.StartMs, step.EndMs)
	var endTimePtr *time.Time
	if hasEnd {
		endTimePtr = &endTime
	}

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
		EndTime:   endTimePtr,
		Input:     step.Input,
		Output:    step.Output,
		Metadata:  meta,
	}

	level, isError := langfuseLevel(step.Status)
	body.Level = level
	if isError && step.Reason != "" {
		body.StatusMessage = step.Reason
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
	startTime, endTime, hasEnd := stepTimeRange(step.StartMs, step.EndMs)
	var endTimePtr *time.Time
	if hasEnd {
		endTimePtr = &endTime
	}

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
		EndTime:   endTimePtr,
		Input:     step.Input,
		Output:    step.Output,
		Metadata:  meta,
	}

	level, isError := langfuseLevel(step.Status)
	body.Level = level
	if isError {
		body.StatusMessage = fmt.Sprintf("tool %s failed", step.Name)
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

	failedEvents := a.sendWithRetry(events)

	if len(failedEvents) > 0 {
		a.mu.Lock()
		capacity := a.maxBatchQueue - len(a.batch)
		if capacity < 0 {
			capacity = 0
		}
		if len(failedEvents) > capacity {
			log.Printf("WARNING: Langfuse dropping %d events to prevent unbounded queue growth (max %d)",
				len(failedEvents)-capacity, a.maxBatchQueue)
			failedEvents = failedEvents[:capacity]
		}
		if len(failedEvents) > 0 {
			a.batch = append(failedEvents, a.batch...)
			log.Printf("WARNING: Langfuse re-queued %d failed events for next flush", len(failedEvents))
		}
		a.mu.Unlock()
	}
}

type ingestionResponse struct {
	Successes []ingestionResultItem `json:"successes"`
	Errors    []ingestionErrorItem  `json:"errors"`
}

type ingestionResultItem struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
}

type ingestionErrorItem struct {
	ID      string `json:"id"`
	Status  int    `json:"status"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type sendError struct {
	statusCode int
	message    string
}

func (e *sendError) Error() string {
	return fmt.Sprintf("langfuse returned %d: %s", e.statusCode, e.message)
}

func isRetryableStatus(statusCode int) bool {
	return statusCode >= 500 || statusCode == 429
}

func isRetryable(err error) bool {
	var se *sendError
	if errors.As(err, &se) {
		return isRetryableStatus(se.statusCode)
	}
	return true
}

func splitBatch(events []ingestionEvent, chunkSize int) [][]ingestionEvent {
	if chunkSize <= 0 {
		chunkSize = 50
	}
	var chunks [][]ingestionEvent
	for i := 0; i < len(events); i += chunkSize {
		end := i + chunkSize
		if end > len(events) {
			end = len(events)
		}
		chunks = append(chunks, events[i:end])
	}
	return chunks
}

func backoffDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return time.Second
	}
	base := time.Second
	shift := uint(attempt - 1)
	if shift > 4 {
		shift = 4
	}
	delay := base * time.Duration(1<<shift)
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	jitter := time.Duration(rand.Int64N(int64(delay) / 5))
	return delay + jitter
}

func (a *LangfuseAdapter) sendWithRetry(events []ingestionEvent) []ingestionEvent {
	chunks := splitBatch(events, a.batchChunkSize)
	var allFailed []ingestionEvent

	for _, chunk := range chunks {
		failed := a.sendChunkWithRetry(chunk)
		allFailed = append(allFailed, failed...)
	}
	return allFailed
}

func (a *LangfuseAdapter) sendChunkWithRetry(events []ingestionEvent) []ingestionEvent {
	remaining := events
	for attempt := 0; attempt <= a.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoffDelay(attempt))
		}
		result, err := a.sendBatchParsed(remaining)
		if err != nil {
			if !isRetryable(err) {
				log.Printf("WARNING: Langfuse non-retryable error, dropping %d events: %v", len(remaining), err)
				return nil
			}
			log.Printf("WARNING: Langfuse send attempt %d/%d failed: %v", attempt+1, a.maxRetries+1, err)
			continue
		}
		remaining = extractRetryableFailures(remaining, result)
		if len(remaining) == 0 {
			return nil
		}
		log.Printf("WARNING: Langfuse %d/%d events failed (attempt %d/%d)",
			len(remaining), len(events), attempt+1, a.maxRetries+1)
	}
	return remaining
}

func extractRetryableFailures(sent []ingestionEvent, result *ingestionResponse) []ingestionEvent {
	if result == nil || len(result.Errors) == 0 {
		return nil
	}

	failedIDs := make(map[string]bool)
	for _, e := range result.Errors {
		if isRetryableStatus(e.Status) {
			failedIDs[e.ID] = true
		} else {
			msg := e.Message
			if msg == "" {
				msg = e.Error
			}
			log.Printf("WARNING: Langfuse event %s permanently failed (status %d): %s", e.ID, e.Status, msg)
		}
	}

	if len(failedIDs) == 0 {
		return nil
	}

	var retryable []ingestionEvent
	for _, ev := range sent {
		if failedIDs[ev.ID] {
			retryable = append(retryable, ev)
		}
	}
	return retryable
}

func (a *LangfuseAdapter) sendBatchParsed(events []ingestionEvent) (*ingestionResponse, error) {
	payload := map[string]any{
		"batch": events,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal batch: %w", err)
	}

	req, err := http.NewRequest("POST", a.baseURL+"/api/public/ingestion", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(a.publicKey, a.secretKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if readErr != nil {
			log.Printf("WARNING: Langfuse response body read failed (status %d): %v", resp.StatusCode, readErr)
			return &ingestionResponse{}, nil
		}
		var result ingestionResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			log.Printf("WARNING: Langfuse response parse failed (status %d), assuming success for %d events: %v",
				resp.StatusCode, len(events), err)
			return &ingestionResponse{}, nil
		}
		succeeded := len(result.Successes)
		failed := len(result.Errors)
		if failed > 0 {
			log.Printf("  Langfuse: sent %d events (%d succeeded, %d failed)", len(events), succeeded, failed)
		} else {
			log.Printf("  Langfuse: sent %d events", len(events))
		}
		return &result, nil
	}

	return nil, &sendError{
		statusCode: resp.StatusCode,
		message:    string(respBody),
	}
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



