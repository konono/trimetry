package ui

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/konono/trimetry/internal/model"
)

const barWidth = 16

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
)

type ComboProgress struct {
	Label      string
	ModelName  string
	ScenarioID string
	Total      int
	Completed  int
	Failed     int
	Timeout    int
	Cancelled  int
}

func (c *ComboProgress) done() int {
	return c.Completed + c.Failed + c.Timeout + c.Cancelled
}

type Display struct {
	w            *bufio.Writer
	combos       []ComboProgress
	comboIdx     map[string]int
	scenarioName map[string]string
	total        int
	completed    int
	maxLabel     int
	startTime    time.Time
	isTTY        bool
	verbose      bool
	drawn        bool
	mu           sync.Mutex
}

func (d *Display) c(code string) string {
	if !d.isTTY {
		return ""
	}
	return code
}

func (d *Display) flush() {
	d.w.Flush()
}

func (d *Display) statusIcon(status model.ExecutionStatus) string {
	switch status {
	case model.ExecStatusFailed:
		return d.c(ansiRed) + "✗" + d.c(ansiReset)
	case model.ExecStatusTimeout:
		return d.c(ansiYellow) + "⏱" + d.c(ansiReset)
	case model.ExecStatusCancelled:
		return d.c(ansiDim) + "⊘" + d.c(ansiReset)
	default:
		return d.c(ansiGreen) + "✓" + d.c(ansiReset)
	}
}

func NewDisplay(run *model.BenchmarkRun, scenarios []model.Scenario, models []model.ModelConfig, trialsPerCombo, concurrency int, w io.Writer, verbose bool) *Display {
	scenarioNames := make(map[string]string)
	for _, s := range scenarios {
		name := s.Name
		if name == "" {
			name = s.ScenarioID
		}
		scenarioNames[s.ScenarioID] = name
	}

	var combos []ComboProgress
	idx := make(map[string]int)
	maxLabel := 0
	for _, m := range models {
		for _, s := range scenarios {
			key := model.ScenarioModelKey(s.ScenarioID, m.Name)
			label := m.Name + " × " + scenarioNames[s.ScenarioID]
			idx[key] = len(combos)
			combos = append(combos, ComboProgress{
				Label:      label,
				ModelName:  m.Name,
				ScenarioID: s.ScenarioID,
				Total:      trialsPerCombo,
			})
			if dw := displayWidth(label); dw > maxLabel {
				maxLabel = dw
			}
		}
	}

	total := len(combos) * trialsPerCombo

	d := &Display{
		w:            bufio.NewWriter(w),
		combos:       combos,
		comboIdx:     idx,
		scenarioName: scenarioNames,
		total:        total,
		maxLabel:     maxLabel,
		startTime:    run.StartedAt,
		isTTY:        isTerminal(w),
		verbose:      verbose,
	}

	d.printHeader(run, len(scenarios), len(models), trialsPerCombo, concurrency)

	if d.isTTY {
		d.drawProgress()
		d.drawn = true
	}

	d.flush()

	return d
}

// The mutex is held during I/O intentionally: concurrent ANSI escape sequences
// would corrupt the terminal. The buffered writer minimizes the critical section
// by batching output into a single write syscall.
func (d *Display) OnTrialComplete(trial model.Trial) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := model.ScenarioModelKey(trial.ScenarioID, trial.ModelName)
	if i, ok := d.comboIdx[key]; ok {
		switch trial.ExecutionStatus {
		case model.ExecStatusCompleted:
			d.combos[i].Completed++
		case model.ExecStatusFailed:
			d.combos[i].Failed++
		case model.ExecStatusTimeout:
			d.combos[i].Timeout++
		case model.ExecStatusCancelled:
			d.combos[i].Cancelled++
		}
	}
	d.completed++

	if d.isTTY {
		d.eraseProgress()
		if d.verbose {
			d.printTrialLineTTY(trial)
		}
		d.drawProgress()
		d.drawn = true
	} else {
		d.printTrialLine(trial)
	}

	d.flush()
}

func (d *Display) OnRetry(trialID string, attempt, maxRetries int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isTTY {
		d.eraseProgress()
		fmt.Fprintf(d.w, "  %s↻ retry %d/%d for %s%s\n",
			d.c(ansiYellow), attempt, maxRetries, trialID, d.c(ansiReset))
		d.drawProgress()
		d.drawn = true
	} else {
		fmt.Fprintf(d.w, "[trimetry] ↻ retry %d/%d for %s\n",
			attempt, maxRetries, trialID)
	}

	d.flush()
}

