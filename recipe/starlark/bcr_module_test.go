// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/albertocavalcante/go-bcr"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func TestBcrModule_NilClient(t *testing.T) {
	// Without BCR client, all functions should return errors
	interp := NewInterpreter()

	tests := []struct {
		name string
		code string
	}{
		{"metadata", `bcr.metadata("rules_go")`},
		{"source", `bcr.source("rules_go", "0.50.1")`},
		{"latest", `bcr.latest("rules_go")`},
		{"versions", `bcr.versions("rules_go")`},
		{"exists", `bcr.exists("rules_go")`},
		{"version_exists", `bcr.version_exists("rules_go", "0.50.1")`},
		{"is_yanked", `bcr.is_yanked("rules_go", "0.50.1")`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := `
recipe(
    name = "test",
    version = "1.0.0",
    steps = [],
)
result = ` + tt.code

			_, err := interp.LoadString("test.star", code)
			if err == nil {
				t.Errorf("expected error for %s without BCR client", tt.name)
			}
			if err != nil && !strings.Contains(err.Error(), "BCR client not configured") {
				t.Errorf("expected 'BCR client not configured' error, got: %v", err)
			}
		})
	}
}

func TestBcrModule_WithMockServer(t *testing.T) {
	// Create mock BCR server
	meta := map[string]any{
		"versions":        []string{"0.49.0", "0.50.0", "0.50.1"},
		"yanked_versions": map[string]string{"0.49.0": "deprecated"},
		"maintainers": []map[string]any{
			{"name": "Alice", "github": "alice", "email": "alice@example.com"},
		},
		"homepage":   "https://github.com/bazelbuild/rules_go",
		"repository": []string{"github:bazelbuild/rules_go"},
	}

	source := map[string]any{
		"url":          "https://example.com/rules_go-0.50.1.zip",
		"integrity":    "sha256-abc123",
		"strip_prefix": "rules_go-0.50.1",
		"patches": map[string]string{
			"fix.patch": "sha256-xyz789",
		},
		"patch_strip": 1,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/modules/rules_go/metadata.json":
			json.NewEncoder(w).Encode(meta)
		case "/modules/rules_go/0.50.1/source.json":
			json.NewEncoder(w).Encode(source)
		case "/modules/nonexistent/metadata.json":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := bcr.New(bcr.WithBaseURL(srv.URL))

	// Test bcr module functions using direct Starlark execution
	t.Run("latest", func(t *testing.T) {
		interp := NewInterpreter(WithBCRClient(client))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		// Call bcr.latest directly
		bcrMod := globals["bcr"].(*starlarkstruct.Struct)
		latestFn, _ := bcrMod.Attr("latest")
		result, err := starlark.Call(thread, latestFn, starlark.Tuple{starlark.String("rules_go")}, nil)
		if err != nil {
			t.Fatalf("bcr.latest error: %v", err)
		}
		if result.(starlark.String) != "0.50.1" {
			t.Errorf("expected 0.50.1, got %v", result)
		}
	})

	t.Run("versions", func(t *testing.T) {
		interp := NewInterpreter(WithBCRClient(client))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		bcrMod := globals["bcr"].(*starlarkstruct.Struct)
		versionsFn, _ := bcrMod.Attr("versions")
		result, err := starlark.Call(thread, versionsFn, starlark.Tuple{starlark.String("rules_go")}, nil)
		if err != nil {
			t.Fatalf("bcr.versions error: %v", err)
		}
		list := result.(*starlark.List)
		if list.Len() != 3 {
			t.Errorf("expected 3 versions, got %d", list.Len())
		}
	})

	t.Run("exists_true", func(t *testing.T) {
		interp := NewInterpreter(WithBCRClient(client))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		bcrMod := globals["bcr"].(*starlarkstruct.Struct)
		existsFn, _ := bcrMod.Attr("exists")
		result, err := starlark.Call(thread, existsFn, starlark.Tuple{starlark.String("rules_go")}, nil)
		if err != nil {
			t.Fatalf("bcr.exists error: %v", err)
		}
		if result != starlark.True {
			t.Errorf("expected true, got %v", result)
		}
	})

	t.Run("exists_false", func(t *testing.T) {
		interp := NewInterpreter(WithBCRClient(client))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		bcrMod := globals["bcr"].(*starlarkstruct.Struct)
		existsFn, _ := bcrMod.Attr("exists")
		result, err := starlark.Call(thread, existsFn, starlark.Tuple{starlark.String("nonexistent")}, nil)
		if err != nil {
			t.Fatalf("bcr.exists error: %v", err)
		}
		if result != starlark.False {
			t.Errorf("expected false, got %v", result)
		}
	})

	t.Run("version_exists", func(t *testing.T) {
		interp := NewInterpreter(WithBCRClient(client))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		bcrMod := globals["bcr"].(*starlarkstruct.Struct)
		fn, _ := bcrMod.Attr("version_exists")
		result, err := starlark.Call(thread, fn, starlark.Tuple{
			starlark.String("rules_go"),
			starlark.String("0.50.1"),
		}, nil)
		if err != nil {
			t.Fatalf("bcr.version_exists error: %v", err)
		}
		if result != starlark.True {
			t.Errorf("expected true, got %v", result)
		}
	})

	t.Run("is_yanked_true", func(t *testing.T) {
		interp := NewInterpreter(WithBCRClient(client))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		bcrMod := globals["bcr"].(*starlarkstruct.Struct)
		fn, _ := bcrMod.Attr("is_yanked")
		result, err := starlark.Call(thread, fn, starlark.Tuple{
			starlark.String("rules_go"),
			starlark.String("0.49.0"),
		}, nil)
		if err != nil {
			t.Fatalf("bcr.is_yanked error: %v", err)
		}
		if result != starlark.True {
			t.Errorf("expected true (yanked), got %v", result)
		}
	})

	t.Run("is_yanked_false", func(t *testing.T) {
		interp := NewInterpreter(WithBCRClient(client))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		bcrMod := globals["bcr"].(*starlarkstruct.Struct)
		fn, _ := bcrMod.Attr("is_yanked")
		result, err := starlark.Call(thread, fn, starlark.Tuple{
			starlark.String("rules_go"),
			starlark.String("0.50.1"),
		}, nil)
		if err != nil {
			t.Fatalf("bcr.is_yanked error: %v", err)
		}
		if result != starlark.False {
			t.Errorf("expected false (not yanked), got %v", result)
		}
	})

	t.Run("source", func(t *testing.T) {
		interp := NewInterpreter(WithBCRClient(client))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		bcrMod := globals["bcr"].(*starlarkstruct.Struct)
		fn, _ := bcrMod.Attr("source")
		result, err := starlark.Call(thread, fn, starlark.Tuple{
			starlark.String("rules_go"),
			starlark.String("0.50.1"),
		}, nil)
		if err != nil {
			t.Fatalf("bcr.source error: %v", err)
		}

		src := result.(*starlarkstruct.Struct)
		url, _ := src.Attr("url")
		if url.(starlark.String) != "https://example.com/rules_go-0.50.1.zip" {
			t.Errorf("wrong url: %v", url)
		}

		integrity, _ := src.Attr("integrity")
		if integrity.(starlark.String) != "sha256-abc123" {
			t.Errorf("wrong integrity: %v", integrity)
		}

		patchStrip, _ := src.Attr("patch_strip")
		if patchStrip.(starlark.Int).BigInt().Int64() != 1 {
			t.Errorf("wrong patch_strip: %v", patchStrip)
		}
	})

	t.Run("metadata", func(t *testing.T) {
		interp := NewInterpreter(WithBCRClient(client))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		bcrMod := globals["bcr"].(*starlarkstruct.Struct)
		fn, _ := bcrMod.Attr("metadata")
		result, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("rules_go")}, nil)
		if err != nil {
			t.Fatalf("bcr.metadata error: %v", err)
		}

		meta := result.(*starlarkstruct.Struct)

		homepage, _ := meta.Attr("homepage")
		if homepage.(starlark.String) != "https://github.com/bazelbuild/rules_go" {
			t.Errorf("wrong homepage: %v", homepage)
		}

		latest, _ := meta.Attr("latest")
		if latest.(starlark.String) != "0.50.1" {
			t.Errorf("wrong latest: %v", latest)
		}

		versions, _ := meta.Attr("versions")
		if versions.(*starlark.List).Len() != 3 {
			t.Errorf("expected 3 versions, got %d", versions.(*starlark.List).Len())
		}

		maintainers, _ := meta.Attr("maintainers")
		if maintainers.(*starlark.List).Len() != 1 {
			t.Errorf("expected 1 maintainer, got %d", maintainers.(*starlark.List).Len())
		}

		latestStable, _ := meta.Attr("latest_stable")
		if latestStable.(starlark.String) != "0.50.1" {
			t.Errorf("wrong latest_stable: %v", latestStable)
		}
	})

	t.Run("latest_stable", func(t *testing.T) {
		interp := NewInterpreter(WithBCRClient(client))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		bcrMod := globals["bcr"].(*starlarkstruct.Struct)
		fn, _ := bcrMod.Attr("latest_stable")
		result, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("rules_go")}, nil)
		if err != nil {
			t.Fatalf("bcr.latest_stable error: %v", err)
		}
		// 0.50.1 is stable (no -rc, -alpha, etc) and not yanked
		if result.(starlark.String) != "0.50.1" {
			t.Errorf("expected 0.50.1, got %v", result)
		}
	})

	t.Run("is_prerelease", func(t *testing.T) {
		interp := NewInterpreter(WithBCRClient(client))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		bcrMod := globals["bcr"].(*starlarkstruct.Struct)
		fn, _ := bcrMod.Attr("is_prerelease")

		// Test stable version
		result, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("1.0.0")}, nil)
		if err != nil {
			t.Fatalf("bcr.is_prerelease error: %v", err)
		}
		if result != starlark.False {
			t.Errorf("1.0.0 should not be prerelease")
		}

		// Test prerelease version
		result, err = starlark.Call(thread, fn, starlark.Tuple{starlark.String("1.0.0-rc1")}, nil)
		if err != nil {
			t.Fatalf("bcr.is_prerelease error: %v", err)
		}
		if result != starlark.True {
			t.Errorf("1.0.0-rc1 should be prerelease")
		}

		// Test alpha
		result, err = starlark.Call(thread, fn, starlark.Tuple{starlark.String("2.0.0-alpha")}, nil)
		if err != nil {
			t.Fatalf("bcr.is_prerelease error: %v", err)
		}
		if result != starlark.True {
			t.Errorf("2.0.0-alpha should be prerelease")
		}
	})
}

