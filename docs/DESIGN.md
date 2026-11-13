# Chrysalis — Design Document

> **Name:** `chrysalis` (repo, Go module, binary, env prefix `CHRYSALIS_`) / "Chrysalis" (product name). **[DECIDED]**
> The chrysalis is where a caterpillar becomes the *imago*, the finished butterfly. Inside it, *imaginal* discs build the adult from what the caterpillar breaks down into. It's a protected space where messy, half-formed work becomes something finished. The app's whole lifecycle is expressed through the optional Metamorphosis board template (§3.12), not the name.
> **License:** Apache-2.0. **[DECIDED]**
> **Repository:** public open source from the first commit. **[DECIDED]**
> **Status:** Ready for implementation

---

## 0. How to use this document (for implementers, including Claude Code)

- This doc is the source of truth for **what** to build and **why**. Choose implementation details freely within it.
- **Private context lives in `local/CONTEXT.md`**, which is gitignored (§0.1). If it exists, read it before starting. It describes the maintainer's own deployment, the primary user's needs in detail, and private branding. **Never copy, quote or paraphrase anything from `local/` into a committed file.** That covers code, comments, docs, tests, fixtures, commit messages, PR descriptions and issue text.
- **Stop and ask the maintainer** before any of the following:
  - changing anything outside the app's own namespace (identity provider config, ingress controller, cluster-wide resources);
  - deviating from a decision marked **[DECIDED]**;
  - adding a feature not listed here.
- **Don't build speculatively.** Users will want features they haven't discovered yet. Find them by shipping a phase and watching real use, not by guessing. Out-of-scope items are listed so they aren't built early.
- Library and version facts here come from training-data knowledge as of mid-2026. **Verify current versions, maintenance status and licenses** before adopting any dependency.
- Follow the maintainer's existing public-repo pattern, as in `runlevel-six/tomekeeper` and `runlevel-six/scryglass`:
  - ship generic manifests and templates;
  - keep real deployment specifics in a gitignored local overlay.

### 0.1 Public-repo hygiene [DECIDED]

**`.gitignore` comes first.** The first commit contains `.gitignore`, `LICENSE`, `NOTICE` and a minimal `README.md`, *before* anything exists in `local/`. The ignore list includes:

```
local/
.env
*.env.local
playwright-report/
test-results/
*.trace.zip
data/            # local dev SQLite + attachments
```

**`local/` layout.** It's private and never committed:

```
local/
  CONTEXT.md        # private design context (maintainer-provided)
  deploy/k8s/       # real Kustomize overlay; references ../../../deploy/k8s/base
  branding/         # private theme.json, icons, logo
  testdata/         # real Trello exports, real-ish fixtures
  seeds/            # personal quote collections
  denylist.txt      # private terms that must never appear in commits (see below)
```

**Never commit:**
- names or details of real users;
- health or personal information;
- real hostnames, domains, IPs or email addresses;
- identity-provider or OIDC configuration with real values, or any secret;
- cluster specifics (node names, storage class names);
- private branding assets;
- real exports or screenshots containing real data.

**Placeholders in docs, examples and tests:** `example.com`, `board.example.home`, users `alice`/`bob`, boards like "Garden plans."

**Guardrails:**
- **betterleaks** for secret scanning, in a pre-commit hook and in CI.
- **`scripts/check-private.sh`**, a generic script committed to the repo:
  - if `local/denylist.txt` exists, it fails when any listed term (case-insensitive) appears in the staged diff or the commit message;
  - it runs as a pre-commit hook, installed via `make hooks` or the pre-commit framework;
  - CI can't run it, since the denylist is private. That's expected.
- Playwright traces, screenshots and videos are gitignored. They can capture real data when run against a real deployment.
- **If something private is ever committed:**
  - unpushed: rewrite history before pushing;
  - pushed: tell the maintainer immediately and rotate anything secret.

  Don't try to fix a pushed leak quietly.
- **Commit identity [DECIDED]:** the maintainer's GitHub account. The exact name and email are in `local/CONTEXT.md`. Set them repo-locally (`git config user.name` / `user.email`) and verify before the first push.
  - The email appears in commit metadata only. Don't write it into committed files.
  - For a security contact, use GitHub's private vulnerability reporting in `SECURITY.md`, not an email address.
  - `scripts/check-private.sh` scans diffs and messages, not author metadata, so the commit identity doesn't trip the denylist.

### 0.2 License obligations (Apache-2.0)

- Include `LICENSE` (full Apache-2.0 text) and a `NOTICE` file.
- **Copyright holder: Jason Hall. [DECIDED]**
  - `NOTICE` reads `Chrysalis` / `Copyright 2026 Jason Hall`.
  - `LICENSE` stays the unmodified Apache-2.0 text; the appendix boilerplate is only an example.
  - Source headers carry `Copyright 2026 Jason Hall` plus the SPDX line.
- Add `SPDX-License-Identifier: Apache-2.0` headers to source files.
- **Dependency licenses** must be compatible with distributing an Apache-2.0 binary and image:
  - MIT, BSD, ISC, Apache-2.0, MPL-2.0 (file-level) and OFL (fonts) are fine;
  - **no GPL/AGPL** dependencies in the shipped artifacts;
  - flag LGPL or anything unusual to the maintainer.
- Enforce this in CI with `go-licenses` (Go) and a license checker for npm.
- Bundled artwork and seed content must be original or public domain (§3.12, §3.13). Record sources in `seeds/quotes/SOURCES.md` and `web/src/art/SOURCES.md`.

---

## 1. Problem and goals

Kanban-style task boards, commercial and open source alike, show everything at once. For people who are easily overwhelmed by busy interfaces, a full board is the forest that hides the trees they need to focus on. That's common with ADHD and anxiety. Paid tiers of commercial tools come closer through collapsing and customization, but they still fall short, and they cost money every month.

