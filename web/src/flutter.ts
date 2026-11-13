// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// The completion flutter (docs/DESIGN.md §3.12): when a card is finished, a
// small butterfly lifts off it and drifts up as it fades (about 1.2 s). It only plays
// in response to something the person just did to one card, never for bulk
// actions or imports, never under reduced motion, and it can be switched off
// in Settings.

import flutterSvg from "./art/flutter.svg?raw";
import { DEFAULT_PREFS, motionAllowed, type Prefs } from "./appearance";

// The art's groups are ids; several flutters can be on screen at once, so
// use classes instead (ids must be unique).
const markup = flutterSvg
  .replace('id="wing-left"', 'class="wing-left"')
  .replace('id="wing-right"', 'class="wing-right"')
  .replace('id="body"', 'class="body"');

export const FLUTTER_MS = 1200;

/** Play the flutter from a card, if the person's settings allow it. */
export function flutterFrom(cardId: string, prefs: Partial<Prefs> | undefined) {
  const p = { ...DEFAULT_PREFS, ...(prefs ?? {}) };
  if (!p.completionFlutter || !motionAllowed(p)) return;
  const card = document.querySelector<HTMLElement>(`[data-card-id="${CSS.escape(cardId)}"]`);
  if (!card) return;
  const r = card.getBoundingClientRect();
  const el = document.createElement("div");
  el.className = "flutter";
  el.setAttribute("aria-hidden", "true");
  el.innerHTML = markup; // our own bundled SVG, not user content
  // Start near the card's top-right, where the eye already is.
  el.style.left = `${Math.round(r.right - 44)}px`;
  el.style.top = `${Math.round(r.top + 4)}px`;
  document.body.appendChild(el);
  window.setTimeout(() => el.remove(), FLUTTER_MS + 50);
}

/**
 * Play the flutter once a moved card has rendered in its new place (after a
 * drag or "Move to…" into a done column), so it rises from where the card
 * landed rather than where it was.
 */
export function flutterAfterMove(cardId: string, prefs: Partial<Prefs> | undefined) {
  requestAnimationFrame(() => requestAnimationFrame(() => flutterFrom(cardId, prefs)));
}
