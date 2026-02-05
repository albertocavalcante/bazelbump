// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package github provides GitHub API access for bazelbump.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/albertocavalcante/bazelbump/internal/netconfig"
)

const (
	bazelReleasesURL = "https://api.github.com/repos/bazelbuild/bazel/releases"
	requestTimeout   = 10 * time.Second
)

// Client provides GitHub API access with network mode awareness.
type Client struct {
	config *netconfig.Config
	client *http.Client

	// Cache for releases (in-memory, session-scoped)
	mu             sync.RWMutex
	cachedReleases []Release
	cacheTime      time.Time
	cacheTTL       time.Duration
}

// Release represents a GitHub release.
type Release struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// NewClient creates a new GitHub client.
func NewClient(cfg *netconfig.Config) *Client {
	return &Client{
		config: cfg,
		client: &http.Client{
			Timeout: requestTimeout,
		},
		cacheTTL: 5 * time.Minute,
	}
}

// VersionExists checks if a version exists as a GitHub release.
// Returns (exists, error). Error is nil even if version doesn't exist.
// Returns error only for network/API failures.
func (c *Client) VersionExists(ctx context.Context, version string) (bool, error) {
	if c.config.IsOffline() {
		return false, fmt.Errorf("network access disabled (offline mode)")
	}

	releases, err := c.fetchReleases(ctx)
	if err != nil {
		return false, err
	}

	for _, r := range releases {
		if r.TagName == version {
			return true, nil
		}
	}

	return false, nil
}

// GetReleases returns all Bazel releases from GitHub.
func (c *Client) GetReleases(ctx context.Context) ([]Release, error) {
	if c.config.IsOffline() {
		return nil, fmt.Errorf("network access disabled (offline mode)")
	}

	return c.fetchReleases(ctx)
}

// fetchReleases fetches releases with caching.
func (c *Client) fetchReleases(ctx context.Context) ([]Release, error) {
	// Check cache
	c.mu.RLock()
	if c.cachedReleases != nil && time.Since(c.cacheTime) < c.cacheTTL {
		releases := c.cachedReleases
		c.mu.RUnlock()
		return releases, nil
	}
	c.mu.RUnlock()

	// Fetch from GitHub
	releases, err := c.doFetchReleases(ctx)
	if err != nil {
		return nil, err
	}

	// Update cache
	c.mu.Lock()
	c.cachedReleases = releases
	c.cacheTime = time.Now()
	c.mu.Unlock()

	return releases, nil
}

func (c *Client) doFetchReleases(ctx context.Context) ([]Release, error) {
	var allReleases []Release
	const maxPages = 3 // Limit to avoid too many requests

	for page := 1; page <= maxPages; page++ {
		releases, err := c.fetchReleasesPage(ctx, page)
		if err != nil {
			return nil, err
		}
		if len(releases) == 0 {
			break
		}
		allReleases = append(allReleases, releases...)
	}

	return allReleases, nil
}

// fetchReleasesPage fetches a single page of releases.
func (c *Client) fetchReleasesPage(ctx context.Context, page int) ([]Release, error) {
	url := fmt.Sprintf("%s?per_page=100&page=%d", bazelReleasesURL, page)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if c.config.GitHubToken != "" {
		req.Header.Set("Authorization", "token "+c.config.GitHubToken)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "bazelbump")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		// Drain body before closing
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("GitHub API rate limit exceeded (set GITHUB_TOKEN to increase limit)")
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}

	return releases, nil
}
