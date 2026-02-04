// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func TestExecModule_Disabled(t *testing.T) {
	// Without exec enabled, all functions should return errors
	interp := NewInterpreter()

	tests := []struct {
		name string
		code string
	}{
		{"run", `exec.run("echo hello")`},
		{"which", `exec.which("sh")`},
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
				t.Errorf("expected error for %s without exec enabled", tt.name)
			}
			if err != nil && !strings.Contains(err.Error(), "exec is disabled") {
				t.Errorf("expected 'exec is disabled' error, got: %v", err)
			}
		})
	}
}

func TestExecModule_EnabledAttribute(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		interp := NewInterpreter()
		globals := interp.globals

		execMod := globals["exec"].(*starlarkstruct.Struct)
		enabled, _ := execMod.Attr("enabled")
		if enabled != starlark.False {
			t.Errorf("expected enabled=False when exec disabled")
		}
	})

	t.Run("enabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		interp := NewInterpreter(WithExecEnabled(tmpDir))
		globals := interp.globals

		execMod := globals["exec"].(*starlarkstruct.Struct)
		enabled, _ := execMod.Attr("enabled")
		if enabled != starlark.True {
			t.Errorf("expected enabled=True when exec enabled")
		}
	})
}

func TestExecModule_Run(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("simple_command", func(t *testing.T) {
		interp := NewInterpreter(WithExecEnabled(tmpDir))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		execMod := globals["exec"].(*starlarkstruct.Struct)
		runFn, _ := execMod.Attr("run")

		result, err := starlark.Call(thread, runFn, starlark.Tuple{starlark.String("echo hello")}, nil)
		if err != nil {
			t.Fatalf("exec.run error: %v", err)
		}

		res := result.(*starlarkstruct.Struct)

		stdout, _ := res.Attr("stdout")
		if strings.TrimSpace(string(stdout.(starlark.String))) != "hello" {
			t.Errorf("expected stdout='hello', got %q", stdout)
		}

		exitCode, _ := res.Attr("exit_code")
		if exitCode.(starlark.Int).BigInt().Int64() != 0 {
			t.Errorf("expected exit_code=0, got %v", exitCode)
		}

		success, _ := res.Attr("success")
		if success != starlark.True {
			t.Errorf("expected success=True")
		}
	})

	t.Run("failing_command_check_false", func(t *testing.T) {
		interp := NewInterpreter(WithExecEnabled(tmpDir))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		execMod := globals["exec"].(*starlarkstruct.Struct)
		runFn, _ := execMod.Attr("run")

		// With check=False, should return result even on failure
		result, err := starlark.Call(thread, runFn, starlark.Tuple{starlark.String("exit 42")}, []starlark.Tuple{
			{starlark.String("check"), starlark.False},
		})
		if err != nil {
			t.Fatalf("exec.run error: %v", err)
		}

		res := result.(*starlarkstruct.Struct)
		exitCode, _ := res.Attr("exit_code")
		if exitCode.(starlark.Int).BigInt().Int64() != 42 {
			t.Errorf("expected exit_code=42, got %v", exitCode)
		}

		success, _ := res.Attr("success")
		if success != starlark.False {
			t.Errorf("expected success=False")
		}
	})

	t.Run("failing_command_check_true", func(t *testing.T) {
		interp := NewInterpreter(WithExecEnabled(tmpDir))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		execMod := globals["exec"].(*starlarkstruct.Struct)
		runFn, _ := execMod.Attr("run")

		// With check=True (default), should return error on failure
		_, err := starlark.Call(thread, runFn, starlark.Tuple{starlark.String("exit 1")}, nil)
		if err == nil {
			t.Error("expected error for failing command with check=True")
		}
		if !strings.Contains(err.Error(), "exit code 1") {
			t.Errorf("expected 'exit code 1' in error, got: %v", err)
		}
	})

	t.Run("with_cwd", func(t *testing.T) {
		// Create a subdirectory
		subDir := filepath.Join(tmpDir, "subdir")
		os.MkdirAll(subDir, 0755)

		interp := NewInterpreter(WithExecEnabled(tmpDir))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		execMod := globals["exec"].(*starlarkstruct.Struct)
		runFn, _ := execMod.Attr("run")

		result, err := starlark.Call(thread, runFn, starlark.Tuple{starlark.String("pwd")}, []starlark.Tuple{
			{starlark.String("cwd"), starlark.String("subdir")},
		})
		if err != nil {
			t.Fatalf("exec.run error: %v", err)
		}

		res := result.(*starlarkstruct.Struct)
		stdout, _ := res.Attr("stdout")
		if !strings.Contains(string(stdout.(starlark.String)), "subdir") {
			t.Errorf("expected pwd to contain 'subdir', got %q", stdout)
		}
	})

	t.Run("cwd_outside_project", func(t *testing.T) {
		interp := NewInterpreter(WithExecEnabled(tmpDir))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		execMod := globals["exec"].(*starlarkstruct.Struct)
		runFn, _ := execMod.Attr("run")

		// Try to escape project directory
		_, err := starlark.Call(thread, runFn, starlark.Tuple{starlark.String("pwd")}, []starlark.Tuple{
			{starlark.String("cwd"), starlark.String("../../..")},
		})
		if err == nil {
			t.Error("expected error for cwd outside project")
		}
		if !strings.Contains(err.Error(), "outside project directory") {
			t.Errorf("expected 'outside project directory' error, got: %v", err)
		}
	})

	t.Run("with_env", func(t *testing.T) {
		interp := NewInterpreter(WithExecEnabled(tmpDir))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		execMod := globals["exec"].(*starlarkstruct.Struct)
		runFn, _ := execMod.Attr("run")

		envDict := starlark.NewDict(1)
		envDict.SetKey(starlark.String("MY_VAR"), starlark.String("my_value"))

		result, err := starlark.Call(thread, runFn, starlark.Tuple{starlark.String("echo $MY_VAR")}, []starlark.Tuple{
			{starlark.String("env"), envDict},
		})
		if err != nil {
			t.Fatalf("exec.run error: %v", err)
		}

		res := result.(*starlarkstruct.Struct)
		stdout, _ := res.Attr("stdout")
		if strings.TrimSpace(string(stdout.(starlark.String))) != "my_value" {
			t.Errorf("expected stdout='my_value', got %q", stdout)
		}
	})

	t.Run("timeout_exceeded", func(t *testing.T) {
		interp := NewInterpreter(WithExecEnabled(tmpDir))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		execMod := globals["exec"].(*starlarkstruct.Struct)
		runFn, _ := execMod.Attr("run")

		// Try to exceed max timeout
		_, err := starlark.Call(thread, runFn, starlark.Tuple{starlark.String("sleep 1")}, []starlark.Tuple{
			{starlark.String("timeout"), starlark.String("10h")},
		})
		if err == nil {
			t.Error("expected error for timeout exceeding maximum")
		}
		if !strings.Contains(err.Error(), "exceeds maximum") {
			t.Errorf("expected 'exceeds maximum' error, got: %v", err)
		}
	})
}

