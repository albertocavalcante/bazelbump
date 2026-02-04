# Example test file for bazelbump recipes
# Uses struct attribute access: result.field instead of result["field"]

def test_ensure_load_creates_operation():
    """Test that ensure_load creates an operation."""
    op = ensure_load("cc_library", "@rules_cc//cc:defs.bzl")
    assert.eq(op.type, "ensure_load")
    assert.eq(op.rule, "cc_library")
    assert.eq(op.load_from, "@rules_cc//cc:defs.bzl")

def test_scan_step_structure():
    """Test that scan returns a step struct."""
    result = scan(
        id="detect",
        files=["**/BUILD"],
        patterns=[r"cc_library\("],
    )
    assert.eq(result.id, "detect")
    assert.eq(result.files, ["**/BUILD"])
    assert.eq(result.type, "scan")

def test_recipe_with_requires():
    """Test that recipe accepts requires parameter."""
    r = recipe(
        name="test-recipe",
        version="1.0.0",
        requires=["exec"],
        steps=[],
    )
    assert.eq(r.requires, ["exec"])
    assert.eq(r.name, "test-recipe")
    assert.eq(r.version, "1.0.0")

def test_ensure_bazel_dep():
    """Test ensure_bazel_dep operation."""
    op = ensure_bazel_dep("rules_cc", "0.1.0")
    assert.eq(op.type, "ensure_bazel_dep")
    assert.eq(op.name, "rules_cc")
    assert.eq(op.version, "0.1.0")
    assert.eq(op.dev_dependency, False)

def test_ensure_bazel_dep_dev():
    """Test ensure_bazel_dep with dev_dependency."""
    op = ensure_bazel_dep("rules_testing", "0.5.0", dev_dependency=True)
    assert.eq(op.dev_dependency, True)

def test_edit_step():
    """Test edit step structure."""
    ops = [
        ensure_load("cc_library", "@rules_cc//cc:defs.bzl"),
        ensure_bazel_dep("rules_cc", "0.1.0"),
    ]
    step = edit(id="migrate", operations=ops, depends_on=["scan"])
    assert.eq(step.type, "edit")
    assert.eq(step.id, "migrate")
    assert.len(step.operations, 2)
    assert.eq(step.depends_on, ["scan"])

def test_version_parse():
    """Test version parsing."""
    v = parse_version("1.2.3")
    assert.eq(v.major, 1)
    assert.eq(v.minor, 2)
    assert.eq(v.patch, 3)
    assert.eq(v.prerelease, "")

def test_version_parse_with_prerelease():
    """Test version parsing with prerelease."""
    v = parse_version("1.2.3-rc1")
    assert.eq(v.major, 1)
    assert.eq(v.minor, 2)
    assert.eq(v.patch, 3)
    assert.eq(v.prerelease, "rc1")

def test_version_compare():
    """Test version comparison."""
    assert.eq(version_compare("1.0.0", "2.0.0"), -1)
    assert.eq(version_compare("2.0.0", "1.0.0"), 1)
    assert.eq(version_compare("1.0.0", "1.0.0"), 0)
    assert.eq(version_compare("1.0.0-alpha", "1.0.0"), -1)
    assert.eq(version_compare("1.0.0", "1.0.0-alpha"), 1)

def test_version_at_least():
    """Test version_at_least helper."""
    assert.true(version_at_least("2.0.0", "1.0.0"))
    assert.true(version_at_least("1.0.0", "1.0.0"))
    assert.false(version_at_least("0.9.0", "1.0.0"))

def test_remove_flag():
    """Test remove_flag operation."""
    op = remove_flag("incompatible_foo")
    assert.eq(op.type, "remove_flag")
    assert.eq(op.flag, "incompatible_foo")
    assert.eq(op.file, ".bazelrc")

def test_replace_text():
    """Test replace_text operation."""
    op = replace_text("old_text", "new_text", ["*.bzl"], regex=True)
    assert.eq(op.type, "replace_text")
    assert.eq(op.old, "old_text")
    assert.eq(op.new, "new_text")
    assert.eq(op.regex, True)

def test_run_command():
    """Test run_command operation."""
    op = run_command("bazel build //...", description="Build all", timeout="10m")
    assert.eq(op.type, "run_command")
    assert.eq(op.command, "bazel build //...")
    assert.eq(op.description, "Build all")
    assert.eq(op.timeout, "10m")

def test_verify_step():
    """Test verify step structure."""
    step = verify(
        id="check",
        command="bazel test //...",
        timeout="30m",
        allow_failure=True,
    )
    assert.eq(step.type, "verify")
    assert.eq(step.id, "check")
    assert.eq(step.command, "bazel test //...")
    assert.eq(step.allow_failure, True)

def test_empty_operations():
    """Test assert.empty with no operations."""
    step = edit(id="noop", operations=[])
    assert.empty(step.operations)

def test_struct_builtin():
    """Test that struct is available."""
    s = struct(name="test", value=42)
    assert.eq(s.name, "test")
    assert.eq(s.value, 42)

def test_json_module():
    """Test JSON encode/decode."""
    data = {"key": "value", "list": [1, 2, 3]}
    encoded = json.encode(data)
    decoded = json.decode(encoded)
    assert.eq(decoded["key"], "value")
    assert.len(decoded["list"], 3)

def test_module_namespaces():
    """Test that module namespaces work as structs."""
    # version module
    v = version.parse("1.2.3")
    assert.eq(v.major, 1)
    assert.true(version.at_least("2.0.0", "1.0.0"))

    # project module
    ctx = project.context()
    assert.eq(ctx.bazel_version, "8.0.0")

    # fs module
    assert.false(fs.exists("/nonexistent"))

def test_operation_tracker():
    """Test the operation tracker helper."""
    tracker = make_operation_tracker()

    # Record some operations using struct method access
    tracker.record("test_op", foo="bar")
    tracker.record("test_op", foo="baz")
    tracker.record("other_op", x=1)

    # Check operations were recorded
    assert.len(tracker.operations, 3)

    # Filter by type
    test_ops = tracker.get_by_type("test_op")
    assert.len(test_ops, 2)
    assert.not_empty(test_ops)

    # Clear
    tracker.clear()
    assert.empty(tracker.operations)