// Lock is held during I/O for the same reason as OnTrialComplete.
func (d *Display) Finalize(run *model.BenchmarkRun, summaries []model.ScenarioSummary, outputDir string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isTTY {
		d.finalizePlain(run, summaries, outputDir)
		d.flush()
		return
	}

	d.eraseProgress()
	d.drawProgress()
	fmt.Fprintln(d.w)

	fmt.Fprintln(d.w, d.c(ansiCyan)+"╭──────────────────────────────────────────────────────────────╮"+d.c(ansiReset))
	fmt.Fprintln(d.w, d.c(ansiCyan)+"│  "+d.c(ansiBold)+"Results"+d.c(ansiReset)+d.c(ansiCyan)+strings.Repeat(" ", 53)+"│"+d.c(ansiReset))
	fmt.Fprintln(d.w, d.c(ansiCyan)+"╰──────────────────────────────────────────────────────────────╯"+d.c(ansiReset))

	for _, s := range summaries {
		d.printScenarioSummary(s)
	}

	fmt.Fprintln(d.w, d.c(ansiDim)+"  ─────────────────────────────────────────────────────────────"+d.c(ansiReset))
	fmt.Fprintln(d.w)
	if run.FinishedAt != nil {
		fmt.Fprintf(d.w, "  Duration: %s\n", formatDuration(run.FinishedAt.Sub(run.StartedAt)))
	}
	fmt.Fprintf(d.w, "  Reports:  %s/%s/\n", outputDir, run.BenchmarkRunID)

	d.flush()
}

func (d *Display) finalizePlain(run *model.BenchmarkRun, summaries []model.ScenarioSummary, outputDir string) {
	elapsed := ""
	if run.FinishedAt != nil {
		elapsed = formatDuration(run.FinishedAt.Sub(run.StartedAt))
	}
	passed := 0
	failed := 0
	timeout := 0
	cancelled := 0
	for _, c := range d.combos {
		passed += c.Completed
		failed += c.Failed
		timeout += c.Timeout
		cancelled += c.Cancelled
	}
	line := fmt.Sprintf("[trimetry] completed %d/%d in %s (%d passed, %d failed",
		d.completed, d.total, elapsed, passed, failed)
	if timeout > 0 {
		line += fmt.Sprintf(", %d timeout", timeout)
	}
	if cancelled > 0 {
		line += fmt.Sprintf(", %d cancelled", cancelled)
	}
	line += ")"
	fmt.Fprintln(d.w, line)

	for _, s := range summaries {
		name := s.ScenarioID
		if s.ScenarioName != "" {
			name = s.ScenarioName
		}
		line := fmt.Sprintf("[trimetry]   %s × %s: %d/%d completed",
			s.ModelName, name, s.CompletedCount, s.TrialCount)
		if s.FailedCount > 0 {
			line += fmt.Sprintf(", %d failed", s.FailedCount)
		}
		if s.TimeoutCount > 0 {
			line += fmt.Sprintf(", %d timeout", s.TimeoutCount)
		}
		if s.CancelledCount > 0 {
			line += fmt.Sprintf(", %d cancelled", s.CancelledCount)
		}
		if s.AccuracyRate != nil {
			line += fmt.Sprintf(", accuracy %.1f%%", *s.AccuracyRate*100)
		}
		if s.Metrics != nil && s.Metrics.WallTimeMs != nil {
			line += fmt.Sprintf(", avg %s", formatDuration(time.Duration(s.Metrics.WallTimeMs.Mean*float64(time.Millisecond))))
		}
		fmt.Fprintln(d.w, line)
	}
}

