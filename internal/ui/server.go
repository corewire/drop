/*
Copyright (c) 2026 Breee

SPDX-License-Identifier: MIT
*/

// Package ui provides the HTTP server for the Drop Control Center UI.
package ui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dropv1alpha1 "github.com/Breee/drop/api/v1alpha1"
)

//go:embed static
var staticFiles embed.FS

// NodeSummary is a simplified node for the UI.
type NodeSummary struct {
	Name   string            `json:"name"`
	Ready  bool              `json:"ready"`
	Labels map[string]string `json:"labels"`
	Arch   string            `json:"arch,omitempty"`
	OS     string            `json:"os,omitempty"`
}

// CachedImageSummary is a simplified CachedImage for the UI.
type CachedImageSummary struct {
	Name          string   `json:"name"`
	Image         string   `json:"image"`
	Tag           string   `json:"tag,omitempty"`
	Digest        string   `json:"digest,omitempty"`
	Phase         string   `json:"phase"`
	Ready         string   `json:"ready"`
	NodesReady    int32    `json:"nodesReady"`
	NodesTargeted int32    `json:"nodesTargeted"`
	NodesPulling  int32    `json:"nodesPulling"`
	CachedNodes   []string `json:"cachedNodes"`
	SetName       string   `json:"setName,omitempty"`
	PolicyRef     string   `json:"policyRef,omitempty"`
	Age           string   `json:"age"`
}

// CachedImageSetSummary is a simplified CachedImageSet for the UI.
type CachedImageSetSummary struct {
	Name          string `json:"name"`
	Phase         string `json:"phase"`
	ImagesManaged int32  `json:"imagesManaged"`
	ImagesReady   int32  `json:"imagesReady"`
	DiscoveryRef  string `json:"discoveryRef,omitempty"`
	Age           string `json:"age"`
}

// DiscoveredImageEntry is a single image from a discovery source.
type DiscoveredImageEntry struct {
	Image  string `json:"image"`
	Score  int64  `json:"score"`
	Source string `json:"source"`
}

// DiscoveryPolicySummary is a simplified DiscoveryPolicy for the UI.
type DiscoveryPolicySummary struct {
	Name             string                 `json:"name"`
	Phase            string                 `json:"phase"`
	ImageCount       int32                  `json:"imageCount"`
	SourceCount      int32                  `json:"sourceCount"`
	SyncInterval     string                 `json:"syncInterval,omitempty"`
	MaxImages        int32                  `json:"maxImages"`
	LastSync         string                 `json:"lastSync,omitempty"`
	DiscoveredImages []DiscoveredImageEntry `json:"discoveredImages,omitempty"`
	Spec             interface{}            `json:"spec"`
	Age              string                 `json:"age"`
}

// StatusSummary provides overall cluster-level status for the UI.
type StatusSummary struct {
	TotalNodes     int `json:"totalNodes"`
	ReadyNodes     int `json:"readyNodes"`
	TotalImages    int `json:"totalImages"`
	ReadyImages    int `json:"readyImages"`
	PullingImages  int `json:"pullingImages"`
	PendingImages  int `json:"pendingImages"`
	DegradedImages int `json:"degradedImages"`
	TotalSets      int `json:"totalSets"`
	TotalPolicies  int `json:"totalPolicies"`
}

// FullPayload is the combined response for SSE updates.
type FullPayload struct {
	Nodes            []NodeSummary            `json:"nodes"`
	CachedImages     []CachedImageSummary     `json:"cachedImages"`
	CachedImageSets  []CachedImageSetSummary  `json:"cachedImageSets"`
	DiscoveryPolicies []DiscoveryPolicySummary `json:"discoveryPolicies"`
	Status           StatusSummary            `json:"status"`
	Timestamp        string                   `json:"timestamp"`
}

// Server is the Drop UI HTTP server.
type Server struct {
	client   client.Client
	pollInterval time.Duration
}

