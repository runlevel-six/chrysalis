// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Shared pieces of the centering line (docs/DESIGN.md §3.13): query keys,
// the line's query, and how an entry's source reads.

import { useQuery } from "@tanstack/react-query";

import { api, type QuoteEntry } from "./api";

// Every key starts with "centering", so one live-sync notice refreshes them all.
export const centeringKeys = {
  line: ["centering", "line"] as const,
  settings: ["centering", "settings"] as const,
  collections: ["centering", "collections"] as const,
  entries: (collectionId: string) => ["centering", "entries", collectionId] as const,
  trash: ["centering", "trash"] as const,
};

/**
 * What the line shows now. The server decides (and keeps every device in
 * step); this only asks. It doesn't refetch on its own timer: entries change
 * at a page load or when you come back to the app, never mid-glance.
 */
export function useCenteringLine() {
  return useQuery({ queryKey: centeringKeys.line, queryFn: api.centeringLine, staleTime: 5 * 60_000 });
}

/** "A gardener, Notebook 3 (Plain)": who said it, where, in which translation. */
export function entrySource(e: Pick<QuoteEntry, "attribution" | "reference" | "translation">): string {
  const who = [e.attribution, e.reference].filter(Boolean).join(", ");
  if (!e.translation) return who;
  return who ? `${who} (${e.translation})` : e.translation;
}
