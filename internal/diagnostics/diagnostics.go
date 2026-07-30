package diagnostics

import (
	"fmt"
	"strings"
	"time"

	"github.com/konono/trimetry/internal/config"
	"github.com/konono/trimetry/internal/model"
	"github.com/konono/trimetry/internal/telemetry"
)

func Run(cfg *config.Config) {
	fmt.Println("=== Tracing Diagnostics ===")
	fmt.Println()

	fmt.Printf("Telemetry enabled:  %v\n", cfg.Telemetry.Enabled)
	fmt.Printf("Provider:           %s\n", cfg.Telemetry.Provider)

	if !cfg.Telemetry.Enabled {
		fmt.Println("\nTelemetry is disabled. No tracing diagnostics to run.")
		return
	}

	switch cfg.Telemetry.Provider {
	case "mlflow":
		diagnoseMlflow(cfg)
	default:
		diagnoseLangfuse(cfg)
	}
}

func newDiagnosticTrial() (string, telemetry.TrialContext, telemetry.TrialResult) {
	testTrialID := fmt.Sprintf("diag_%d", time.Now().UnixMilli())
	ctx := telemetry.TrialContext{
		BenchmarkRunID: "diag-run",
		TrialID:        testTrialID,
		ScenarioID:     "diagnostics",
		TrialNumber:    1,
		ModelName:      "diagnostic-model",
		ModelProvider:  "diagnostic",
	}
	result := telemetry.TrialResult{
		TrialID:         testTrialID,
		ExecutionStatus: model.ExecStatusCompleted,
		Metrics: &model.TrialMetrics{
			WallTimeMs:       100,
			TokenUsageSource: "unknown",
			CostSource:       "unknown",
		},
	}
	return testTrialID, ctx, result
}

func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func diagnoseMlflow(cfg *config.Config) {
	fmt.Printf("Tracking URI:       %s\n", cfg.Telemetry.TrackingURI)
	fmt.Printf("Token:              %s...\n", safePrefix(cfg.Telemetry.Token, 12))
	fmt.Printf("Workspace:          %s\n", cfg.Telemetry.Workspace)

	fmt.Println("\n--- Testing MLflow connectivity ---")
	ma := telemetry.NewMLflowAdapter(
		cfg.Telemetry.TrackingURI,
		cfg.Telemetry.Token,
		cfg.Telemetry.Workspace,
		cfg.Telemetry.TLSSkipVerify,
	)

	if err := ma.TestConnectivity(); err != nil {
		fmt.Printf("  ERROR: MLflow connectivity test failed: %v\n", err)
		return
	}
	fmt.Println("  MLflow connectivity: OK")

	testTrialID, trialCtx, trialResult := newDiagnosticTrial()
	ma.StartTrial(trialCtx)
	ma.FinishTrial(trialResult)

	fmt.Printf("  Diagnostic run sent with trialId: %s\n", testTrialID)

	fmt.Println("\n--- Summary ---")
	fmt.Println("  If no errors appeared above, MLflow connectivity is working.")
	fmt.Println("  Run a real benchmark to verify full integration.")
}

func diagnoseLangfuse(cfg *config.Config) {
	fmt.Printf("Base URL:           %s\n", cfg.Telemetry.BaseURL)
	fmt.Printf("Public Key:         %s...\n", safePrefix(cfg.Telemetry.PublicKey, 12))
	fmt.Printf("Secret Key:         %s...\n", safePrefix(cfg.Telemetry.SecretKey, 8))

	fmt.Println("\n--- Testing Langfuse connectivity ---")
	la := telemetry.NewLangfuseAdapter(
		cfg.Telemetry.BaseURL,
		cfg.Telemetry.PublicKey,
		cfg.Telemetry.SecretKey,
	)

	testTrialID, trialCtx, trialResult := newDiagnosticTrial()
	la.StartTrial(trialCtx)
	la.FinishTrial(trialResult)
	la.Flush()

	fmt.Printf("  Diagnostic trace sent with trialId: %s\n", testTrialID)

	fmt.Println("\n--- Checking for opencode traces ---")
	traces, err := la.FetchTracesBySession("diag-run")
	if err != nil {
		fmt.Printf("  ERROR: Failed to fetch traces: %v\n", err)
	} else {
		fmt.Printf("  Found %d trace(s) with sessionId=diag-run\n", len(traces))
		for _, t := range traces {
			fmt.Printf("    - Trace ID: %s, Name: %s\n", t.ID, t.Name)
			tagsStr := strings.Join(t.Tags, ", ")
			fmt.Printf("      Tags: [%s]\n", tagsStr)
		}
	}

	fmt.Println("\n--- Summary ---")
	fmt.Println("  If the diagnostic trace appeared above, Langfuse connectivity is working.")
	fmt.Println("  Run a real benchmark to verify opencode trace collection.")
}