// NewServer creates a new UI server backed by the provided Kubernetes client.
func NewServer(c client.Client, pollInterval time.Duration) *Server {
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second
	}
	return &Server{client: c, pollInterval: pollInterval}
}

// Handler returns the HTTP handler for the UI server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(fmt.Sprintf("ui: failed to sub static FS: %v", err))
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	mux.HandleFunc("/api/v1/nodes", s.withCORS(s.handleNodes))
	mux.HandleFunc("/api/v1/cachedimages", s.withCORS(s.handleCachedImages))
	mux.HandleFunc("/api/v1/cachedimagesets", s.withCORS(s.handleCachedImageSets))
	mux.HandleFunc("/api/v1/discoverypolicies", s.withCORS(s.handleDiscoveryPolicies))
	mux.HandleFunc("/api/v1/status", s.withCORS(s.handleStatus))
	mux.HandleFunc("/api/v1/all", s.withCORS(s.handleAll))
	mux.HandleFunc("/events", s.handleSSE)

	return mux
}

func (s *Server) withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
	}
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.fetchNodes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, nodes)
}

func (s *Server) handleCachedImages(w http.ResponseWriter, r *http.Request) {
	images, err := s.fetchCachedImages(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, images)
}

func (s *Server) handleCachedImageSets(w http.ResponseWriter, r *http.Request) {
	sets, err := s.fetchCachedImageSets(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sets)
}

func (s *Server) handleDiscoveryPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := s.fetchDiscoveryPolicies(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, policies)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	payload, err := s.buildFullPayload(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, payload.Status)
}

func (s *Server) handleAll(w http.ResponseWriter, r *http.Request) {
	payload, err := s.buildFullPayload(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, payload)
}

// handleSSE streams live updates to the browser via Server-Sent Events.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	logger := log.FromContext(r.Context()).WithName("ui-sse")

	send := func() {
		payload, err := s.buildFullPayload(r.Context())
		if err != nil {
			logger.V(1).Info("SSE fetch error", "error", err)
			return
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	send() // immediate first event

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

// --- Kubernetes fetch helpers ---

func (s *Server) fetchNodes(ctx context.Context) ([]NodeSummary, error) {
	var nodeList corev1.NodeList
	if err := s.client.List(ctx, &nodeList); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	result := make([]NodeSummary, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		n := &nodeList.Items[i]
		ready := false
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		result = append(result, NodeSummary{
			Name:   n.Name,
			Ready:  ready,
			Labels: n.Labels,
			Arch:   n.Status.NodeInfo.Architecture,
			OS:     n.Status.NodeInfo.OperatingSystem,
		})
	}
	return result, nil
}

func (s *Server) fetchCachedImages(ctx context.Context) ([]CachedImageSummary, error) {
	var list dropv1alpha1.CachedImageList
	if err := s.client.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("list cachedimages: %w", err)
	}

	result := make([]CachedImageSummary, 0, len(list.Items))
	for i := range list.Items {
		ci := &list.Items[i]
		setName := ci.Labels["drop.corewire.io/imageset"]
		policyRef := ""
		if ci.Spec.PolicyRef != nil {
			policyRef = ci.Spec.PolicyRef.Name
		}
		cachedNodes := ci.Status.CachedNodes
		if cachedNodes == nil {
			cachedNodes = []string{}
		}
		result = append(result, CachedImageSummary{
			Name:          ci.Name,
			Image:         ci.Spec.Image,
			Tag:           ci.Spec.Tag,
			Digest:        ci.Spec.Digest,
			Phase:         ci.Status.Phase,
			Ready:         ci.Status.Ready,
			NodesReady:    ci.Status.NodesReady,
			NodesTargeted: ci.Status.NodesTargeted,
			NodesPulling:  ci.Status.NodesPulling,
			CachedNodes:   cachedNodes,
			SetName:       setName,
			PolicyRef:     policyRef,
			Age:           formatAge(ci.CreationTimestamp.Time),
		})
	}
	return result, nil
}

func (s *Server) fetchCachedImageSets(ctx context.Context) ([]CachedImageSetSummary, error) {
	var list dropv1alpha1.CachedImageSetList
	if err := s.client.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("list cachedimageset: %w", err)
	}

	result := make([]CachedImageSetSummary, 0, len(list.Items))
	for i := range list.Items {
		cis := &list.Items[i]
		discRef := ""
		if cis.Spec.DiscoveryPolicyRef != nil {
			discRef = cis.Spec.DiscoveryPolicyRef.Name
		}
		result = append(result, CachedImageSetSummary{
			Name:          cis.Name,
			Phase:         cis.Status.Phase,
			ImagesManaged: cis.Status.ImagesManaged,
			ImagesReady:   cis.Status.ImagesReady,
			DiscoveryRef:  discRef,
			Age:           formatAge(cis.CreationTimestamp.Time),
		})
	}
	return result, nil
}

