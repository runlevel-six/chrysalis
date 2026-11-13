// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Live sync (docs/DESIGN.md §5.5). The server streams small change notices;
// this refetches whatever they touch. Notices this tab caused are skipped,
// since it already shows the change.
//
// Refetches wait while something is in flight here (a drag, or a save not
// yet acknowledged), so another device's change never yanks a card out from
// under the pointer or flickers an optimistic update.

import { useEffect } from "react";
import { useQueryClient, type QueryClient, type QueryKey } from "@tanstack/react-query";

/** This tab's id, sent with every request so its own notices come back marked. */
export const CLIENT_ID = (() => {
  try {
    return crypto.randomUUID();
  } catch {
    return `tab-${Math.random().toString(36).slice(2)}${Date.now().toString(36)}`;
  }
})();

type Notice = {
  id: number;
  type: "change" | "resync";
  boardId?: string;
  kind?: string;
  entityId?: string;
  cardId?: string;
  origin?: string;
};

let holds = 0;
/** Pause applying remote changes (e.g. while dragging). Returns the release. */
export function holdSync(): () => void {
  holds++;
  let released = false;
  return () => {
    if (!released) {
      released = true;
      holds--;
    }
  };
}

/** The query prefixes a notice makes stale. */
export function keysFor(n: Notice): QueryKey[] | "all" {
  if (n.type === "resync") return "all";
  switch (n.kind) {
    case "prefs":
      return [["me"]];
    case "boards":
      return [["boards"]];
    case "centering":
      return [["centering"]];
    case "view":
      return n.entityId ? [["board", n.entityId, "detail"]] : [];
    case "board":
      return [["board", n.boardId], ["boards"]];
  }
  const keys: QueryKey[] = [];
  if (n.boardId) keys.push(["board", n.boardId]);
  if (n.cardId) keys.push(["card", n.cardId]);
  return keys;
}

function applier(qc: QueryClient) {
  const pending: QueryKey[] = [];
  let all = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const flush = () => {
    timer = undefined;
    if (holds > 0 || qc.isMutating() > 0) {
      timer = setTimeout(flush, 300);
      return;
    }
    if (all) {
      void qc.invalidateQueries();
    } else {
      const seen = new Set<string>();
      for (const k of pending) {
        const s = JSON.stringify(k);
        if (!seen.has(s)) {
          seen.add(s);
          void qc.invalidateQueries({ queryKey: k });
        }
      }
    }
    pending.length = 0;
    all = false;
  };
  return (n: Notice) => {
    if (n.origin === CLIENT_ID) return;
    const keys = keysFor(n);
    if (keys === "all") all = true;
    else pending.push(...keys);
    // Batch bursts (an import, a quick run of edits) into one refetch.
    timer ??= setTimeout(flush, 150);
  };
}

/** Subscribe to a board's notices (or just this user's, with no board). */
export function useLiveSync(boardId: string | undefined, enabled = true) {
  const qc = useQueryClient();
  useEffect(() => {
    if (!enabled || typeof EventSource === "undefined") return;
    const apply = applier(qc);
    const url = `/api/v1/events${boardId ? `?board=${encodeURIComponent(boardId)}` : ""}`;
    const es = new EventSource(url, { withCredentials: true });
    const onMessage = (ev: MessageEvent<string>) => {
      try {
        apply(JSON.parse(ev.data) as Notice);
      } catch {
        // a malformed notice: ignore it
      }
    };
    es.addEventListener("change", onMessage);
    es.addEventListener("resync", onMessage);
    // After a dropped connection the browser reconnects by itself, sending
    // the last id; anything missed is replayed, or a resync arrives.
    let wasOpen = false;
    es.onopen = () => {
      if (wasOpen) void qc.invalidateQueries(); // conservative after a gap
      wasOpen = true;
    };
    return () => es.close();
  }, [boardId, enabled, qc]);
}
