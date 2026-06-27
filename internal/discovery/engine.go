package discovery

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	dropv1alpha1 "github.com/corewire/drop/api/v1alpha1"
)

const signalStatusFailed = "failed"

// QueryRawData holds raw per-image samples from a single query execution.
// For prometheus range queries each image may have multiple samples.
// For prometheus instant and registry queries each image has exactly one sample.
type QueryRawData struct {
	// Samples maps image reference → ordered list of (timestamp, value) pairs.
	// Timestamp is Unix seconds; value is the numeric sample value.
	Samples map[string][]TimedSample
	// QueryType is the DiscoveryQueryType that produced this data.
	QueryType dropv1alpha1.DiscoveryQueryType
}

// TimedSample pairs a Unix timestamp (seconds) with a float64 value.
type TimedSample struct {
	Timestamp float64
	Value     float64
}

// PipelineResult is the output of a full pipeline execution.
type PipelineResult struct {
	QueryResults  []dropv1alpha1.QueryResult
	SignalResults []dropv1alpha1.SignalResult
	Images        []dropv1alpha1.DiscoveredImage
}

// HTTPClientFunc builds an HTTP client for a query (used by the controller to inject auth/TLS).
type HTTPClientFunc func(ctx context.Context, queryName string) (*http.Client, error)

// scoredItem is an intermediate ranked image used during the ranking stage.
type scoredItem struct {
	image   string
	score   float64
	signals []dropv1alpha1.ImageSignalValue
	ranking *dropv1alpha1.ImageRankingDetail
}

// ExecutePipeline runs all stages of the discovery pipeline and returns a PipelineResult.
//
// queryHTTPClient is called once per query to obtain an HTTP client with appropriate
// auth/TLS configuration. Pass nil to use a plain default client for every query.
func ExecutePipeline(
	ctx context.Context,
	spec dropv1alpha1.DiscoveryPolicySpec,
	queryHTTPClient HTTPClientFunc,
) PipelineResult {
	if queryHTTPClient == nil {
		queryHTTPClient = func(_ context.Context, _ string) (*http.Client, error) {
			return &http.Client{Timeout: 30 * time.Second}, nil
		}
	}

	// ──────────────────────────────────────────────────────────
	// Stage 1 — Execute queries
	// ──────────────────────────────────────────────────────────
	rawByQuery := make(map[string]*QueryRawData, len(spec.Queries))
	qResults := make([]dropv1alpha1.QueryResult, 0, len(spec.Queries))

	for _, q := range spec.Queries {
		httpClient, err := queryHTTPClient(ctx, q.Name)
		if err != nil {
			qResults = append(qResults, dropv1alpha1.QueryResult{
				Name:    q.Name,
				Type:    q.Type,
				Status:  dropv1alpha1.QueryResultStatusFailed,
				Message: fmt.Sprintf("building HTTP client: %v", err),
			})
			continue
		}

		raw, qr := executeQuery(ctx, q, httpClient)
		qResults = append(qResults, qr)
		if raw != nil {
			rawByQuery[q.Name] = raw
		}
	}

	// ──────────────────────────────────────────────────────────
	// Stage 2 — Derive signals
	// ──────────────────────────────────────────────────────────
	signalValues := make(map[string]map[string]float64, len(spec.Signals))
	sResults := make([]dropv1alpha1.SignalResult, 0, len(spec.Signals))

	for _, sig := range spec.Signals {
		raw, ok := rawByQuery[sig.QueryRef]
		if !ok {
			sResults = append(sResults, dropv1alpha1.SignalResult{
				Name:    sig.Name,
				Status:  signalStatusFailed,
				Message: fmt.Sprintf("query %q did not produce results (query failed or missing)", sig.QueryRef),
			})
			continue
		}

		values, sr := deriveSignal(sig, raw)
		sResults = append(sResults, sr)
		if values != nil {
			signalValues[sig.Name] = values
		}
	}

	// ──────────────────────────────────────────────────────────
	// Stage 3 — Rank images
	// ──────────────────────────────────────────────────────────
	allImages := collectImages(rawByQuery)

	// Apply image filter
	if spec.ImageFilter != "" {
		re, err := regexp.Compile(spec.ImageFilter)
		if err == nil {
			var filtered []string
			for _, img := range allImages {
				if re.MatchString(img) {
					filtered = append(filtered, img)
				}
			}
			allImages = filtered
		}
	}

	discovered := rankImages(spec.Ranking, signalValues, allImages)

	// Apply maxImages cap; mark selected
	maxImages := int(spec.MaxImages)
	if maxImages <= 0 {
		maxImages = 50
	}
	for i := range discovered {
		discovered[i].Selected = i < maxImages
	}
	if len(discovered) > maxImages {
		discovered = discovered[:maxImages]
	}

	return PipelineResult{
		QueryResults:  qResults,
		SignalResults: sResults,
		Images:        discovered,
	}
}

