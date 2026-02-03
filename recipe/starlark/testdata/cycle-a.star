# Copyright 2025 bazelbump authors
# SPDX-License-Identifier: Apache-2.0

# This file loads cycle-b.star to create a circular dependency

load("./cycle-b.star", "VALUE_B")

VALUE_A = "from-a"

# We export both values to demonstrate the cycle
COMBINED = VALUE_A + "-" + VALUE_B
