// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package bisect

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/albertocavalcante/bazelbump/flagdb"
	"github.com/albertocavalcante/bazelbump/internal/github"
	"github.com/albertocavalcante/bazelbump/internal/netconfig"
	"github.com/albertocavalcante/bazelbump/internal/version"
)

// ValidationError contains details about why a version is invalid.
type ValidationError struct {
	Version     string
	Granularity Granularity
	Reason      string
	Suggestions []string
	// HelpText provides additional context (e.g., about network mode)
	HelpText string
}

func (e *ValidationError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "invalid version %q: %s", e.Version, e.Reason)
	if len(e.Suggestions) > 0 {
		sb.WriteString("\n\nDid you mean one of these?\n")
		for _, s := range e.Suggestions {
			fmt.Fprintf(&sb, "  - %s\n", s)
		}
	}
	if e.HelpText != "" {
		sb.WriteString("\n")
		sb.WriteString(e.HelpText)
	}
	return sb.String()
}

// ValidationResult contains the result of version validation.
type ValidationResult struct {
	Valid bool
	// ExistsOnGitHub is true if version was found via GitHub API (not in local DB)
	ExistsOnGitHub bool
	// HasFlagData is true if we have flag data for this version
	HasFlagData bool
	// Warning contains non-fatal warnings (e.g., "no flag data available")
	Warning string
}

// Validator validates Bazel versions with air-gap awareness.
type Validator struct {
	db       *flagdb.DB
	netCfg   *netconfig.Config
	ghClient *github.Client
}

// NewValidator creates a new version validator.
func NewValidator(db *flagdb.DB, netCfg *netconfig.Config) *Validator {
	return &Validator{
		db:       db,
		netCfg:   netCfg,
		ghClient: github.NewClient(netCfg),
	}
}

// ValidateVersions validates both from and to versions for the given granularity.
// Returns warnings that should be displayed to the user.
func (v *Validator) ValidateVersions(ctx context.Context, from, to string, granularity Granularity) ([]string, error) {
	var warnings []string

	fromResult, err := v.ValidateVersion(ctx, from, granularity)
	if err != nil {
		return nil, fmt.Errorf("--from: %w", err)
	}
	if fromResult.Warning != "" {
		warnings = append(warnings, fmt.Sprintf("--from: %s", fromResult.Warning))
	}

	toResult, err := v.ValidateVersion(ctx, to, granularity)
	if err != nil {
		return nil, fmt.Errorf("--to: %w", err)
	}
	if toResult.Warning != "" {
		warnings = append(warnings, fmt.Sprintf("--to: %s", toResult.Warning))
	}

	// Validate from < to
	if granularity == GranularityRelease || granularity == GranularityRC {
		fromVer, err1 := version.Parse(from)
		toVer, err2 := version.Parse(to)
		if err1 == nil && err2 == nil {
			if fromVer.Compare(toVer) >= 0 {
				return nil, fmt.Errorf("--from version (%s) must be less than --to version (%s)", from, to)
			}
		}
	}

	return warnings, nil
}

// ValidateVersion validates a single version for the given granularity.
func (v *Validator) ValidateVersion(ctx context.Context, ver string, granularity Granularity) (*ValidationResult, error) {
	switch granularity {
	case GranularityRelease:
		return v.validateReleaseVersion(ctx, ver)
	case GranularityRC:
		return v.validateRCVersion(ctx, ver)
	case GranularityCommit:
		return v.validateCommitVersion(ver)
	default:
		return &ValidationResult{Valid: true}, nil
	}
}

