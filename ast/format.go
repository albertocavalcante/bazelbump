// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"fmt"
	"strings"

	"github.com/albertocavalcante/starlark-syntax-go/bzl"
)

// Format formats a bzl.File AST back to source code.
// This preserves comments attached to AST nodes.
func Format(f *bzl.File) []byte {
	var buf strings.Builder
	formatFile(&buf, f)
	return []byte(buf.String())
}

// FormatWithoutRewriting formats without applying any transformations.
func FormatWithoutRewriting(f *bzl.File) []byte {
	return Format(f)
}

// formatFile formats a file to the buffer.
func formatFile(buf *strings.Builder, f *bzl.File) {
	for i, stmt := range f.Stmt {
		// Write comments before
		writeComments(buf, stmt.Comment().Before, "")

		// Write statement
		formatExpr(buf, stmt, 0)

		// Write suffix comments
		writeSuffixComments(buf, stmt.Comment().Suffix)

		// Add newline between statements
		if i < len(f.Stmt)-1 {
			buf.WriteString("\n")
		}
	}

	// Write trailing comments
	writeComments(buf, f.Comments.After, "")
}

// writeComments writes a slice of comments.
func writeComments(buf *strings.Builder, comments []bzl.Comment, indent string) {
	for _, c := range comments {
		buf.WriteString(indent)
		buf.WriteString(c.Token)
		buf.WriteString("\n")
	}
}

// writeSuffixComments writes suffix (end-of-line) comments.
func writeSuffixComments(buf *strings.Builder, comments []bzl.Comment) {
	for _, c := range comments {
		buf.WriteString("  ")
		buf.WriteString(c.Token)
	}
}

// formatExpr formats an expression.
func formatExpr(buf *strings.Builder, expr bzl.Expr, indent int) {
	indentStr := strings.Repeat("    ", indent)

	switch e := expr.(type) {
	case *bzl.LoadStmt:
		formatLoad(buf, e, indentStr)

	case *bzl.CallExpr:
		formatCall(buf, e, indentStr)

	case *bzl.StringExpr:
		// Use Token if available (preserves original escaping)
		// Otherwise, escape the Value properly
		if e.Token != "" {
			buf.WriteString(e.Token)
		} else if e.TripleQuote {
			buf.WriteString(`"""`)
			buf.WriteString(escapeString(e.Value, true))
			buf.WriteString(`"""`)
		} else {
			buf.WriteString(`"`)
			buf.WriteString(escapeString(e.Value, false))
			buf.WriteString(`"`)
		}

	case *bzl.Ident:
		buf.WriteString(e.Name)

	case *bzl.ListExpr:
		formatList(buf, e, indent)

	case *bzl.DictExpr:
		formatDict(buf, e, indent)

	case *bzl.LiteralExpr:
		buf.WriteString(e.Token)

	case *bzl.AssignExpr:
		formatExpr(buf, e.LHS, 0)
		buf.WriteString(" ")
		buf.WriteString(e.Op)
		buf.WriteString(" ")
		formatExpr(buf, e.RHS, 0)

	case *bzl.BinaryExpr:
		formatExpr(buf, e.X, 0)
		buf.WriteString(" ")
		buf.WriteString(e.Op)
		buf.WriteString(" ")
		formatExpr(buf, e.Y, 0)

	case *bzl.UnaryExpr:
		buf.WriteString(e.Op)
		formatExpr(buf, e.X, 0)

	case *bzl.KeyValueExpr:
		formatExpr(buf, e.Key, 0)
		buf.WriteString(": ")
		formatExpr(buf, e.Value, 0)

	case *bzl.DotExpr:
		formatExpr(buf, e.X, 0)
		buf.WriteString(".")
		buf.WriteString(e.Name)

	case *bzl.IndexExpr:
		formatExpr(buf, e.X, 0)
		buf.WriteString("[")
		formatExpr(buf, e.Y, 0)
		buf.WriteString("]")

	case *bzl.ParenExpr:
		buf.WriteString("(")
		formatExpr(buf, e.X, 0)
		buf.WriteString(")")

	case *bzl.TupleExpr:
		if !e.NoBrackets {
			buf.WriteString("(")
		}
		for i, elem := range e.List {
			if i > 0 {
				buf.WriteString(", ")
			}
			formatExpr(buf, elem, 0)
		}
		if !e.NoBrackets {
			buf.WriteString(")")
		}

	case *bzl.Comprehension:
		if e.Curly {
			buf.WriteString("{")
		} else {
			buf.WriteString("[")
		}
		formatExpr(buf, e.Body, 0)
		for _, clause := range e.Clauses {
			buf.WriteString(" ")
			formatExpr(buf, clause, 0)
		}
		if e.Curly {
			buf.WriteString("}")
		} else {
			buf.WriteString("]")
		}

	case *bzl.ForClause:
		buf.WriteString("for ")
		formatExpr(buf, e.Vars, 0)
		buf.WriteString(" in ")
		formatExpr(buf, e.X, 0)

	case *bzl.IfClause:
		buf.WriteString("if ")
		formatExpr(buf, e.Cond, 0)

	case *bzl.CommentBlock:
		writeComments(buf, e.Comment().Before, indentStr)

	case *bzl.File:
		formatFile(buf, e)

	default:
		// Fallback for unknown types
		buf.WriteString(fmt.Sprintf("<%T>", expr))
	}
}

