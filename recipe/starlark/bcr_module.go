// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"context"
	"fmt"
	"time"

	"github.com/albertocavalcante/go-bcr"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// makeBcrModule creates the bcr.* module for querying the Bazel Central Registry.
// If client is nil, all functions will return errors indicating BCR is not configured.
func makeBcrModule(client *bcr.Client) starlark.Value {
	members := starlark.StringDict{
		"metadata":       starlark.NewBuiltin("bcr.metadata", bcrMetadata(client)),
		"source":         starlark.NewBuiltin("bcr.source", bcrSource(client)),
		"latest":         starlark.NewBuiltin("bcr.latest", bcrLatest(client)),
		"versions":       starlark.NewBuiltin("bcr.versions", bcrVersions(client)),
		"exists":         starlark.NewBuiltin("bcr.exists", bcrExists(client)),
		"version_exists": starlark.NewBuiltin("bcr.version_exists", bcrVersionExists(client)),
		"is_yanked":      starlark.NewBuiltin("bcr.is_yanked", bcrIsYanked(client)),
	}
	return starlarkstruct.FromStringDict(starlarkstruct.Default, members)
}

// bcrTimeout is the default timeout for BCR requests.
const bcrTimeout = 30 * time.Second

// requireClient returns an error if the BCR client is not configured.
func requireClient(client *bcr.Client, funcName string) error {
	if client == nil {
		return fmt.Errorf("%s: BCR client not configured (use WithBCRClient option)", funcName)
	}
	return nil
}

// bcrMetadata implements bcr.metadata(module) -> struct
// Returns module metadata including versions, maintainers, homepage.
func bcrMetadata(client *bcr.Client) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := requireClient(client, "bcr.metadata"); err != nil {
			return nil, err
		}

		var module string
		if err := starlark.UnpackArgs("bcr.metadata", args, kwargs, "module", &module); err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), bcrTimeout)
		defer cancel()

		meta, err := client.Metadata(ctx, module)
		if err != nil {
			return nil, fmt.Errorf("bcr.metadata: %w", err)
		}

		// Convert versions to Starlark list
		versions := make([]starlark.Value, len(meta.Versions))
		for i, v := range meta.Versions {
			versions[i] = starlark.String(v)
		}

		// Convert yanked versions to Starlark dict
		yanked := starlark.NewDict(len(meta.YankedVersions))
		for v, reason := range meta.YankedVersions {
			yanked.SetKey(starlark.String(v), starlark.String(reason))
		}

		// Convert maintainers to Starlark list of structs
		maintainers := make([]starlark.Value, len(meta.Maintainers))
		for i, m := range meta.Maintainers {
			maintainers[i] = starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
				"name":   starlark.String(m.Name),
				"email":  starlark.String(m.Email),
				"github": starlark.String(m.GitHub),
			})
		}

		// Convert repository to Starlark list
		repos := make([]starlark.Value, len(meta.Repository))
		for i, r := range meta.Repository {
			repos[i] = starlark.String(r)
		}

		return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
			"versions":        starlark.NewList(versions),
			"yanked_versions": yanked,
			"maintainers":     starlark.NewList(maintainers),
			"homepage":        starlark.String(meta.Homepage),
			"repository":      starlark.NewList(repos),
			"latest":          starlark.String(meta.Latest()),
		}), nil
	}
}

// bcrSource implements bcr.source(module, version) -> struct
// Returns source information for a specific module version.
func bcrSource(client *bcr.Client) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := requireClient(client, "bcr.source"); err != nil {
			return nil, err
		}

		var module, version string
		if err := starlark.UnpackArgs("bcr.source", args, kwargs,
			"module", &module,
			"version", &version,
		); err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), bcrTimeout)
		defer cancel()

		src, err := client.Source(ctx, module, version)
		if err != nil {
			return nil, fmt.Errorf("bcr.source: %w", err)
		}

		// Convert patches to Starlark dict
		patches := starlark.NewDict(len(src.Patches))
		for name, integrity := range src.Patches {
			patches.SetKey(starlark.String(name), starlark.String(integrity))
		}

		return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
			"type":         starlark.String(src.SourceType()),
			"url":          starlark.String(src.URL),
			"integrity":    starlark.String(src.Integrity),
			"strip_prefix": starlark.String(src.StripPrefix),
			"patches":      patches,
			"patch_strip":  starlark.MakeInt(src.PatchStrip),
		}), nil
	}
}

// bcrLatest implements bcr.latest(module) -> string
// Returns the latest non-yanked version of a module.
func bcrLatest(client *bcr.Client) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := requireClient(client, "bcr.latest"); err != nil {
			return nil, err
		}

		var module string
		if err := starlark.UnpackArgs("bcr.latest", args, kwargs, "module", &module); err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), bcrTimeout)
		defer cancel()

		version, err := client.Latest(ctx, module)
		if err != nil {
			return nil, fmt.Errorf("bcr.latest: %w", err)
		}

		return starlark.String(version), nil
	}
}

// bcrVersions implements bcr.versions(module) -> list[string]
// Returns all available versions for a module.
func bcrVersions(client *bcr.Client) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := requireClient(client, "bcr.versions"); err != nil {
			return nil, err
		}

		var module string
		if err := starlark.UnpackArgs("bcr.versions", args, kwargs, "module", &module); err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), bcrTimeout)
		defer cancel()

		meta, err := client.Metadata(ctx, module)
		if err != nil {
			return nil, fmt.Errorf("bcr.versions: %w", err)
		}

		versions := make([]starlark.Value, len(meta.Versions))
		for i, v := range meta.Versions {
			versions[i] = starlark.String(v)
		}

		return starlark.NewList(versions), nil
	}
}

// bcrExists implements bcr.exists(module) -> bool
// Returns true if the module exists in the registry.
func bcrExists(client *bcr.Client) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := requireClient(client, "bcr.exists"); err != nil {
			return nil, err
		}

		var module string
		if err := starlark.UnpackArgs("bcr.exists", args, kwargs, "module", &module); err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), bcrTimeout)
		defer cancel()

		exists, err := client.Exists(ctx, module)
		if err != nil {
			return nil, fmt.Errorf("bcr.exists: %w", err)
		}

		return starlark.Bool(exists), nil
	}
}

// bcrVersionExists implements bcr.version_exists(module, version) -> bool
// Returns true if the specific version exists.
func bcrVersionExists(client *bcr.Client) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := requireClient(client, "bcr.version_exists"); err != nil {
			return nil, err
		}

		var module, version string
		if err := starlark.UnpackArgs("bcr.version_exists", args, kwargs,
			"module", &module,
			"version", &version,
		); err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), bcrTimeout)
		defer cancel()

		exists, err := client.VersionExists(ctx, module, version)
		if err != nil {
			return nil, fmt.Errorf("bcr.version_exists: %w", err)
		}

		return starlark.Bool(exists), nil
	}
}

// bcrIsYanked implements bcr.is_yanked(module, version) -> bool
// Returns true if the version is yanked.
func bcrIsYanked(client *bcr.Client) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := requireClient(client, "bcr.is_yanked"); err != nil {
			return nil, err
		}

		var module, version string
		if err := starlark.UnpackArgs("bcr.is_yanked", args, kwargs,
			"module", &module,
			"version", &version,
		); err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), bcrTimeout)
		defer cancel()

		meta, err := client.Metadata(ctx, module)
		if err != nil {
			return nil, fmt.Errorf("bcr.is_yanked: %w", err)
		}

		return starlark.Bool(meta.IsYanked(version)), nil
	}
}
