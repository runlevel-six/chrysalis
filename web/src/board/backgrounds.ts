// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Board backgrounds (docs/DESIGN.md §3.5). Gradients are presets stored by
// id, so their colors can be refined later without touching stored boards.
// All are soft and low-contrast: columns keep their own surfaces, and the
// board header gets a backing, so text stays readable on any of them.

import { attachmentUrl, type Board } from "../api";

// Each preset has a dark variant, used in dark mode so a bright background
// never glares behind dark columns.
export const GRADIENTS: { id: string; name: string; css: string; dark: string }[] = [
  {
    id: "meadow",
    name: "Meadow",
    css: "linear-gradient(160deg, #e8f0d8 0%, #f6efe0 55%, #f3dfd0 100%)",
    dark: "linear-gradient(160deg, #1f2a1b 0%, #2a2a22 55%, #33261f 100%)",
  },
  {
    id: "dawn",
    name: "Dawn",
    css: "linear-gradient(160deg, #fbe3d0 0%, #f6d6dc 50%, #e6e4f3 100%)",
    dark: "linear-gradient(160deg, #33251e 0%, #33222a 50%, #26263a 100%)",
  },
  {
    id: "sky",
    name: "Sky",
    css: "linear-gradient(180deg, #d9ecf5 0%, #eef6f8 70%, #f6f3ea 100%)",
    dark: "linear-gradient(180deg, #1c2a33 0%, #1f2a2e 70%, #262620 100%)",
  },
  {
    id: "milkweed",
    name: "Milkweed",
    css: "linear-gradient(160deg, #e4ecd0 0%, #cfdfba 100%)",
    dark: "linear-gradient(160deg, #222b1a 0%, #2c3a22 100%)",
  },
  {
    id: "dusk",
    name: "Dusk",
    css: "linear-gradient(160deg, #2a3324 0%, #3a2f3c 60%, #2b3442 100%)",
    dark: "linear-gradient(160deg, #2a3324 0%, #3a2f3c 60%, #2b3442 100%)",
  },
];

// The curated colors also have dark variants. A color picked with "Any
// color" is used exactly as chosen.
export const BACKGROUND_COLORS: { value: string; name: string; dark: string }[] = [
  { value: "#f6efe0", name: "Cream", dark: "#2b2820" },
  { value: "#e8f0d8", name: "Sage", dark: "#232b1e" },
  { value: "#d9ecf5", name: "Pale sky", dark: "#1d2a33" },
  { value: "#f6d6dc", name: "Blush", dark: "#33242a" },
  { value: "#ece4f6", name: "Lavender", dark: "#2a2533" },
  { value: "#f3dfd0", name: "Apricot", dark: "#33281f" },
  { value: "#2f3a2a", name: "Forest", dark: "#2f3a2a" },
  { value: "#2b3442", name: "Night", dark: "#2b3442" },
];

/** CSS for a board's background, or undefined for the theme's own surface. */
export function backgroundStyle(b: Board, dark: boolean): React.CSSProperties | undefined {
  const { type, value } = b.background;
  if (!value) return undefined;
  if (type === "color") {
    const preset = BACKGROUND_COLORS.find((c) => c.value === value);
    return { background: dark && preset ? preset.dark : value };
  }
  if (type === "gradient") {
    const g = GRADIENTS.find((x) => x.id === value);
    return g ? { background: dark ? g.dark : g.css } : undefined;
  }
  if (type === "image") {
    // In dark mode a photo is dimmed, so it sits back behind the columns.
    const dim = dark ? "linear-gradient(rgb(0 0 0 / 0.45), rgb(0 0 0 / 0.45)), " : "";
    return { background: `${dim}center / cover no-repeat url("${attachmentUrl(value)}")` };
  }
  return undefined;
}

/** A column tint: a palette key (themed, contrast-checked) or an older raw color. */
export const columnTint = (color: string) => (color.startsWith("#") ? color : `var(--label-${color})`);