func (d *Display) printScenarioSummary(s model.ScenarioSummary) {
	name := s.ScenarioID
	if s.ScenarioName != "" {
		name = s.ScenarioName
	}
	label := s.ModelName + " × " + name

	fmt.Fprintln(d.w)
	fmt.Fprintf(d.w, "  %s%s%s\n", d.c(ansiBold), label, d.c(ansiReset))

	statusParts := []string{}
	statusParts = append(statusParts, fmt.Sprintf("%s✓ %d/%d completed%s", d.c(ansiGreen), s.CompletedCount, s.TrialCount, d.c(ansiReset)))
	if s.FailedCount > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%s✗ %d failed%s", d.c(ansiRed), s.FailedCount, d.c(ansiReset)))
	}
	if s.TimeoutCount > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%s⏱ %d timeout%s", d.c(ansiYellow), s.TimeoutCount, d.c(ansiReset)))
	}
	if s.CancelledCount > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%s⊘ %d cancelled%s", d.c(ansiDim), s.CancelledCount, d.c(ansiReset)))
	}
	if s.AccuracyRate != nil {
		statusParts = append(statusParts, fmt.Sprintf("Accuracy: %.1f%%", *s.AccuracyRate*100))
	}
	fmt.Fprintf(d.w, "    %s\n", strings.Join(statusParts, "   "))

	m := s.Metrics
	if m == nil {
		return
	}

	if m.WallTimeMs != nil {
		w := m.WallTimeMs
		fmt.Fprintf(d.w, "    %sLatency%s   mean %s   median %s   p90 %s   min %s   max %s   σ %s\n",
			d.c(ansiDim), d.c(ansiReset),
			formatDuration(time.Duration(w.Mean*float64(time.Millisecond))),
			formatDuration(time.Duration(w.Median*float64(time.Millisecond))),
			formatDuration(time.Duration(w.P90*float64(time.Millisecond))),
			formatDuration(time.Duration(w.Min*float64(time.Millisecond))),
			formatDuration(time.Duration(w.Max*float64(time.Millisecond))),
			formatMs(w.StdDev),
		)
	}

	tokenParts := []string{}
	if m.InputTokens != nil {
		tokenParts = append(tokenParts, fmt.Sprintf("in %s", formatNumber(int64(math.Round(m.InputTokens.Mean)))))
	}
	if m.OutputTokens != nil {
		tokenParts = append(tokenParts, fmt.Sprintf("out %s", formatNumber(int64(math.Round(m.OutputTokens.Mean)))))
	}
	if m.ReasoningTokens != nil && m.ReasoningTokens.Mean > 0 {
		tokenParts = append(tokenParts, fmt.Sprintf("reasoning %s", formatNumber(int64(math.Round(m.ReasoningTokens.Mean)))))
	}
	if m.TotalTokens != nil {
		tokenParts = append(tokenParts, fmt.Sprintf("total %s", formatNumber(int64(math.Round(m.TotalTokens.Mean)))))
	}
	if m.CacheReadTokens != nil && m.CacheReadTokens.Mean > 0 {
		tokenParts = append(tokenParts, fmt.Sprintf("cache-r %s", formatNumber(int64(math.Round(m.CacheReadTokens.Mean)))))
	}
	if m.CacheWriteTokens != nil && m.CacheWriteTokens.Mean > 0 {
		tokenParts = append(tokenParts, fmt.Sprintf("cache-w %s", formatNumber(int64(math.Round(m.CacheWriteTokens.Mean)))))
	}
	if len(tokenParts) > 0 {
		fmt.Fprintf(d.w, "    %sTokens%s    %s  %s(mean/trial)%s\n",
			d.c(ansiDim), d.c(ansiReset),
			strings.Join(tokenParts, "   "),
			d.c(ansiDim), d.c(ansiReset))
	}

	if m.EstimatedCost != nil && m.EstimatedCost.Max > 0 {
		perTrial := m.EstimatedCost.Median
		if m.EstimatedCost.Mean > 0 {
			perTrial = m.EstimatedCost.Mean
		}
		totalCost := perTrial * float64(m.EstimatedCost.Count)
		fmt.Fprintf(d.w, "    %sCost%s      $%.4f/trial   $%.4f total\n",
			d.c(ansiDim), d.c(ansiReset),
			perTrial, totalCost)
	}

	if m.TTFTMs != nil {
		fmt.Fprintf(d.w, "    %sTTFT%s      mean %s   p90 %s\n",
			d.c(ansiDim), d.c(ansiReset),
			formatDuration(time.Duration(m.TTFTMs.Mean*float64(time.Millisecond))),
			formatDuration(time.Duration(m.TTFTMs.P90*float64(time.Millisecond))))
	}
}

