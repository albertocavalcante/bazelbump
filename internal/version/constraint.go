// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"fmt"
	"strings"
)

// Constraint represents a version constraint that can match versions.
type Constraint interface {
	// Matches returns true if the version satisfies this constraint.
	Matches(v Version) bool
	// String returns the string representation of the constraint.
	String() string
}

// ConstraintSet represents multiple constraints that all must match.
type ConstraintSet []Constraint

// Matches returns true if the version satisfies all constraints in the set.
func (cs ConstraintSet) Matches(v Version) bool {
	for _, c := range cs {
		if !c.Matches(v) {
			return false
		}
	}
	return true
}

// String returns the string representation of the constraint set.
func (cs ConstraintSet) String() string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = c.String()
	}
	return strings.Join(parts, ", ")
}

// ParseConstraint parses a single constraint string.
// Supported operators: >=, <=, >, <, ==, !=, - (exclude)
// Examples: ">=6.4.0", "<=9.0.0", "-7.1.0", "==8.0.0"
func ParseConstraint(s string) (Constraint, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty constraint")
	}

	// Check for exclude operator (BCR style: -7.1.0 means exclude)
	if strings.HasPrefix(s, "-") {
		versionStr := strings.TrimPrefix(s, "-")
		v, err := Parse(versionStr)
		if err != nil {
			return nil, fmt.Errorf("invalid version in constraint %q: %w", s, err)
		}
		return &excludeConstraint{version: v, raw: s}, nil
	}

	// Check for comparison operators (order matters: >= before >, <= before <)
	operators := []string{">=", "<=", "!=", "==", ">", "<", "="}
	for _, op := range operators {
		if strings.HasPrefix(s, op) {
			versionStr := strings.TrimPrefix(s, op)
			versionStr = strings.TrimSpace(versionStr)
			v, err := Parse(versionStr)
			if err != nil {
				return nil, fmt.Errorf("invalid version in constraint %q: %w", s, err)
			}
			return &comparisonConstraint{op: normalizeOp(op), version: v, raw: s}, nil
		}
	}

	// No operator, treat as exact match
	v, err := Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid version in constraint %q: %w", s, err)
	}
	return &comparisonConstraint{op: "==", version: v, raw: "==" + s}, nil
}

// normalizeOp normalizes "=" to "==" for consistency.
func normalizeOp(op string) string {
	if op == "=" {
		return "=="
	}
	return op
}

// ParseConstraints parses a list of BCR-style constraint strings.
// Example: [">=6.4.0", "<=9.0.0", "-7.1.0"]
func ParseConstraints(ss []string) (ConstraintSet, error) {
	constraints := make(ConstraintSet, 0, len(ss))
	for _, s := range ss {
		c, err := ParseConstraint(s)
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, c)
	}
	return constraints, nil
}

// comparisonConstraint implements Constraint for comparison operators.
type comparisonConstraint struct {
	op      string  // One of: >=, <=, >, <, ==, !=
	version Version // The version to compare against
	raw     string  // Original string representation
}

// Matches implements Constraint.
func (c *comparisonConstraint) Matches(v Version) bool {
	cmp := v.Compare(c.version)
	switch c.op {
	case ">=":
		return cmp >= 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case "<":
		return cmp < 0
	case "==":
		return cmp == 0
	case "!=":
		return cmp != 0
	default:
		return false
	}
}

// String implements Constraint.
func (c *comparisonConstraint) String() string {
	return c.raw
}

// excludeConstraint implements Constraint for exclusion (BCR's - prefix).
type excludeConstraint struct {
	version Version // The version to exclude
	raw     string  // Original string representation
}

// Matches implements Constraint.
func (c *excludeConstraint) Matches(v Version) bool {
	// Exclude means the version must NOT equal this version
	return v.Compare(c.version) != 0
}

// String implements Constraint.
func (c *excludeConstraint) String() string {
	return c.raw
}

// Ensure types implement the Constraint interface.
var (
	_ Constraint = (*comparisonConstraint)(nil)
	_ Constraint = (*excludeConstraint)(nil)
)