// validateReleaseVersion checks if a release version exists.
// First checks local flag database, then optionally GitHub if online.
func (v *Validator) validateReleaseVersion(ctx context.Context, ver string) (*ValidationResult, error) {
	// Check local flag database first
	if v.db.HasVersion(ver) {
		return &ValidationResult{
			Valid:       true,
			HasFlagData: true,
		}, nil
	}

	// Parse the version to understand what the user meant
	parsed, err := version.Parse(ver)
	if err != nil {
		return nil, &ValidationError{
			Version:     ver,
			Granularity: GranularityRelease,
			Reason:      fmt.Sprintf("cannot parse version: %v", err),
			Suggestions: getAvailableMajorVersions(v.db),
		}
	}

	// If online mode, try GitHub fallback
	if v.netCfg.IsOnline() {
		exists, err := v.ghClient.VersionExists(ctx, ver)
		if err != nil {
			// Network error - provide helpful message but don't fail
			suggestions := findSimilarVersions(parsed, v.db.Versions(), 5)
			return nil, &ValidationError{
				Version:     ver,
				Granularity: GranularityRelease,
				Reason:      "version not found in local flag database",
				Suggestions: suggestions,
				HelpText:    fmt.Sprintf("Note: GitHub lookup failed: %v", err),
			}
		}

		if exists {
			// Version exists on GitHub but not in our local DB
			return &ValidationResult{
				Valid:          true,
				ExistsOnGitHub: true,
				HasFlagData:    false,
				Warning: fmt.Sprintf("version %s exists but flag data not available locally (bisect will work, but no flag diff)", ver),
			}, nil
		}

		// Version doesn't exist anywhere
		suggestions := findSimilarVersions(parsed, v.db.Versions(), 5)
		return nil, &ValidationError{
			Version:     ver,
			Granularity: GranularityRelease,
			Reason:      "version does not exist (confirmed via GitHub)",
			Suggestions: suggestions,
		}
	}

	// Offline mode - can only check local DB
	suggestions := findSimilarVersions(parsed, v.db.Versions(), 5)
	return nil, &ValidationError{
		Version:     ver,
		Granularity: GranularityRelease,
		Reason:      "version not found in local flag database",
		Suggestions: suggestions,
		HelpText:    "Tip: Use --online to check if this version exists on GitHub",
	}
}

// validateRCVersion checks if an RC version is valid.
func (v *Validator) validateRCVersion(ctx context.Context, ver string) (*ValidationResult, error) {
	parsed, err := version.Parse(ver)
	if err != nil {
		return nil, &ValidationError{
			Version:     ver,
			Granularity: GranularityRC,
			Reason:      fmt.Sprintf("cannot parse version: %v", err),
		}
	}

	// If online mode, verify RC exists on GitHub
	if v.netCfg.IsOnline() {
		exists, err := v.ghClient.VersionExists(ctx, ver)
		if err != nil {
			// Network error - allow to proceed with warning
			return &ValidationResult{
				Valid:   true,
				Warning: fmt.Sprintf("could not verify RC via GitHub: %v", err),
			}, nil
		}

		if !exists {
			return nil, &ValidationError{
				Version:     ver,
				Granularity: GranularityRC,
				Reason:      "RC version does not exist (confirmed via GitHub)",
				HelpText:    "Check https://github.com/bazelbuild/bazel/releases for available RCs",
			}
		}

		return &ValidationResult{
			Valid:          true,
			ExistsOnGitHub: true,
		}, nil
	}

	// Offline mode - do basic format validation only
	if !parsed.IsPrerelease() {
		// It's a release version, check base exists in local DB
		baseVersion := fmt.Sprintf("%d.%d.%d", parsed.Major(), parsed.Minor(), parsed.Patch())
		if !v.db.HasVersion(baseVersion) {
			suggestions := findSimilarVersions(parsed, v.db.Versions(), 5)
			return nil, &ValidationError{
				Version:     ver,
				Granularity: GranularityRC,
				Reason:      fmt.Sprintf("base version %s not found in local database", baseVersion),
				Suggestions: suggestions,
				HelpText:    "Tip: Use --online to verify RC versions via GitHub",
			}
		}
	}

	return &ValidationResult{
		Valid:   true,
		Warning: "RC version not verified (offline mode)",
	}, nil
}

// validateCommitVersion validates a commit SHA format.
func (v *Validator) validateCommitVersion(ver string) (*ValidationResult, error) {
	// Check if it looks like a commit SHA or a version tag
	if isCommitSHA(ver) {
		if len(ver) < 7 {
			return nil, &ValidationError{
				Version:     ver,
				Granularity: GranularityCommit,
				Reason:      "commit SHA must be at least 7 characters",
			}
		}
		return &ValidationResult{Valid: true}, nil
	}

	// It might be a version tag like "7.0.0" which bazelisk can resolve
	_, err := version.Parse(ver)
	if err != nil {
		return nil, &ValidationError{
			Version:     ver,
			Granularity: GranularityCommit,
			Reason:      "must be a commit SHA (7+ hex chars) or version tag",
		}
	}

	return &ValidationResult{Valid: true}, nil
}

// =============================================================================
// Legacy functions for backward compatibility
// =============================================================================

// ValidateVersions validates versions using default offline config.
// Deprecated: Use Validator.ValidateVersions for air-gap aware validation.
func ValidateVersions(ctx context.Context, from, to string, granularity Granularity, db *flagdb.DB) error {
	v := NewValidator(db, netconfig.Default())
	_, err := v.ValidateVersions(ctx, from, to, granularity)
	return err
}

