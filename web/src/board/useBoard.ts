// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Board data: one query for everything on the board, plus actions that
// update the cache immediately (optimistic), call the API, and record an
// undo. If the server refuses, the cache is refetched and a calm message
// shown, so the screen never disagrees with the server for long.

import { CLIENT_ID, holdSync } from "../live";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { generateKeyBetween } from "fractional-indexing";

import {
  api,
  ApiError,
  type Board,
  type BoardDetail,
  type BoardSettings,
  type Card,
  type CardPatch,
  type Column,
  type ColumnPatch,
  type Focus,
  type Label,
  type LabelColor,
} from "../api";
import { useUndo } from "../undo";

export const detailKey = (boardId: string) => ["board", boardId, "detail"] as const;

// Positions compare as plain strings (code units), exactly like the server's
// SQLite BINARY collation. Never use localeCompare for these.
export const byPosition = <T extends { position: string; id: string }>(a: T, b: T) =>
  a.position < b.position ? -1 : a.position > b.position ? 1 : a.id < b.id ? -1 : a.id > b.id ? 1 : 0;

/** A key between two neighbors' positions; falls back to the end if the neighbors collide. */
export function keyBetween(before: string | null, after: string | null): string {
  try {
    return generateKeyBetween(before, after);
  } catch {
    return generateKeyBetween(before, null);
  }
}

export function useBoardDetail(boardId: string) {
  return useQuery({ queryKey: detailKey(boardId), queryFn: () => api.boardDetail(boardId) });
}

const SAVE_FAILED = "That didn't save. Check your connection; your board has been refreshed.";

