package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dropv1alpha1 "github.com/corewire/drop/api/v1alpha1"
)

// TestExecutePipeline_PrometheusInstant verifies the full pipeline with a Prometheus instant query.
func TestExecutePipeline_PrometheusInstant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := prometheusResponse{
			Status: prometheusStatusSuccess,
			Data: struct {
				ResultType string             `json:"resultType"`
				Result     []prometheusResult `json:"result"`
			}{
				ResultType: "vector",
				Result: []prometheusResult{
					{Metric: map[string]string{"image": "nginx:1.25"}, Value: []interface{}{float64(1000), "30"}},
					{Metric: map[string]string{"image": "redis:7.0"}, Value: []interface{}{float64(1000), "10"}},
					{Metric: map[string]string{"image": "alpine:3.19"}, Value: []interface{}{float64(1000), "20"}},
				},
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	spec := dropv1alpha1.DiscoveryPolicySpec{
		Queries: []dropv1alpha1.DiscoveryQuery{
			{
				Name:       "usage",
				Type:       dropv1alpha1.DiscoveryQueryTypePrometheus,
				Prometheus: &dropv1alpha1.DiscoveryPrometheusQuery{Endpoint: srv.URL, Query: "test", QueryType: dropv1alpha1.QueryTypeInstant},
			},
		},
		Signals: []dropv1alpha1.DiscoverySignal{
			{Name: "score", QueryRef: "usage", Type: dropv1alpha1.SignalTypeAggregate, Aggregate: &dropv1alpha1.AggregateSignalConfig{Method: dropv1alpha1.AggregationSum}},
		},
		Ranking:   &dropv1alpha1.DiscoveryRanking{Strategy: dropv1alpha1.RankingStrategySignal, Signal: &dropv1alpha1.SignalRankingConfig{SignalRef: "score"}},
		MaxImages: 10,
	}

	clientFn := func(_ context.Context, _ string) (*http.Client, error) { return srv.Client(), nil }
	result := ExecutePipeline(context.Background(), spec, clientFn)

	if len(result.QueryResults) != 1 {
		t.Fatalf("expected 1 query result, got %d", len(result.QueryResults))
	}
	if result.QueryResults[0].Status != dropv1alpha1.QueryResultStatusSuccess {
		t.Fatalf("expected success, got %s: %s", result.QueryResults[0].Status, result.QueryResults[0].Message)
	}
	if len(result.Images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(result.Images))
	}
	// Ranked by score desc: nginx(30) > alpine(20) > redis(10)
	if result.Images[0].Image != "nginx:1.25" {
		t.Errorf("expected nginx:1.25 first, got %s", result.Images[0].Image)
	}
	if result.Images[0].Rank != 1 {
		t.Errorf("expected rank 1, got %d", result.Images[0].Rank)
	}
	if !result.Images[0].Selected {
		t.Error("top image should be selected")
	}
}

