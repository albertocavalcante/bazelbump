# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

# A simple recipe that doesn't use load()

recipe(
    name = "simple-recipe",
    version = "1.0.0",
    description = "A simple test recipe",
    steps = [
        scan(
            id = "detect",
            name = "Detect patterns",
            files = ["BUILD"],
            patterns = [r"cc_binary\s*\("],
        ),
    ],
)
