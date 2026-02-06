// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package bisect

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/albertocavalcante/bazelbump/flagdb"
	"github.com/albertocavalcante/bazelbump/internal/version"
)

// Pre-compiled regex patterns for version detection.
var (
	commitPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	rcPattern     = regexp.MustCompile(`-rc\d+$|rc\d+$`)
	commitExtract = regexp.MustCompile(`[0-9a-f]{7,40}`)
)

// Granularity specifies the precision level for bisect.
type Granularity int

const (
	// GranularityAuto automatically detects from version format.
	GranularityAuto Granularity = iota
	// GranularityRelease bisects between release versions (7.0.0, 7.1.0).
	GranularityRelease
	// GranularityRC bisects between release candidates (7.0.0-rc1, 7.0.0-rc2).
	GranularityRC
	// GranularityCommit bisects between git commits.
	GranularityCommit
)

func (g Granularity) String() string {
	switch g {
	case GranularityRelease:
		return "release"
	case GranularityRC:
		return "rc"
	case GranularityCommit:
		return "commit"
	default:
		return "auto"
	}
}

// ParseGranularity parses a granularity string.
func ParseGranularity(s string) (Granularity, error) {
	switch strings.ToLower(s) {
	case "auto", "":
		return GranularityAuto, nil
	case "release":
		return GranularityRelease, nil
	case "rc":
		return GranularityRC, nil
	case "commit":
		return GranularityCommit, nil
	default:
		return GranularityAuto, fmt.Errorf("unknown granularity: %s (use: release, rc, commit)", s)
	}
}

// Engine represents a bisect execution engine.
type Engine interface {
	// Name returns the engine name for display.
	Name() string

	// Description returns a human-readable description.
	Description() string

	// IsNative returns true if this is a native engine (not delegated).
	IsNative() bool

	// RequiresNetwork returns true if this engine needs network access.
	RequiresNetwork() bool

	// GetVersions returns versions between from and to (inclusive).
	GetVersions(ctx context.Context, from, to string) ([]string, error)

	// TestVersion runs the test command at a specific version.
	TestVersion(ctx context.Context, ver string, testCmd string, projectPath string, timeout time.Duration) (*TestResult, error)
}

// TestResult contains the result of testing a version.
type TestResult struct {
	Version  string
	Passed   bool
	Output   string
	Duration time.Duration
	ExitCode int
}

// EngineInfo describes an engine for display.
type EngineInfo struct {
	Name            string
	Description     string
	IsNative        bool
	RequiresNetwork bool
	Granularity     Granularity
}

// SelectEngine chooses the appropriate engine based on granularity.
func SelectEngine(granularity Granularity, db *flagdb.DB) (Engine, error) {
	switch granularity {
	case GranularityRelease:
		return &ReleaseEngine{db: db}, nil
	case GranularityRC:
		return &RCEngine{}, nil
	case GranularityCommit:
		return &CommitEngine{}, nil
	default:
		return nil, fmt.Errorf("cannot select engine for granularity: %s", granularity)
	}
}

// DetectGranularity determines granularity from version strings.
func DetectGranularity(from, to string) Granularity {
	// Check if either looks like a commit (7+ hex chars)
	if commitPattern.MatchString(from) || commitPattern.MatchString(to) {
		return GranularityCommit
	}

	// Check if either is an RC
	if rcPattern.MatchString(from) || rcPattern.MatchString(to) {
		return GranularityRC
	}

	// Default to release
	return GranularityRelease
}

// =============================================================================
// Release Engine (Native)
// =============================================================================

// ReleaseEngine bisects between release versions using the flag database.
type ReleaseEngine struct {
	db *flagdb.DB
}

func (e *ReleaseEngine) Name() string {
	return "native (flag database)"
}

func (e *ReleaseEngine) Description() string {
	return "Bisects between Bazel release versions using embedded flag database. Fast and works offline."
}

func (e *ReleaseEngine) IsNative() bool {
	return true
}

func (e *ReleaseEngine) RequiresNetwork() bool {
	return false
}

func (e *ReleaseEngine) GetVersions(ctx context.Context, from, to string) ([]string, error) {
	allVersions := e.db.Versions()

	fromVer, err := version.Parse(from)
	if err != nil {
		return nil, fmt.Errorf("parse from version: %w", err)
	}

	toVer, err := version.Parse(to)
	if err != nil {
		return nil, fmt.Errorf("parse to version: %w", err)
	}

	var result []string
	for _, v := range allVersions {
		if v.Compare(fromVer) < 0 || v.Compare(toVer) > 0 {
			continue
		}
		// Skip prereleases for release-level bisect
		if v.IsPrerelease() {
			continue
		}
		result = append(result, v.String())
	}

	return result, nil
}

func (e *ReleaseEngine) TestVersion(ctx context.Context, ver string, testCmd string, projectPath string, timeout time.Duration) (*TestResult, error) {
	return runBazelTest(ctx, ver, testCmd, projectPath, timeout)
}

// =============================================================================
// RC Engine (Native, requires network)
// =============================================================================

// RCEngine bisects between release candidates using GitHub releases API.
type RCEngine struct {
	token string // GitHub token (optional, avoids rate limiting)
}

func (e *RCEngine) Name() string {
	return "native (GitHub releases)"
}

func (e *RCEngine) Description() string {
	return "Bisects between release candidates by fetching RC versions from GitHub releases API."
}

func (e *RCEngine) IsNative() bool {
	return true
}

func (e *RCEngine) RequiresNetwork() bool {
	return true
}