export function useBoardActions(boardId: string) {
  const qc = useQueryClient();
  const { push, notify } = useUndo();
  const key = detailKey(boardId);

  const get = useCallback(() => qc.getQueryData<BoardDetail>(key), [qc, key]);
  const patch = useCallback(
    (fn: (d: BoardDetail) => BoardDetail) => qc.setQueryData<BoardDetail>(key, (d) => (d ? fn(d) : d)),
    [qc, key],
  );
  const putCard = (c: Card) => patch((d) => ({ ...d, cards: [...d.cards.filter((x) => x.id !== c.id), c] }));
  const dropCard = (id: string) => patch((d) => ({ ...d, cards: d.cards.filter((x) => x.id !== id) }));
  const putColumn = (c: Column) => patch((d) => ({ ...d, columns: [...d.columns.filter((x) => x.id !== c.id), c] }));

  const fail = (err: unknown) => {
    void qc.invalidateQueries({ queryKey: key });
    notify(err instanceof ApiError && err.status === 400 ? err.message : SAVE_FAILED);
  };

  /** Run an optimistic change: apply locally, send, then replace with the server's copy. */
  async function send<T>(local: () => void, remote: () => Promise<T>, settle: (v: T) => void): Promise<T | undefined> {
    local();
    try {
      const v = await remote();
      settle(v);
      return v;
    } catch (err) {
      fail(err);
      return undefined;
    }
  }

  const lastPosition = (items: { position: string; id: string }[]) =>
    items.length ? [...items].sort(byPosition).at(-1)!.position : null;

  // Cards

  const addCard = async (columnId: string, title: string) => {
    const d = get();
    if (!d) return;
    const position = keyBetween(lastPosition(d.cards.filter((c) => c.columnId === columnId)), null);
    const temp: Card = {
      id: `tmp-${crypto.randomUUID()}`, boardId, columnId, title: title.trim(), position, labelIds: [], cover: { type: "none" },
      startAt: null, dueAt: null, dueHasTime: false, checklistDone: 0, checklistTotal: 0, hasDescription: false, completedAt: null, archivedAt: null,
      version: 0, createdAt: "", updatedAt: "",
    };
    const created = await send(
      () => putCard(temp),
      () => api.createCard(columnId, title, position),
      (c) => {
        dropCard(temp.id);
        putCard(c);
      },
    );
    if (created) push(`Added “${created.title}”`, () => trashCardQuiet(created));
  };

  const updateCard = async (card: Card, p: CardPatch, label?: string) => {
    const before: CardPatch = {};
    if (p.title !== undefined) before.title = card.title;
    if (p.columnId !== undefined) before.columnId = card.columnId;
    if (p.position !== undefined || p.columnId !== undefined) before.position = card.position;
    if (p.completed !== undefined) before.completed = card.completedAt !== null;
    if (p.archived !== undefined) before.archived = card.archivedAt !== null;
    if (p.labelIds !== undefined) before.labelIds = card.labelIds;
    if (p.startAt !== undefined) before.startAt = card.startAt;
    if (p.dueAt !== undefined || p.dueHasTime !== undefined) {
      before.dueAt = card.dueAt;
      before.dueHasTime = card.dueHasTime;
    }
    if (p.cover !== undefined) before.cover = card.cover;

    const now = new Date().toISOString();
    const local: Card = {
      ...card,
      ...(p.title !== undefined && { title: p.title.trim() }),
      ...(p.columnId !== undefined && { columnId: p.columnId }),
      ...(p.position !== undefined && { position: p.position }),
      ...(p.completed !== undefined && { completedAt: p.completed ? now : null }),
      ...(p.archived !== undefined && { archivedAt: p.archived ? now : null }),
      ...(p.labelIds !== undefined && { labelIds: p.labelIds }),
      ...(p.startAt !== undefined && { startAt: p.startAt }),
      ...(p.dueAt !== undefined && { dueAt: p.dueAt }),
      ...(p.dueHasTime !== undefined && { dueHasTime: p.dueHasTime }),
      ...(p.cover !== undefined && { cover: p.cover as Card["cover"] }),
    };
    const movedAway =
      (p.columnId !== undefined && !get()?.columns.some((c) => c.id === p.columnId)) || p.archived === true;
    const updated = await send(
      () => (movedAway ? dropCard(card.id) : putCard(local)),
      () => api.updateCard(card.id, p),
      (c) => (c.boardId === boardId && !c.archivedAt ? putCard(c) : dropCard(c.id)),
    );
    if (updated && label) {
      push(label, async () => {
        const back = await api.updateCard(card.id, before);
        if (back.boardId === boardId && !back.archivedAt) putCard(back);
        else void qc.invalidateQueries({ queryKey: key });
        if (updated.boardId !== boardId) void qc.invalidateQueries({ queryKey: detailKey(updated.boardId) });
      });
    }
    if (updated && updated.boardId !== boardId) void qc.invalidateQueries({ queryKey: detailKey(updated.boardId) });
    return updated;
  };

  const moveCard = (card: Card, columnId: string, position: string) => {
    if (card.columnId === columnId && card.position === position) return;
    const d = get();
    const from = d?.columns.find((c) => c.id === card.columnId)?.name;
    const to = d?.columns.find((c) => c.id === columnId)?.name;
    const label = from && to && from !== to ? `Moved “${card.title}” to ${to}` : `Moved “${card.title}”`;
    return updateCard(card, { columnId, position }, label);
  };

  const trashCardQuiet = async (card: Card) => {
    await api.trashCard(card.id);
    dropCard(card.id);
  };

  const trashCard = async (card: Card) => {
    const done = await send(
      () => dropCard(card.id),
      () => api.trashCard(card.id),
      () => {},
    );
    if (done) {
      push(`Moved “${card.title}” to the trash`, async () => putCard(await api.restoreCard(card.id)));
    }
  };

  const restoreCard = async (card: Card) => {
    await send(
      () => {},
      () => api.restoreCard(card.id),
      (c) => putCard(c),
    );
  };

  // Columns

  const addColumn = async (name: string) => {
    const d = get();
    if (!d) return;
    const position = keyBetween(lastPosition(d.columns), null);
    const temp: Column = {
      id: `tmp-${crypto.randomUUID()}`, boardId, name: name.trim(), color: "", colorMode: "header",
      softLimit: null, isDone: false, position, version: 0, updatedAt: "",
    };
    const created = await send(
      () => putColumn(temp),
      () => api.createColumn(boardId, name, position),
      (c) => {
        patch((d) => ({ ...d, columns: d.columns.filter((x) => x.id !== temp.id) }));
        putColumn(c);
      },
    );
    if (created) push(`Added column “${created.name}”`, async () => {
      await api.trashColumn(created.id);
      patch((d) => ({ ...d, columns: d.columns.filter((x) => x.id !== created.id) }));
    });
  };

  const updateColumn = async (col: Column, p: ColumnPatch, label?: string) => {
    const before: ColumnPatch = {};
    if (p.name !== undefined) before.name = col.name;
    if (p.position !== undefined) before.position = col.position;
    if (p.isDone !== undefined) before.isDone = col.isDone;
    const updated = await send(
      () => putColumn({ ...col, ...p, softLimit: p.softLimit === undefined ? col.softLimit : p.softLimit || null }),
      () => api.updateColumn(col.id, p),
      (c) => putColumn(c),
    );
    if (updated && label) push(label, async () => putColumn(await api.updateColumn(col.id, before)));
  };

  const trashColumn = async (col: Column) => {
    const done = await send(
      () => patch((d) => ({ ...d, columns: d.columns.filter((x) => x.id !== col.id) })),
      () => api.trashColumn(col.id),
      () => {},
    );
    if (done) {
      push(`Moved column “${col.name}” to the trash`, async () => {
        await api.restoreColumn(col.id);
        await qc.invalidateQueries({ queryKey: key }); // its cards come back too
      });
    }
  };

  const restoreColumn = async (col: Column) => {
    await send(
      () => {},
      () => api.restoreColumn(col.id),
      () => void qc.invalidateQueries({ queryKey: key }),
    );
  };

  // Labels

  const putLabel = (l: Label) => patch((d) => ({ ...d, labels: [...d.labels.filter((x) => x.id !== l.id), l] }));

  const createLabel = async (name: string, color: LabelColor) => {
    const created = await send(() => {}, () => api.createLabel(boardId, name, color), putLabel);
    return created;
  };

  const updateLabel = async (label: Label, p: { name?: string; color?: LabelColor }) => {
    await send(() => putLabel({ ...label, ...p }), () => api.updateLabel(label.id, p), putLabel);
  };

  const trashLabel = async (label: Label) => {
    const done = await send(
      () => patch((d) => ({ ...d, labels: d.labels.filter((x) => x.id !== label.id) })),
      () => api.trashLabel(label.id),
      () => void qc.invalidateQueries({ queryKey: key }), // cards drop the label
    );
    if (done) {
      push(`Deleted label “${label.name || label.color}”`, async () => {
        await api.restoreLabel(label.id);
        await qc.invalidateQueries({ queryKey: key });
      });
    }
  };

  // Board

  const updateBoard = async (p: { name?: string; icon?: string; settings?: BoardSettings; background?: Board["background"] }, label?: string) => {
    const d = get();
    if (!d) return;
    const before = { name: d.board.name, icon: d.board.icon, settings: d.board.settings, background: d.board.background };
    const updated = await send(
      () =>
        patch((x) => ({
          ...x,
          board: { ...x.board, ...p },
          view: p.settings ? { ...x.view, defaultCardState: p.settings.defaultCardState } : x.view,
        })),
      () => api.updateBoard(boardId, p),
      (b) => {
        patch((x) => ({ ...x, board: b }));
        void qc.invalidateQueries({ queryKey: ["boards"] });
      },
    );
    if (updated && label) {
      push(label, async () => {
        const b = await api.updateBoard(boardId, before);
        patch((x) => ({ ...x, board: b }));
      });
    }
  };

  return {
    addCard, updateCard, moveCard, trashCard, restoreCard,
    addColumn, updateColumn, trashColumn, restoreColumn,
    createLabel, updateLabel, trashLabel, updateBoard,
  };
}