func TestExecModule_Which(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("existing_command", func(t *testing.T) {
		interp := NewInterpreter(WithExecEnabled(tmpDir))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		execMod := globals["exec"].(*starlarkstruct.Struct)
		whichFn, _ := execMod.Attr("which")

		result, err := starlark.Call(thread, whichFn, starlark.Tuple{starlark.String("sh")}, nil)
		if err != nil {
			t.Fatalf("exec.which error: %v", err)
		}

		path := string(result.(starlark.String))
		if path == "" {
			t.Error("expected non-empty path for 'sh'")
		}
		if !strings.Contains(path, "sh") {
			t.Errorf("expected path to contain 'sh', got %q", path)
		}
	})

	t.Run("nonexistent_command", func(t *testing.T) {
		interp := NewInterpreter(WithExecEnabled(tmpDir))
		thread := &starlark.Thread{Name: "test"}
		globals := interp.globals

		execMod := globals["exec"].(*starlarkstruct.Struct)
		whichFn, _ := execMod.Attr("which")

		result, err := starlark.Call(thread, whichFn, starlark.Tuple{starlark.String("nonexistent_command_xyz123")}, nil)
		if err != nil {
			t.Fatalf("exec.which error: %v", err)
		}

		path := string(result.(starlark.String))
		if path != "" {
			t.Errorf("expected empty path for nonexistent command, got %q", path)
		}
	})
}