// executeQuery fetches raw data for a single DiscoveryQuery.
func executeQuery(ctx context.Context, q dropv1alpha1.DiscoveryQuery, httpClient *http.Client) (*QueryRawData, dropv1alpha1.QueryResult) {
	qr := dropv1alpha1.QueryResult{Name: q.Name, Type: q.Type}

	switch q.Type {
	case dropv1alpha1.DiscoveryQueryTypePrometheus:
		if q.Prometheus == nil {
			qr.Status = dropv1alpha1.QueryResultStatusFailed
			qr.Message = "prometheus config is required when type=prometheus"
			return nil, qr
		}
		raw, err := executePrometheusQuery(ctx, q.Prometheus, httpClient)
		if err != nil {
			qr.Status = dropv1alpha1.QueryResultStatusFailed
			qr.Message = err.Error()
			return nil, qr
		}
		total := countSamples(raw.Samples)
		series := int32(len(raw.Samples))
		qr.Series = &series
		qr.Samples = &total
		qr.Status = dropv1alpha1.QueryResultStatusSuccess
		return raw, qr

	case dropv1alpha1.DiscoveryQueryTypeRegistry:
		if q.Registry == nil {
			qr.Status = dropv1alpha1.QueryResultStatusFailed
			qr.Message = "registry config is required when type=registry"
			return nil, qr
		}
		raw, err := executeRegistryQuery(ctx, q.Registry, httpClient)
		if err != nil {
			qr.Status = dropv1alpha1.QueryResultStatusFailed
			qr.Message = err.Error()
			return nil, qr
		}
		series := int32(len(raw.Samples))
		qr.Series = &series
		qr.Status = dropv1alpha1.QueryResultStatusSuccess
		return raw, qr

	case dropv1alpha1.DiscoveryQueryTypeLoki:
		qr.Status = dropv1alpha1.QueryResultStatusFailed
		qr.Message = "loki query execution is not yet implemented"
		return nil, qr

	default:
		qr.Status = dropv1alpha1.QueryResultStatusFailed
		qr.Message = fmt.Sprintf("unsupported query type: %s", q.Type)
		return nil, qr
	}
}

// executePrometheusQuery runs a Prometheus range or instant query and returns raw samples.
func executePrometheusQuery(ctx context.Context, cfg *dropv1alpha1.DiscoveryPrometheusQuery, httpClient *http.Client) (*QueryRawData, error) {
	var lookback time.Duration
	if cfg.Lookback != nil {
		lookback = cfg.Lookback.Duration
	}
	var step time.Duration
	if cfg.Step != nil {
		step = cfg.Step.Duration
	}

	src := NewPrometheusSource(cfg.Endpoint, cfg.Query, cfg.QueryType, lookback, nil, step, httpClient)
	results, err := src.FetchRaw(ctx)
	if err != nil {
		return nil, err
	}

	raw := &QueryRawData{
		Samples:   results,
		QueryType: dropv1alpha1.DiscoveryQueryTypePrometheus,
	}
	return raw, nil
}

// executeRegistryQuery lists tags from an OCI registry and returns raw samples.
func executeRegistryQuery(ctx context.Context, cfg *dropv1alpha1.DiscoveryRegistryQuery, httpClient *http.Client) (*QueryRawData, error) {
	src := NewRegistrySource(cfg.URL, cfg.Repositories, cfg.TagFilter, cfg.TopX, cfg.ImageTemplate, httpClient)
	results, err := src.Fetch(ctx)
	if err != nil {
		return nil, err
	}

	raw := &QueryRawData{
		Samples:   make(map[string][]TimedSample, len(results)),
		QueryType: dropv1alpha1.DiscoveryQueryTypeRegistry,
	}
	now := float64(time.Now().Unix())
	for _, r := range results {
		raw.Samples[r.Image] = []TimedSample{{Timestamp: now, Value: float64(r.Score)}}
	}
	return raw, nil
}

