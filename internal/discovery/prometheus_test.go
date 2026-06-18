package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrometheusSource_Fetch(t *testing.T) {
	tests := []struct {
		name       string
		response   interface{}
		statusCode int
		wantCount  int
		wantErr    bool
		wantFirst  string
	}{
		{
			name: "valid response with image labels",
			response: prometheusResponse{
				Status: "success",
				Data: struct {
					ResultType string             `json:"resultType"`
					Result     []prometheusResult `json:"result"`
				}{
					ResultType: "vector",
					Result: []prometheusResult{
						{
							Metric: map[string]string{"image": "nginx:1.25"},
							Value:  []interface{}{1234567890.0, "10"},
						},
						{
							Metric: map[string]string{"image": "redis:7.0"},
							Value:  []interface{}{1234567890.0, "5"},
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantCount:  2,
			wantFirst:  "nginx:1.25",
		},
		{
			name: "skips results without image label",
			response: prometheusResponse{
				Status: "success",
				Data: struct {
					ResultType string             `json:"resultType"`
					Result     []prometheusResult `json:"result"`
				}{
					ResultType: "vector",
					Result: []prometheusResult{
						{
							Metric: map[string]string{"image": "nginx:1.25"},
							Value:  []interface{}{1234567890.0, "10"},
						},
						{
							Metric: map[string]string{"container": "sidecar"},
							Value:  []interface{}{1234567890.0, "3"},
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantCount:  1,
			wantFirst:  "nginx:1.25",
		},
		{
			name:       "HTTP error returns error",
			response:   "internal server error",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
		{
			name: "empty results",
			response: prometheusResponse{
				Status: "success",
				Data: struct {
					ResultType string             `json:"resultType"`
					Result     []prometheusResult `json:"result"`
				}{
					ResultType: "vector",
					Result:     []prometheusResult{},
				},
			},
			statusCode: http.StatusOK,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/query" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.WriteHeader(tt.statusCode)
				if err := json.NewEncoder(w).Encode(tt.response); err != nil {
					t.Fatal(err)
				}
			}))
			defer server.Close()

			source := NewPrometheusSource(server.URL, "test_query", 0, "", "", server.Client())
			results, err := source.Fetch(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
			}

			if tt.wantFirst != "" && len(results) > 0 {
				if results[0].Image != tt.wantFirst {
					t.Errorf("first image = %q, want %q", results[0].Image, tt.wantFirst)
				}
			}
		})
	}
}
