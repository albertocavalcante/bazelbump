// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// ExecConfig holds configuration for the exec module.
type ExecConfig struct {
	// Enabled controls whether exec functions are available.
	// When false, all exec functions return errors.
	Enabled bool

	// ProjectPath is the base directory for command execution.
	// Commands are restricted to this directory and subdirectories.
	ProjectPath string

	// MaxTimeout is the maximum allowed timeout for commands.
	// Default: 5 minutes.
	MaxTimeout time.Duration

	// DefaultTimeout is the default timeout if not specified.
	// Default: 30 seconds.
	DefaultTimeout time.Duration
}

// DefaultExecConfig returns the default exec configuration.
func DefaultExecConfig() ExecConfig {
	return ExecConfig{
		Enabled:        false,
		ProjectPath:    ".",
		MaxTimeout:     5 * time.Minute,
		DefaultTimeout: 30 * time.Second,
	}
}

// makeExecModule creates the exec.* module for running shell commands.
// If config.Enabled is false, all functions return errors.
func makeExecModule(config ExecConfig) starlark.Value {
	members := starlark.StringDict{
		"run":     starlark.NewBuiltin("exec.run", execRun(config)),
		"which":   starlark.NewBuiltin("exec.which", execWhich(config)),
		"enabled": starlark.Bool(config.Enabled),
	}
	return starlarkstruct.FromStringDict(starlarkstruct.Default, members)
}

// requireEnabled returns an error if exec is not enabled.
func requireEnabled(config ExecConfig, funcName string) error {
	if !config.Enabled {
		return fmt.Errorf("%s: exec is disabled (use --allow-exec flag to enable)", funcName)
	}
	return nil
}

// execRun implements exec.run(command, ...) -> struct
// Runs a shell command and returns the result.
func execRun(config ExecConfig) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := requireEnabled(config, "exec.run"); err != nil {
			return nil, err
		}

		var (
			command string
			timeout string
			capture bool = true
			check   bool = true
			envDict *starlark.Dict
			cwd     string
		)

		if err := starlark.UnpackArgs("exec.run", args, kwargs,
			"command", &command,
			"timeout?", &timeout,
			"capture?", &capture,
			"check?", &check,
			"env?", &envDict,
			"cwd?", &cwd,
		); err != nil {
			return nil, err
		}

		// Parse timeout
		var dur time.Duration
		if timeout == "" {
			dur = config.DefaultTimeout
		} else {
			var err error
			dur, err = time.ParseDuration(timeout)
			if err != nil {
				return nil, fmt.Errorf("exec.run: invalid timeout %q: %v", timeout, err)
			}
		}

		// Enforce max timeout
		if dur > config.MaxTimeout {
			return nil, fmt.Errorf("exec.run: timeout %v exceeds maximum allowed %v", dur, config.MaxTimeout)
		}

		// Resolve working directory
		workDir := config.ProjectPath
		if cwd != "" {
			workDir = filepath.Join(config.ProjectPath, cwd)
		}

		// Validate working directory is within project
		absWorkDir, err := filepath.Abs(workDir)
		if err != nil {
			return nil, fmt.Errorf("exec.run: invalid working directory: %v", err)
		}
		absProject, err := filepath.Abs(config.ProjectPath)
		if err != nil {
			return nil, fmt.Errorf("exec.run: invalid project path: %v", err)
		}
		if !strings.HasPrefix(absWorkDir, absProject) {
			return nil, fmt.Errorf("exec.run: working directory %q is outside project directory", cwd)
		}

		// Convert env dict to slice
		var env []string
		if envDict != nil {
			for _, item := range envDict.Items() {
				key, ok := starlark.AsString(item[0])
				if !ok {
					return nil, fmt.Errorf("exec.run: env key must be string")
				}
				val, ok := starlark.AsString(item[1])
				if !ok {
					return nil, fmt.Errorf("exec.run: env value must be string")
				}
				env = append(env, key+"="+val)
			}
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), dur)
		defer cancel()

		// Create command
		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = absWorkDir
		if len(env) > 0 {
			cmd.Env = env
		}

		// Capture output
		var stdout, stderr bytes.Buffer
		if capture {
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
		}

		// Run command
		runErr := cmd.Run()

		// Get exit code
		exitCode := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else if ctx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("exec.run: command timed out after %v", dur)
			} else {
				return nil, fmt.Errorf("exec.run: failed to execute command: %v", runErr)
			}
		}

		// Check for failure if requested
		if check && exitCode != 0 {
			errMsg := stderr.String()
			if errMsg == "" {
				errMsg = stdout.String()
			}
			return nil, fmt.Errorf("exec.run: command failed with exit code %d: %s", exitCode, strings.TrimSpace(errMsg))
		}

		return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
			"stdout":    starlark.String(stdout.String()),
			"stderr":    starlark.String(stderr.String()),
			"exit_code": starlark.MakeInt(exitCode),
			"success":   starlark.Bool(exitCode == 0),
		}), nil
	}
}

// execWhich implements exec.which(name) -> string
// Returns the path to a command, or empty string if not found.
func execWhich(config ExecConfig) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := requireEnabled(config, "exec.which"); err != nil {
			return nil, err
		}

		var name string
		if err := starlark.UnpackArgs("exec.which", args, kwargs, "name", &name); err != nil {
			return nil, err
		}

		path, err := exec.LookPath(name)
		if err != nil {
			return starlark.String(""), nil
		}

		return starlark.String(path), nil
	}
}
