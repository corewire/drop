package discovery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	dropv1alpha1 "github.com/corewire/drop/api/v1alpha1"
)

// TestLokiSource_FetchRaw_Generic verifies the generic (non-parser) FetchRaw path,
// which produces value=1.0 samples keyed by the "image" stream label.
func TestLokiSource_FetchRaw_Generic(t *testing.T) {
	now := time.Now()
	streams := []lokiStream{
		{
			Stream: map[string]string{"image": "nginx:1.25"},
			Values: [][]string{
				{nanoStringLoki(now.Add(-2 * time.Second)), "log line 1"},
				{nanoStringLoki(now.Add(-1 * time.Second)), "log line 2"},
			},
		},
		{
			Stream: map[string]string{"image": "redis:7.0"},
			Values: [][]string{
				{nanoStringLoki(now), "log line 3"},
			},
		},
		{
			// no image label → should be skipped
			Stream: map[string]string{"app": "kubelet"},
			Values: [][]string{
				{nanoStringLoki(now), "unrelated line"},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := lokiResponse{
			Status: lokiStatusSuccess,
			Data:   lokiData{ResultType: "streams", Result: streams},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	src := NewLokiSource(srv.URL, `{app="test"}`, time.Hour, nil, srv.Client())
	samples, err := src.FetchRaw(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples["nginx:1.25"]) != 2 {
		t.Errorf("expected 2 samples for nginx:1.25, got %d", len(samples["nginx:1.25"]))
	}
	if len(samples["redis:7.0"]) != 1 {
		t.Errorf("expected 1 sample for redis:7.0, got %d", len(samples["redis:7.0"]))
	}
	for _, s := range samples["nginx:1.25"] {
		if s.Value != 1.0 {
			t.Errorf("expected generic sample value 1.0, got %f", s.Value)
		}
	}
}

// TestLokiSource_FetchRaw_KubernetesEvents verifies the kubernetesEvents parser
// with message-based duration extraction and eventPair fallback.
func TestLokiSource_FetchRaw_KubernetesEvents(t *testing.T) {
	now := time.Now()
	streams := []lokiStream{
		{
			Stream: map[string]string{
				"reason":              "Pulling",
				"involvedObject_name": "pod-abc",
				"message":             `Pulling image "nginx:1.25"`,
			},
			Values: [][]string{{nanoStringLoki(now.Add(-3 * time.Second)), ""}},
		},
		{
			Stream: map[string]string{
				"reason":              "Pulled",
				"involvedObject_name": "pod-abc",
				"message":             `Successfully pulled image "nginx:1.25" in 2.5s (2.5s including waiting)`,
			},
			Values: [][]string{{nanoStringLoki(now.Add(-500 * time.Millisecond)), ""}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := lokiResponse{
			Status: lokiStatusSuccess,
			Data:   lokiData{ResultType: "streams", Result: streams},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	src := NewLokiSource(srv.URL, `{app="kubelet"}`, time.Hour, &dropv1alpha1.LokiParser{
		Type:         dropv1alpha1.LokiParserTypeKubernetesEvents,
		ReasonField:  "reason",
		PodField:     "involvedObject_name",
		MessageField: "message",
	}, srv.Client())
	samples, err := src.FetchRaw(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect one duration sample for nginx:1.25 (2.5s from message)
	if len(samples["nginx:1.25"]) != 1 {
		t.Fatalf("expected 1 sample for nginx:1.25, got %d", len(samples["nginx:1.25"]))
	}
	if got := samples["nginx:1.25"][0].Value; got != 2.5 {
		t.Errorf("expected duration 2.5s, got %f", got)
	}
}

// TestLokiSource_FetchRaw_KubernetesEvents_EventPair verifies that when no duration
// is present in the message, the Pulling→Pulled timestamp delta is used.
func TestLokiSource_FetchRaw_KubernetesEvents_EventPair(t *testing.T) {
	now := time.Now()
	pullingTime := now.Add(-3 * time.Second)
	pulledTime := now.Add(-1 * time.Second)

	streams := []lokiStream{
		{
			Stream: map[string]string{
				"reason":              "Pulling",
				"involvedObject_name": "pod-xyz",
				"message":             `Pulling image "alpine:3.19"`,
			},
			Values: [][]string{{nanoStringLoki(pullingTime), ""}},
		},
		{
			Stream: map[string]string{
				"reason":              "Pulled",
				"involvedObject_name": "pod-xyz",
				"message":             `Successfully pulled image "alpine:3.19"`, // no duration
			},
			Values: [][]string{{nanoStringLoki(pulledTime), ""}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := lokiResponse{
			Status: lokiStatusSuccess,
			Data:   lokiData{ResultType: "streams", Result: streams},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	src := NewLokiSource(srv.URL, `{app="kubelet"}`, time.Hour, &dropv1alpha1.LokiParser{
		Type:         dropv1alpha1.LokiParserTypeKubernetesEvents,
		ReasonField:  "reason",
		PodField:     "involvedObject_name",
		MessageField: "message",
	}, srv.Client())
	samples, err := src.FetchRaw(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(samples["alpine:3.19"]) != 1 {
		t.Fatalf("expected 1 sample for alpine:3.19, got %d", len(samples["alpine:3.19"]))
	}
	// eventPair duration ≈ 2 seconds (pulledTime - pullingTime)
	got := samples["alpine:3.19"][0].Value
	if got < 1.9 || got > 2.1 {
		t.Errorf("expected eventPair duration ~2s, got %f", got)
	}
}

// TestLokiSource_FetchRaw_KubernetesEvents_AlloyJSON verifies that events shipped by
// Grafana Alloy (loki.source.kubernetes_events, log_format=json) parse with the default
// parser fields. Alloy emits "msg"/"name" in the JSON body, not "message"/"involvedObject_name".
func TestLokiSource_FetchRaw_KubernetesEvents_AlloyJSON(t *testing.T) {
	now := time.Now()
	streams := []lokiStream{
		{
			Stream: map[string]string{"namespace": "default", "job": "kubelet"},
			Values: [][]string{{nanoStringLoki(now.Add(-2 * time.Second)),
				`{"reason":"Pulled","name":"runner-abc","msg":"Successfully pulled image \"nginx:1.25\" in 740ms (740ms including waiting). Image size: 20461242 bytes."}`}},
		},
		{
			Stream: map[string]string{"namespace": "default", "job": "kubelet"},
			Values: [][]string{{nanoStringLoki(now.Add(-1 * time.Second)),
				`{"reason":"Failed","name":"runner-def","msg":"Failed to pull image \"broken:v1\": not found"}`}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := lokiResponse{Status: lokiStatusSuccess, Data: lokiData{ResultType: "streams", Result: streams}}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// Default parser fields (no msg/name overrides) — relies on alias fallback.
	src := NewLokiSource(srv.URL, `{job="kubelet"}`, time.Hour, &dropv1alpha1.LokiParser{
		Type: dropv1alpha1.LokiParserTypeKubernetesEvents,
	}, srv.Client())
	samples, err := src.FetchRaw(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples["nginx:1.25"]) != 1 {
		t.Fatalf("expected 1 sample for nginx:1.25, got %d", len(samples["nginx:1.25"]))
	}
	if got := samples["nginx:1.25"][0].Value; got < 0.73 || got > 0.75 {
		t.Errorf("expected ~0.74s duration, got %f", got)
	}
	if len(samples["nginx:1.25"+lokiSizeBytesSuffix]) != 1 {
		t.Fatalf("expected 1 size sample for nginx:1.25, got %d", len(samples["nginx:1.25"+lokiSizeBytesSuffix]))
	}
	if got := samples["nginx:1.25"+lokiSizeBytesSuffix][0].Value; got != 20461242 {
		t.Errorf("expected image size 20461242, got %f", got)
	}
	if len(samples["broken:v1"+lokiFailedSuffix]) != 1 {
		t.Errorf("expected 1 failure sample for broken:v1, got %d", len(samples["broken:v1"+lokiFailedSuffix]))
	}
}

// TestLokiSource_FetchRaw_HTTPError verifies that HTTP errors are surfaced.
func TestLokiSource_FetchRaw_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := NewLokiSource(srv.URL, `{app="test"}`, time.Hour, nil, srv.Client())
	_, err := src.FetchRaw(t.Context())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestLokiInferReasonFromMessage verifies the plain-text reason inference.
func TestLokiInferReasonFromMessage(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{`Successfully pulled image "nginx:1.25" in 2s`, "Pulled"},
		{`Pulling image "nginx:1.25"`, "Pulling"},
		{`Failed to pull image "nginx:1.25": not found`, "Failed"},
		{`Back-off pulling image "nginx:1.25"`, "Backoff"},
		{`Container image "nginx:1.25" already present on machine`, "AlreadyPresent"},
		{`some unrelated log line`, ""},
	}
	for _, tt := range tests {
		got := lokiInferReasonFromMessage(tt.msg)
		if got != tt.want {
			t.Errorf("msg=%q: got %q, want %q", tt.msg, got, tt.want)
		}
	}
}

// TestLokiParsePullDuration verifies duration parsing from event messages.
func TestLokiParsePullDuration(t *testing.T) {
	tests := []struct {
		msg  string
		want float64
	}{
		{`Successfully pulled image "nginx:1.25" in 2.5s`, 2.5},
		{`Successfully pulled image "nginx:1.25" in 500ms`, 0.5},
		{`Successfully pulled image "nginx:1.25" in 1m`, 60},
		{`Successfully pulled image "nginx:1.25" in 1h`, 3600},
		{`Successfully pulled image "nginx:1.25"`, 0}, // no duration
	}
	for _, tt := range tests {
		got := lokiParsePullDuration(tt.msg)
		if got != tt.want {
			t.Errorf("msg=%q: got %f, want %f", tt.msg, got, tt.want)
		}
	}
}

// TestLokiParseImageSizeBytes verifies image size parsing from Pulled event messages.
func TestLokiParseImageSizeBytes(t *testing.T) {
	tests := []struct {
		msg  string
		want float64
	}{
		{`Successfully pulled image "nginx:1.25" in 2.5s. Image size: 20461242 bytes.`, 20461242},
		{`Successfully pulled image "redis:7" in 1s (1s including waiting). image size: 123 bytes.`, 123},
		{`Successfully pulled image "alpine:3.19" in 800ms`, 0},
		{`Image size: bad bytes`, 0},
	}
	for _, tt := range tests {
		got := lokiParseImageSizeBytes(tt.msg)
		if got != tt.want {
			t.Errorf("msg=%q: got %f, want %f", tt.msg, got, tt.want)
		}
	}
}

// nanoStringLoki formats a time as a nanosecond epoch string for Loki responses.
func nanoStringLoki(t time.Time) string {
	return strconv.FormatInt(t.UnixNano(), 10)
}
