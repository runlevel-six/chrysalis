#!/usr/bin/env node
// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0
//
// Fails if any production npm dependency (the code that ends up in the
// shipped bundle) has a license outside the allowlist (docs/DESIGN.md §0.2).
// Dev-only tools aren't distributed, so they aren't checked.
//
// Usage: node scripts/check-npm-licenses.mjs   (run from the repo root)

import { execFileSync } from "node:child_process";

const allowed = new Set(["MIT", "ISC", "BSD-2-Clause", "BSD-3-Clause", "Apache-2.0", "MPL-2.0", "0BSD", "CC0-1.0"]);

const out = execFileSync("npm", ["query", ".prod"], { cwd: new URL("../web/", import.meta.url), encoding: "utf8" });
const pkgs = JSON.parse(out).filter((p) => p.name !== "chrysalis-web");

// SPDX expressions like "(MIT OR Apache-2.0)" pass if any alternative is allowed.
const ok = (license) =>
  typeof license === "string" &&
  license
    .replace(/[()]/g, "")
    .split(/\s+OR\s+/)
    .some((l) => allowed.has(l.trim()));

const bad = pkgs.filter((p) => !ok(p.license));
for (const p of bad) console.error(`not allowed: ${p.name}@${p.version} (${JSON.stringify(p.license)})`);
console.log(`checked ${pkgs.length} production packages, ${bad.length} problem(s)`);
process.exit(bad.length ? 1 : 0);
