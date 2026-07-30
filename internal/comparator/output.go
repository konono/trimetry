package comparator

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/konono/trimetry/internal/fileutil"
)

func WriteJSON(path string, report *ComparisonReport) error {
	return fileutil.WriteJSON(path, report)
}

func WriteMarkdown(path string, report *ComparisonReport) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	fmt.Fprintf(w, "# Benchmark Comparison\n\n")
	fmt.Fprintf(w, "- **Baseline**: %s (%s)\n", report.Baseline, report.BaselineRunID)
	fmt.Fprintf(w, "- **Candidate**: %s (%s)\n\n", report.Candidate, report.CandidateRunID)

	if len(report.Warnings) > 0 {
		fmt.Fprintf(w, "### Warnings\n\n")
		for _, warn := range report.Warnings {
			fmt.Fprintf(w, "- %s\n", warn)
		}
		fmt.Fprintln(w)
	}

	for _, sc := range report.Comparisons {
		fmt.Fprintf(w, "## %s (baseline: %s vs candidate: %s)\n\n", sc.ScenarioID, sc.BaselineModel, sc.CandidateModel)

		fmt.Fprintf(w, "| Metric | Baseline | Candidate |\n")
		fmt.Fprintf(w, "|--------|----------|----------|\n")
		fmt.Fprintf(w, "| Trials | %d | %d |\n", sc.Baseline.TrialCount, sc.Candidate.TrialCount)
		fmt.Fprintf(w, "| Completed | %d | %d |\n", sc.Baseline.CompletedCount, sc.Candidate.CompletedCount)
		fmt.Fprintf(w, "| Failed | %d | %d |\n", sc.Baseline.FailedCount, sc.Candidate.FailedCount)
		fmt.Fprintf(w, "| Timeout | %d | %d |\n", sc.Baseline.TimeoutCount, sc.Candidate.TimeoutCount)
		if sc.Baseline.CancelledCount > 0 || sc.Candidate.CancelledCount > 0 {
			fmt.Fprintf(w, "| Cancelled | %d | %d |\n", sc.Baseline.CancelledCount, sc.Candidate.CancelledCount)
		}
		fmt.Fprintf(w, "| Failure Rate | %.1f%% | %.1f%% |\n", sc.Baseline.FailureRate*100, sc.Candidate.FailureRate*100)
		if sc.Baseline.AccuracyRate != nil || sc.Candidate.AccuracyRate != nil {
			ba, ca := 0.0, 0.0
			if sc.Baseline.AccuracyRate != nil { ba = *sc.Baseline.AccuracyRate * 100 }
			if sc.Candidate.AccuracyRate != nil { ca = *sc.Candidate.AccuracyRate * 100 }
			fmt.Fprintf(w, "| Accuracy Rate | %.1f%% | %.1f%% |\n", ba, ca)
		}
		fmt.Fprintln(w)

		if len(sc.Diffs) > 0 {
			fmt.Fprintf(w, "| Metric | Baseline Mean | Candidate Mean | Diff | Change%% | Baseline P50 | Candidate P50 |\n")
			fmt.Fprintf(w, "|--------|--------------|----------------|------|---------|-------------|---------------|\n")
			for _, d := range sc.Diffs {
				sign := "+"
				if d.AbsDiff < 0 {
					sign = ""
				}
				fmt.Fprintf(w, "| %s | %.2f | %.2f | %s%.2f | %+.1f%% | %.2f | %.2f |\n",
					d.Metric, d.BaselineMean, d.CandidateMean,
					sign, d.AbsDiff, d.ChangePercent,
					d.BaselineP50, d.CandidateP50)
			}
			fmt.Fprintln(w)
		}
	}

	return nil
}

func Print(report *ComparisonReport) {
	fmt.Printf("\n=== Benchmark Comparison ===\n")
	fmt.Printf("Baseline:  %s (%s)\n", report.Baseline, report.BaselineRunID)
	fmt.Printf("Candidate: %s (%s)\n\n", report.Candidate, report.CandidateRunID)

	for _, warn := range report.Warnings {
		fmt.Printf("WARNING: %s\n", warn)
	}
	if len(report.Warnings) > 0 {
		fmt.Println()
	}

	for _, sc := range report.Comparisons {
		fmt.Printf("--- %s (baseline: %s vs candidate: %s) ---\n", sc.ScenarioID, sc.BaselineModel, sc.CandidateModel)
		fmt.Printf("  Trials:  baseline=%d  candidate=%d\n",
			sc.Baseline.TrialCount, sc.Candidate.TrialCount)
		fmt.Printf("  Failure: baseline=%.1f%%  candidate=%.1f%%\n",
			sc.Baseline.FailureRate*100, sc.Candidate.FailureRate*100)
		if sc.Baseline.CancelledCount > 0 || sc.Candidate.CancelledCount > 0 {
			fmt.Printf("  Cancelled: baseline=%d  candidate=%d\n",
				sc.Baseline.CancelledCount, sc.Candidate.CancelledCount)
		}
		fmt.Println()

		if len(sc.Diffs) > 0 {
			fmt.Printf("  %-20s %12s %12s %12s %8s\n",
				"Metric", "Baseline", "Candidate", "Diff", "Change")
			fmt.Printf("  %s\n", strings.Repeat("-", 68))
			for _, d := range sc.Diffs {
				fmt.Printf("  %-20s %12.2f %12.2f %+12.2f %+7.1f%%\n",
					d.Metric, d.BaselineMean, d.CandidateMean, d.AbsDiff, d.ChangePercent)
			}
			fmt.Println()
		}
	}
}
