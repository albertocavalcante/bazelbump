# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

# A recipe that tries to load a file that doesn't exist

load("./nonexistent.star", "SOME_VALUE")

recipe(
    name = "loads-missing",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
