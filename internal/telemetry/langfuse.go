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

	// traceID cache: sessionID -> resolved trace info (from plugin's OTLP trace)
	traceCache map[string]resolvedTrace
	// trial context cache: trialID -> TrialContext (stored in StartTrial, used in FinishTrial)
	trialContexts map[string]TrialContext
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
		traceCache:     make(map[string]resolvedTrace),
		trialContexts:  make(map[string]TrialContext),
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

// StartTrial stores the trial context for use in FinishTrial.
// The opencode plugin creates the trace via OTLP; trimetry only annotates it.
func (a *LangfuseAdapter) StartTrial(ctx TrialContext) {
	a.mu.Lock()
	a.trialContexts[ctx.TrialID] = ctx
	a.mu.Unlock()
}

type resolvedTrace struct {
	TraceID      string
	RootSpanID   string // OTLP spanId of the root AGENT observation
}

func (a *LangfuseAdapter) FinishTrial(result TrialResult) {
	a.mu.Lock()
	tc := a.trialContexts[result.TrialID]
	delete(a.trialContexts, result.TrialID)
	a.mu.Unlock()

	var resolved resolvedTrace
	if result.AdapterName == "claude" || result.AdapterName == "cursor" {
		resolved = a.createDirectTrace(tc, result)
	} else {
		resolved = a.resolveTrace(result)
	}

	if resolved.TraceID != "" {
		a.annotateTrace(resolved, tc, result)
	}

	if resolved.TraceID == "" {
		log.Printf("WARNING: Langfuse: could not resolve traceId for trial %s (sessionID=%q), skipping trace annotation",
			result.TrialID, result.SessionID)
		return
	}

	for _, eval := range result.Evaluations {
		if eval.Score != nil {
			a.addScore(resolved.TraceID, eval)
		}
	}
}

// resolveTrace finds the Langfuse trace info for a trial.
// For adapters with a session ID (opencode), it queries the Langfuse API.
// For adapters without (fake, codex, etc.), there is no plugin trace to
// annotate, so it returns empty (scores/annotation are skipped).
func (a *LangfuseAdapter) resolveTrace(result TrialResult) resolvedTrace {
	if result.SessionID == "" {
		return resolvedTrace{}
	}

	a.mu.Lock()
	if cached, ok := a.traceCache[result.SessionID]; ok {
		a.mu.Unlock()
		return cached
	}
	a.mu.Unlock()

	// Retry lookup: the plugin's OTLP flush may not have been ingested yet.
	// Langfuse v4 writes to ClickHouse asynchronously, so it can take 10-30s.
	var resolved resolvedTrace
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			time.Sleep(5 * time.Second)
		}
		resolved = a.lookupTraceBySession(result.SessionID)
		if resolved.TraceID != "" {
			break
		}
	}
	if resolved.TraceID != "" {
		a.mu.Lock()
		a.traceCache[result.SessionID] = resolved
		a.mu.Unlock()
	}
	return resolved
}

