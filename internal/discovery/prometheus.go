package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	dropv1alpha1 "github.com/corewire/drop/api/v1alpha1"
)

// PrometheusSource queries Prometheus for image references.
type PrometheusSource struct {
	Endpoint          string
	Query             string
	QueryType         dropv1alpha1.QueryType         // range or instant
	Lookback          time.Duration                  // time window for range queries
	AggregationMethod dropv1alpha1.AggregationMethod // sum, count, avg, max
	Step              string                         // resolution step for range queries (default "5m")
	HTTPClient        *http.Client
}

// NewPrometheusSource creates a new Prometheus discovery source.
func NewPrometheusSource(endpoint, query string, queryType dropv1alpha1.QueryType, lookback time.Duration, aggregationMethod dropv1alpha1.AggregationMethod, step string, httpClient *http.Client) *PrometheusSource {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if step == "" {
		step = "5m"
	}
	if aggregationMethod == "" {
		aggregationMethod = dropv1alpha1.AggregationSum
	}
	if queryType == "" {
		queryType = dropv1alpha1.QueryTypeRange
	}
	return &PrometheusSource{
		Endpoint:          endpoint,
		Query:             query,
		QueryType:         queryType,
		Lookback:          lookback,
		AggregationMethod: aggregationMethod,
		Step:              step,
		HTTPClient:        httpClient,
	}
}

// prometheusResponse represents the Prometheus query API response.
type prometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string             `json:"resultType"`
		Result     []prometheusResult `json:"result"`
	} `json:"data"`
}

type prometheusResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
	Values [][]interface{}   `json:"values"` // for range queries
}

// Fetch queries Prometheus and returns discovered images sorted by score.
func (p *PrometheusSource) Fetch(ctx context.Context) ([]ImageResult, error) {
	u, err := url.Parse(p.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parsing endpoint: %w", err)
	}

	q := u.Query()
	q.Set("query", p.Query)

	if p.QueryType == dropv1alpha1.QueryTypeRange {
		// Range query: aggregate over time window
		u.Path = "/api/v1/query_range"
		now := time.Now().UTC()
		q.Set("start", now.Add(-p.Lookback).Format(time.RFC3339))
		q.Set("end", now.Format(time.RFC3339))
		q.Set("step", p.Step)
	} else {
		// Instant query: single point in time
		u.Path = "/api/v1/query"
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying prometheus: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("prometheus returned status %d: %s", resp.StatusCode, string(body))
	}

	var promResp prometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&promResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if promResp.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed with status: %s", promResp.Status)
	}

	results := make([]ImageResult, 0, len(promResp.Data.Result))
	for _, r := range promResp.Data.Result {
		image, ok := r.Metric["image"]
		if !ok || image == "" {
			continue
		}

		var score int64
		if p.QueryType == dropv1alpha1.QueryTypeRange {
			// Range query: aggregate values according to configured method
			score = aggregateRangeValues(r.Values, p.AggregationMethod)
		} else {
			// Instant query: use single value
			score = extractScore(r.Value)
		}

		results = append(results, ImageResult{
			Image: image,
			Score: score,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// extractScore parses the metric value from a Prometheus instant query result.
func extractScore(value []interface{}) int64 {
	if len(value) < 2 {
		return 0
	}
	strVal, ok := value[1].(string)
	if !ok {
		return 0
	}
	var score float64
	if _, err := fmt.Sscanf(strVal, "%f", &score); err != nil {
		return 0
	}
	return int64(score)
}

// aggregateRangeValues aggregates all values from a query_range result using the specified method.
func aggregateRangeValues(values [][]interface{}, method dropv1alpha1.AggregationMethod) int64 {
	var total float64
	var max float64
	var count int64
	maxSet := false

	for _, pair := range values {
		if len(pair) < 2 {
			continue
		}
		strVal, ok := pair[1].(string)
		if !ok {
			continue
		}
		var v float64
		if _, err := fmt.Sscanf(strVal, "%f", &v); err != nil {
			continue
		}
		total += v
		count++
		if !maxSet || v > max {
			max = v
			maxSet = true
		}
	}

	switch method {
	case dropv1alpha1.AggregationCount:
		return count
	case dropv1alpha1.AggregationAvg:
		if count == 0 {
			return 0
		}
		return int64(total / float64(count))
	case dropv1alpha1.AggregationMax:
		return int64(max)
	default: // AggregationSum
		return int64(total)
	}
}
