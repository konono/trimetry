package telemetry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestAdapter(url string) *LangfuseAdapter {
	return NewLangfuseAdapter(LangfuseOptions{
		BaseURL:        url,
		PublicKey:      "pk-test",
		SecretKey:      "sk-test",
		BatchChunkSize: 5,
		MaxRetries:     2,
		MaxBatchQueue:  20,
	})
}

func addTestEvents(a *LangfuseAdapter, n int) {
	for i := 0; i < n; i++ {
		a.addEvent(ingestionEvent{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "trace-create",
			Timestamp: time.Now(),
			Body:      map[string]string{"id": fmt.Sprintf("trace-%d", i)},
		})
	}
}

func TestSplitBatch(t *testing.T) {
	tests := []struct {
		name      string
		count     int
		chunkSize int
		wantLen   int
	}{
		{"empty", 0, 5, 0},
		{"smaller than chunk", 3, 5, 1},
		{"exact chunk", 5, 5, 1},
		{"two chunks", 7, 5, 2},
		{"three chunks", 11, 5, 3},
		{"zero chunk size defaults to 50", 10, 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := make([]ingestionEvent, tt.count)
			for i := range events {
				events[i] = ingestionEvent{ID: fmt.Sprintf("e-%d", i)}
			}
			chunks := splitBatch(events, tt.chunkSize)
			if len(chunks) != tt.wantLen {
				t.Errorf("splitBatch(%d, %d) = %d chunks, want %d", tt.count, tt.chunkSize, len(chunks), tt.wantLen)
			}
			total := 0
			for _, c := range chunks {
				total += len(c)
			}
			if total != tt.count {
				t.Errorf("total events in chunks = %d, want %d", total, tt.count)
			}
		})
	}
}

func TestFlush_ClearsBatchOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(ingestionResponse{
			Successes: []ingestionResultItem{{ID: "evt-0", Status: 200}},
		})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	addTestEvents(a, 3)

	a.Flush()

	a.mu.Lock()
	remaining := len(a.batch)
	a.mu.Unlock()

	if remaining != 0 {
		t.Errorf("batch should be empty after successful flush, got %d events", remaining)
	}
}

func TestFlush_RequeuesOnSendFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	a.maxRetries = 1
	addTestEvents(a, 3)

	a.Flush()

	a.mu.Lock()
	remaining := len(a.batch)
	a.mu.Unlock()

	if remaining != 3 {
		t.Errorf("batch should have 3 re-queued events after failure, got %d", remaining)
	}
}

