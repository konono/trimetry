package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	yaml := `
benchmark:
  name: test-bench
  trials: 3
  concurrency: 1
  timeout_seconds: 60
  environment: test

scenarios:
  - id: s1
    version: "1"
    input: "hello"

models:
  - name: model-a
    provider: fake

telemetry:
  enabled: false

report:
  output_directory: /tmp/test-results
  formats:
    - json
`
	path := writeTempFile(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Benchmark.Name != "test-bench" {
		t.Errorf("expected name test-bench, got %s", cfg.Benchmark.Name)
	}
	if cfg.Benchmark.Trials != 3 {
		t.Errorf("expected trials 3, got %d", cfg.Benchmark.Trials)
	}
	if len(cfg.Scenarios) != 1 {
		t.Errorf("expected 1 scenario, got %d", len(cfg.Scenarios))
	}
	if cfg.Scenarios[0].TimeoutSeconds != 60 {
		t.Errorf("expected scenario timeout 60, got %d", cfg.Scenarios[0].TimeoutSeconds)
	}
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantErrs []string
	}{
		{
			name: "missing name",
			yaml: `
benchmark:
  trials: 1
scenarios:
  - id: s1
    version: "1"
    input: "hello"
models:
  - name: m1
    provider: p1
telemetry:
  enabled: false
`,
			wantErrs: []string{"benchmark.name is required"},
		},
		{
			name: "duplicate scenario",
			yaml: `
benchmark:
  name: test
  trials: 1
scenarios:
  - id: s1
    version: "1"
    input: "hello"
  - id: s1
    version: "2"
    input: "world"
models:
  - name: m1
    provider: p1
telemetry:
  enabled: false
`,
			wantErrs: []string{"duplicate scenario id: s1"},
		},
		{
			name: "no scenarios",
			yaml: `
benchmark:
  name: test
  trials: 1
scenarios: []
models:
  - name: m1
    provider: p1
telemetry:
  enabled: false
`,
			wantErrs: []string{"at least one scenario is required"},
		},
		{
			name: "no models",
			yaml: `
benchmark:
  name: test
  trials: 1
scenarios:
  - id: s1
    version: "1"
    input: "hello"
models: []
telemetry:
  enabled: false
`,
			wantErrs: []string{"at least one model is required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, tt.yaml)
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}
			errs := cfg.Validate()
			for _, want := range tt.wantErrs {
				found := false
				for _, e := range errs {
					if e == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error %q not found in %v", want, errs)
				}
			}
		})
	}
}

func TestConfigHashDeterministic(t *testing.T) {
	yaml := `
benchmark:
  name: hash-test
  trials: 1
scenarios:
  - id: s1
    version: "1"
    input: "hello"
models:
  - name: m1
    provider: p1
telemetry:
  enabled: false
`
	path := writeTempFile(t, yaml)
	cfg1, _ := Load(path)
	cfg2, _ := Load(path)

	if cfg1.Hash() != cfg2.Hash() {
		t.Errorf("config hash not deterministic: %s != %s", cfg1.Hash(), cfg2.Hash())
	}
}

func TestSecretDetection(t *testing.T) {
	yaml := `
benchmark:
  name: secret-test
  trials: 1
scenarios:
  - id: s1
    version: "1"
    input: "hello"
models:
  - name: m1
    provider: p1
telemetry:
  enabled: false
adapter:
  options:
    api_key: "sk-ant-secret-key-here"
`
	path := writeTempFile(t, yaml)
	cfg, _ := Load(path)
	errs := cfg.Validate()

	found := false
	for _, e := range errs {
		if len(e) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected secret detection warning")
	}
}

func TestEffectiveHashRedactsSecrets(t *testing.T) {
	yaml1 := `
benchmark:
  name: test
  trials: 1
scenarios:
  - id: s1
    version: "1"
    input: hello
models:
  - name: m1
    provider: p1
telemetry:
  enabled: true
  provider: langfuse
  base_url: http://localhost
  public_key: pk-1
  secret_key: sk-1
`
	yaml2 := `
benchmark:
  name: test
  trials: 1
scenarios:
  - id: s1
    version: "1"
    input: hello
models:
  - name: m1
    provider: p1
telemetry:
  enabled: true
  provider: langfuse
  base_url: http://localhost
  public_key: pk-2
  secret_key: sk-2
`
	path1 := writeTempFile(t, yaml1)
	path2 := writeTempFile(t, yaml2)
	cfg1, err := Load(path1)
	if err != nil {
		t.Fatalf("Load cfg1 failed: %v", err)
	}
	cfg2, err := Load(path2)
	if err != nil {
		t.Fatalf("Load cfg2 failed: %v", err)
	}

	h1 := cfg1.EffectiveHash()
	h2 := cfg2.EffectiveHash()
	if h1 != h2 {
		t.Errorf("EffectiveHash should be equal when only secrets differ: %s != %s", h1, h2)
	}

	// Raw hashes should differ since the YAML content is different
	if cfg1.Hash() == cfg2.Hash() {
		t.Error("raw Hash should differ when file content differs")
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