// lookupTraceBySession queries the Langfuse v2 observations API to find
// the root AGENT observation for a given sessionId and extract its traceId
// and spanId (observation id).
func (a *LangfuseAdapter) lookupTraceBySession(sessionID string) resolvedTrace {
	// Use filter parameter to work around langfuse#15636 where sessionId
	// query parameter may be ignored in some versions.
	filter := fmt.Sprintf(`[{"type":"string","column":"sessionId","operator":"=","value":"%s"}]`, sessionID)
	u := fmt.Sprintf("%s/api/public/v2/observations?type=AGENT&limit=1&filter=%s",
		a.baseURL, url.QueryEscape(filter))

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		log.Printf("WARNING: Langfuse traceId lookup failed: %v", err)
		return resolvedTrace{}
	}
	req.SetBasicAuth(a.publicKey, a.secretKey)

	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("WARNING: Langfuse traceId lookup failed: %v", err)
		return resolvedTrace{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("WARNING: Langfuse traceId lookup returned %d: %s", resp.StatusCode, string(body))
		return resolvedTrace{}
	}

	var result struct {
		Data []struct {
			ID        string `json:"id"`
			TraceID   string `json:"traceId"`
			SessionID string `json:"sessionId"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("WARNING: Langfuse traceId lookup parse failed: %v", err)
		return resolvedTrace{}
	}
	if len(result.Data) == 0 {
		return resolvedTrace{}
	}
	obs := result.Data[0]
	if obs.SessionID != sessionID {
		log.Printf("WARNING: Langfuse: sessionId mismatch (want %s, got %s), skipping", sessionID, obs.SessionID)
		return resolvedTrace{}
	}
	return resolvedTrace{
		TraceID:    obs.TraceID,
		RootSpanID: obs.ID,
	}
}

// annotateTrace sends a minimal OTLP span with langfuse.trace.* attributes
// to annotate the plugin's trace at the trace level (name, tags, metadata,
// input, output). This avoids creating a visible sibling span.
func (a *LangfuseAdapter) annotateTrace(resolved resolvedTrace, tc TrialContext, result TrialResult) {
	traceName := fmt.Sprintf("%s_trial-%d", tc.ScenarioID, tc.TrialNumber)

	tags := []string{
		"benchmark",
		fmt.Sprintf("scenario:%s", tc.ScenarioID),
		fmt.Sprintf("model:%s", tc.ModelName),
	}
	if tc.BenchmarkName != "" {
		tags = append(tags, fmt.Sprintf("benchmark:%s", tc.BenchmarkName))
	}

	metadata := map[string]any{
		"benchmarkRunId":  tc.BenchmarkRunID,
		"scenarioId":      tc.ScenarioID,
		"scenarioVersion": tc.ScenarioVersion,
		"trialId":         result.TrialID,
		"trialNumber":     tc.TrialNumber,
		"modelName":       tc.ModelName,
		"modelProvider":   tc.ModelProvider,
		"adapterName":     result.AdapterName,
		"executionStatus": string(result.ExecutionStatus),
		"trimetryVersion": version.Version,
	}
	if len(tc.ModelParameters) > 0 {
		metadata["modelParameters"] = tc.ModelParameters
	}
	if result.Metrics != nil {
		metadata["wallTimeMs"] = result.Metrics.WallTimeMs
		metadata["retryCount"] = result.Metrics.RetryCount
		if result.Metrics.TotalTokens != nil {
			metadata["totalTokens"] = *result.Metrics.TotalTokens
		}
		if result.Metrics.EstimatedCost != nil {
			metadata["estimatedCost"] = *result.Metrics.EstimatedCost
		}
		if result.Metrics.AccuracyScore != nil {
			metadata["accuracyScore"] = *result.Metrics.AccuracyScore
		}
		if result.Metrics.LLMLatencyMs != nil {
			metadata["llmLatencyMs"] = *result.Metrics.LLMLatencyMs
		}
		if result.Metrics.TTFTMs != nil {
			metadata["ttftMs"] = *result.Metrics.TTFTMs
		}
	}

	_, enrichment := prepareEnrichedSteps(result)
	if enrichment.Environment != nil {
		metadata["hostName"] = enrichment.Environment.HostName
		metadata["hostArch"] = enrichment.Environment.HostArch
	}

	tagsJSON, _ := json.Marshal(tags)
	metadataJSON, _ := json.Marshal(metadata)
	inputJSON, _ := json.Marshal([]map[string]any{
		{"role": "user", "content": tc.Input},
	})
	outputJSON, _ := json.Marshal([]map[string]any{
		{"role": "assistant", "content": result.Output},
	})

	otelAttrs := []map[string]any{
		otelAttribute("langfuse.trace.name", traceName),
		otelAttribute("langfuse.trace.tags", string(tagsJSON)),
		otelAttribute("langfuse.trace.metadata", string(metadataJSON)),
		otelAttribute("langfuse.trace.input", string(inputJSON)),
		otelAttribute("langfuse.trace.output", string(outputJSON)),
		otelAttribute("langfuse.observation.input", string(inputJSON)),
		otelAttribute("langfuse.observation.output", string(outputJSON)),
		otelAttribute("langfuse.observation.metadata", string(metadataJSON)),
		otelAttribute("langfuse.internal.is_app_root", "false"),
	}

	now := time.Now()
	// Use a zero-duration span so it doesn't appear as a visible bar in the timeline
	nowNano := fmt.Sprintf("%d", now.UnixNano())

	payload := map[string]any{
		"resourceSpans": []map[string]any{
			{
				"resource": map[string]any{
					"attributes": []map[string]any{
						otelAttribute("service.name", "trimetry"),
					},
				},
				"scopeSpans": []map[string]any{
					{
						"scope": map[string]any{"name": "trimetry"},
						"spans": []map[string]any{
							a.buildAnnotationSpan(resolved, nowNano, otelAttrs),
						},
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("WARNING: Langfuse OTLP marshal failed: %v", err)
		return
	}

	req, err := http.NewRequest("POST", a.baseURL+"/api/public/otel/v1/traces", bytes.NewReader(body))
	if err != nil {
		log.Printf("WARNING: Langfuse OTLP request failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(a.publicKey, a.secretKey)

	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("WARNING: Langfuse OTLP send failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("WARNING: Langfuse OTLP returned %d: %s", resp.StatusCode, string(respBody))
	} else {
		log.Printf("  Langfuse: annotated trace %s for trial %s", resolved.TraceID, result.TrialID)
	}
}

// createDirectTrace builds the full OTLP trace hierarchy for adapters that
// don't have an external Langfuse plugin (e.g. Claude, Cursor). The structure
// mirrors the opencode Langfuse plugin: Turn (agent) → Generations → Tools.
func (a *LangfuseAdapter) createDirectTrace(tc TrialContext, result TrialResult) resolvedTrace {
	traceID := id.NewHexID(16)
	turnSpanID := id.NewHexID(8)
	adapterName := result.AdapterName

	traceName := fmt.Sprintf("%s_trial-%d", tc.ScenarioID, tc.TrialNumber)

	inputJSON, _ := json.Marshal([]map[string]any{
		{"role": "user", "content": tc.Input},
	})
	outputJSON, _ := json.Marshal([]map[string]any{
		{"role": "assistant", "content": result.Output},
	})

	startNano := fmt.Sprintf("%d", result.StartedAt.UnixNano())
	endNano := startNano
	if !result.StartedAt.IsZero() && result.Metrics != nil {
		endTime := result.StartedAt.Add(time.Duration(result.Metrics.WallTimeMs) * time.Millisecond)
		endNano = fmt.Sprintf("%d", endTime.UnixNano())
	}

	metadata := map[string]any{
		"benchmarkRunId":  tc.BenchmarkRunID,
		"scenarioId":      tc.ScenarioID,
		"scenarioVersion": tc.ScenarioVersion,
		"trialId":         result.TrialID,
		"trialNumber":     tc.TrialNumber,
		"modelName":       tc.ModelName,
		"modelProvider":   tc.ModelProvider,
		"adapterName":     adapterName,
		"executionStatus": string(result.ExecutionStatus),
		"trimetryVersion": version.Version,
	}
	metadataJSON, _ := json.Marshal(metadata)

	tags := []string{
		"benchmark",
		fmt.Sprintf("scenario:%s", tc.ScenarioID),
		fmt.Sprintf("model:%s", tc.ModelName),
	}
	if tc.BenchmarkName != "" {
		tags = append(tags, fmt.Sprintf("benchmark:%s", tc.BenchmarkName))
	}
	tagsJSON, _ := json.Marshal(tags)

	// Turn span (root)
	turnAttrs := []map[string]any{
		otelAttribute("langfuse.observation.type", "agent"),
		otelBoolAttribute("langfuse.internal.is_app_root", true),
		otelAttribute("langfuse.observation.input", string(inputJSON)),
		otelAttribute("langfuse.observation.output", string(outputJSON)),
		otelAttribute("langfuse.observation.metadata", string(metadataJSON)),
		otelAttribute("langfuse.trace.name", traceName),
		otelAttribute("langfuse.trace.tags", string(tagsJSON)),
	}
	if result.SessionID != "" {
		turnAttrs = append(turnAttrs, otelAttribute("session.id", result.SessionID))
	}

	spans := []map[string]any{
		{
			"traceId":           traceID,
			"spanId":            turnSpanID,
			"name":              adapterName + ".turn",
			"kind":              1,
			"startTimeUnixNano": startNano,
			"endTimeUnixNano":   endNano,
			"attributes":        turnAttrs,
			"status":            map[string]any{"code": 1},
		},
	}

	// User message event
	userSpanID := id.NewHexID(8)
	userInputJSON, _ := json.Marshal([]map[string]any{
		{"role": "user", "content": tc.Input},
	})
	spans = append(spans, map[string]any{
		"traceId":           traceID,
		"spanId":            userSpanID,
		"parentSpanId":      turnSpanID,
		"name":              adapterName + ".message.user",
		"kind":              1,
		"startTimeUnixNano": startNano,
		"endTimeUnixNano":   startNano,
		"attributes": []map[string]any{
			otelAttribute("langfuse.observation.type", "event"),
			otelAttribute("langfuse.observation.input", string(userInputJSON)),
		},
		"status": map[string]any{"code": 1},
	})

	// Generation + Tool spans from steps
	steps, _ := prepareEnrichedSteps(result)
	var currentGenSpanID string

	for _, step := range steps {
		spanID := id.NewHexID(8)

		switch step.Type {
		case "generation":
			currentGenSpanID = spanID
			genStartNano := fmt.Sprintf("%d", time.UnixMilli(step.StartMs).UnixNano())
			genEndNano := genStartNano
			if step.EndMs > 0 {
				genEndNano = fmt.Sprintf("%d", time.UnixMilli(step.EndMs).UnixNano())
			}

			genOutput := buildGenerationOutput(step)
			genOutputJSON, _ := json.Marshal(genOutput)

			modelName := tc.ModelName
			if step.Model != "" {
				modelName = step.Model
			}

			genMetaJSON, _ := json.Marshal(map[string]any{"agent": adapterName})
			genAttrs := []map[string]any{
				otelAttribute("langfuse.observation.type", "generation"),
				otelAttribute("langfuse.observation.model.name", modelName),
				otelAttribute("langfuse.observation.output", string(genOutputJSON)),
				otelAttribute("langfuse.observation.metadata", string(genMetaJSON)),
				otelBoolAttribute("langfuse.internal.is_app_root", false),
			}

			if step.Tokens != nil {
				usageDetails := map[string]int64{
					"input":       step.Tokens.Input,
					"output":      step.Tokens.Output,
					"reasoning":   step.Tokens.Reasoning,
					"cache_read":  step.Tokens.CacheRead,
					"cache_write": step.Tokens.CacheWrite,
					"total":       step.Tokens.Total,
				}
				usageJSON, _ := json.Marshal(usageDetails)
				genAttrs = append(genAttrs, otelAttribute("langfuse.observation.usage_details", string(usageJSON)))
			}

			if step.Input != nil {
				stepInputJSON, _ := json.Marshal(step.Input)
				genAttrs = append(genAttrs, otelAttribute("langfuse.observation.input", string(stepInputJSON)))
			}

			spans = append(spans, map[string]any{
				"traceId":           traceID,
				"spanId":            spanID,
				"parentSpanId":      turnSpanID,
				"name":              adapterName + ".generation",
				"kind":              1,
				"startTimeUnixNano": genStartNano,
				"endTimeUnixNano":   genEndNano,
				"attributes":        genAttrs,
				"status":            map[string]any{"code": 1},
			})

		case "tool":
			parentID := currentGenSpanID
			if parentID == "" {
				parentID = turnSpanID
			}

			toolStartNano := fmt.Sprintf("%d", time.UnixMilli(step.StartMs).UnixNano())
			toolEndNano := toolStartNano
			if step.EndMs > 0 {
				toolEndNano = fmt.Sprintf("%d", time.UnixMilli(step.EndMs).UnixNano())
			}

			toolInputJSON, _ := json.Marshal(step.Input)
			toolOutputJSON, _ := json.Marshal(map[string]any{
				"title":  step.Title,
				"output": step.Output,
			})

			toolMetaJSON, _ := json.Marshal(map[string]any{
				"callID": step.CallID,
				"tool":   step.Name,
			})
			toolAttrs := []map[string]any{
				otelAttribute("langfuse.observation.type", "span"),
				otelAttribute("langfuse.observation.input", string(toolInputJSON)),
				otelAttribute("langfuse.observation.output", string(toolOutputJSON)),
				otelAttribute("langfuse.observation.metadata", string(toolMetaJSON)),
				otelBoolAttribute("langfuse.internal.is_app_root", false),
			}

			spans = append(spans, map[string]any{
				"traceId":           traceID,
				"spanId":            spanID,
				"parentSpanId":      parentID,
				"name":              step.Name,
				"kind":              1,
				"startTimeUnixNano": toolStartNano,
				"endTimeUnixNano":   toolEndNano,
				"attributes":        toolAttrs,
				"status":            map[string]any{"code": 1},
			})
		}
	}

	payload := map[string]any{
		"resourceSpans": []map[string]any{
			{
				"resource": map[string]any{
					"attributes": []map[string]any{
						otelAttribute("service.name", "trimetry"),
					},
				},
				"scopeSpans": []map[string]any{
					{
						"scope": map[string]any{"name": "trimetry"},
						"spans": spans,
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("WARNING: Langfuse OTLP marshal failed: %v", err)
		return resolvedTrace{}
	}

	req, err := http.NewRequest("POST", a.baseURL+"/api/public/otel/v1/traces", bytes.NewReader(body))
	if err != nil {
		log.Printf("WARNING: Langfuse OTLP request failed: %v", err)
		return resolvedTrace{}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-langfuse-ingestion-version", "4")
	req.Header.Set("x-langfuse-public-key", a.publicKey)
	req.SetBasicAuth(a.publicKey, a.secretKey)

	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("WARNING: Langfuse OTLP send failed: %v", err)
		return resolvedTrace{}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("WARNING: Langfuse OTLP returned %d: %s", resp.StatusCode, string(respBody))
		return resolvedTrace{}
	}

	log.Printf("  Langfuse: created trace %s with %d spans for trial %s", traceID, len(spans), result.TrialID)
	return resolvedTrace{TraceID: traceID, RootSpanID: turnSpanID}
}

// buildGenerationOutput formats a generation step's output in the opencode
// plugin's assistant message format for Langfuse.
func buildGenerationOutput(step adapter.StepDetail) []map[string]any {
	msg := map[string]any{"role": "assistant"}

	if text, ok := step.Output.(string); ok && text != "" {
		msg["content"] = text
	}

	if len(step.ThinkingParts) > 0 {
		var thinking []map[string]any
		for _, t := range step.ThinkingParts {
			thinking = append(thinking, map[string]any{
				"type":    "thinking",
				"content": t,
			})
		}
		msg["thinking"] = thinking
	}

	if len(step.ToolsCalled) > 0 {
		var toolCalls []map[string]any
		for _, name := range step.ToolsCalled {
			toolCalls = append(toolCalls, map[string]any{
				"name": name,
			})
		}
		msg["tool_calls"] = toolCalls
	}

	return []map[string]any{msg}
}

func (a *LangfuseAdapter) buildAnnotationSpan(resolved resolvedTrace, nowNano string, attrs []map[string]any) map[string]any {
	span := map[string]any{
		"traceId":           resolved.TraceID,
		"spanId":            id.NewHexID(8),
		"name":              "trimetry.annotate",
		"kind":              1,
		"startTimeUnixNano": nowNano,
		"endTimeUnixNano":   nowNano,
		"attributes":        attrs,
		"status":            map[string]any{"code": 1},
	}
	if resolved.RootSpanID != "" {
		span["parentSpanId"] = resolved.RootSpanID
	}
	return span
}

func otelBoolAttribute(key string, value bool) map[string]any {
	return map[string]any{
		"key":   key,
		"value": map[string]any{"boolValue": value},
	}
}

func otelTagsAttribute(key string, tags []string) map[string]any {
	values := make([]map[string]any, len(tags))
	for i, t := range tags {
		values[i] = map[string]any{"stringValue": t}
	}
	return map[string]any{
		"key":   key,
		"value": map[string]any{"arrayValue": map[string]any{"values": values}},
	}
}

func otelAttribute(key string, value any) map[string]any {
	attr := map[string]any{"key": key}
	switch v := value.(type) {
	case string:
		attr["value"] = map[string]any{"stringValue": v}
	case int:
		attr["value"] = map[string]any{"intValue": fmt.Sprintf("%d", v)}
	case int64:
		attr["value"] = map[string]any{"intValue": fmt.Sprintf("%d", v)}
	case float64:
		attr["value"] = map[string]any{"doubleValue": v}
	default:
		attr["value"] = map[string]any{"stringValue": fmt.Sprintf("%v", v)}
	}
	return attr
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
