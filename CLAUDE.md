# Agent instructions

- `docs/DESIGN.md` is the source of truth for what to build and why. Read it first.
- If `local/CONTEXT.md` exists, read it too. **Never copy, quote or paraphrase
  anything from `local/` into a committed file**, commit message, PR or issue.
- Stop and ask the maintainer before deviating from a **[DECIDED]** item, adding
  a feature not in the design, or touching anything outside the app's namespace.
- Run `make hooks` once per clone. Hooks run `scripts/check-private.sh`
  (denylist) and gitleaks before every commit.
- Use placeholders only: `example.com`, `board.example.home`, users `alice`/`bob`.
- Source files carry `Copyright 2026 Jason Hall` and
  `SPDX-License-Identifier: Apache-2.0` headers.
