package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/konono/trimetry/internal/adapter"
	"github.com/konono/trimetry/internal/model"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Benchmark  BenchmarkConfig  `yaml:"benchmark"`
	Scenarios  []model.Scenario `yaml:"scenarios"`
	Models     []model.ModelConfig `yaml:"models"`
	Telemetry  TelemetryConfig  `yaml:"telemetry"`
	Report     ReportConfig     `yaml:"report"`
	Adapter    AdapterConfig    `yaml:"adapter"`

	rawHash string `yaml:"-"`
}

type BenchmarkConfig struct {
	Name           string `yaml:"name"`
	Trials         int    `yaml:"trials"`
	Concurrency    int    `yaml:"concurrency"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	Retries        int    `yaml:"retries"`
	Environment    string `yaml:"environment"`
}

type TelemetryConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Provider      string `yaml:"provider"`
	FlushOnTrialEnd bool `yaml:"flush_on_trial_end"`
	EnrichmentDir string `yaml:"enrichment_dir"`
	// Langfuse
	BaseURL       string `yaml:"base_url"`
	PublicKey     string `yaml:"public_key"`
	SecretKey     string `yaml:"secret_key"`
	// MLflow
	TrackingURI   string `yaml:"tracking_uri"`
	Token         string `yaml:"token"`
	Workspace     string `yaml:"workspace"`
	TLSSkipVerify bool   `yaml:"tls_skip_verify"`
}

type ReportConfig struct {
	OutputDirectory string   `yaml:"output_directory"`
	Formats         []string `yaml:"formats"`
	MaskOutput      bool     `yaml:"mask_output"`
}

type AdapterConfig struct {
	Type    string            `yaml:"type"`
	Options map[string]string `yaml:"options"`
}

var secretPrefixes = []string{"sk-ant-", "sk-lf-", "sk_live_", "ghp_", "gho_", "github_pat_", "hf_"}

func (c *Config) RedactOptions() map[string]string {
	safe := make(map[string]string)
	for k, v := range c.Adapter.Options {
		if looksLikeSecret(v) {
			safe[k] = "[REDACTED]"
		} else {
			safe[k] = v
		}
	}
	return safe
}

func (c *Config) ApplyDryRun() {
	c.Adapter.Type = "fake"
	c.Telemetry.Enabled = false
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	h := sha256.Sum256(data)
	cfg.rawHash = fmt.Sprintf("sha256:%x", h)

	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Benchmark.Trials <= 0 {
		c.Benchmark.Trials = 5
	}
	if c.Benchmark.Concurrency <= 0 {
		c.Benchmark.Concurrency = 1
	}
	if c.Benchmark.TimeoutSeconds <= 0 {
		c.Benchmark.TimeoutSeconds = 300
	}
	if c.Benchmark.Environment == "" {
		c.Benchmark.Environment = "local"
	}
	if c.Report.OutputDirectory == "" {
		c.Report.OutputDirectory = "benchmark-results"
	}
	if len(c.Report.Formats) == 0 {
		c.Report.Formats = []string{"json", "markdown"}
	}
	if c.Adapter.Type == "" {
		c.Adapter.Type = "opencode"
	}
	for i := range c.Scenarios {
		if c.Scenarios[i].TimeoutSeconds <= 0 {
			c.Scenarios[i].TimeoutSeconds = c.Benchmark.TimeoutSeconds
		}
	}

	if c.Telemetry.EnrichmentDir == "" {
		if dir := os.Getenv("TRIMETRY_ENRICHMENT_DIR"); dir != "" {
			c.Telemetry.EnrichmentDir = dir
		} else {
			c.Telemetry.EnrichmentDir = "/tmp/trimetry-enrichment"
		}
	}

	if c.Telemetry.Provider == "" {
		c.Telemetry.Provider = "langfuse"
	}

	switch c.Telemetry.Provider {
	case "mlflow":
		c.Telemetry.resolveMLflowEnv()
	default:
		c.Telemetry.resolveLangfuseEnv()
	}
}

func (t *TelemetryConfig) resolveLangfuseEnv() {
	if t.BaseURL == "" {
		t.BaseURL = os.Getenv("LANGFUSE_BASEURL")
	}
	if t.PublicKey == "" {
		t.PublicKey = os.Getenv("LANGFUSE_PUBLIC_KEY")
	}
	if t.SecretKey == "" {
		t.SecretKey = os.Getenv("LANGFUSE_SECRET_KEY")
	}
}

func (t *TelemetryConfig) resolveMLflowEnv() {
	if t.TrackingURI == "" {
		t.TrackingURI = os.Getenv("MLFLOW_TRACKING_URI")
	}
	if t.Token == "" {
		t.Token = os.Getenv("MLFLOW_TRACKING_TOKEN")
	}
	if t.Workspace == "" {
		t.Workspace = os.Getenv("MLFLOW_TRACKING_WORKSPACE")
	}
}

func (c *Config) Validate() []string {
	var errs []string

	if c.Benchmark.Name == "" {
		errs = append(errs, "benchmark.name is required")
	}
	if c.Benchmark.Trials < 1 {
		errs = append(errs, "benchmark.trials must be >= 1")
	}
	if c.Benchmark.Concurrency < 1 {
		errs = append(errs, "benchmark.concurrency must be >= 1")
	}
	if c.Benchmark.TimeoutSeconds < 1 {
		errs = append(errs, "benchmark.timeout_seconds must be >= 1")
	}

	if len(c.Scenarios) == 0 {
		errs = append(errs, "at least one scenario is required")
	}
	seen := make(map[string]bool)
	for i, s := range c.Scenarios {
		if s.ScenarioID == "" {
			errs = append(errs, fmt.Sprintf("scenarios[%d].id is required", i))
		} else if seen[s.ScenarioID] {
			errs = append(errs, fmt.Sprintf("duplicate scenario id: %s", s.ScenarioID))
		} else {
			seen[s.ScenarioID] = true
		}
		if s.ScenarioVersion == "" {
			errs = append(errs, fmt.Sprintf("scenarios[%d].version is required", i))
		}
		if s.Input == "" {
			errs = append(errs, fmt.Sprintf("scenarios[%d].input is required", i))
		}
	}

	if len(c.Models) == 0 {
		errs = append(errs, "at least one model is required")
	}
	for i, m := range c.Models {
		if m.Name == "" {
			errs = append(errs, fmt.Sprintf("models[%d].name is required", i))
		}
		if m.Provider == "" {
			errs = append(errs, fmt.Sprintf("models[%d].provider is required", i))
		}
	}

	if c.Telemetry.Enabled {
		switch c.Telemetry.Provider {
		case "mlflow":
			if c.Telemetry.TrackingURI == "" {
				errs = append(errs, "telemetry.tracking_uri or MLFLOW_TRACKING_URI env is required when provider is mlflow")
			}
		case "langfuse", "":
			if c.Telemetry.BaseURL == "" {
				errs = append(errs, "telemetry.base_url or LANGFUSE_BASEURL env is required when telemetry is enabled")
			}
			if c.Telemetry.PublicKey == "" {
				errs = append(errs, "telemetry.public_key or LANGFUSE_PUBLIC_KEY env is required when telemetry is enabled")
			}
			if c.Telemetry.SecretKey == "" {
				errs = append(errs, "telemetry.secret_key or LANGFUSE_SECRET_KEY env is required when telemetry is enabled")
			}
		default:
			errs = append(errs, fmt.Sprintf("unsupported telemetry provider: %s (supported: langfuse, mlflow)", c.Telemetry.Provider))
		}
	}

	for _, f := range c.Report.Formats {
		if f != "json" && f != "markdown" {
			errs = append(errs, fmt.Sprintf("unsupported report format: %s (supported: json, markdown)", f))
		}
	}

	supported := adapter.SupportedTypes()
	valid := false
	for _, t := range supported {
		if c.Adapter.Type == t {
			valid = true
			break
		}
	}
	if !valid {
		errs = append(errs, fmt.Sprintf("adapter.type %q is not supported (supported: %s)", c.Adapter.Type, strings.Join(supported, ", ")))
	}

	errs = append(errs, c.checkForSecrets()...)

	return errs
}

func (c *Config) checkForSecrets() []string {
	var errs []string
	check := func(field, value string) {
		for _, prefix := range secretPrefixes {
			if strings.HasPrefix(value, prefix) {
				errs = append(errs, fmt.Sprintf("%s appears to contain a secret (starts with %q)", field, prefix))
				return
			}
		}
	}

	for k, v := range c.Adapter.Options {
		check(fmt.Sprintf("adapter.options.%s", k), v)
	}
	return errs
}

// Hash returns the SHA256 of the original YAML file (before defaults/env resolution).
func (c *Config) Hash() string {
	if c.rawHash != "" {
		return c.rawHash
	}
	return c.EffectiveHash()
}

// EffectiveHash returns the SHA256 of the current in-memory config (after defaults, env, and runtime overrides).
// Authentication fields are redacted before hashing so the hash is safe to include in shared reports.
func (c *Config) EffectiveHash() string {
	redacted := *c
	redacted.Telemetry.SecretKey = ""
	redacted.Telemetry.PublicKey = ""
	redacted.Telemetry.Token = ""
	if len(redacted.Adapter.Options) > 0 {
		redacted.Adapter.Options = redacted.RedactOptions()
	}
	data, err := yaml.Marshal(&redacted)
	if err != nil {
		return "sha256:error"
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h)
}

func looksLikeSecret(v string) bool {
	for _, prefix := range secretPrefixes {
		if strings.HasPrefix(v, prefix) {
			return true
		}
	}
	return false
}