func TestFlush_Parse207_PartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Batch []ingestionEvent `json:"batch"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		resp := ingestionResponse{}
		for _, ev := range req.Batch {
			if ev.ID == "evt-1" {
				resp.Errors = append(resp.Errors, ingestionErrorItem{
					ID: ev.ID, Status: 500, Message: "server error",
				})
			} else {
				resp.Successes = append(resp.Successes, ingestionResultItem{
					ID: ev.ID, Status: 200,
				})
			}
		}
		w.WriteHeader(207)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	a.maxRetries = 0
	addTestEvents(a, 3)

	a.Flush()

	a.mu.Lock()
	remaining := len(a.batch)
	var ids []string
	for _, ev := range a.batch {
		ids = append(ids, ev.ID)
	}
	a.mu.Unlock()

	if remaining != 1 {
		t.Errorf("expected 1 re-queued event, got %d", remaining)
	}
	if remaining == 1 && ids[0] != "evt-1" {
		t.Errorf("expected re-queued event id 'evt-1', got %q", ids[0])
	}
}

func TestFlush_NonRetryableError(t *testing.T) {
	postCount := atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/public/ingestion" {
			postCount.Add(1)
		}
		w.WriteHeader(401)
		w.Write([]byte("unauthorized"))
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	a.maxRetries = 3
	addTestEvents(a, 2)

	a.Flush()

	count := int(postCount.Load())
	if count != 1 {
		t.Errorf("expected 1 POST to ingestion (no retry for 401), got %d", count)
	}

	a.mu.Lock()
	remaining := len(a.batch)
	a.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected 0 events (non-retryable errors should be dropped), got %d", remaining)
	}
}

func TestSendWithRetry_BackoffAndSuccess(t *testing.T) {
	postCount := atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/public/ingestion" {
			n := postCount.Add(1)
			if n <= 2 {
				w.WriteHeader(500)
				w.Write([]byte("server error"))
				return
			}
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(ingestionResponse{})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	a.maxRetries = 3
	addTestEvents(a, 2)

	a.Flush()

	a.mu.Lock()
	remaining := len(a.batch)
	a.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected empty batch after retry success, got %d events", remaining)
	}

	count := int(postCount.Load())
	if count != 3 {
		t.Errorf("expected 3 POST requests (2 failures + 1 success), got %d", count)
	}
}

func TestSendWithRetry_MaxRetriesExhausted(t *testing.T) {
	postCount := atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/public/ingestion" {
			postCount.Add(1)
		}
		w.WriteHeader(500)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	a.maxRetries = 2
	addTestEvents(a, 2)

	a.Flush()

	a.mu.Lock()
	remaining := len(a.batch)
	a.mu.Unlock()

	if remaining != 2 {
		t.Errorf("expected 2 re-queued events after max retries, got %d", remaining)
	}

	count := int(postCount.Load())
	expected := 3 // initial + 2 retries
	if count != expected {
		t.Errorf("expected %d POST requests, got %d", expected, count)
	}
}

func TestFlush_SafetyValve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	a.maxRetries = 0
	a.maxBatchQueue = 5

	a.mu.Lock()
	for i := 0; i < 3; i++ {
		a.batch = append(a.batch, ingestionEvent{ID: fmt.Sprintf("new-%d", i)})
	}
	a.mu.Unlock()

	events := make([]ingestionEvent, 10)
	for i := range events {
		events[i] = ingestionEvent{ID: fmt.Sprintf("failed-%d", i)}
	}
	a.mu.Lock()
	orig := a.batch
	a.batch = append(events, orig...)
	a.mu.Unlock()

	a.Flush()

	a.mu.Lock()
	remaining := len(a.batch)
	a.mu.Unlock()

	if remaining > 5 {
		t.Errorf("safety valve should cap batch at %d, got %d", a.maxBatchQueue, remaining)
	}
}

func TestFlush_BatchSplitting_MultipleChunks(t *testing.T) {
	postCount := atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/public/ingestion" {
			postCount.Add(1)
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(ingestionResponse{})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	a.batchChunkSize = 3
	addTestEvents(a, 10)

	a.Flush()

	count := int(postCount.Load())
	expected := 4 // 3+3+3+1
	if count != expected {
		t.Errorf("expected %d POST requests for 10 events / chunk 3, got %d", expected, count)
	}
}

func TestFlush_ConcurrentSafety(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(ingestionResponse{})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)

	var wg sync.WaitGroup
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				a.addEvent(ingestionEvent{
					ID:        fmt.Sprintf("g%d-evt-%d", id, i),
					Type:      "trace-create",
					Timestamp: time.Now(),
				})
				if i%3 == 0 {
					a.Flush()
				}
			}
			a.Flush()
		}(g)
	}

	wg.Wait()

	a.mu.Lock()
	remaining := len(a.batch)
	a.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected empty batch after all flushes, got %d events", remaining)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"500 is retryable", &sendError{statusCode: 500}, true},
		{"502 is retryable", &sendError{statusCode: 502}, true},
		{"429 is retryable", &sendError{statusCode: 429}, true},
		{"401 is not retryable", &sendError{statusCode: 401}, false},
		{"400 is not retryable", &sendError{statusCode: 400}, false},
		{"network error is retryable", fmt.Errorf("connection refused"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryable(tt.err)
			if got != tt.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestExtractRetryableFailures(t *testing.T) {
	events := []ingestionEvent{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}

	t.Run("no errors returns nil", func(t *testing.T) {
		result := &ingestionResponse{
			Successes: []ingestionResultItem{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		}
		got := extractRetryableFailures(events, result)
		if len(got) != 0 {
			t.Errorf("expected 0 retryable, got %d", len(got))
		}
	})

	t.Run("500 error is retryable", func(t *testing.T) {
		result := &ingestionResponse{
			Successes: []ingestionResultItem{{ID: "a"}, {ID: "c"}},
			Errors:    []ingestionErrorItem{{ID: "b", Status: 500}},
		}
		got := extractRetryableFailures(events, result)
		if len(got) != 1 || got[0].ID != "b" {
			t.Errorf("expected [b], got %v", got)
		}
	})

	t.Run("400 error is not retryable", func(t *testing.T) {
		result := &ingestionResponse{
			Successes: []ingestionResultItem{{ID: "a"}, {ID: "c"}},
			Errors:    []ingestionErrorItem{{ID: "b", Status: 400}},
		}
		got := extractRetryableFailures(events, result)
		if len(got) != 0 {
			t.Errorf("expected 0 retryable (400 is permanent), got %d", len(got))
		}
	})

	t.Run("nil result returns nil", func(t *testing.T) {
		got := extractRetryableFailures(events, nil)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}