// deriveSignal computes per-image float64 values for a single signal.
func deriveSignal(sig dropv1alpha1.DiscoverySignal, raw *QueryRawData) (map[string]float64, dropv1alpha1.SignalResult) {
	sr := dropv1alpha1.SignalResult{Name: sig.Name}

	switch sig.Type {
	case dropv1alpha1.SignalTypeAggregate:
		if sig.Aggregate == nil {
			sr.Status = signalStatusFailed
			sr.Message = "aggregate config is required when type=aggregate"
			return nil, sr
		}
		values := aggregateSamples(raw.Samples, sig.Aggregate.Method, nil)
		sr.Images = int32(len(values))
		sr.Status = "success"
		return values, sr

	case dropv1alpha1.SignalTypeTimeWeightedAggregate:
		if sig.TimeWeightedAggregate == nil {
			sr.Status = signalStatusFailed
			sr.Message = "timeWeightedAggregate config is required when type=timeWeightedAggregate"
			return nil, sr
		}
		values, err := deriveTimeWeightedAggregate(raw.Samples, sig.TimeWeightedAggregate)
		if err != nil {
			sr.Status = signalStatusFailed
			sr.Message = err.Error()
			return nil, sr
		}
		sr.Images = int32(len(values))
		sr.Status = "success"
		return values, sr

	case dropv1alpha1.SignalTypeWindowAggregate:
		if sig.WindowAggregate == nil {
			sr.Status = signalStatusFailed
			sr.Message = "windowAggregate config is required when type=windowAggregate"
			return nil, sr
		}
		values, err := deriveWindowAggregate(raw.Samples, sig.WindowAggregate)
		if err != nil {
			sr.Status = signalStatusFailed
			sr.Message = err.Error()
			return nil, sr
		}
		sr.Images = int32(len(values))
		sr.Status = "success"
		return values, sr

	case dropv1alpha1.SignalTypeEventPullTime:
		sr.Status = signalStatusFailed
		sr.Message = "eventPullTime signal derivation is not yet implemented"
		return nil, sr

	default:
		sr.Status = signalStatusFailed
		sr.Message = fmt.Sprintf("unsupported signal type: %s", sig.Type)
		return nil, sr
	}
}

// aggregateSamples applies an AggregationMethod to per-image sample lists.
// cutoffUnix, when non-nil, excludes samples with timestamp < cutoffUnix.
func aggregateSamples(samples map[string][]TimedSample, method dropv1alpha1.AggregationMethod, cutoffUnix *float64) map[string]float64 {
	out := make(map[string]float64, len(samples))
	for image, pts := range samples {
		vals := make([]float64, 0, len(pts))
		for _, pt := range pts {
			if cutoffUnix != nil && pt.Timestamp < *cutoffUnix {
				continue
			}
			vals = append(vals, pt.Value)
		}
		if len(vals) == 0 {
			continue
		}
		out[image] = applyMethod(vals, method)
	}
	return out
}

// applyMethod applies a single AggregationMethod to a non-empty slice of values.
func applyMethod(vals []float64, method dropv1alpha1.AggregationMethod) float64 {
	switch method {
	case dropv1alpha1.AggregationCount:
		return float64(len(vals))
	case dropv1alpha1.AggregationAvg:
		var sum float64
		for _, v := range vals {
			sum += v
		}
		return sum / float64(len(vals))
	case dropv1alpha1.AggregationMax:
		m := vals[0]
		for _, v := range vals[1:] {
			if v > m {
				m = v
			}
		}
		return m
	case dropv1alpha1.AggregationMin:
		m := vals[0]
		for _, v := range vals[1:] {
			if v < m {
				m = v
			}
		}
		return m
	default: // sum
		var s float64
		for _, v := range vals {
			s += v
		}
		return s
	}
}

// deriveTimeWeightedAggregate applies per-hour weights before aggregating.
func deriveTimeWeightedAggregate(samples map[string][]TimedSample, cfg *dropv1alpha1.TimeWeightedAggregateSignalConfig) (map[string]float64, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("loading timezone %q: %w", cfg.Timezone, err)
	}

	defaultWeightQ := cfg.DefaultWeight.AsApproximateFloat64()

	out := make(map[string]float64, len(samples))
	for image, pts := range samples {
		var weighted []float64
		for _, pt := range pts {
			t := time.Unix(int64(pt.Timestamp), 0).In(loc)
			hour := int32(t.Hour())

			w := defaultWeightQ
			for _, win := range cfg.Windows {
				if hour >= win.StartHour && hour < win.EndHour {
					w = win.Weight.AsApproximateFloat64()
					break
				}
			}
			weighted = append(weighted, pt.Value*w)
		}
		if len(weighted) == 0 {
			continue
		}
		out[image] = applyMethod(weighted, cfg.Method)
	}
	return out, nil
}

