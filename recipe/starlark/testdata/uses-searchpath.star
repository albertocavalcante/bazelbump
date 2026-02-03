# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

# A recipe that loads via search path (// prefix)

load("//rules.star", "RULE_MAPPINGS", "get_load_path")

recipe(
    name = "uses-searchpath-recipe",
    version = "1.0.0",
    description = "A recipe that uses search path loads",
    steps = [
        scan(
            id = "detect",
            files = ["BUILD"],
            patterns = [r"cc_binary\s*\("],
        ),
        edit(
            id = "add-deps",
            operations = [
                ensure_bazel_dep("rules_cc", "0.1.1"),
            ],
        ),
    ],
)
