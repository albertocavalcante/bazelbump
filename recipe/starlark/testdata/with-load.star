# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

# A recipe that loads helper functions from a local library

load("./lib/helpers.star", "make_scan_step", "make_edit_step", "DEFAULT_VERSION", "CC_PATTERNS")

recipe(
    name = "with-load-recipe",
    version = DEFAULT_VERSION,
    description = "A recipe using loaded helpers",
    steps = [
        make_scan_step("detect-cc", ["BUILD"], CC_PATTERNS),
        make_edit_step("add-dep", "rules_cc", "0.1.1"),
    ],
)