// deriveWindowAggregate aggregates only samples in a specific time window.
func deriveWindowAggregate(samples map[string][]TimedSample, cfg *dropv1alpha1.WindowAggregateSignalConfig) (map[string]float64, error) {
	now := time.Now().UTC()

	var cutoff *float64
	var windowEnd *float64

	if cfg.RelativeWindow != nil {
		c := float64(now.Add(-cfg.RelativeWindow.Duration).Unix())
		cutoff = &c
	} else if cfg.Window != nil {
		if cfg.Timezone == "" {
			return nil, fmt.Errorf("timezone is required when window is set")
		}
		loc, err := time.LoadLocation(cfg.Timezone)
		if err != nil {
			return nil, fmt.Errorf("loading timezone %q: %w", cfg.Timezone, err)
		}
		startT, err := parseTimeOfDay(cfg.Window.Start, now.In(loc))
		if err != nil {
			return nil, fmt.Errorf("parsing window start: %w", err)
		}
		endT, err := parseTimeOfDay(cfg.Window.End, now.In(loc))
		if err != nil {
			return nil, fmt.Errorf("parsing window end: %w", err)
		}
		c := float64(startT.Unix())
		e := float64(endT.Unix())
		cutoff = &c
		windowEnd = &e
	}

	out := make(map[string]float64, len(samples))
	for image, pts := range samples {
		vals := make([]float64, 0, len(pts))
		for _, pt := range pts {
			if cutoff != nil && pt.Timestamp < *cutoff {
				continue
			}
			if windowEnd != nil && pt.Timestamp > *windowEnd {
				continue
			}
			vals = append(vals, pt.Value)
		}
		if len(vals) == 0 {
			continue
		}
		out[image] = applyMethod(vals, cfg.Method)
	}
	return out, nil
}

// parseTimeOfDay parses a "HH:MM" time string relative to a reference day.
func parseTimeOfDay(hhmm string, ref time.Time) (time.Time, error) {
	parts := strings.SplitN(hhmm, ":", 2)
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid time format %q (want HH:MM)", hhmm)
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil {
		return time.Time{}, fmt.Errorf("invalid time format %q (want HH:MM)", hhmm)
	}
	return time.Date(ref.Year(), ref.Month(), ref.Day(), h, m, 0, 0, ref.Location()), nil
}

// rankImages converts per-signal values into an ordered DiscoveredImage slice.
func rankImages(ranking *dropv1alpha1.DiscoveryRanking, signals map[string]map[string]float64, images []string) []dropv1alpha1.DiscoveredImage {
	if ranking == nil || len(images) == 0 {
		// No ranking configured: return images in alphabetical order with score 0.
		out := make([]dropv1alpha1.DiscoveredImage, len(images))
		for i, img := range images {
			out[i] = dropv1alpha1.DiscoveredImage{Image: img, Rank: int32(i + 1), FinalScore: "0"}
		}
		return out
	}

	var items []scoredItem

	switch ranking.Strategy {
	case dropv1alpha1.RankingStrategySignal:
		ref := ""
		if ranking.Signal != nil {
			ref = ranking.Signal.SignalRef
		}
		sigMap := signals[ref]
		for _, img := range images {
			v := sigMap[img]
			items = append(items, scoredItem{
				image: img,
				score: v,
				signals: []dropv1alpha1.ImageSignalValue{{
					Name:     ref,
					RawValue: strconv.FormatFloat(v, 'f', -1, 64),
				}},
				ranking: &dropv1alpha1.ImageRankingDetail{Strategy: string(ranking.Strategy)},
			})
		}

	case dropv1alpha1.RankingStrategyWeightedSum:
		if ranking.WeightedSum != nil {
			items = weightedSumRank(ranking.WeightedSum, signals, images)
		}

	case dropv1alpha1.RankingStrategyModelExposure:
		if ranking.ModelExposure != nil {
			items = modelExposureRank(ranking.ModelExposure, signals, images)
		}

	default:
		// Unknown strategy: score 0
		for _, img := range images {
			items = append(items, scoredItem{image: img})
		}
	}

	// Sort descending by score, then alphabetically for stability
	sort.Slice(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return items[i].image < items[j].image
	})

	out := make([]dropv1alpha1.DiscoveredImage, len(items))
	for i, it := range items {
		out[i] = dropv1alpha1.DiscoveredImage{
			Image:      it.image,
			Rank:       int32(i + 1),
			FinalScore: strconv.FormatFloat(it.score, 'f', -1, 64),
			Signals:    it.signals,
			Ranking:    it.ranking,
		}
	}
	return out
}