func (s *Server) fetchDiscoveryPolicies(ctx context.Context) ([]DiscoveryPolicySummary, error) {
	var list dropv1alpha1.DiscoveryPolicyList
	if err := s.client.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("list discoverypolicies: %w", err)
	}

	result := make([]DiscoveryPolicySummary, 0, len(list.Items))
	for i := range list.Items {
		dp := &list.Items[i]

		phase := ""
		for _, c := range dp.Status.Conditions {
			if c.Type == "Ready" {
				phase = c.Reason
				break
			}
		}

		lastSync := ""
		if dp.Status.LastSyncTime != nil {
			lastSync = formatAge(dp.Status.LastSyncTime.Time)
		}

		discovered := make([]DiscoveredImageEntry, 0, len(dp.Status.DiscoveredImages))
		for _, img := range dp.Status.DiscoveredImages {
			discovered = append(discovered, DiscoveredImageEntry{
				Image:  img.Image,
				Score:  img.Score,
				Source: img.Source,
			})
		}

		result = append(result, DiscoveryPolicySummary{
			Name:             dp.Name,
			Phase:            phase,
			ImageCount:       dp.Status.ImageCount,
			SourceCount:      dp.Status.SourceCount,
			SyncInterval:     dp.Spec.SyncInterval.Duration.String(),
			MaxImages:        dp.Spec.MaxImages,
			LastSync:         lastSync,
			DiscoveredImages: discovered,
			Spec:             dp.Spec,
			Age:              formatAge(dp.CreationTimestamp.Time),
		})
	}
	return result, nil
}

func (s *Server) buildFullPayload(ctx context.Context) (*FullPayload, error) {
	nodes, err := s.fetchNodes(ctx)
	if err != nil {
		return nil, err
	}
	images, err := s.fetchCachedImages(ctx)
	if err != nil {
		return nil, err
	}
	sets, err := s.fetchCachedImageSets(ctx)
	if err != nil {
		return nil, err
	}
	policies, err := s.fetchDiscoveryPolicies(ctx)
	if err != nil {
		return nil, err
	}

	status := StatusSummary{
		TotalNodes:    len(nodes),
		TotalImages:   len(images),
		TotalSets:     len(sets),
		TotalPolicies: len(policies),
	}
	for _, n := range nodes {
		if n.Ready {
			status.ReadyNodes++
		}
	}
	for _, img := range images {
		switch img.Phase {
		case "Ready":
			status.ReadyImages++
		case "Pulling":
			status.PullingImages++
		case "Pending":
			status.PendingImages++
		case "Degraded":
			status.DegradedImages++
		}
	}

	return &FullPayload{
		Nodes:             nodes,
		CachedImages:      images,
		CachedImageSets:   sets,
		DiscoveryPolicies: policies,
		Status:            status,
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// formatAge returns a human-readable duration since t.
func formatAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
