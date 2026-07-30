package adapter

import (
	"strings"
	"testing"
)

func TestScanJSONLines(t *testing.T) {
	type item struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	tests := []struct {
		name      string
		raw       string
		wantCount int
		wantNames []string
	}{
		{
			name: "normal multi-line input",
			raw: strings.Join([]string{
				`{"name":"alpha","value":1}`,
				`{"name":"beta","value":2}`,
				`{"name":"gamma","value":3}`,
			}, "\n"),
			wantCount: 3,
			wantNames: []string{"alpha", "beta", "gamma"},
		},
		{
			name: "blank lines are skipped",
			raw: strings.Join([]string{
				`{"name":"first","value":1}`,
				``,
				``,
				`{"name":"second","value":2}`,
			}, "\n"),
			wantCount: 2,
			wantNames: []string{"first", "second"},
		},
		{
			name: "invalid JSON lines are skipped",
			raw: strings.Join([]string{
				`{"name":"valid","value":1}`,
				`not valid json at all`,
				`{"name":"also_valid","value":3}`,
			}, "\n"),
			wantCount: 2,
			wantNames: []string{"valid", "also_valid"},
		},
		{
			name:      "empty input",
			raw:       "",
			wantCount: 0,
			wantNames: nil,
		},
		{
			name: "mixed blank and invalid lines",
			raw: strings.Join([]string{
				``,
				`{bad}`,
				``,
				`{"name":"only","value":99}`,
				`{also bad`,
				``,
			}, "\n"),
			wantCount: 1,
			wantNames: []string{"only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []item
			scanJSONLines(tt.raw, func(i item) {
				got = append(got, i)
			})

			if len(got) != tt.wantCount {
				t.Fatalf("count = %d, want %d", len(got), tt.wantCount)
			}

			for i, wantName := range tt.wantNames {
				if got[i].Name != wantName {
					t.Errorf("item[%d].Name = %q, want %q", i, got[i].Name, wantName)
				}
			}
		})
	}
}
