package discovery

import "context"

// ImageResult represents a discovered image with a ranking score.
type ImageResult struct {
	Image string
	Score int64
}

// Source is the interface that all discovery backends must implement.
type Source interface {
	// Fetch queries the backend and returns discovered images.
	Fetch(ctx context.Context) ([]ImageResult, error)
}