// TestExecutePipeline_Registry verifies the full pipeline with a registry query.
func TestExecutePipeline_Registry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tagListResponse{
			Name: "team/app",
			Tags: []string{"v1.0", "v1.1", "v1.2"},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	spec := dropv1alpha1.DiscoveryPolicySpec{
		Queries: []dropv1alpha1.DiscoveryQuery{
			{
				Name: "tags",
				Type: dropv1alpha1.DiscoveryQueryTypeRegistry,
				Registry: &dropv1alpha1.DiscoveryRegistryQuery{
					URL:          srv.URL,
					Repositories: []string{"team/app"},
				},
			},
		},
		Signals: []dropv1alpha1.DiscoverySignal{
			{Name: "tag-score", QueryRef: "tags", Type: dropv1alpha1.SignalTypeAggregate, Aggregate: &dropv1alpha1.AggregateSignalConfig{Method: dropv1alpha1.AggregationSum}},
		},
		Ranking:   &dropv1alpha1.DiscoveryRanking{Strategy: dropv1alpha1.RankingStrategySignal, Signal: &dropv1alpha1.SignalRankingConfig{SignalRef: "tag-score"}},
		MaxImages: 10,
	}

	clientFn := func(_ context.Context, _ string) (*http.Client, error) { return srv.Client(), nil }
	result := ExecutePipeline(context.Background(), spec, clientFn)

	if len(result.QueryResults) != 1 {
		t.Fatalf("expected 1 query result, got %d", len(result.QueryResults))
	}
	if result.QueryResults[0].Status != dropv1alpha1.QueryResultStatusSuccess {
		t.Fatalf("expected success, got %s: %s", result.QueryResults[0].Status, result.QueryResults[0].Message)
	}
	if len(result.Images) != 3 {
		t.Fatalf("expected 3 images, got %d: %v", len(result.Images), result.Images)
	}
	// v1.2 has the highest score (index 3), then v1.1 (2), then v1.0 (1)
	registryHost := srv.URL[len("http://"):]
	expectedFirst := registryHost + "/team/app:v1.2"
	if result.Images[0].Image != expectedFirst {
		t.Errorf("expected %s first, got %s", expectedFirst, result.Images[0].Image)
	}
}