func (d *Display) printHeader(run *model.BenchmarkRun, numScenarios, numModels, trialsPerCombo, concurrency int) {
	scenarioWord := "scenarios"
	if numScenarios == 1 {
		scenarioWord = "scenario"
	}
	modelWord := "models"
	if numModels == 1 {
		modelWord = "model"
	}

	if !d.isTTY {
		fmt.Fprintf(d.w, "[trimetry] %s (%s) — %d %s × %d %s × %d trials = %d total (concurrency %d)\n",
			run.Name, run.BenchmarkRunID, numScenarios, scenarioWord, numModels, modelWord, trialsPerCombo, d.total, concurrency)
		return
	}

	line1 := fmt.Sprintf("  ▲ trimetry %s", run.RunnerVersion)
	line2 := fmt.Sprintf("  %s", run.Name)
	line3 := fmt.Sprintf("  Run: %s   Git: %s   Env: %s", run.BenchmarkRunID, run.GitCommit, run.Environment)
	line4 := fmt.Sprintf("  Started: %s", run.StartedAt.Format("2006-01-02 15:04:05"))

	width := max(displayWidth(line1), displayWidth(line2), displayWidth(line3), displayWidth(line4)) + 4
	if width < 62 {
		width = 62
	}

	fmt.Fprintln(d.w)
	fmt.Fprint(d.w, d.c(ansiCyan))
	fmt.Fprintf(d.w, "╭%s╮\n", strings.Repeat("─", width))
	fmt.Fprintf(d.w, "│%s│\n", pad(line1, width))
	fmt.Fprintf(d.w, "│%s│\n", pad("", width))
	fmt.Fprintf(d.w, "│%s│\n", pad(line2, width))
	fmt.Fprintf(d.w, "│%s│\n", pad(line3, width))
	fmt.Fprintf(d.w, "│%s│\n", pad(line4, width))
	fmt.Fprintf(d.w, "╰%s╯\n", strings.Repeat("─", width))
	fmt.Fprint(d.w, d.c(ansiReset))

	fmt.Fprintln(d.w)
	fmt.Fprintf(d.w, "  %d %s × %d %s × %d trials = %s%d total%s  (concurrency %d)\n",
		numScenarios, scenarioWord, numModels, modelWord, trialsPerCombo, d.c(ansiBold), d.total, d.c(ansiReset), concurrency)
	fmt.Fprintln(d.w)
}

func (d *Display) progressLines() int {
	return len(d.combos) + 2
}

func (d *Display) drawProgress() {
	for i, c := range d.combos {
		prefix := "├"
		if i == 0 && len(d.combos) > 1 {
			prefix = "┌"
		}
		if i == len(d.combos)-1 {
			prefix = "└"
		}

		bar := d.progressBar(c.done(), c.Total)
		status := d.comboStatus(c)

		fmt.Fprintf(d.w, "  %s %s  %s  %d/%d  %s\n",
			prefix, pad(c.Label, d.maxLabel), bar, c.done(), c.Total, status)
	}

	fmt.Fprintln(d.w)

	elapsed := formatDuration(time.Since(d.startTime))
	pct := 0
	if d.total > 0 {
		pct = d.completed * 100 / d.total
	}
	overallBar := d.progressBar(d.completed, d.total)
	fmt.Fprintf(d.w, "  ▸ %d/%d completed (%d%%)  %s  Elapsed: %s\n",
		d.completed, d.total, pct, overallBar, elapsed)
}

func (d *Display) eraseProgress() {
	if !d.drawn {
		return
	}
	lines := d.progressLines()
	fmt.Fprintf(d.w, "\033[%dA", lines)
	for i := 0; i < lines; i++ {
		fmt.Fprint(d.w, "\033[2K\n")
	}
	fmt.Fprintf(d.w, "\033[%dA", lines)
}

func (d *Display) comboStatus(c ComboProgress) string {
	done := c.done()
	if done == 0 {
		return d.c(ansiDim) + "wait" + d.c(ansiReset)
	}

	hasMixed := (c.Completed > 0 && (c.Failed > 0 || c.Timeout > 0 || c.Cancelled > 0)) ||
		(c.Failed > 0 && (c.Timeout > 0 || c.Cancelled > 0)) ||
		(c.Timeout > 0 && c.Cancelled > 0)

	if hasMixed {
		parts := []string{}
		if c.Completed > 0 {
			parts = append(parts, fmt.Sprintf("%s✓%d%s", d.c(ansiGreen), c.Completed, d.c(ansiReset)))
		}
		if c.Failed > 0 {
			parts = append(parts, fmt.Sprintf("%s✗%d%s", d.c(ansiRed), c.Failed, d.c(ansiReset)))
		}
		if c.Timeout > 0 {
			parts = append(parts, fmt.Sprintf("%s⏱%d%s", d.c(ansiYellow), c.Timeout, d.c(ansiReset)))
		}
		if c.Cancelled > 0 {
			parts = append(parts, fmt.Sprintf("%s⊘%d%s", d.c(ansiDim), c.Cancelled, d.c(ansiReset)))
		}
		status := strings.Join(parts, " ")
		if done == c.Total {
			status += "  " + d.c(ansiDim) + "done" + d.c(ansiReset)
		}
		return status
	}

	icon := d.statusIcon(c.statusType())

	if done == c.Total {
		return icon + "  " + d.c(ansiDim) + "done" + d.c(ansiReset)
	}
	return icon
}

