# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

# A recipe that loads from parent directory using ../

load("../lib/helpers.star", "make_scan_step", "DEFAULT_VERSION")

recipe(
    name = "uses-parent-recipe",
    version = DEFAULT_VERSION,
    description = "A recipe that loads from parent directory",
    steps = [
        make_scan_step("detect", ["BUILD"], [r"py_binary\s*\("]),
    ],
)