// TestExecutePipeline_WeightedSum verifies weighted sum ranking.
func TestExecutePipeline_WeightedSum(t *testing.T) {
	// Two queries with different image sets
	srv1 := httptest.NewServer(prometheusInstantHandler(map[string]string{
		"nginx:1.25": "100",
		"redis:7.0":  "10",
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(prometheusInstantHandler(map[string]string{
		"nginx:1.25": "5",
		"redis:7.0":  "50",
	}))
	defer srv2.Close()

	weight700m := resource.MustParse("700m")
	weight300m := resource.MustParse("300m")

	spec := dropv1alpha1.DiscoveryPolicySpec{
		Queries: []dropv1alpha1.DiscoveryQuery{
			{Name: "q1", Type: dropv1alpha1.DiscoveryQueryTypePrometheus, Prometheus: &dropv1alpha1.DiscoveryPrometheusQuery{Endpoint: srv1.URL, Query: "test", QueryType: dropv1alpha1.QueryTypeInstant}},
			{Name: "q2", Type: dropv1alpha1.DiscoveryQueryTypePrometheus, Prometheus: &dropv1alpha1.DiscoveryPrometheusQuery{Endpoint: srv2.URL, Query: "test", QueryType: dropv1alpha1.QueryTypeInstant}},
		},
		Signals: []dropv1alpha1.DiscoverySignal{
			{Name: "sig1", QueryRef: "q1", Type: dropv1alpha1.SignalTypeAggregate, Aggregate: &dropv1alpha1.AggregateSignalConfig{Method: dropv1alpha1.AggregationSum}},
			{Name: "sig2", QueryRef: "q2", Type: dropv1alpha1.SignalTypeAggregate, Aggregate: &dropv1alpha1.AggregateSignalConfig{Method: dropv1alpha1.AggregationSum}},
		},
		Ranking: &dropv1alpha1.DiscoveryRanking{
			Strategy: dropv1alpha1.RankingStrategyWeightedSum,
			WeightedSum: &dropv1alpha1.WeightedSumRankingConfig{
				Normalize:     dropv1alpha1.NormalizeMethodMinMax,
				MissingSignal: dropv1alpha1.MissingSignalBehaviorZero,
				Terms: []dropv1alpha1.WeightedSumTerm{
					{SignalRef: "sig1", Weight: weight700m},
					{SignalRef: "sig2", Weight: weight300m},
				},
			},
		},
		MaxImages: 10,
	}

	srvMap := map[string]*http.Client{"q1": srv1.Client(), "q2": srv2.Client()}
	clientFn := func(_ context.Context, queryName string) (*http.Client, error) {
		return srvMap[queryName], nil
	}
	result := ExecutePipeline(context.Background(), spec, clientFn)

	if len(result.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(result.Images))
	}
	// nginx: sig1=100 (norm=1), sig2=5 (norm=0) → 0.7*1 + 0.3*0 = 0.7
	// redis:  sig1=10 (norm=0), sig2=50 (norm=1) → 0.7*0 + 0.3*1 = 0.3
	// nginx should rank first
	if result.Images[0].Image != "nginx:1.25" {
		t.Errorf("expected nginx:1.25 first (weightedSum), got %s", result.Images[0].Image)
	}
}

// TestExecutePipeline_MaxImages verifies the maxImages cap is applied.
func TestExecutePipeline_MaxImages(t *testing.T) {
	srv := httptest.NewServer(prometheusInstantHandler(map[string]string{
		"img1:v1": "10",
		"img2:v2": "20",
		"img3:v3": "30",
		"img4:v4": "40",
		"img5:v5": "50",
	}))
	defer srv.Close()

	spec := dropv1alpha1.DiscoveryPolicySpec{
		Queries: []dropv1alpha1.DiscoveryQuery{
			{Name: "q", Type: dropv1alpha1.DiscoveryQueryTypePrometheus, Prometheus: &dropv1alpha1.DiscoveryPrometheusQuery{Endpoint: srv.URL, Query: "test", QueryType: dropv1alpha1.QueryTypeInstant}},
		},
		Signals: []dropv1alpha1.DiscoverySignal{
			{Name: "s", QueryRef: "q", Type: dropv1alpha1.SignalTypeAggregate, Aggregate: &dropv1alpha1.AggregateSignalConfig{Method: dropv1alpha1.AggregationSum}},
		},
		Ranking:   &dropv1alpha1.DiscoveryRanking{Strategy: dropv1alpha1.RankingStrategySignal, Signal: &dropv1alpha1.SignalRankingConfig{SignalRef: "s"}},
		MaxImages: 3,
	}

	clientFn := func(_ context.Context, _ string) (*http.Client, error) { return srv.Client(), nil }
	result := ExecutePipeline(context.Background(), spec, clientFn)

	if len(result.Images) != 3 {
		t.Fatalf("expected 3 images (maxImages cap), got %d", len(result.Images))
	}
	for _, img := range result.Images {
		if !img.Selected {
			t.Errorf("image %s should be selected (within cap)", img.Image)
		}
	}
}

// TestExecutePipeline_QueryFailure verifies failed query results are reported correctly.
func TestExecutePipeline_QueryFailure(t *testing.T) {
	spec := dropv1alpha1.DiscoveryPolicySpec{
		Queries: []dropv1alpha1.DiscoveryQuery{
			{Name: "bad-query", Type: dropv1alpha1.DiscoveryQueryTypePrometheus, Prometheus: &dropv1alpha1.DiscoveryPrometheusQuery{Endpoint: "http://127.0.0.1:19999", Query: "test"}},
		},
		Signals: []dropv1alpha1.DiscoverySignal{
			{Name: "s", QueryRef: "bad-query", Type: dropv1alpha1.SignalTypeAggregate, Aggregate: &dropv1alpha1.AggregateSignalConfig{Method: dropv1alpha1.AggregationSum}},
		},
		Ranking:   &dropv1alpha1.DiscoveryRanking{Strategy: dropv1alpha1.RankingStrategySignal, Signal: &dropv1alpha1.SignalRankingConfig{SignalRef: "s"}},
		MaxImages: 10,
	}

	result := ExecutePipeline(context.Background(), spec, nil)

	if len(result.QueryResults) != 1 {
		t.Fatalf("expected 1 query result, got %d", len(result.QueryResults))
	}
	if result.QueryResults[0].Status != dropv1alpha1.QueryResultStatusFailed {
		t.Errorf("expected failed query result, got %s", result.QueryResults[0].Status)
	}
	if len(result.SignalResults) != 1 || result.SignalResults[0].Status != signalStatusFailed {
		t.Errorf("expected failed signal result when query fails")
	}
	if len(result.Images) != 0 {
		t.Errorf("expected no images when query fails, got %d", len(result.Images))
	}
}

// TestExecutePipeline_WindowAggregate verifies the windowAggregate signal type (relative window).
func TestExecutePipeline_WindowAggregate(t *testing.T) {
	now := float64(time.Now().Unix())
	oneHourAgo := now - 3600
	threeHoursAgo := now - 10800

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := prometheusResponse{
			Status: prometheusStatusSuccess,
			Data: struct {
				ResultType string             `json:"resultType"`
				Result     []prometheusResult `json:"result"`
			}{
				ResultType: "matrix",
				Result: []prometheusResult{
					{
						Metric: map[string]string{"image": "nginx:1.25"},
						Values: [][]interface{}{
							{threeHoursAgo, "5"}, // outside 2h window
							{oneHourAgo, "10"},   // inside 2h window
							{now - 600, "15"},    // inside 2h window
						},
					},
				},
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	window := metav1.Duration{Duration: 2 * time.Hour}
	spec := dropv1alpha1.DiscoveryPolicySpec{
		Queries: []dropv1alpha1.DiscoveryQuery{
			{Name: "q", Type: dropv1alpha1.DiscoveryQueryTypePrometheus, Prometheus: &dropv1alpha1.DiscoveryPrometheusQuery{Endpoint: srv.URL, Query: "test", QueryType: dropv1alpha1.QueryTypeRange, Lookback: &metav1.Duration{Duration: 4 * time.Hour}}},
		},
		Signals: []dropv1alpha1.DiscoverySignal{
			{
				Name:     "recent",
				QueryRef: "q",
				Type:     dropv1alpha1.SignalTypeWindowAggregate,
				WindowAggregate: &dropv1alpha1.WindowAggregateSignalConfig{
					Method:         dropv1alpha1.AggregationSum,
					RelativeWindow: &window,
				},
			},
		},
		Ranking:   &dropv1alpha1.DiscoveryRanking{Strategy: dropv1alpha1.RankingStrategySignal, Signal: &dropv1alpha1.SignalRankingConfig{SignalRef: "recent"}},
		MaxImages: 10,
	}

	clientFn := func(_ context.Context, _ string) (*http.Client, error) { return srv.Client(), nil }
	result := ExecutePipeline(context.Background(), spec, clientFn)

	if len(result.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(result.Images))
	}
	// Only the two samples within the 2h window (10 + 15 = 25) should be summed
	if result.Images[0].FinalScore != "25" {
		t.Errorf("expected score 25 (window sum), got %s", result.Images[0].FinalScore)
	}
}

// TestApplyMethod covers all aggregation methods.
func TestApplyMethod(t *testing.T) {
	vals := []float64{10, 20, 30, 5}
	tests := []struct {
		method dropv1alpha1.AggregationMethod
		want   float64
	}{
		{dropv1alpha1.AggregationSum, 65},
		{dropv1alpha1.AggregationCount, 4},
		{dropv1alpha1.AggregationAvg, 16.25},
		{dropv1alpha1.AggregationMax, 30},
		{dropv1alpha1.AggregationMin, 5},
	}
	for _, tt := range tests {
		got := applyMethod(vals, tt.method)
		if got != tt.want {
			t.Errorf("applyMethod(%s) = %v, want %v", tt.method, got, tt.want)
		}
	}
}

// prometheusInstantHandler returns an HTTP handler that serves a fixed instant vector.
func prometheusInstantHandler(imageValues map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		results := make([]prometheusResult, 0, len(imageValues))
		for img, val := range imageValues {
			results = append(results, prometheusResult{
				Metric: map[string]string{"image": img},
				Value:  []interface{}{float64(1000), val},
			})
		}
		resp := prometheusResponse{
			Status: prometheusStatusSuccess,
			Data: struct {
				ResultType string             `json:"resultType"`
				Result     []prometheusResult `json:"result"`
			}{ResultType: "vector", Result: results},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
}
