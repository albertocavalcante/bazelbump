# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

# This file loads cycle-a.star to create a circular dependency

load("./cycle-a.star", "VALUE_A")

VALUE_B = "from-b"

# We export both values to demonstrate the cycle
COMBINED = VALUE_A + "-" + VALUE_B