The core need is **reducing visual and cognitive load**. The user should see only what they chose to see, while keeping the familiar board model.

### Goals (in priority order)

1. **Never overwhelm.** Everything collapses. What the user sees is what they chose to see, and it stays that way across sessions and devices.
2. **Never lose work, never punish mistakes.**
   - Autosave everything.
   - Undo every action.
   - Deleted items go to a trash, not oblivion.
3. **Feels like theirs.**
   - Rich look-and-feel customization: colors, backgrounds, card covers, themes.
   - A gentle butterfly identity (§3.12).
   - An optional centering line of Scripture or quotes (§3.13).
4. **Works everywhere they do:** laptops, phone and tablet, installable as a PWA.
5. **Reliable on a single small server.** Fast, simple, cheap to maintain and back up.
6. **Easy to self-host** for other households: generic SSO (OIDC or forward-auth), Docker Compose plus Kubernetes.

### Non-goals

- **Being a system of record for regulated or sensitive data.** The README notes that securing such data is the deployer's responsibility.
- Real-time collaborative text editing (several people typing in the same text at once).
- Enterprise multi-tenancy (separate orgs, billing, tenant isolation).
- Workflow automation (deferred).

---

## 2. Users and tenancy [DECIDED]

**The app is multi-user with per-board membership. It is not multi-tenant.**

- Every user has their own boards.
- Boards can be shared at the **viewer** or **editor** role, plus the owner. The sharing UI is Phase 2; the data model supports it from day one.
- **View state is always per-user.** Expanded/collapsed cards and columns, filters and focus mode belong to the person viewing. A collaborator expanding a card doesn't expand it for the owner.
- Users are created on first SSO login, just-in-time. Identity comes from OIDC `sub` (or the trusted `Remote-User` header), plus email and display name.

Why not full multi-tenancy: it adds tenant scoping to every query plus admin surfaces and isolation testing. That cost serves no one in a household deployment. Per-board ACLs cover "share a board with a colleague."

---

## 3. UX specification

This section matters most. When in doubt, optimize for **calm, predictable and forgiving**.

### 3.1 Core objects

- **Board:**
  - a name;
  - a background (color, gradient or uploaded image);
  - optional emoji or icon;
  - columns.
- **Column** (often called a "list"):
  - fully custom names, colors and order;
  - optional WIP-style soft limit (display only, never blocking);
  - can be collapsed.
- **Card:**
  - title;
  - rich description (text plus inline images);
  - cover (color or image);
  - labels;
  - due date (optionally with start date);
  - checklists;
  - attachments;
  - comments (lightweight; mostly useful on shared boards);
  - archived flag.

### 3.2 Card display states [DECIDED]

Each card, **per user**, is in one of three states.

1. **Collapsed (default for new cards; configurable per board).**
   - Shows the title only.
   - Optional per-board toggles add label color dots, the due date and checklist progress (e.g. `2/5`).
   - All toggles are off by default.
2. **Expanded inline.**
   - The card grows in place inside its column. The rest of the column pushes down.
   - It shows the description, checklists, labels, dates and attachments.
   - All fields are editable inline.
   - **Cards expand and collapse independently.** Any number can be open at once. This is explicitly unlike tools that open one card at a time in a modal, and is a core requirement.
3. **Full view.**
   - The card opens as a large overlay filling most of the viewport. Use this for long descriptions, many images or deep checklists.
   - Closing it returns to whatever inline state the card had before.

**How to get into each state:**

| Action | Desktop | Touch (phone/tablet) |
|---|---|---|
| Toggle collapsed ↔ expanded | Click the chevron, or click the title area; `Space` when focused | Tap the chevron or title |
| Open full view | Double-click the card, the "open full" icon, or `Enter` when focused | "Open full" icon only |
| Edit title | Click the title text while expanded, or `E` | Tap the title text while expanded |
| Drag | Click-drag anywhere on the card except interactive controls | **Long-press** then drag |

Double-tap is **not** used on touch. It conflicts with browser zoom gestures and is error-prone. Every gesture needs a visible button equivalent, because hidden gestures are bad for discoverability and add anxiety.

### 3.3 Persistence of view state [DECIDED]

View state is stored **server-side, per user**, and synced across devices. When the user opens any device, they land exactly where they left off. View state includes:

- each card's state (collapsed or expanded; full view is transient and not persisted);
- each column's collapsed state;
- the last-opened board;
- scroll position per board (best effort);
- active filters, focus mode and the hide-completed setting.

Other rules:

- Writes are debounced (~500 ms) and batched.
- Cache view state locally (IndexedDB) so the UI is instant on load, then reconcile with the server.
- **Last write wins** across devices for view state. That's acceptable; it isn't content.

### 3.4 Overwhelm controls

- **Collapse all / expand all**, per column and per board.
- **Collapsible columns.** A collapsed column becomes a narrow vertical strip showing its name and card count. Card count can be turned off per board.
- **Focus mode.** One column, or a filtered set of cards, at full emphasis; everything else is dimmed or hidden. The toggle is one click and easy to leave.
- **Hide completed.** Cards marked done, or in columns flagged as "done columns," can be hidden with a single toggle.
- **Archive** instead of delete for "I'm done with this but don't want to think about deleting it."

### 3.5 Customization (look and feel)

Treat this as a first-class feature, not polish.

- **Board background:**
  - solid color from a curated palette, or a custom color picker;
  - gradient presets;
  - uploaded image (stored as an attachment, scaled server-side).
