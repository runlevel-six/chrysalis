// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package main

// Time zone data built into the binary, so the centering line can find each
// person's local midnight (docs/DESIGN.md §3.13) even on an image without
// /usr/share/zoneinfo.
import _ "time/tzdata"