// ValidateVersion validates a single version using default offline config.
// Deprecated: Use Validator.ValidateVersion for air-gap aware validation.
func ValidateVersion(ctx context.Context, ver string, granularity Granularity, db *flagdb.DB) error {
	v := NewValidator(db, netconfig.Default())
	_, err := v.ValidateVersion(ctx, ver, granularity)
	return err
}

// =============================================================================
// Helper functions
// =============================================================================

// isCommitSHA checks if a string looks like a git commit SHA.
func isCommitSHA(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// findSimilarVersions finds versions similar to the target.
func findSimilarVersions(target version.Version, available []version.Version, limit int) []string {
	type scored struct {
		ver   string
		score int
	}

	var candidates []scored

	for _, v := range available {
		score := versionSimilarity(target, v)
		candidates = append(candidates, scored{v.String(), score})
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Take top N
	var result []string
	for i := 0; i < len(candidates) && i < limit; i++ {
		result = append(result, candidates[i].ver)
	}

	return result
}

// versionSimilarity scores how similar two versions are.
func versionSimilarity(a, b version.Version) int {
	score := 0

	// Same major = 100 points
	if a.Major() == b.Major() {
		score += 100
	} else {
		// Close major = some points
		diff := abs(a.Major() - b.Major())
		if diff <= 1 {
			score += 50
		}
	}

	// Same minor = 50 points
	if a.Minor() == b.Minor() {
		score += 50
	} else {
		diff := abs(a.Minor() - b.Minor())
		if diff <= 2 {
			score += 25
		}
	}

	// Same patch = 25 points
	if a.Patch() == b.Patch() {
		score += 25
	}

	return score
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// getAvailableMajorVersions returns a summary of available major versions.
func getAvailableMajorVersions(db *flagdb.DB) []string {
	versions := db.Versions()
	majors := make(map[int]version.Version)

	// Get the latest version for each major
	for _, v := range versions {
		if v.IsPrerelease() {
			continue
		}
		if existing, ok := majors[v.Major()]; !ok || v.Compare(existing) > 0 {
			majors[v.Major()] = v
		}
	}

	// Sort majors
	var majorNums []int
	for m := range majors {
		majorNums = append(majorNums, m)
	}
	sort.Ints(majorNums)

	// Format as strings
	var result []string
	for _, m := range majorNums {
		result = append(result, majors[m].String())
	}

	return result
}

// ListAvailableVersions returns all available versions for a granularity.
func ListAvailableVersions(ctx context.Context, granularity Granularity, db *flagdb.DB) ([]string, error) {
	switch granularity {
	case GranularityRelease:
		versions := db.Versions()
		var result []string
		for _, v := range versions {
			if !v.IsPrerelease() {
				result = append(result, v.String())
			}
		}
		return result, nil

	case GranularityRC:
		// Would fetch from GitHub releases API
		// For now, return error suggesting to check GitHub
		return nil, fmt.Errorf("RC versions are fetched from GitHub releases; check https://github.com/bazelbuild/bazel/releases")

	case GranularityCommit:
		return nil, fmt.Errorf("commit versions are git SHAs; use 'git log' in the Bazel repository")

	default:
		return nil, fmt.Errorf("unknown granularity: %s", granularity)
	}
}

// FormatAvailableVersions formats available versions for display.
func FormatAvailableVersions(versions []string) string {
	if len(versions) == 0 {
		return "No versions available"
	}

	var sb strings.Builder
	sb.WriteString("Available versions:\n")

	// Group by major version
	groups := make(map[int][]string)
	for _, v := range versions {
		parsed, err := version.Parse(v)
		if err != nil {
			continue
		}
		groups[parsed.Major()] = append(groups[parsed.Major()], v)
	}

	// Sort majors
	var majors []int
	for m := range groups {
		majors = append(majors, m)
	}
	sort.Ints(majors)

	for _, major := range majors {
		vers := groups[major]
		fmt.Fprintf(&sb, "\n  %d.x: ", major)
		// Show first and last few
		if len(vers) <= 5 {
			sb.WriteString(strings.Join(vers, ", "))
		} else {
			sb.WriteString(vers[0])
			sb.WriteString(" ... ")
			sb.WriteString(strings.Join(vers[len(vers)-3:], ", "))
		}
	}

	return sb.String()
}
