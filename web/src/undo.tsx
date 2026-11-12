// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Undo, not confirm (docs/DESIGN.md §3.6, §7.2). Every reversible action
// pushes its inverse here. The latest one shows in a toast for ~8 seconds,
// and Cmd/Ctrl+Z (outside text fields) pops the stack. The stack holds the
// last 50 actions in this tab and isn't persisted; the trash is the durable
// safety net.

import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";

type Entry = { id: number; label: string; undo: () => Promise<unknown> };
type Toast = { id: number; message: string; entry?: Entry };

type UndoApi = {
  /** Record an undoable action and show it in the toast. */
  push: (label: string, undo: () => Promise<unknown>) => void;
  /** Show a message with no undo (e.g. a calm error). */
  notify: (message: string) => void;
};

const UndoContext = createContext<UndoApi>({ push: () => {}, notify: () => {} });

export const useUndo = () => useContext(UndoContext);

const MAX = 50;
const TOAST_MS = 8000;

function isTyping(el: Element | null) {
  return (
    el instanceof HTMLInputElement ||
    el instanceof HTMLTextAreaElement ||
    el instanceof HTMLSelectElement ||
    (el instanceof HTMLElement && el.isContentEditable)
  );
}

export function UndoProvider({ children }: { children: ReactNode }) {
  const stack = useRef<Entry[]>([]);
  const nextId = useRef(1);
  const [toast, setToast] = useState<Toast | null>(null);
  const timer = useRef<number | undefined>(undefined);

  const show = useCallback((t: Toast) => {
    window.clearTimeout(timer.current);
    setToast(t);
    timer.current = window.setTimeout(() => setToast((cur) => (cur?.id === t.id ? null : cur)), TOAST_MS);
  }, []);

  const notify = useCallback((message: string) => show({ id: nextId.current++, message }), [show]);

  const push = useCallback(
    (label: string, undo: () => Promise<unknown>) => {
      const entry = { id: nextId.current++, label, undo };
      stack.current = [...stack.current, entry].slice(-MAX);
      show({ id: entry.id, message: label, entry });
    },
    [show],
  );

  const run = useCallback(
    async (entry: Entry) => {
      stack.current = stack.current.filter((e) => e.id !== entry.id);
      setToast(null);
      try {
        await entry.undo();
      } catch {
        notify("Couldn't undo that. Check your connection and try again.");
      }
    },
    [notify],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key.toLowerCase() !== "z" || !(e.metaKey || e.ctrlKey) || e.shiftKey || e.altKey) return;
      if (isTyping(document.activeElement)) return; // let text fields undo their own typing
      const entry = stack.current.at(-1);
      if (!entry) return;
      e.preventDefault();
      void run(entry);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [run]);

  return (
    <UndoContext.Provider value={{ push, notify }}>
      {children}
      <div className="toast-region" role="status" aria-live="polite" aria-label="Notifications">
        {toast && (
          <div className="toast">
            <span>{toast.message}</span>
            {toast.entry && (
              <button type="button" className="toast-undo" onClick={() => void run(toast.entry!)}>
                Undo
              </button>
            )}
          </div>
        )}
      </div>
    </UndoContext.Provider>
  );
}
