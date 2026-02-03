// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package version provides Bazel version parsing and comparison.
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a parsed Bazel version.
// Immutable after construction - use Parse() to create.
type Version struct {
	major      int
	minor      int
	patch      int
	prerelease string // e.g., "rc1", "pre.20210101"
	raw        string
}

// Parse parses a version string like "7.0.0", "8.1.0-rc1", "9.0.0".
func Parse(s string) (Version, error) {
	if s == "" {
		return Version{}, fmt.Errorf("empty version string")
	}

	v := Version{raw: s}

	// Split off prerelease suffix
	base := s
	if idx := strings.IndexAny(s, "-+"); idx != -1 {
		base = s[:idx]
		v.prerelease = s[idx+1:]
	}

	// Parse major.minor.patch
	parts := strings.Split(base, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return Version{}, fmt.Errorf("invalid version format: %q", s)
	}

	var err error
	v.major, err = strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %q", parts[0])
	}

	if len(parts) >= 2 {
		v.minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return Version{}, fmt.Errorf("invalid minor version: %q", parts[1])
		}
	}

	if len(parts) >= 3 {
		v.patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return Version{}, fmt.Errorf("invalid patch version: %q", parts[2])
		}
	}

	return v, nil
}

// MustParse parses a version string, panicking on error.
// Use only for compile-time constants.
func MustParse(s string) Version {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

// String returns the original version string.
func (v Version) String() string {
	return v.raw
}

// Major returns the major version number.
func (v Version) Major() int { return v.major }

// Minor returns the minor version number.
func (v Version) Minor() int { return v.minor }

// Patch returns the patch version number.
func (v Version) Patch() int { return v.patch }

// Prerelease returns the prerelease suffix (empty if stable).
func (v Version) Prerelease() string { return v.prerelease }

// IsPrerelease returns true if this is a prerelease version.
func (v Version) IsPrerelease() bool { return v.prerelease != "" }

// Compare returns -1 if v < other, 0 if v == other, 1 if v > other.
func (v Version) Compare(other Version) int {
	if v.major != other.major {
		return cmp(v.major, other.major)
	}
	if v.minor != other.minor {
		return cmp(v.minor, other.minor)
	}
	if v.patch != other.patch {
		return cmp(v.patch, other.patch)
	}
	// Prerelease versions sort before stable
	// e.g., 7.0.0-rc1 < 7.0.0
	if v.prerelease == "" && other.prerelease != "" {
		return 1
	}
	if v.prerelease != "" && other.prerelease == "" {
		return -1
	}
	// Both have prerelease, compare lexically
	return strings.Compare(v.prerelease, other.prerelease)
}

// Less returns true if v < other.
func (v Version) Less(other Version) bool {
	return v.Compare(other) < 0
}

// Equal returns true if v == other.
func (v Version) Equal(other Version) bool {
	return v.Compare(other) == 0
}

// AtLeast returns true if v >= other.
func (v Version) AtLeast(other Version) bool {
	return v.Compare(other) >= 0
}

// InRange returns true if min <= v <= max.
func (v Version) InRange(min, max Version) bool {
	return v.Compare(min) >= 0 && v.Compare(max) <= 0
}

func cmp(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// Range represents a version range for compatibility checks.
type Range struct {
	min      Version
	max      Version
	minIncl  bool
	maxIncl  bool
	hasMin   bool
	hasMax   bool
	operator string // original operator like ">=", "~"
}

// NewRange creates a version range from an operator and version.
// Supported operators: >, >=, <, <=, ==, !=, ~ (tilde range)
func NewRange(op string, v Version) Range {
	r := Range{operator: op}
	switch op {
	case ">":
		r.min, r.hasMin, r.minIncl = v, true, false
	case ">=":
		r.min, r.hasMin, r.minIncl = v, true, true
	case "<":
		r.max, r.hasMax, r.maxIncl = v, true, false
	case "<=":
		r.max, r.hasMax, r.maxIncl = v, true, true
	case "==":
		r.min, r.max = v, v
		r.hasMin, r.hasMax = true, true
		r.minIncl, r.maxIncl = true, true
	case "~":
		// Tilde range: ~1.2.3 means >=1.2.3 <1.3.0
		r.min = v
		r.hasMin, r.minIncl = true, true
		r.max = Version{major: v.major, minor: v.minor + 1, raw: fmt.Sprintf("%d.%d.0", v.major, v.minor+1)}
		r.hasMax, r.maxIncl = true, false
	}
	return r
}

// Contains returns true if the version is within this range.
func (r Range) Contains(v Version) bool {
	if r.hasMin {
		cmp := v.Compare(r.min)
		if r.minIncl && cmp < 0 {
			return false
		}
		if !r.minIncl && cmp <= 0 {
			return false
		}
	}
	if r.hasMax {
		cmp := v.Compare(r.max)
		if r.maxIncl && cmp > 0 {
			return false
		}
		if !r.maxIncl && cmp >= 0 {
			return false
		}
	}
	return true
}

// String returns the range as a string.
func (r Range) String() string {
	if r.operator == "==" {
		return "==" + r.min.String()
	}
	if r.operator == "!=" {
		return "!=" + r.min.String()
	}
	if r.operator == "~" {
		return "~" + r.min.String()
	}
	if r.hasMin && r.hasMax {
		minOp, maxOp := ">=", "<="
		if !r.minIncl {
			minOp = ">"
		}
		if !r.maxIncl {
			maxOp = "<"
		}
		return fmt.Sprintf("%s%s, %s%s", minOp, r.min, maxOp, r.max)
	}
	if r.hasMin {
		if r.minIncl {
			return ">=" + r.min.String()
		}
		return ">" + r.min.String()
	}
	if r.hasMax {
		if r.maxIncl {
			return "<=" + r.max.String()
		}
		return "<" + r.max.String()
	}
	return "*"
}