func (e *RCEngine) GetVersions(ctx context.Context, from, to string) ([]string, error) {
	// Fetch releases from GitHub
	releases, err := e.fetchGitHubReleases(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch GitHub releases: %w", err)
	}

	// Parse from/to versions
	fromVer, err := version.Parse(from)
	if err != nil {
		return nil, fmt.Errorf("parse from version: %w", err)
	}
	toVer, err := version.Parse(to)
	if err != nil {
		return nil, fmt.Errorf("parse to version: %w", err)
	}

	// Filter to range
	var result []string
	for _, r := range releases {
		v, err := version.Parse(r)
		if err != nil {
			continue
		}
		if v.Compare(fromVer) < 0 || v.Compare(toVer) > 0 {
			continue
		}
		result = append(result, r)
	}

	// Sort by version
	sort.Slice(result, func(i, j int) bool {
		vi, _ := version.Parse(result[i])
		vj, _ := version.Parse(result[j])
		return vi.Compare(vj) < 0
	})

	return result, nil
}

func (e *RCEngine) fetchGitHubReleases(ctx context.Context) ([]string, error) {
	url := "https://api.github.com/repos/bazelbuild/bazel/releases?per_page=100"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Use token if available
	if e.token == "" {
		e.token = os.Getenv("GITHUB_TOKEN")
		if e.token == "" {
			e.token = os.Getenv("BAZELISK_GITHUB_TOKEN")
		}
	}
	if e.token != "" {
		req.Header.Set("Authorization", "token "+e.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	var versions []string
	for _, r := range releases {
		versions = append(versions, r.TagName)
	}

	return versions, nil
}

func (e *RCEngine) TestVersion(ctx context.Context, ver string, testCmd string, projectPath string, timeout time.Duration) (*TestResult, error) {
	return runBazelTest(ctx, ver, testCmd, projectPath, timeout)
}

// =============================================================================
// Commit Engine (Delegated to bazelisk)
// =============================================================================

// CommitEngine bisects between commits by delegating to bazelisk.
type CommitEngine struct{}

func (e *CommitEngine) Name() string {
	return "bazelisk (delegated)"
}

func (e *CommitEngine) Description() string {
	return "Delegates to bazelisk --bisect for commit-level precision. Requires bazelisk and GitHub API access."
}

func (e *CommitEngine) IsNative() bool {
	return false
}

func (e *CommitEngine) RequiresNetwork() bool {
	return true
}

func (e *CommitEngine) GetVersions(ctx context.Context, from, to string) ([]string, error) {
	// For commit engine, we don't enumerate versions ourselves
	// bazelisk handles this internally
	return []string{from, to}, nil
}

func (e *CommitEngine) TestVersion(ctx context.Context, ver string, testCmd string, projectPath string, timeout time.Duration) (*TestResult, error) {
	// This shouldn't be called directly for commit engine
	// Instead, use RunBazeliskBisect
	return runBazelTest(ctx, ver, testCmd, projectPath, timeout)
}

// RunBazeliskBisect runs bazelisk's native bisect for commit-level precision.
func RunBazeliskBisect(ctx context.Context, from, to string, testCmd string, projectPath string, onProgress func(string)) (*CommitBisectResult, error) {
	// Build the bazelisk command
	bisectArg := fmt.Sprintf("--bisect=%s..%s", from, to)
	args := append([]string{bisectArg}, strings.Fields(testCmd)...)

	cmd := exec.CommandContext(ctx, "bazelisk", args...)
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Parse the output to extract results
	result := parseCommitBisectOutput(outputStr)

	if err != nil && result.FirstBadCommit == "" {
		return nil, fmt.Errorf("bazelisk bisect failed: %w\nOutput: %s", err, outputStr)
	}

	result.RawOutput = outputStr
	return result, nil
}

// CommitBisectResult contains the results of commit-level bisect.
type CommitBisectResult struct {
	FirstBadCommit  string
	LastGoodCommit  string
	CommitMessage   string
	CommitURL       string
	StepsTaken      int
	RawOutput       string
}

func parseCommitBisectOutput(output string) *CommitBisectResult {
	result := &CommitBisectResult{}

	// Parse bazelisk output format:
	// "--- Failed at abc123"
	// "--- Succeeded at def456"
	// At the end, it prints the first bad commit

	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if strings.Contains(line, "first bad commit") || strings.Contains(line, "First bad") {
			// Try to extract commit from next line or same line
			if matches := commitExtract.FindString(line); matches != "" {
				result.FirstBadCommit = matches
			} else if i+1 < len(lines) {
				if matches := commitExtract.FindString(lines[i+1]); matches != "" {
					result.FirstBadCommit = matches
				}
			}
		}
		if strings.Contains(line, "Succeeded at") || strings.Contains(line, "Failed at") {
			result.StepsTaken++
		}
	}

	if result.FirstBadCommit != "" {
		result.CommitURL = fmt.Sprintf("https://github.com/bazelbuild/bazel/commit/%s", result.FirstBadCommit)
	}

	return result
}

// =============================================================================
// Common Helpers
// =============================================================================

func runBazelTest(ctx context.Context, ver string, testCmd string, projectPath string, timeout time.Duration) (*TestResult, error) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", testCmd)
	cmd.Dir = projectPath
	cmd.Env = append(os.Environ(), "USE_BAZEL_VERSION="+ver)

	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return &TestResult{
		Version:  ver,
		Passed:   err == nil,
		Output:   string(output),
		Duration: duration,
		ExitCode: exitCode,
	}, nil
}
