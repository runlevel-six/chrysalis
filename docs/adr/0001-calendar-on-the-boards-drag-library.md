# ADR-0001: Build the calendar on the board's drag library

- **Status:** Accepted
- **Date:** 2026-11-13

## Context

Phase 2 starts with a calendar view (DESIGN.md §3.8): month and week views of cards with due dates, on one board or
all of them, with drag to reschedule and an optional panel of undated cards that can be dragged onto a day. The
design asked for an evaluation of FullCalendar (MIT core) against Schedule-X before choosing.

What the calendar has to satisfy, beyond §3.8:

- **Accessibility (§3.11):** keyboard drag and drop with screen-reader announcements, 44 px targets, axe in CI.
- **Strict CSP:** no inline `<style>` elements (style attributes set through the CSSOM are fine).
- **Calm, themed look (§3.6, §3.12):** every color from the theme's tokens, in light and dark.
- **Small surface:** cards have a due date (or date and time), and an optional start date. There are no
  multi-hour events, resources or recurrence.

What the evaluation found (November 2026):

| | FullCalendar | Schedule-X |
| --- | --- | --- |
| License | MIT (core, day grid, time grid, interaction) | MIT (calendar, React adapter, drag and drop) |
| State | Core and React adapter at 7.x; the grid and interaction plugins still at 6.x | Calendar 4.x; drag-and-drop plugin still 3.x |
| CSP | 6.x injects its stylesheet as a runtime `<style>` element, which our policy blocks | Ships a CSS file (fine) |
| Dependencies | `temporal-polyfill` plus a headless core in 7.x | Preact and `@preact/signals` alongside React, plus `temporal-polyfill` |
| Keyboard drag | Not supported for events | Not supported |
| Theming | Its own CSS variables; partial | Its own CSS variables; partial |

Neither supports keyboard dragging, both are mid-way through a major version, and both bring their own drag system
next to the board's (dnd-kit), so the two would behave differently on one screen.

## Decision

Build the month and week views in the app, on **dnd-kit** (already used for cards, columns and checklist items) and
the theme tokens. Days are droppable, card chips are draggable, and the keyboard sensor and announcements work as
they do on the board. Opening a chip opens the card in full view on its board.

## Consequences

- One drag system, one accessibility story, one look; no new runtime dependencies.
- The calendar is ours to maintain: date arithmetic for the month grid and week (using the browser's `Intl` for the
  first day of the week and names), and the reschedule rules (keep the time of day; keep the start date's distance
  from the due date).
- If a later phase needs things a library does well and we don't (time-of-day layout of overlapping events,
  resources, recurrence), revisit this; recurring cards are Phase 3.
