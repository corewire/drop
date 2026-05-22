package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"
)

// RegistrySource queries OCI registries for image tags.
type RegistrySource struct {
	URL           string
	Repositories  []string
	TagFilter     string
	TopX          int32
	ImageTemplate string
	HTTPClient    *http.Client
}

// NewRegistrySource creates a new registry discovery source.
func NewRegistrySource(url string, repos []string, tagFilter string, topX int32, imageTemplate string, httpClient *http.Client) *RegistrySource {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &RegistrySource{
		URL:           strings.TrimSuffix(url, "/"),
		Repositories:  repos,
		TagFilter:     tagFilter,
		TopX:          topX,
		ImageTemplate: imageTemplate,
		HTTPClient:    httpClient,
	}
}

// tagListResponse represents the OCI Distribution API tag list response.
type tagListResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// Fetch queries the registry for tags and returns discovered images.
func (rs *RegistrySource) Fetch(ctx context.Context) ([]ImageResult, error) {
	var allResults []ImageResult

	for _, repo := range rs.Repositories {
		results, err := rs.fetchRepo(ctx, repo)
		if err != nil {
			return nil, fmt.Errorf("fetching tags for %s: %w", repo, err)
		}
		allResults = append(allResults, results...)
	}

	// Sort by score descending (higher index = more recent)
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	return allResults, nil
}

func (rs *RegistrySource) fetchRepo(ctx context.Context, repo string) ([]ImageResult, error) {
	u := fmt.Sprintf("%s/v2/%s/tags/list", rs.URL, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := rs.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registry returned status %d: %s", resp.StatusCode, string(body))
	}

	var tagList tagListResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagList); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Filter tags
	tags := tagList.Tags
	if rs.TagFilter != "" {
		re, err := regexp.Compile(rs.TagFilter)
		if err != nil {
			return nil, fmt.Errorf("compiling tag filter: %w", err)
		}
		var filtered []string
		for _, tag := range tags {
			if re.MatchString(tag) {
				filtered = append(filtered, tag)
			}
		}
		tags = filtered
	}

	// Limit to topX
	if rs.TopX > 0 && int32(len(tags)) > rs.TopX {
		tags = tags[len(tags)-int(rs.TopX):]
	}

	// Build image refs
	results := make([]ImageResult, 0, len(tags))
	for i, tag := range tags {
		imageRef, err := rs.buildImageRef(repo, tag)
		if err != nil {
			return nil, fmt.Errorf("building image ref for tag %s: %w", tag, err)
		}
		results = append(results, ImageResult{
			Image: imageRef,
			Score: int64(i + 1), // Higher index = more recent
		})
	}

	return results, nil
}

// templateData provides variables for the image template.
type templateData struct {
	Registry   string
	Repository string
	Tag        string
}

func (rs *RegistrySource) buildImageRef(repo, tag string) (string, error) {
	if rs.ImageTemplate != "" {
		tmpl, err := template.New("image").Parse(rs.ImageTemplate)
		if err != nil {
			return "", fmt.Errorf("parsing image template: %w", err)
		}

		data := templateData{
			Registry:   rs.URL,
			Repository: repo,
			Tag:        tag,
		}

		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err != nil {
			return "", fmt.Errorf("executing image template: %w", err)
		}
		return buf.String(), nil
	}

	// Default: registry/repo:tag
	registry := strings.TrimPrefix(rs.URL, "https://")
	registry = strings.TrimPrefix(registry, "http://")
	return fmt.Sprintf("%s/%s:%s", registry, repo, tag), nil
}
