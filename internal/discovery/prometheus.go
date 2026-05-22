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
)

// PrometheusSource queries Prometheus for image references.
type PrometheusSource struct {
	Endpoint   string
	Query      string
	HTTPClient *http.Client
}

// NewPrometheusSource creates a new Prometheus discovery source.
func NewPrometheusSource(endpoint, query string, httpClient *http.Client) *PrometheusSource {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &PrometheusSource{
		Endpoint:   endpoint,
		Query:      query,
		HTTPClient: httpClient,
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
}

// Fetch queries Prometheus and returns discovered images sorted by score.
func (p *PrometheusSource) Fetch(ctx context.Context) ([]ImageResult, error) {
	u, err := url.Parse(p.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parsing endpoint: %w", err)
	}
	u.Path = "/api/v1/query"
	q := u.Query()
	q.Set("query", p.Query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying prometheus: %w", err)
	}
	defer resp.Body.Close()

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

	var results []ImageResult
	for _, r := range promResp.Data.Result {
		image, ok := r.Metric["image"]
		if !ok || image == "" {
			continue
		}

		score := extractScore(r.Value)
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
