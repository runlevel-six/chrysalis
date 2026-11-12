// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0
//
// Renders the PNG icons in public/ from the SVG masters in src/art/.
// Run after changing the art:  cd web && npm run icons
//
//   favicon.png           32px, from mark-small.svg (transparent), for
//                         browsers without SVG favicon support
//   apple-touch-icon.png  180px, from icon-maskable.svg (opaque; iOS needs it)
//   icon-192.png/512.png  from icon-maskable.svg, for the PWA manifest

import { readFileSync } from "node:fs";
import { chromium } from "@playwright/test";

const art = (name) => readFileSync(new URL(`../src/art/${name}`, import.meta.url), "utf8");
const out = (name) => new URL(`../public/${name}`, import.meta.url).pathname;

const icons = [
  { file: "favicon.png", svg: "mark-small.svg", size: 32, transparent: true },
  { file: "apple-touch-icon.png", svg: "icon-maskable.svg", size: 180 },
  { file: "icon-192.png", svg: "icon-maskable.svg", size: 192 },
  { file: "icon-512.png", svg: "icon-maskable.svg", size: 512 },
];

const browser = await chromium.launch();
for (const { file, svg, size, transparent } of icons) {
  const page = await browser.newPage({ viewport: { width: size, height: size } });
  await page.setContent(
    `<style>html,body{margin:0;background:transparent}svg{display:block;width:${size}px;height:${size}px}</style>${art(svg)}`,
  );
  await page.locator("svg").screenshot({ path: out(file), omitBackground: !!transparent });
  await page.close();
  console.log(`wrote public/${file}`);
}
await browser.close();