/**
 * Per-user view state (docs/DESIGN.md §3.3): which cards are expanded and
 * which columns are collapsed. Changes show immediately and are saved in a
 * debounced (~500 ms) batch; last write wins across devices.
 */
export function useViewState(boardId: string, detail: BoardDetail | undefined) {
  const qc = useQueryClient();
  // Card and column toggles batch up; focus and hide-completed ride along in
  // the same request (the latest value wins).
  const pending = useRef<{
    cards: Record<string, boolean>;
    columns: Record<string, boolean>;
    focus?: Focus;
    hideCompleted?: boolean;
    centeringLine?: boolean;
  }>({
    cards: {},
    columns: {},
  });
  const timer = useRef<number | undefined>(undefined);
  // While a change waits to be saved, another device's view notice must not
  // refetch the server's older copy over it.
  const release = useRef<(() => void) | undefined>(undefined);

  const flush = useCallback(
    (keepalive = false) => {
      window.clearTimeout(timer.current);
      const done = release.current ?? (() => {});
      release.current = undefined;
      const batch = pending.current;
      if (!Object.keys(batch.cards).length && !Object.keys(batch.columns).length && !batch.focus && batch.hideCompleted === undefined && batch.centeringLine === undefined) {
        done();
        return;
      }
      pending.current = { cards: {}, columns: {} };
      if (keepalive) {
        // Page is going away: fetch with keepalive survives navigation.
        void fetch(`/api/v1/boards/${encodeURIComponent(boardId)}/view`, {
          method: "PUT", keepalive: true, credentials: "same-origin",
          headers: { "Content-Type": "application/json", "X-Chrysalis-Client": CLIENT_ID }, body: JSON.stringify(batch),
        });
        done();
        return;
      }
      api.setView(boardId, batch).finally(done).catch(() => {
        // Not content: merge back and try again on the next change.
        pending.current = {
          cards: { ...batch.cards, ...pending.current.cards },
          columns: { ...batch.columns, ...pending.current.columns },
          focus: pending.current.focus ?? batch.focus,
          hideCompleted: pending.current.hideCompleted ?? batch.hideCompleted,
          centeringLine: pending.current.centeringLine ?? batch.centeringLine,
        };
      });
    },
    [boardId],
  );

  const schedule = useCallback(
    (ms: number) => {
      release.current ??= holdSync();
      window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => flush(), ms);
    },
    [flush],
  );

  useEffect(() => {
    const onHide = () => flush(true);
    window.addEventListener("pagehide", onHide);
    return () => {
      window.removeEventListener("pagehide", onHide);
      flush();
    };
  }, [flush]);

  const set = useCallback(
    (kind: "cards" | "columns", values: Record<string, boolean>) => {
      qc.setQueryData<BoardDetail>(detailKey(boardId), (d) =>
        d ? { ...d, view: { ...d.view, [kind]: { ...d.view[kind], ...values } } } : d,
      );
      pending.current[kind] = { ...pending.current[kind], ...values };
      schedule(500);
    },
    [qc, boardId, schedule],
  );

  const setBoardLevel = useCallback(
    (change: { focus?: Focus; hideCompleted?: boolean; centeringLine?: boolean }) => {
      qc.setQueryData<BoardDetail>(detailKey(boardId), (d) => (d ? { ...d, view: { ...d.view, ...change } } : d));
      pending.current = { ...pending.current, ...change };
      window.clearTimeout(timer.current);
      // Sooner than card toggles: these change the whole board.
      timer.current = window.setTimeout(() => flush(), 150);
    },
    [qc, boardId, schedule],
  );

  return useMemo(() => {
    const view = detail?.view;
    const isExpanded = (cardId: string) => view?.cards[cardId] ?? view?.defaultCardState === "expanded";
    const isCollapsed = (columnId: string) => view?.columns[columnId] ?? false;
    return {
      isExpanded,
      isCollapsed,
      setExpanded: (ids: string[], expanded: boolean) =>
        set("cards", Object.fromEntries(ids.filter((id) => !id.startsWith("tmp-")).map((id) => [id, expanded]))),
      setCollapsed: (id: string, collapsed: boolean) => set("columns", { [id]: collapsed }),
      focus: view?.focus ?? ({ type: "none" } as Focus),
      hideCompleted: view?.hideCompleted ?? false,
      setFocus: (focus: Focus) => setBoardLevel({ focus }),
      setHideCompleted: (hideCompleted: boolean) => setBoardLevel({ hideCompleted }),
      /** The per-board override for the centering line; null follows the person's setting. */
      centeringLine: view?.centeringLine ?? null,
      setCenteringLine: (centeringLine: boolean) => setBoardLevel({ centeringLine }),
    };
  }, [detail?.view, set, setBoardLevel]);
}
