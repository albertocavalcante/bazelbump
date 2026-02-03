# Troubleshooting

Common issues and solutions for bazelbump.

## Installation Issues

### "could not find flag data directory"

**Problem:** bazelbump cannot find the flag database.

**Solution:** Sync the flag data before running bazelbump:

```bash
just sync-flags
```

Or set the `BAZELBUMP_DATA_DIR` environment variable:

```bash
export BAZELBUMP_DATA_DIR=/path/to/bazelbump/data/flags
bazelbump analyze --to 9.0.0
```

### "unknown version: X.Y.Z"

**Problem:** The requested Bazel version is not in the flag database.

**Solution:** Check available versions and sync if needed:

```bash
# List available versions in your flag database
ls data/flags/

# Sync all versions
just sync-flags

# Sync a specific version
just sync-flags-version 9.0.0
```

## Analysis Issues

### "could not determine source version"

**Problem:** bazelbump cannot auto-detect the current Bazel version.

**Solution:** Specify the `--from` version explicitly:

```bash
bazelbump analyze --from 8.0.0 --to 9.0.0
```

Auto-detection requires either:
- A `.bazelversion` file in the project root
- A `USE_BAZEL_VERSION` environment variable

### No issues found, but build fails after upgrade

**Possible causes:**

1. **Starlark API changes** - bazelbump currently focuses on flags, not Starlark APIs. Check the [Bazel release notes](https://github.com/bazelbuild/bazel/releases) for API changes.

2. **Dependency compatibility** - Your dependencies may not support the target Bazel version. Check `bazel_compatibility` in your dependencies' `MODULE.bazel` files.

3. **BUILD file changes** - Some rules have renamed attributes or changed behavior. Review the release notes for your rules (rules_go, rules_python, etc.).

### Analysis misses some flags

**Problem:** Flags in `.bazelrc` are not being detected.

**Check:**

1. **File naming** - bazelbump scans files matching `*.bazelrc` and `.bazelrc`. Other config files (like `bazelrc.txt`) are not scanned.

2. **Import statements** - Imported bazelrc files are not followed. Run analysis on each file separately if needed.

3. **Hidden directories** - Files in hidden directories (starting with `.`) except `.bazelrc` in the project root are skipped.

## Output Issues

### Colors not showing

**Problem:** Output appears without colors.

**Check:**

1. Terminal support - bazelbump detects if stdout is a terminal. Piping output disables colors automatically.

2. `NO_COLOR` environment variable - If set, colors are disabled. Unset it:
   ```bash
   unset NO_COLOR
   ```

### JSON parsing errors

**Problem:** JSON output is malformed or unexpected.

**Solution:** Ensure you're using `--json` flag:

```bash
# Correct
bazelbump analyze --to 9.0.0 --json | jq .

# Wrong - mixes text and JSON
bazelbump analyze --to 9.0.0 | jq .
```

## CI/CD Issues

### Exit code 1 in CI but no issues visible

**Problem:** CI fails with exit code 1 but no output.

**Check:**

1. **Redirect stderr** - Some messages go to stderr:
   ```bash
   bazelbump analyze --to 9.0.0 2>&1
   ```

2. **Missing flag data** - Ensure flag data is synced in CI:
   ```yaml
   - name: Sync flag data
     run: just sync-flags

   - name: Analyze
     run: bazelbump analyze --to 9.0.0
   ```

### Slow sync in CI

**Problem:** `just sync-flags` is slow in CI.

**Solutions:**

1. **Cache the flag data:**
   ```yaml
   # GitHub Actions
   - uses: actions/cache@v4
     with:
       path: data/flags
       key: bazelbump-flags-${{ hashFiles('tools/sync-flags/**') }}
   ```

2. **Commit flag data** - For stable versions, commit `data/flags/` to your repository.

3. **Sync only needed versions:**
   ```bash
   just sync-flags-version 8.0.0
   just sync-flags-version 9.0.0
   ```

## Performance Issues

### Analysis is slow on large projects

**Current behavior:** bazelbump scans all `.bazelrc` files in the project tree.

**Optimizations:**

1. **Specify path directly:**
   ```bash
   bazelbump analyze --to 9.0.0 --path .
   ```

2. **Exclude directories** - bazelbump skips hidden directories. Move test fixtures or examples into hidden directories if they contain many bazelrc files.

## Getting Help

If your issue is not covered here:

1. **Check the spec** - Review [SPEC.md](../../SPEC.md) for planned features and known limitations.

2. **File an issue** - Open a [GitHub issue](https://github.com/albertocavalcante/bazelbump/issues) with:
   - bazelbump version (`bazelbump version`)
   - Command you ran
   - Expected vs actual behavior
   - Relevant config files (sanitized)

3. **Enable verbose output** - For debugging, check internal state:
   ```bash
   # Check what versions are available
   ls data/flags/

   # Check flag data for a version
   jq '.[] | select(.name == "flag_name")' data/flags/9.0.0/all.json
   ```