- **Column color:** a header tint or a full column tint.
- **Card cover:** a color band or an image (from the card's own attachments).
- **Labels:** user-defined name and color, scoped per board.
- **Theme:** light, dark or system. All colors are CSS custom properties (§3.12).
- **Density:** comfortable or compact.
- **Text contrast:** card text must meet WCAG AA against any background the user picks. Auto-adjust the text color, or add a scrim behind text on image backgrounds.

### 3.6 Calm-by-default principles

- **No guilt UI.** No red "OVERDUE" badges, no flashing, no counts of failures. Overdue items get a subtle indicator only if the user turns "highlight overdue" on in settings.
- **Autosave everything.**
  - Show a subtle "Saved" indicator only.
  - There is no Save button anywhere.
  - Unsent edits survive reloads (draft kept in IndexedDB until the server acknowledges).
- **Undo, not confirm.**
  - Single destructive actions (delete card, archive, move, clear checklist) execute immediately and show an **Undo** toast for about 8 seconds.
  - Only bulk or irreversible actions ask for confirmation: deleting a column with cards in it, deleting a board, emptying the trash.
- **Trash.** Deleted cards, columns and boards are soft-deleted and restorable for 30 days (configurable).
- **Motion.** Respect `prefers-reduced-motion`, with an in-app override. Default animations are short (≤150 ms) and purposeful.
- **Quick capture.**
  - Pressing `N`, or tapping "+" at the bottom of a column, opens an inline title input.
  - `Enter` creates the card and keeps the input open for the next one.
  - `Esc` closes it.
  - Nothing else is required to create a card.
- **Predictability.** Things don't move unless the user moved them. No auto-sorting by default. New cards go to the bottom of a column unless the user chooses otherwise.

### 3.7 Mobile and tablet layout

- **Phone:**
  - one column per screen width, horizontal scroll-snap between columns;
  - column tabs or dots at the top to jump between columns.
- **Tablet:** two or three columns visible, depending on width.
- **Moving cards:**
  - drag (long-press) works across columns, with edge auto-scroll;
  - a **"Move to…"** menu on every card is the reliable alternative to drag on small screens.
- Installable as a **PWA** (home-screen icon, standalone display).

### 3.8 Calendar view (Phase 2)

- Month and week views of all cards with due dates, on one board or across all boards (toggle).
- Card chips use the card's cover or label color.
- Drag a chip to reschedule. Click it to open the card in full view.
- Undated cards appear in an optional side panel. Dragging one onto a day gives it a date.

### 3.9 Reminders (Phase 2; opt-in, always)

- **Off globally by default.** The user turns them on in settings. Each card's reminder is also individually opt-in.
- **Reminder options:**
  - at the due time;
  - a relative offset (e.g. 1 day before);
  - a custom time.
- **Delivery:**
  - in-app notification list (always);
  - **Web Push** (some mobile platforms only allow it for web apps installed to the home screen; verify current support);
  - optional email via SMTP if configured: **deferred to Phase 3** (maintainer decision, 2026-11-13).
- **Snooze:** 1 hour, tonight, tomorrow, or pick a time.
- **Quiet hours:** configurable; no pushes during them.
- **Wording is gentle:** "Heads up: *Call the clinic* is due tomorrow," never "You missed…"

### 3.10 Keyboard shortcuts (desktop)

`N` new card in the focused column · `Space` toggle expand · `Enter` full view · `E` edit title · `Esc` close or cancel · `Cmd/Ctrl+Z` undo · `F` focus mode · `/` search or filter · `?` shortcut help.

### 3.11 Accessibility

- Keyboard drag-and-drop and screen-reader announcements; the chosen drag-and-drop library must support both.
- Focus rings are visible.
- Tap targets are at least 44×44 px.
- Run an axe accessibility check in CI (see §9).

### 3.12 Themes, butterfly identity and branding

The motifs follow the calm-by-default rules (§3.6). Each one either sits still in the background or plays briefly, only in response to something the user did. Every motion respects reduced-motion settings and can be switched off on its own.

**Themes as token sets.**
- Every theme is a named set of CSS custom properties: surfaces, text, accents, label palette, column tints and focus ring.
- Each theme has a light and a dark variant.
- Built-in themes, committed:
  - **Default** (neutral);
  - **Butterfly Garden**: an original palette inspired by pollinator gardens (monarch orange, milkweed green, coneflower pink, sky). **Design it independently.** Don't derive it from any private branding theme in `local/branding/`.
- Additional themes load at runtime from the branding directory (below).
- The theme is chosen per user. Each board can override the accent color.
- **Every theme must pass WCAG AA** for text on surfaces and for text against every label color. Add an automated test that computes this from the token files. Branding themes are validated at load time; contrast failures log warnings.
- **Rule of thumb from palette work:** saturated oranges usually fail AA as body text on light surfaces. Use them for decoration and large UI, and use deeper greens or rusts for text and controls.
- **Color-vision check:** red-orange vs. green is the most commonly confused pair. Label dots must be distinguishable, and label names appear when a card is expanded.

**Motifs (all original artwork; no stock or copyrighted illustrations):**
- **App icon and favicon:** an original, simple butterfly mark. It can be replaced via branding.
- **Empty states:** a soft line-art butterfly or flower with one gentle sentence, e.g. "A fresh start. Add your first card." on an empty board.
- **Completion flutter:**
  - when a card is marked complete or dropped into a done column, a small butterfly lifts off the card (where it landed, for a move) and drifts up as it fades out (about 1.2 s; the maintainer lengthened it from ≤600 ms so it can be enjoyed);
  - it's off under reduced motion, and has its own settings toggle;
  - it never plays for bulk actions or imports.
- **Background presets** for Butterfly Garden: a few soft gradients and subtle botanical SVG patterns (low contrast, so text stays readable). The empty "choose background" panel invites uploading the user's own photos (§3.5).
- **Board templates:**
  - "Metamorphosis": *Egg → Caterpillar → Chrysalis → Imago*, with Imago pre-flagged as a done column;
  - plain *To do → Doing → Done*.

  Offered on board creation, never forced.
- **Not doing:** cursor trails, ambient flying animations, or anything that moves while the user isn't interacting. These pull attention, which is what the app exists to prevent.

**Branding directory.**
- `CHRYSALIS_BRANDING_DIR` points at an optional directory mounted into the container. It contains:
  - `theme.json`: additional token sets (same schema as built-in themes);
  - `icon.svg` / `icon.png`: app icon and PWA manifest icons;
  - optional product display name.
- Document the schema in `docs/branding.md` with a fictional example brand. Real branding lives only in `local/branding/`.

### 3.13 Centering line: Scripture and quotes

A single, quiet line at the top of the board that helps the user get centered. It must never compete with the board itself.

**Display:**
- One line, small italic text, under the board header. Roughly 28–32 px tall, matching board header spacing.
- Long entries truncate with an ellipsis. Tap or click expands the full text and the reference inline, pushing the board down slightly; tap again to collapse. This mirrors the card-collapse model.
- Small controls appear on hover or focus (on touch, after a tap expands it):
  - **next** and **previous**;
  - **pin** / **unpin**;
  - **hide for today**.
- On phones it sits under the board title and stays one line.
- **Off by default** for every new user. Turned on in settings, with a per-board show or hide override.
- No animation when an entry changes. Entries change only at a rotation boundary or on page load, never mid-glance.

**Content model:**
- A user has one or more **collections** of entries. Each entry has:
  - `text`;
  - `attribution` (author) or `reference` (e.g. "Philippians 4:6–7");
  - optional `translation`;
  - optional `tags`;
  - a position.
- Collections are shareable like boards (owner, editor, viewer), so one household member can curate a collection another sees. The sharing UI is Phase 2; Phase 1 uses import.
- A user picks which collections feed their line.

**Selection modes (per user):**
- **Rotate:**
  - interval: every page load, hourly, daily (default; changes at local midnight) or weekly;
  - order: sequential or shuffle;
  - shuffle avoids repeats until the pool is exhausted.
- **Pinned:** one entry stays until unpinned. Pinning from the line itself is one click.
- "Hide for today" suppresses the line until local midnight on all devices.
- Rotation state syncs server-side across devices, like view state (§3.3).

**Managing entries:**
- A settings screen lists collections and entries: add, edit, reorder, delete, with undo and trash as everywhere else.
- **Import and export** as YAML or JSON, from the settings UI and from an admin CLI:

  `chrysalis admin quotes import --user <email> --collection "<name>" file.yaml`

  (e.g. via `kubectl exec`). Imports are additive by default; `--replace` replaces a collection.

**Seed content and copyright:**
- No entries are enabled by default. Optional seed packs are opt-in from settings:
  - **Scripture (public-domain translations only):** KJV is public domain in the US. The World English Bible (WEB) is public domain. The Berean Standard Bible was dedicated to the public domain in 2023. Verify each translation's current status before shipping a pack.
  - **Quotes:** only clearly public-domain quotes or ones the maintainers wrote. Verify every attribution; misattributed quotes are common.
- Most modern translations are copyrighted. **Don't ship them.** Users can type in any entry for their own use.
- Seed packs live as YAML in `seeds/quotes/` with a `SOURCES.md` noting each pack's license.

---

## 4. Feature scope by phase

### Phase 0 — Skeleton

- Repo scaffold with hygiene from §0.1 first: `.gitignore`, `LICENSE`, `NOTICE`, hooks.
- CI, container build.
- Auth (both modes, §6), health endpoints.
- One empty board end to end.
- Deployed to the maintainer's environment via the local overlay.

**Exit:** the maintainer can log in through their SSO on the real ingress, create a board, reload it, and see it persist.

### Phase 1 — MVP: replace the current board tool for daily use

- Boards: create, rename, reorder, background, archive, delete to trash.
- Columns: custom name, color, order, collapse, delete to trash.
- Cards:
  - the three display states with per-user synced view state (§3.2–3.3);
  - title, rich description with inline images, cover, labels, due date and start date, checklists (multiple per card, reorderable items), attachments, archive;
  - drag-and-drop within and across columns; "Move to…" (including to another board).
- Overwhelm controls (§3.4): collapse all, column collapse, focus mode, hide completed.
- Customization (§3.5). Calm defaults (§3.6): autosave, undo toasts, trash.
- Themes and motifs (§3.12):
  - Default and Butterfly Garden themes;
  - the branding directory loader;
  - original icon, empty states, completion flutter (with its toggle);
  - board templates.
- Centering line (§3.13):
  - display, rotate and pin modes, hide for today, cross-device sync;
  - settings screen for one's own collections;
  - YAML/JSON import and export, plus the admin import CLI.
- Live sync across devices (server-sent events).
- PWA: installable; offline **read-only** viewing of last-synced boards (§5.6).
- Mobile and tablet layouts (§3.7).
- **Trello import** (§8) for boards, lists, cards, labels, checklists, due dates and comments. Attachments are imported as links.
- Backups plus a **tested restore** (§7.4).

**Exit:** the primary user uses it for a full week as their only board. The maintainer collects feedback before Phase 2 is finalized.

### Phase 2 — Next

- **Calendar view** (§3.8). This is first priority in Phase 2.
- **Reminders** (§3.9).
- **Custom fields** per board: text, number, date, single-select, checkbox. Values optionally appear on collapsed cards as small chips, behind the same toggle system as labels and dates.
- **Board sharing UI** (viewer/editor) and comments surfaced in the UI.
- **Trello attachment import**: downloads files using a user-supplied Trello API key and token (§8).
- **Card templates** and a "copy card" action.
- **Shared quote collections UI** and opt-in **seed packs** (§3.13), after license verification.

### Phase 3 — Later / maybe

- Automations ("when a card moves to Done, check all items").
- **Email reminders** via SMTP (§3.9), moved from Phase 2.
- ICS calendar feed of due dates. This needs a token-authenticated URL that bypasses SSO, a security tradeoff to design deliberately.
- Offline **editing** with a sync queue and conflict handling.
- Recurring cards.

---

## 5. Architecture

### 5.1 Overview

```
Browser / PWA (React SPA)
   │  HTTPS (session cookie)        ▲ SSE (live updates)
   ▼                                │
[Ingress + identity provider]  ──(OIDC or forward-auth)──
   │
   ▼
chrysalis (single Go binary)
   ├─ serves embedded SPA (embed.FS)
   ├─ REST JSON API  /api/v1/...
   ├─ SSE hub        /api/v1/events
   ├─ auth middleware (OIDC | trusted-header)
   ├─ attachment service (local FS | S3)
   └─ background jobs (trash purge, reminders, image thumbnails, backups)
   │
   ├─ SQLite (WAL) on PVC / volume
   └─ attachments on PVC / volume  (or S3-compatible bucket)
```

One process with one replica. That's appropriate for SQLite and a household-scale workload.

### 5.2 Backend [DECIDED: Go]

**Rationale:**

- A single static binary with the frontend embedded keeps the container tiny.
- Deployment is the same story for Compose and Kubernetes.
- It's low maintenance.

The backend is mostly CRUD, auth and file handling. UX quality comes from the frontend. Write idiomatic, readable Go, and explain non-obvious patterns in comments and PR descriptions.

Recommended building blocks (verify current status and licenses):

| Concern | Choice | Notes |
|---|---|---|
| HTTP routing | `net/http` (Go ≥1.22 pattern routing), optionally `chi` | Keep it minimal. |
| SQLite driver | `modernc.org/sqlite` (pure Go) | No CGO, so static builds are easy. Performance is ample at this scale. |
| Queries | `sqlc` | Type-safe generated queries. |
| Migrations | `goose` or `atlas` | Embed migrations; run automatically on startup. |
| OIDC | `github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2` | |
| Sessions | Server-side session table + secure cookie | `HttpOnly`, `Secure`, `SameSite=Lax`. |
| Images | `golang.org/x/image` / `disintegration/imaging` | Thumbnails and background scaling; strip EXIF. |
| Config | Env vars + optional YAML file | 12-factor; every setting has an env var. |
| Logging | `log/slog` (JSON) | |

### 5.3 Frontend [DECIDED: React + TypeScript]

**Rationale:** React has the most mature, accessible drag-and-drop and rich-text ecosystem, and AI coding tools are strongest in it. Ecosystem maturity matters more here than framework elegance.

| Concern | Choice | Notes |
|---|---|---|
| Build | Vite | |
| Server state | TanStack Query | Optimistic updates with rollback. |
| Drag-and-drop | `@atlaskit/pragmatic-drag-and-drop` (preferred) or `dnd-kit` | Pragmatic DnD was built for kanban-style boards. Must support touch, keyboard and auto-scroll. Prototype both briefly if unsure. |
| Rich text | TipTap (ProseMirror) | Store as ProseMirror JSON. Inline images upload as attachments. |
| UI primitives | Radix UI (or similar headless, accessible library) | Menus, dialogs, popovers. |
| Styling | Tailwind CSS + CSS custom properties for theming | |
| Calendar (Phase 2) | Evaluate FullCalendar (MIT core) vs. Schedule-X | Must support drag-to-reschedule. Check license. |
| PWA | `vite-plugin-pwa` (Workbox) | |
| Local cache | IndexedDB via `idb` or Dexie | View state, last-synced boards, unsent drafts. |
| Card ordering | Fractional indexing (`fractional-indexing` package or equivalent) | Ordering keys are strings. |
| Fonts | Self-hosted, OFL-licensed only | No runtime font CDNs (CSP, privacy). |

### 5.4 Ordering

- Columns, cards and checklist items carry a `position` string (fractional index).
- A move updates one row. The server rebalances keys only if they grow too long; this is rare and done as a background job.

### 5.5 Concurrency and sync

- Every content row has an integer `version`.
- Mutations send the expected version:
  - **For simple fields** (title, dates, labels, position), a mismatch means last write wins. The server returns the new state and the client reconciles silently.
  - **For descriptions**, a mismatch returns `409`. The client shows a calm "This card was edited on another device — keep yours / use theirs / view both" prompt.
- **SSE:**
  - Clients subscribe per board, plus a per-user channel (view state, notifications, centering line).
  - Events carry entity type, id, version and the payload.
  - Clients reconnect with `Last-Event-ID`. If the gap is too large, the server sends a "resync" and the client refetches.
- View-state writes go through the same path but are low priority and debounced.

### 5.6 Offline [DECIDED: read-only offline in Phase 1]

- The service worker caches the app shell.
- The client keeps the last-synced full state of recently opened boards in IndexedDB.
- When offline, boards open **read-only**, with a small, calm "Offline — viewing last synced copy" banner.
- Unsent text drafts are kept locally and retried when back online.
- Full offline editing is Phase 3. It's a large source of bugs and isn't needed to meet the goals.

---

## 6. Authentication and authorization

### 6.1 Modes [DECIDED: support both]

`CHRYSALIS_AUTH_MODE` takes one of three values:

1. **`oidc`** (default).
   - The app is an OIDC relying party using the authorization-code flow with PKCE.
   - It works with Authelia, Authentik, Keycloak, Pocket ID and other OIDC providers.
   - Settings: `CHRYSALIS_OIDC_ISSUER`, `CHRYSALIS_OIDC_CLIENT_ID`, `CHRYSALIS_OIDC_CLIENT_SECRET`, `CHRYSALIS_OIDC_REDIRECT_URL`, `CHRYSALIS_OIDC_SCOPES` (default `openid profile email`).
   - Optional `CHRYSALIS_OIDC_ALLOWED_GROUPS` gates access by a groups claim.
2. **`header`** (trusted forward-auth).
   - The ingress does auth (Authelia forward-auth, oauth2-proxy, etc.).
   - The app reads `Remote-User`, `Remote-Email`, `Remote-Name` and `Remote-Groups`; the header names are configurable.
   - **Headers are trusted only from `CHRYSALIS_TRUSTED_PROXY_CIDRS`.** Requests from elsewhere are rejected.
   - Kubernetes manifests must include a NetworkPolicy that allows ingress to the app pod only from the ingress controller.
   - Startup **fails** if `header` mode is set and no trusted CIDRs are configured.
3. **`dev`** (local development and tests only).
   - A fixed user, enabled only with `CHRYSALIS_DEV_MODE=true`.
   - Logs a loud warning. Never in shipped manifests.

OIDC is the recommended mode, because it doesn't depend on network topology for security.

### 6.2 Sessions and security

- Server-side sessions keyed by a random ID in the cookie. Idle timeout is configurable; default is 30 days rolling, because constant re-login is friction in a personal app.
- CSRF protection:
  - double-submit token or `SameSite=Lax`;
  - all mutations are non-GET;
  - check `Origin`.
- Strict CSP. No third-party script or font origins at runtime; bundle everything.
- Authorization: every board- and collection-scoped query joins on its members table for the current user. Write tests asserting that user B cannot read or mutate user A's boards or collections via any endpoint (§9).

---

## 7. Data

### 7.1 Schema (SQLite; illustrative, refine as needed)

Conventions:

- IDs are ULIDs (text).
- Timestamps are UTC RFC 3339.
- Content tables have `created_at`, `updated_at`, `deleted_at` (soft delete) and `version`.

```sql
users(id, oidc_sub UNIQUE, email, display_name, created_at, last_seen_at)
sessions(id, user_id, created_at, expires_at, user_agent)
user_prefs(user_id PK, theme, density, reduced_motion, completion_flutter,
           highlight_overdue, reminders_enabled, quiet_hours_start, quiet_hours_end,
           timezone, json_extra)

boards(id, owner_id, name, icon, background_type /* color|gradient|image */,
       background_value, settings_json /* default card state, collapsed-card toggles,
       card count on collapsed columns, done-column ids, accent override */,
       position, archived_at, ...)
board_members(board_id, user_id, role /* owner|editor|viewer */, PRIMARY KEY(board_id,user_id))

columns(id, board_id, name, color, color_mode /* header|full */, soft_limit,
        is_done_column, position, ...)

cards(id, board_id, column_id, title, description_json /* ProseMirror JSON */,
      cover_type /* none|color|image */, cover_value, start_at, due_at,
      due_has_time, completed_at, position, archived_at, ...)

labels(id, board_id, name, color, position)
card_labels(card_id, label_id)

checklists(id, card_id, title, position, ...)
checklist_items(id, checklist_id, text, done_at, due_at, position, ...)

attachments(id, card_id NULL, board_id, uploader_id, kind /* file|image|link */,
            filename, mime, size_bytes, storage_key, url /* for links */,
            width, height, thumb_key, created_at, deleted_at)

comments(id, card_id, author_id, body_json, created_at, updated_at, deleted_at)

-- Per-user view state (synced across devices; NOT content)
user_card_view(user_id, card_id, expanded BOOLEAN, updated_at, PRIMARY KEY(user_id,card_id))
user_column_view(user_id, column_id, collapsed BOOLEAN, updated_at, PRIMARY KEY(user_id,column_id))
user_board_view(user_id, board_id, focus_json, filters_json, hide_completed,
                scroll_json, show_centering_line /* NULL = follow user setting */,
                last_opened_at, PRIMARY KEY(user_id,board_id))

-- Centering line (§3.13)
quote_collections(id, owner_id, name, position, created_at, updated_at, deleted_at, version)
quote_collection_members(collection_id, user_id, role /* owner|editor|viewer */,
                         PRIMARY KEY(collection_id,user_id))
quotes(id, collection_id, text, attribution, reference, translation, tags_json,
       position, created_at, updated_at, deleted_at, version)
user_quote_prefs(user_id PK, enabled /* default false */, collection_ids_json,
                 mode /* rotate|pinned */, interval /* load|hourly|daily|weekly */,
                 order_mode /* sequential|shuffle */, pinned_quote_id,
                 current_quote_id, rotated_at, shuffle_seen_json, hidden_until)

-- Phase 2
reminders(id, card_id, user_id, fire_at, offset_spec, snoozed_until, sent_at, channel_mask)
push_subscriptions(id, user_id, endpoint, keys_json, created_at)
notifications(id, user_id, kind, payload_json, created_at, read_at)
custom_field_defs(id, board_id, name, type, options_json, show_on_collapsed, position)
custom_field_values(card_id, field_id, value_json, PRIMARY KEY(card_id, field_id))

-- Ops
activity_log(id, board_id, user_id, entity_type, entity_id, action, before_json, after_json, at)
schema_migrations(...)
```

- `user_card_view` has **no row** for a card the user has never toggled. Absence means "use the board's default card state."
- `activity_log` supports troubleshooting and future automations. Undo is primarily client-side (§7.2), but the log gives a server-side record.

### 7.2 Undo

- The client keeps an undo stack of recent actions, each with its inverse operation:
  - move → move back;
  - delete → restore;
  - edit field → set old value.
- `Cmd/Ctrl+Z` and the toast both pop it.
- The inverse is executed as a normal API call, which keeps the server simple.
- The stack holds the last ~50 actions per tab and isn't persisted.
- The trash (soft delete) is the durable safety net beyond the session.

### 7.3 Attachments

- **Storage backends:**
  - `local`: a directory on the data volume, the default;
  - `s3`: any S3-compatible endpoint (Ceph RGW, MinIO, etc.).
- **Limits:**
  - upload limit configurable (default 25 MB);
  - MIME is sniffed server-side, not trusted from the client.
- **Images:**
  - EXIF stripped (privacy: GPS);
  - thumbnails generated;
  - backgrounds scaled to a max of 2560 px.
- Files are served only through authenticated endpoints that check membership. Never public URLs.
- Inline images in descriptions reference `attachment:<id>`. The client resolves these to authenticated URLs.

### 7.4 Backups and restore [DECIDED]

- **SQLite:** a scheduled job runs `VACUUM INTO` or the online backup API to a timestamped file. The job is a Kubernetes CronJob, or an internal scheduler for Compose users. Optionally push copies to S3.
  - Don't use a database operator; it's overkill at this scale.
- **Attachments:**
  - local backend: the backup job tars or rsyncs them;
  - S3 backend: rely on bucket versioning or replication.
- **Retention:** configurable, defaulting to 7 daily and 4 weekly copies.
- **Restore must be documented and tested.** Add a script and a CI or integration test that restores a backup into a fresh instance and verifies the contents.
- *Optional alternative to evaluate:* Litestream continuous replication to S3. Present it to the maintainer as an option; don't adopt it unilaterally.

---

## 8. Trello import

- **Input:** Trello board JSON export, uploaded via the UI ("Import from Trello").
  - Trello offers JSON export under Menu → Print, export, and share → Export as JSON.
  - Verify this is available on the free plan; CSV export is believed to be premium-only.
- **Mapping:**

  | Trello | Chrysalis |
  |---|---|
  | board (name, background color; background image if downloadable) | board |
  | lists (order, closed → archived) | columns |
  | cards: name, desc, due, start, dueComplete, closed | cards: title, description, due_at, start_at, completed_at, archived_at |
  | card desc (Markdown) | ProseMirror JSON |
  | labels (name, color mapped to the nearest palette color) | labels |
  | checklists + items (state, order) | checklists + checklist_items |
  | comment actions | comments, attributed to the importing user with an "Originally by <name>" prefix |
  | cover color | card cover |

- **Attachments:**
  - Phase 1 imports them as `link` attachments using their URLs.
  - Downloading Trello-hosted files is believed to require authenticated API access. In Phase 2, accept an optional Trello API key and token, used only for the import and never stored, to fetch files.
- **Behavior:**
  - The import is **idempotent per run**: it creates a new board and never merges into an existing one.
  - It reports what was skipped (power-up data, custom fields, etc.).
- **Testing:**
  - Committed golden-file fixtures must be **synthetic**: a hand-built board with fictional content covering every mapped feature.
  - Real exports go only in `local/testdata/`, and are used for local validation runs.

---

## 9. Testing strategy

- **Backend (Go):**
  - Unit tests for services.
  - Integration tests against a temp-file SQLite with real migrations.
  - **Authorization matrix tests:** for every endpoint, user B is denied access to user A's boards and collections; viewers can't mutate.
  - Auth tests:
    - OIDC flow against a mock issuer;
    - header mode rejects requests from untrusted IPs;
    - startup fails with header mode and no CIDRs.
  - Trello import golden-file tests (synthetic fixtures).
  - Centering-line selection logic with an injected clock:
    - daily rotation changes at the user's local midnight, not UTC;
    - shuffle doesn't repeat until the pool is exhausted;
    - pinned survives rotation boundaries;
    - "hide for today" expires at local midnight.
  - Quote import/export round-trip, additive and `--replace`.
  - Theme contrast test: every built-in theme token pair meets WCAG AA (§3.12). Branding themes are validated at load.
  - Backup and restore round-trip test.
- **Frontend:**
  - Vitest for logic: ordering keys, undo stack, view-state reconciliation.
  - **Playwright end-to-end** on desktop Chromium/WebKit plus emulated phone and tablet viewports with touch. Required flows:
    - create a board, columns and cards;
    - expand three cards independently, reload, and confirm all three are still expanded;
    - the same in a second browser context as the same user, confirming state syncs;
    - collapse a column, then use focus mode;
    - drag a card across columns, mouse and touch long-press;
    - "Move to…" on a phone viewport;
    - delete a card, undo via the toast, and confirm it's restored;
    - delete a card, restore it from the trash;
    - upload an image into a description; set a board background image;
    - go offline and confirm the board is viewable read-only with the banner;
    - turn on the centering line, pin an entry, reload on a second context, and confirm the same entry is pinned;
    - complete a card and confirm the flutter plays; with reduced motion emulated, confirm it doesn't play.
  - axe accessibility checks on the main views, failing on serious or critical issues.
  - Playwright artifacts stay out of git (§0.1).
- **Performance budget:** a board with 20 columns × 50 cards (1,000 cards) loads interactive in under 1.5 s on a modest single-node server (e.g. a mini PC), and drags stay smooth (no long tasks over 100 ms during drag).
- **Human acceptance:** each phase ends with real use by the primary user. Their feedback beats any checklist above.

---

## 10. Deployment

### 10.1 Container

- Multi-stage build:
  1. Node stage builds the SPA;
  2. Go stage embeds it and builds a static binary;
  3. final image is distroless static (or scratch with CA certificates).
- Runs as non-root with a read-only root filesystem. `/data` is the only writable path; branding is mounted read-only.
- Multi-arch `linux/amd64` + `linux/arm64`, so self-hosters on small ARM single-board computers are covered.
- Endpoints:
  - `GET /healthz` (liveness);
  - `GET /readyz` (DB reachable, migrations applied);
  - `GET /metrics` (Prometheus, optional via config).

### 10.2 Docker Compose (committed)

- `compose.yaml`: the app plus a named volume for `/data`, in `oidc` mode, with `.env.example` holding placeholder values only.
- `compose.forward-auth.example.yaml`: a commented example of header mode behind a reverse proxy with forward-auth (Authelia shown as one example). It's a reference only; Compose users bring their own proxy.

### 10.3 Kubernetes (committed)

Kustomize layout:

```
deploy/k8s/
  base/            # Deployment, Service, PVC, ConfigMap, NetworkPolicy, CronJob (backup), ServiceAccount
  overlays/
    example-oidc/
    example-header/
local/deploy/k8s/  # GITIGNORED: the maintainer's real overlay, based on ../../../deploy/k8s/base
```

- **Deployment:**
  - `replicas: 1`, `strategy: Recreate`, since an RWO volume and SQLite mean only one writer;
  - resource requests sized small (e.g. 50m CPU / 128Mi);
  - securityContext hardened.
- **PVC:** RWO, storage class parameterized (e.g. Ceph RBD, Longhorn, local-path).
- **NetworkPolicy:** ingress only from the ingress controller's namespace and labels; egress to the OIDC issuer, DNS and SMTP/S3 if configured.
- **Ingress:** parameterized in overlays. The examples cover a generic Ingress with both auth modes.
- **Branding:** overlays can mount a branding directory at `CHRYSALIS_BRANDING_DIR`, as a ConfigMap for `theme.json` and `binaryData` or a volume for icons. The base has no branding.
- **Secrets:** OIDC client secret and session key, as a Kubernetes Secret created by the overlay. Examples reference a Secret by name and never contain values.
- A Helm chart is **not** required. If community demand appears later, generate one from the Kustomize base.

### 10.4 CI

- On every PR:
  - lint (golangci-lint, eslint, prettier);
  - unit, integration and Playwright tests;
  - axe checks;
  - betterleaks;
  - dependency license checks (§0.2);
  - multi-arch image build.
- Tagged releases publish images to GHCR.

---

## 11. Configuration reference (initial)

| Var | Default | Purpose |
|---|---|---|
| `CHRYSALIS_LISTEN_ADDR` | `:8080` | HTTP listen |
| `CHRYSALIS_BASE_URL` | — | External URL (required; used for OIDC redirect and cookies) |
| `CHRYSALIS_DATA_DIR` | `/data` | SQLite + local attachments |
| `CHRYSALIS_AUTH_MODE` | `oidc` | `oidc` \| `header` \| `dev` |
| `CHRYSALIS_OIDC_*` | — | See §6.1 |
| `CHRYSALIS_TRUSTED_PROXY_CIDRS` | — | Required for `header` mode |
| `CHRYSALIS_HEADER_USER` / `_EMAIL` / `_NAME` / `_GROUPS` | `Remote-User` etc. | Header names |
| `CHRYSALIS_SESSION_TTL` | `720h` | Rolling session lifetime |
| `CHRYSALIS_STORAGE_BACKEND` | `local` | `local` \| `s3` |
| `CHRYSALIS_S3_*` | — | Endpoint, bucket, keys, region |
| `CHRYSALIS_MAX_UPLOAD_MB` | `25` | Upload limit |
| `CHRYSALIS_TRASH_RETENTION_DAYS` | `30` | Soft-delete purge |
| `CHRYSALIS_BACKUP_*` | — | Schedule, retention, target |
| `CHRYSALIS_SMTP_*` | — | Optional email reminders (Phase 3) |
| `CHRYSALIS_VAPID_*` | — | Web Push keys (Phase 2; generate if absent and persist) |
| `CHRYSALIS_METRICS_ENABLED` | `false` | Prometheus endpoint |
| `CHRYSALIS_BRANDING_DIR` | — | Optional: extra themes, icons, display name (§3.12) |

---

## 12. Repository layout (suggested)

```
.gitignore               # first commit (§0.1)
LICENSE                  # Apache-2.0
NOTICE
README.md                # incl. note that securing sensitive data is the deployer's responsibility
CLAUDE.md                # generic agent instructions; points to local/CONTEXT.md if present
cmd/chrysalis/           # main
internal/
  api/                   # handlers, middleware
  auth/                  # oidc, header, dev, sessions
  store/                 # sqlc queries, migrations
  service/               # boards, cards, views, attachments, import, reminders
  quotes/                # centering-line selection, import/export
  sse/                   # event hub
  storage/               # local, s3
  branding/              # theme/icon loader + contrast validation
  jobs/                  # trash purge, backups, reminders, rebalancing
web/                     # React app (Vite)
  src/themes/            # token sets: default, butterfly-garden
  src/art/               # original SVG: icon, empty states, flutter (+ SOURCES.md)
seeds/
  quotes/                # opt-in seed packs + SOURCES.md (licenses)
deploy/
  compose/
  k8s/
docs/                    # Diátaxis: each page is one kind; README.md indexes them by reader
  README.md              # index with reading paths
  DESIGN.md              # this document
  how-to/                # restore from a backup, configure auth (both modes), import from Trello, add a branding theme
  reference/             # configuration (env vars), theme.json schema with a fictional example, API, glossary
  explanation/           # architecture, security model, why calm-by-default
  tutorials/             # e.g. self-host with Docker Compose, start to finish
  adr/                   # decision records
scripts/
  check-private.sh       # denylist check against local/denylist.txt
testdata/                # synthetic fixtures only
local/                   # GITIGNORED (§0.1)
```

---

## 13. Open questions (non-blocking; ask the maintainer when you reach them)

1. **Name collisions.** `chrysalis` is decided. Check GitHub, GHCR and package registries before the first public push. Raise any conflict; don't rename on your own.
2. **Backup target** for the reference deployment: local volume only, or also S3 / off-site?
3. **Default card state for new boards.** Collapsed is assumed. Confirm during the Phase 1 trial.
4. **Contributor terms.** Is a DCO sign-off wanted, or plain Apache-2.0 inbound=outbound? Decide before accepting outside PRs.

---

## 14. Success criteria

- The primary user chooses it over their previous board after the Phase 1 trial week, and says it feels *less* overwhelming, not just equivalent.
- No data-loss incidents; restore has been tested at least once on a real deployment.
- Maintenance takes under an hour a month.
- Nothing private has ever been committed.
