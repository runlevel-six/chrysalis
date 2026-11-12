// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Typed wrappers around the /api/v1 endpoints.

export type User = { id: string; email: string; displayName: string };

export type Board = {
  id: string;
  name: string;
  icon: string;
  background: { type: "color" | "gradient" | "image"; value: string };
  position: string;
  role: "owner" | "editor" | "viewer";
  version: number;
  createdAt: string;
  updatedAt: string;
};

export type Column = {
  id: string;
  boardId: string;
  name: string;
  color: string;
  colorMode: "header" | "full";
  softLimit: number | null;
  isDone: boolean;
  position: string;
  deletedAt?: string;
  version: number;
  updatedAt: string;
};

export type Card = {
  id: string;
  boardId: string;
  columnId: string;
  title: string;
  position: string;
  completedAt: string | null;
  deletedAt?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
};

export type BoardView = {
  defaultCardState: "collapsed" | "expanded";
  cards: Record<string, boolean>; // card id -> expanded
  columns: Record<string, boolean>; // column id -> collapsed
};

export type BoardDetail = { board: Board; columns: Column[]; cards: Card[]; view: BoardView };
export type Trash = { columns: Column[]; cards: Card[] };

export type ColumnPatch = Partial<Pick<Column, "name" | "color" | "colorMode" | "isDone" | "position">> & {
  softLimit?: number; // 0 clears
};
export type CardPatch = { title?: string; columnId?: string; position?: string; completed?: boolean };

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    credentials: "same-origin",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    let message = res.statusText;
    try {
      message = ((await res.json()) as { error?: string }).error ?? message;
    } catch {
      // not JSON; keep the status text
    }
    throw new ApiError(res.status, message);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  me: () => request<User>("GET", "/me"),
  listBoards: () => request<{ boards: Board[] }>("GET", "/boards").then((r) => r.boards),
  getBoard: (id: string) => request<Board>("GET", `/boards/${encodeURIComponent(id)}`),
  createBoard: (name: string) => request<Board>("POST", "/boards", { name }),

  boardDetail: (id: string) => request<BoardDetail>("GET", `/boards/${encodeURIComponent(id)}/detail`),
  boardTrash: (id: string) => request<Trash>("GET", `/boards/${encodeURIComponent(id)}/trash`),
  setView: (boardId: string, view: { cards?: Record<string, boolean>; columns?: Record<string, boolean> }) =>
    request<void>("PUT", `/boards/${encodeURIComponent(boardId)}/view`, view),

  createColumn: (boardId: string, name: string, position?: string) =>
    request<Column>("POST", `/boards/${encodeURIComponent(boardId)}/columns`, { name, position }),
  updateColumn: (id: string, patch: ColumnPatch) => request<Column>("PATCH", `/columns/${encodeURIComponent(id)}`, patch),
  trashColumn: (id: string) => request<Column>("DELETE", `/columns/${encodeURIComponent(id)}`),
  restoreColumn: (id: string) => request<Column>("POST", `/columns/${encodeURIComponent(id)}/restore`),

  createCard: (columnId: string, title: string, position?: string) =>
    request<Card>("POST", `/columns/${encodeURIComponent(columnId)}/cards`, { title, position }),
  updateCard: (id: string, patch: CardPatch) => request<Card>("PATCH", `/cards/${encodeURIComponent(id)}`, patch),
  trashCard: (id: string) => request<Card>("DELETE", `/cards/${encodeURIComponent(id)}`),
  restoreCard: (id: string) => request<Card>("POST", `/cards/${encodeURIComponent(id)}/restore`),
};
