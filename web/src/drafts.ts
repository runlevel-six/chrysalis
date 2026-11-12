// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Unsent edits survive reloads (docs/DESIGN.md §3.6): a description draft is
// kept in IndexedDB until the server acknowledges it. Storage can be missing
// or blocked (private browsing, strict settings); then drafts just aren't
// kept, and editing still works.

export type Draft = { doc: unknown; baseVersion: number; savedAt: number };

const DB = "chrysalis";
const STORE = "drafts";

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

const key = (cardId: string) => `description:${cardId}`;

export const drafts = {
  get: (cardId: string) => run<Draft | undefined>("readonly", (s) => s.get(key(cardId))),
  put: (cardId: string, d: Draft) => run("readwrite", (s) => s.put(d, key(cardId))),
  remove: (cardId: string) => run("readwrite", (s) => s.delete(key(cardId))),
};
