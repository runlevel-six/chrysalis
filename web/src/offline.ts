// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Read-only offline (docs/DESIGN.md §5.6). Every successful API read is also
// kept in IndexedDB. When the network can't be reached, reads come from that
// copy instead and the app switches to read-only with a calm banner. The
// first successful request after that switches it back and refreshes.
//
// The copy is per device and is cleared when the server says you're signed
// out. Storage can be missing or blocked; then offline viewing just isn't
// available, and everything else works as usual.

import { useSyncExternalStore } from "react";

const DB = "chrysalis-offline";
const STORE = "responses";
/** Copies older than this aren't kept: "recently opened" boards only. */
const MAX_AGE_MS = 30 * 24 * 60 * 60 * 1000;
const MAX_ENTRIES = 1500;

type Entry = { data: unknown; savedAt: number };

let opening: Promise<IDBDatabase | null> | null = null;
function open(): Promise<IDBDatabase | null> {
  opening ??= new Promise((resolve) => {
    try {
      const req = indexedDB.open(DB, 1);
      req.onupgradeneeded = () => req.result.createObjectStore(STORE);
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => resolve(null);
    } catch {
      resolve(null);
    }
  });
  return opening;
}

async function run<T>(mode: IDBTransactionMode, fn: (s: IDBObjectStore) => IDBRequest<T>): Promise<T | undefined> {
  const db = await open();
  if (!db) return undefined;
  return new Promise((resolve) => {
    try {
      const req = fn(db.transaction(STORE, mode).objectStore(STORE));
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => resolve(undefined);
    } catch {
      resolve(undefined);
    }
  });
}

let writes = 0;
export async function remember(path: string, data: unknown) {
  await run("readwrite", (s) => s.put({ data, savedAt: Date.now() } satisfies Entry, path));
  if (++writes % 100 === 0) void prune();
}

export async function recall(path: string): Promise<unknown> {
  const e = await run<Entry | undefined>("readonly", (s) => s.get(path));
  if (!e || Date.now() - e.savedAt > MAX_AGE_MS) return undefined;
  return e.data;
}

export async function forgetAll() {
  await run("readwrite", (s) => s.clear());
}

async function prune() {
  const db = await open();
  if (!db) return;
  try {
    const entries: { key: IDBValidKey; savedAt: number }[] = [];
    const tx = db.transaction(STORE, "readwrite");
    const store = tx.objectStore(STORE);
    const cursor = store.openCursor();
    cursor.onsuccess = () => {
      const c = cursor.result;
      if (c) {
        entries.push({ key: c.key, savedAt: (c.value as Entry).savedAt });
        c.continue();
        return;
      }
      entries.sort((a, b) => b.savedAt - a.savedAt);
      const cutoff = Date.now() - MAX_AGE_MS;
      entries.forEach((e, i) => {
        if (i >= MAX_ENTRIES || e.savedAt < cutoff) store.delete(e.key);
      });
    };
  } catch {
    // best effort
  }
}

// ---- Offline state ----

let offline = false;
const listeners = new Set<() => void>();
let onBackOnline: (() => void) | undefined;

export function setOffline(v: boolean) {
  if (v === offline) return;
  offline = v;
  listeners.forEach((fn) => fn());
  if (!v) onBackOnline?.();
}

/** Called once when the app comes back online (e.g. to refetch everything). */
export function whenBackOnline(fn: () => void) {
  onBackOnline = fn;
}

export const isOffline = () => offline;

export function useOffline(): boolean {
  return useSyncExternalStore(
    (fn) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    () => offline,
  );
}

/** A network failure (no response at all), as opposed to an error status. */
export const isNetworkError = (err: unknown) => err instanceof TypeError;
