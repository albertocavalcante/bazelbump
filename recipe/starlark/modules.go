// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"fmt"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// makeModule creates a namespaced module from a map of functions.
// The module is a starlark struct with the given name and functions as attributes.
func makeModule(name string, funcs map[string]*starlark.Builtin) starlark.Value {
	dict := starlark.StringDict{}
	for k, v := range funcs {
		dict[k] = v
	}
	return starlarkstruct.FromStringDict(starlark.String(name), dict)
}

// deprecationTracker tracks which deprecation warnings have been shown.
// This ensures we only show each warning once per recipe execution.
type deprecationTracker struct {
	mu     sync.Mutex
	warned map[string]bool
}

// Global deprecation tracker for the current process.
// In practice, each recipe execution should call resetDeprecationWarnings().
var globalDeprecationTracker = &deprecationTracker{
	warned: make(map[string]bool),
}

// warnOnce logs a deprecation warning once per function name.
func warnOnce(oldName, newName string) {
	globalDeprecationTracker.mu.Lock()
	defer globalDeprecationTracker.mu.Unlock()

	key := oldName
	if globalDeprecationTracker.warned[key] {
		return
	}
	globalDeprecationTracker.warned[key] = true

	fmt.Printf("warning: %s is deprecated, use %s instead\n", oldName, newName)
}

// resetDeprecationWarnings clears the deprecation warning tracker.
// This should be called at the start of each recipe execution.
func resetDeprecationWarnings() {
	globalDeprecationTracker.mu.Lock()
	defer globalDeprecationTracker.mu.Unlock()
	globalDeprecationTracker.warned = make(map[string]bool)
}

// deprecatedBuiltin wraps a builtin function with a deprecation warning.
// The warning is shown once per recipe execution when the old name is used.
func deprecatedBuiltin(oldName, newName string, fn func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)) *starlark.Builtin {
	return starlark.NewBuiltin(oldName, func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		warnOnce(oldName, newName)
		return fn(thread, b, args, kwargs)
	})
}

// deprecatedBuiltinMethod is like deprecatedBuiltin but explicitly named for interpreter
// method receivers. The interpreter is passed for future extensibility (e.g., logging context).
func deprecatedBuiltinMethod(oldName, newName string, _ *Interpreter, fn func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)) *starlark.Builtin {
	return starlark.NewBuiltin(oldName, func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		warnOnce(oldName, newName)
		return fn(thread, b, args, kwargs)
	})
}
