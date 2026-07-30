package adapter

import (
	"strings"
	"testing"
)

func TestNewAdapter(t *testing.T) {
	tests := []struct {
		name        string
		adapterType string
		options     map[string]string
		wantName    string
		wantErr     bool
	}{
		{
			name:        "opencode adapter",
			adapterType: "opencode",
			options:     map[string]string{},
			wantName:    "opencode",
		},
		{
			name:        "claude adapter",
			adapterType: "claude",
			options:     map[string]string{},
			wantName:    "claude",
		},
		{
			name:        "codex adapter",
			adapterType: "codex",
			options:     map[string]string{},
			wantName:    "codex",
		},
		{
			name:        "cursor adapter",
			adapterType: "cursor",
			options:     map[string]string{},
			wantName:    "cursor",
		},
		{
			name:        "fake adapter",
			adapterType: "fake",
			options:     map[string]string{},
			wantName:    "fake",
		},
		{
			name:        "unknown adapter returns error",
			adapterType: "nonexistent",
			options:     map[string]string{},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := NewAdapter(tt.adapterType, tt.options)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if adapter == nil {
				t.Fatalf("adapter is nil, want non-nil")
			}
			if adapter.Name() != tt.wantName {
				t.Errorf("Name() = %q, want %q", adapter.Name(), tt.wantName)
			}
		})
	}
}

func TestNewAdapterUnknownErrorMessage(t *testing.T) {
	_, err := NewAdapter("bogus", map[string]string{})
	if err == nil {
		t.Fatalf("expected error for unknown adapter type")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error %q does not mention the unknown type %q", err.Error(), "bogus")
	}
}

func TestSupportedTypes(t *testing.T) {
	types := SupportedTypes()
	if len(types) == 0 {
		t.Fatalf("SupportedTypes() returned empty slice")
	}

	expected := map[string]bool{
		"opencode": false,
		"claude":   false,
		"codex":    false,
		"cursor":   false,
		"fake":     false,
	}

	for _, typ := range types {
		if _, ok := expected[typ]; ok {
			expected[typ] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("SupportedTypes() missing %q", name)
		}
	}
}

func TestSupportedTypesMatchNewAdapter(t *testing.T) {
	for _, typ := range SupportedTypes() {
		t.Run(typ, func(t *testing.T) {
			adapter, err := NewAdapter(typ, map[string]string{})
			if err != nil {
				t.Fatalf("NewAdapter(%q) returned error: %v", typ, err)
			}
			if adapter == nil {
				t.Fatalf("NewAdapter(%q) returned nil", typ)
			}
		})
	}
}
