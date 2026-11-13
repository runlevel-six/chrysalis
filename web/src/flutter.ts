// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// The completion flutter (docs/DESIGN.md §3.12): when a card is finished, a
// small butterfly lifts off it and fades out (under 600 ms). It only plays
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

export const FLUTTER_MS = 600;

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
