// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Applies a person's appearance preferences to the page (docs/DESIGN.md
// §3.5, §3.6, §3.12): the theme's color tokens as CSS custom properties,
// light or dark, density, and motion.
//
// Tokens are set with style.setProperty on <html>, which the strict CSP
// allows (it blocks inline <style>, not CSSOM). Inline properties win over
// the stylesheet's defaults, which remain as the first-paint fallback.

import defaultTheme from "./themes/default.json";
import gardenTheme from "./themes/butterfly-garden.json";

export type ThemeTokens = Record<string, string>;
export type Theme = { id: string; name: string; light: ThemeTokens; dark: ThemeTokens };

export type Prefs = {
  theme: string;
  mode: "system" | "light" | "dark";
  density: "comfortable" | "compact";
  motion: "system" | "reduce" | "allow";
  completionFlutter: boolean;
  highlightOverdue: boolean;
  remindersEnabled: boolean;
  timeZone: string;
};

export const DEFAULT_PREFS: Prefs = {
  theme: "default",
  mode: "system",
  density: "comfortable",
  motion: "system",
  completionFlutter: true,
  highlightOverdue: false,
  remindersEnabled: false,
  timeZone: "",
};

/** This device's IANA time zone, for reminders and "today". */
export const deviceTimeZone = () => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "";
  } catch {
    return "";
  }
};

export const BUILT_IN_THEMES: Theme[] = [defaultTheme, gardenTheme];

const darkQuery = () => window.matchMedia("(prefers-color-scheme: dark)");
const reduceQuery = () => window.matchMedia("(prefers-reduced-motion: reduce)");

export const isDark = (p: Prefs) => p.mode === "dark" || (p.mode === "system" && darkQuery().matches);

/** Whether animations should run, honoring the in-app override first. */
export const motionAllowed = (p: Prefs) => (p.motion === "system" ? !reduceQuery().matches : p.motion === "allow");

let applied: string[] = [];

export function applyAppearance(p: Prefs, themes: Theme[]) {
  const theme = themes.find((t) => t.id === p.theme) ?? themes[0];
  const dark = isDark(p);
  const tokens = dark ? theme.dark : theme.light;
  const root = document.documentElement;
  for (const k of applied) if (!(k in tokens)) root.style.removeProperty(`--${k}`);
  for (const [k, v] of Object.entries(tokens)) root.style.setProperty(`--${k}`, v);
  applied = Object.keys(tokens);
  root.style.colorScheme = dark ? "dark" : "light";
  root.classList.toggle("density-compact", p.density === "compact");
  root.classList.toggle("reduce-motion", !motionAllowed(p));
  root.classList.add("motion-set");
  root.dataset.theme = theme.id;
  document
    .querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')
    .forEach((m) => (m.content = tokens.surface ?? m.content));
}

/** Re-apply when the system's light/dark or motion setting changes. */
export function watchSystem(onChange: () => void): () => void {
  const qs = [darkQuery(), reduceQuery()];
  qs.forEach((q) => q.addEventListener("change", onChange));
  return () => qs.forEach((q) => q.removeEventListener("change", onChange));
}