func (c *ComboProgress) statusType() model.ExecutionStatus {
	if c.Failed > 0 {
		return model.ExecStatusFailed
	}
	if c.Timeout > 0 {
		return model.ExecStatusTimeout
	}
	if c.Cancelled > 0 {
		return model.ExecStatusCancelled
	}
	return model.ExecStatusCompleted
}

func (d *Display) printTrialLineTTY(trial model.Trial) {
	finishedAt := ""
	if trial.FinishedAt != nil {
		finishedAt = trial.FinishedAt.Format("15:04:05")
	}

	icon := d.statusIcon(trial.ExecutionStatus)

	scenarioLabel := trial.ScenarioID
	if name, ok := d.scenarioName[trial.ScenarioID]; ok {
		scenarioLabel = name
	}
	label := trial.ModelName + " × " + scenarioLabel

	detail := d.trialDetail(trial)

	fmt.Fprintf(d.w, "  %s %s  %s  #%d  %s%s%s\n",
		d.c(ansiDim)+finishedAt+d.c(ansiReset),
		icon,
		label,
		trial.TrialNumber,
		d.c(ansiDim), detail, d.c(ansiReset))
}

func (d *Display) printTrialLine(trial model.Trial) {
	icon := d.statusIcon(trial.ExecutionStatus)

	scenarioLabel := trial.ScenarioID
	if name, ok := d.scenarioName[trial.ScenarioID]; ok {
		scenarioLabel = name
	}
	label := trial.ModelName + " × " + scenarioLabel

	detail := d.trialDetail(trial)

	fmt.Fprintf(d.w, "[trimetry] %s trial %d/%d  %s  #%d  %s\n",
		icon, d.completed, d.total, label, trial.TrialNumber, detail)
}

func (d *Display) trialDetail(trial model.Trial) string {
	if trial.ExecutionStatus == model.ExecStatusCompleted && trial.Metrics != nil {
		dur := formatDuration(time.Duration(trial.Metrics.WallTimeMs) * time.Millisecond)
		detail := dur
		if trial.Metrics.TotalTokens != nil {
			detail += fmt.Sprintf("  %s tok", formatNumber(*trial.Metrics.TotalTokens))
		}
		if trial.Metrics.EstimatedCost != nil && *trial.Metrics.EstimatedCost > 0 {
			detail += fmt.Sprintf("  $%.3f", *trial.Metrics.EstimatedCost)
		}
		return detail
	}
	if trial.ExecutionStatus == model.ExecStatusFailed {
		msg := trial.ErrorMessage
		if len(msg) > 60 {
			msg = msg[:60] + "..."
		}
		return "failed: " + msg
	}
	if trial.ExecutionStatus == model.ExecStatusTimeout && trial.Metrics != nil {
		return fmt.Sprintf("timeout %s", formatDuration(time.Duration(trial.Metrics.WallTimeMs)*time.Millisecond))
	}
	if trial.ExecutionStatus == model.ExecStatusCancelled {
		return "cancelled"
	}
	return ""
}

func (d *Display) progressBar(completed, total int) string {
	if total == 0 {
		return strings.Repeat("░", barWidth)
	}
	filled := completed * barWidth / total
	empty := barWidth - filled

	return d.c(ansiGreen) + strings.Repeat("█", filled) + d.c(ansiReset) +
		d.c(ansiDim) + strings.Repeat("░", empty) + d.c(ansiReset)
}

func displayWidth(s string) int {
	return utf8.RuneCountInString(s)
}

func pad(s string, width int) string {
	dw := displayWidth(s)
	if dw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-dw)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Milliseconds()))
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", m, s)
}

func formatMs(ms float64) string {
	if ms < 1 {
		return fmt.Sprintf("%.2fms", ms)
	}
	if ms < 10 {
		return fmt.Sprintf("%.1fms", ms)
	}
	return formatDuration(time.Duration(ms * float64(time.Millisecond)))
}

func formatNumber(n int64) string {
	neg := n < 0
	s := fmt.Sprintf("%d", n)
	if neg {
		s = s[1:]
	}
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	parts := []string{}
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	result := strings.Join(parts, ",")
	if neg {
		return "-" + result
	}
	return result
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