// weightedSumRank computes Score = Σ weight_k * normalize(signal_k(image)).
func weightedSumRank(cfg *dropv1alpha1.WeightedSumRankingConfig, signals map[string]map[string]float64, images []string) []scoredItem {
	// Compute min/max per signal for minMax normalization
	type minMax struct{ min, max float64 }
	bounds := make(map[string]minMax, len(cfg.Terms))
	for _, term := range cfg.Terms {
		sigMap := signals[term.SignalRef]
		var mn, mx float64
		first := true
		for _, img := range images {
			v, ok := sigMap[img]
			if !ok {
				continue
			}
			if first || v < mn {
				mn = v
			}
			if first || v > mx {
				mx = v
			}
			first = false
		}
		bounds[term.SignalRef] = minMax{min: mn, max: mx}
	}

	normalize := func(v float64, b minMax) float64 {
		if b.max == b.min {
			return 1.0
		}
		return (v - b.min) / (b.max - b.min)
	}

	var out []scoredItem
	for _, img := range images {
		var totalScore float64
		sigVals := make([]dropv1alpha1.ImageSignalValue, 0, len(cfg.Terms))
		terms := make([]dropv1alpha1.RankingTerm, 0, len(cfg.Terms))

		drop := false
		for _, term := range cfg.Terms {
			sigMap := signals[term.SignalRef]
			v, ok := sigMap[img]
			if !ok {
				if cfg.MissingSignal == dropv1alpha1.MissingSignalBehaviorDrop {
					drop = true
					break
				}
				v = 0
			}
			b := bounds[term.SignalRef]
			norm := normalize(v, b)
			wf := term.Weight.AsApproximateFloat64()
			contribution := wf * norm
			totalScore += contribution

			sigVals = append(sigVals, dropv1alpha1.ImageSignalValue{
				Name:            term.SignalRef,
				RawValue:        strconv.FormatFloat(v, 'f', -1, 64),
				NormalizedValue: strconv.FormatFloat(norm, 'f', -1, 64),
			})
			terms = append(terms, dropv1alpha1.RankingTerm{
				Signal:       term.SignalRef,
				Weight:       term.Weight.String(),
				Contribution: strconv.FormatFloat(contribution, 'f', -1, 64),
			})
		}
		if drop {
			continue
		}
		out = append(out, scoredItem{
			image:   img,
			score:   totalScore,
			signals: sigVals,
			ranking: &dropv1alpha1.ImageRankingDetail{
				Strategy: string(dropv1alpha1.RankingStrategyWeightedSum),
				Terms:    terms,
			},
		})
	}
	return out
}

// modelExposureRank computes Score = J_target * (1 - 1/N)^J_pre * p_hat.
func modelExposureRank(cfg *dropv1alpha1.ModelExposureRankingConfig, signals map[string]map[string]float64, images []string) []scoredItem {
	n := float64(cfg.NodeCount)
	if n < 1 {
		n = 1
	}
	oneMinusInvN := 1.0 - 1.0/n

	preMap := signals[cfg.PreWindowUsageSignalRef]
	targetMap := signals[cfg.TargetWindowUsageSignalRef]
	pullMap := signals[cfg.PullTimeSignalRef]

	out := make([]scoredItem, 0, len(images))
	for _, img := range images {
		jPre := preMap[img]
		jTarget := targetMap[img]
		pHat := pullMap[img]

		score := jTarget * math.Pow(oneMinusInvN, jPre) * pHat

		out = append(out, scoredItem{
			image: img,
			score: score,
			signals: []dropv1alpha1.ImageSignalValue{
				{Name: cfg.PreWindowUsageSignalRef, RawValue: strconv.FormatFloat(jPre, 'f', -1, 64)},
				{Name: cfg.TargetWindowUsageSignalRef, RawValue: strconv.FormatFloat(jTarget, 'f', -1, 64)},
				{Name: cfg.PullTimeSignalRef, RawValue: strconv.FormatFloat(pHat, 'f', -1, 64)},
			},
			ranking: &dropv1alpha1.ImageRankingDetail{
				Strategy: string(dropv1alpha1.RankingStrategyModelExposure),
			},
		})
	}
	return out
}

// collectImages returns a sorted, deduplicated list of all image references across all query results.
func collectImages(rawByQuery map[string]*QueryRawData) []string {
	seen := make(map[string]struct{})
	for _, raw := range rawByQuery {
		for img := range raw.Samples {
			seen[img] = struct{}{}
		}
	}
	images := make([]string, 0, len(seen))
	for img := range seen {
		images = append(images, img)
	}
	sort.Strings(images)
	return images
}

// countSamples returns the total number of samples across all images.
func countSamples(samples map[string][]TimedSample) int64 {
	var total int64
	for _, pts := range samples {
		total += int64(len(pts))
	}
	return total
}
