package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistrySource_Fetch(t *testing.T) {
	tests := []struct {
		name          string
		repos         []string
		tagFilter     string
		topX          int32
		imageTemplate string
		tags          []string
		wantCount     int
		wantFirst     string
		wantErr       bool
	}{
		{
			name:      "basic tag listing",
			repos:     []string{"library/nginx"},
			tags:      []string{"1.24", "1.25", "1.26"},
			wantCount: 3,
		},
		{
			name:      "tag filter",
			repos:     []string{"library/nginx"},
			tagFilter: `^1\.2[56]$`,
			tags:      []string{"1.24", "1.25", "1.26"},
			wantCount: 2,
		},
		{
			name:      "topX limit",
			repos:     []string{"library/nginx"},
			topX:      2,
			tags:      []string{"1.24", "1.25", "1.26"},
			wantCount: 2,
		},
		{
			name:          "image template",
			repos:         []string{"gitlab-org/gitlab-runner/gitlab-runner-helper"},
			imageTemplate: "registry.gitlab.com/{{.Repository}}:x86_64-{{.Tag}}",
			tags:          []string{"v16.0", "v16.1"},
			wantCount:     2,
			wantFirst:     "registry.gitlab.com/gitlab-org/gitlab-runner/gitlab-runner-helper:x86_64-v16.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := tagListResponse{
					Name: tt.repos[0],
					Tags: tt.tags,
				}
				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(resp); err != nil {
					t.Fatal(err)
				}
			}))
			defer server.Close()

			source := NewRegistrySource(server.URL, tt.repos, tt.tagFilter, tt.topX, tt.imageTemplate, server.Client())
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
				// Results sorted by score descending, highest score = last tag
				if results[0].Image != tt.wantFirst {
					t.Errorf("first image = %q, want %q", results[0].Image, tt.wantFirst)
				}
			}
		})
	}
}
