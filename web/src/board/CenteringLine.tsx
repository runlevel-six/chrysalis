// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// The centering line (docs/DESIGN.md §3.13): one quiet, italic line under
// the board header. Click (or Enter/Space) shows the whole entry and its
// source, pushing the board down a little; again to fold it back. Small
// controls appear on hover or focus, and on touch once it's open.
//
// It never animates, and the entry only changes when the server says a
// rotation boundary passed (or when the person steps it themselves).

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { api, type CenteringLine as Line } from "../api";
import { centeringKeys, entrySource, useCenteringLine } from "../centering";
import { Icon } from "../ui";
import { useUndo } from "../undo";

export function CenteringLine() {
  const line = useCenteringLine().data;
  const qc = useQueryClient();
  const { push, notify } = useUndo();
  const [expanded, setExpanded] = useState(false);
  const [busy, setBusy] = useState(false);

  const entry = line?.enabled && !line.hidden ? line.entry : null;
  if (!entry || !line) return null;

  const run = async (action: () => Promise<Line>) => {
    if (busy) return undefined;
    setBusy(true);
    try {
      const next = await action();
      qc.setQueryData(centeringKeys.line, next);
      return next;
    } catch {
      notify("That didn't go through. Check your connection and try again.");
      return undefined;
    } finally {
      setBusy(false);
    }
  };
  const source = entrySource(entry);

  return (
    <div className={`centering-line${expanded ? " is-expanded" : ""}`} role="region" aria-label="Centering line">
      <button
        type="button"
        className="centering-text"
        aria-expanded={expanded}
        title={expanded ? undefined : "Show all of it"}
        onClick={() => setExpanded((e) => !e)}
      >
        <span className="centering-quote">{entry.text}</span>
        {expanded && source && <span className="centering-source">— {source}</span>}
      </button>
      <span className="centering-controls">
        {!line.pinned && (
          <>
            <button
              type="button"
              className="icon-button"
              aria-label="Previous entry"
              title="Previous entry"
              onClick={() => void run(api.centeringPrevious)}
            >
              <Icon.ChevronLeft />
            </button>
            <button type="button" className="icon-button" aria-label="Next entry" title="Next entry" onClick={() => void run(api.centeringNext)}>
              <Icon.ChevronRight />
            </button>
          </>
        )}
        <button
          type="button"
          className={`icon-button${line.pinned ? " is-on" : ""}`}
          aria-label={line.pinned ? "Unpin this entry" : "Pin this entry"}
          title={line.pinned ? "Unpin: let it change again" : "Pin: keep this one"}
          onClick={() => void run(line.pinned ? api.centeringUnpin : () => api.centeringPin(entry.id))}
        >
          <Icon.Pin />
        </button>
        <button
          type="button"
          className="icon-button"
          aria-label="Hide for today"
          title="Hide for today"
          onClick={async () => {
            if (await run(api.centeringHide)) push("Centering line hidden until tomorrow", () => run(api.centeringShow));
          }}
        >
          <Icon.EyeOff />
        </button>
      </span>
    </div>
  );
}