// formatLoad formats a load statement.
func formatLoad(buf *strings.Builder, load *bzl.LoadStmt, indent string) {
	buf.WriteString(indent)
	buf.WriteString("load(")

	// Decide on single-line vs multi-line
	multiLine := !load.ForceCompact && len(load.From) > 3

	buf.WriteString(`"`)
	buf.WriteString(load.Module.Value)
	buf.WriteString(`"`)

	if multiLine {
		buf.WriteString(",\n")
		for i, to := range load.To {
			buf.WriteString(indent)
			buf.WriteString("    ")
			if load.From[i].Name != to.Name {
				buf.WriteString(to.Name)
				buf.WriteString(" = ")
			}
			buf.WriteString(`"`)
			buf.WriteString(load.From[i].Name)
			buf.WriteString(`"`)
			if i < len(load.To)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		buf.WriteString(indent)
		buf.WriteString(")")
	} else {
		for i, to := range load.To {
			buf.WriteString(", ")
			if load.From[i].Name != to.Name {
				buf.WriteString(to.Name)
				buf.WriteString(" = ")
			}
			buf.WriteString(`"`)
			buf.WriteString(load.From[i].Name)
			buf.WriteString(`"`)
		}
		buf.WriteString(")")
	}
}

// formatCall formats a call expression.
func formatCall(buf *strings.Builder, call *bzl.CallExpr, indent string) {
	buf.WriteString(indent)

	formatExpr(buf, call.X, 0)
	buf.WriteString("(")

	if call.ForceMultiLine && len(call.List) > 0 {
		buf.WriteString("\n")
		for i, arg := range call.List {
			// Write arg comments
			writeComments(buf, arg.Comment().Before, indent+"    ")

			buf.WriteString(indent)
			buf.WriteString("    ")
			formatExpr(buf, arg, 0)

			// Write suffix comments
			writeSuffixComments(buf, arg.Comment().Suffix)

			if i < len(call.List)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		buf.WriteString(indent)
	} else {
		for i, arg := range call.List {
			if i > 0 {
				buf.WriteString(", ")
			}
			formatExpr(buf, arg, 0)
		}
	}

	buf.WriteString(")")
}

// formatList formats a list expression.
func formatList(buf *strings.Builder, list *bzl.ListExpr, indent int) {
	indentStr := strings.Repeat("    ", indent)
	buf.WriteString("[")

	if list.ForceMultiLine && len(list.List) > 0 {
		buf.WriteString("\n")
		for i, elem := range list.List {
			writeComments(buf, elem.Comment().Before, indentStr+"    ")
			buf.WriteString(indentStr)
			buf.WriteString("    ")
			formatExpr(buf, elem, 0)
			writeSuffixComments(buf, elem.Comment().Suffix)
			if i < len(list.List)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		buf.WriteString(indentStr)
	} else {
		for i, elem := range list.List {
			if i > 0 {
				buf.WriteString(", ")
			}
			formatExpr(buf, elem, 0)
		}
	}

	buf.WriteString("]")
}

// formatDict formats a dict expression.
func formatDict(buf *strings.Builder, dict *bzl.DictExpr, indent int) {
	indentStr := strings.Repeat("    ", indent)
	buf.WriteString("{")

	if dict.ForceMultiLine && len(dict.List) > 0 {
		buf.WriteString("\n")
		for i, elem := range dict.List {
			writeComments(buf, elem.Comment().Before, indentStr+"    ")
			buf.WriteString(indentStr)
			buf.WriteString("    ")
			formatExpr(buf, elem, 0)
			writeSuffixComments(buf, elem.Comment().Suffix)
			if i < len(dict.List)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		buf.WriteString(indentStr)
	} else {
		for i, elem := range dict.List {
			if i > 0 {
				buf.WriteString(", ")
			}
			formatExpr(buf, elem, 0)
		}
	}

	buf.WriteString("}")
}

// escapeString escapes a string value for Starlark output.
func escapeString(s string, triple bool) string {
	var buf strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			if triple {
				buf.WriteRune(r)
			} else {
				buf.WriteString(`\n`)
			}
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		case '"':
			if triple {
				buf.WriteRune(r)
			} else {
				buf.WriteString(`\"`)
			}
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// SortLoadArgs sorts the symbols in a load statement alphabetically.
func SortLoadArgs(load *bzl.LoadStmt) {
	if len(load.From) <= 1 {
		return
	}

	// Create pairs to sort together
	type pair struct {
		from *bzl.Ident
		to   *bzl.Ident
	}
	pairs := make([]pair, len(load.From))
	for i := range load.From {
		pairs[i] = pair{load.From[i], load.To[i]}
	}

	// Sort by the "To" name (the local name)
	for i := 0; i < len(pairs)-1; i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[i].to.Name > pairs[j].to.Name {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}

	// Rebuild the slices
	for i := range pairs {
		load.From[i] = pairs[i].from
		load.To[i] = pairs[i].to
	}
}
