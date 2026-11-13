// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Board management on the board list (docs/DESIGN.md §4, Phase 1): reorder,
// archive and delete to trash, with archived boards and the trash tucked
// away below until wanted.

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { generateKeyBetween } from "fractional-indexing";

import { api, ApiError, type Board } from "./api";
import { Link } from "./router";
import { Dialog, Icon, Menu } from "./ui";
import { useUndo } from "./undo";

export const archivedKey = ["boards", "archived"] as const;
export const trashedKey = ["boards", "trash"] as const;

function useRefresh() {
  const qc = useQueryClient();
  // The board list and both drawers share the ["boards"] prefix.
  return () => qc.invalidateQueries({ queryKey: ["boards"] });
}

/** One board on the list, with its menu. */
export function BoardTile({
  board,
  boards,
  index,
}: {
  board: Board;
  boards: Board[];
  index: number;
}) {
  const refresh = useRefresh();
  const { push, notify } = useUndo();
  const [confirming, setConfirming] = useState(false);
  const owner = board.role === "owner";

  const fail = (err: unknown) =>
    notify(
      err instanceof ApiError
        ? err.message
        : "That didn't save. Check your connection and try again.",
    );

  const move = async (to: number) => {
    // A key between the new neighbours; only this board's row changes.
    const rest = boards.filter((b) => b.id !== board.id);
    const before = rest[to - 1]?.position ?? null;
    const after = rest[to]?.position ?? null;
    const old = board.position;
    try {
      await api.updateBoard(board.id, {
        position: generateKeyBetween(before, after),
      });
      await refresh();
      push(`Moved “${board.name}”`, async () => {
        await api.updateBoard(board.id, { position: old });
        await refresh();
      });
    } catch (err) {
      fail(err);
    }
  };

  const archive = async () => {
    try {
      await api.updateBoard(board.id, { archived: true });
      await refresh();
      push(`Archived “${board.name}”`, async () => {
        await api.updateBoard(board.id, { archived: false });
        await refresh();
      });
    } catch (err) {
      fail(err);
    }
  };

  const trash = async () => {
    setConfirming(false);
    try {
      await api.trashBoard(board.id);
      await refresh();
      push(`Moved “${board.name}” to the trash`, async () => {
        await api.restoreBoard(board.id);
        await refresh();
      });
    } catch (err) {
      fail(err);
    }
  };

  const items = [
    ...(index > 0
      ? [{ label: "Move up", onSelect: () => void move(index - 1) }]
      : []),
    ...(index < boards.length - 1
      ? [{ label: "Move down", onSelect: () => void move(index + 1) }]
      : []),
    ...(owner
      ? [
          { label: "Archive", onSelect: () => void archive() },
          { label: "Delete…", onSelect: () => setConfirming(true) },
        ]
      : []),
  ];

  return (
    <li className="board-row">
      <Link to={`/b/${board.id}`} className="board-tile">
        {board.icon && (
          <span className="board-icon" aria-hidden="true">
            {board.icon}
          </span>
        )}
        {board.name}
      </Link>
      {items.length > 0 && (
        <Menu
          label={`${board.name} options`}
          icon={<Icon.More />}
          items={items}
          iconOnly
        />
      )}
      <Dialog
        open={confirming}
        title={`Delete “${board.name}”?`}
        onClose={() => setConfirming(false)}
      >
        <p>
          The board and everything on it goes to the trash. You can restore it
          from there for 30 days, and after that it's removed for good.
        </p>
        <div className="dialog-actions">
          <button type="button" onClick={() => void trash()}>
            Delete board
          </button>
          <button
            type="button"
            className="text-button"
            onClick={() => setConfirming(false)}
          >
            Keep it
          </button>
        </div>
      </Dialog>
    </li>
  );
}

/** Archived boards and the trash, each folded away until opened. */
export function BoardDrawers() {
  const archived = useQuery({
    queryKey: archivedKey,
    queryFn: api.archivedBoards,
  });
  const trashed = useQuery({
    queryKey: trashedKey,
    queryFn: api.trashedBoards,
  });
  const refresh = useRefresh();
  const { notify } = useUndo();

  const act = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      await refresh();
    } catch (err) {
      notify(
        err instanceof ApiError
          ? err.message
          : "That didn't save. Check your connection and try again.",
      );
    }
  };

  return (
    <>
      {!!archived.data?.length && (
        <details className="board-drawer">
          <summary>Archived boards ({archived.data.length})</summary>
          <ul className="drawer-list">
            {archived.data.map((b) => (
              <li key={b.id}>
                <Link to={`/b/${b.id}`}>
                  {b.icon && <span aria-hidden="true">{b.icon} </span>}
                  {b.name}
                </Link>
                {b.role === "owner" && (
                  <button
                    type="button"
                    className="text-button"
                    onClick={() =>
                      void act(() => api.updateBoard(b.id, { archived: false }))
                    }
                  >
                    Unarchive<span className="visually-hidden"> {b.name}</span>
                  </button>
                )}
              </li>
            ))}
          </ul>
        </details>
      )}
      {!!trashed.data?.length && (
        <details className="board-drawer">
          <summary>Deleted boards ({trashed.data.length})</summary>
          <p className="muted">Deleted boards can be restored for 30 days.</p>
          <ul className="drawer-list">
            {trashed.data.map((b) => (
              <li key={b.id}>
                <span>
                  {b.icon && <span aria-hidden="true">{b.icon} </span>}
                  {b.name}
                </span>
                <button
                  type="button"
                  className="text-button"
                  onClick={() => void act(() => api.restoreBoard(b.id))}
                >
                  Restore<span className="visually-hidden"> {b.name}</span>
                </button>
              </li>
            ))}
          </ul>
        </details>
      )}
    </>
  );
}
