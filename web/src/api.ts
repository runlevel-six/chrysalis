// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Typed wrappers around the /api/v1 endpoints.

import { CLIENT_ID } from "./live";
import type { Prefs, Theme } from "./appearance";

export type User = {
  id: string;
  email: string;
  displayName: string;
  lastBoardId?: string;
  version?: string;
  prefs?: Prefs;
  /** The deployment's product name, when its branding sets one. */
  appName?: string;
};

export type Board = {
  id: string;
  name: string;
  icon: string;
  background: { type: "color" | "gradient" | "image"; value: string };
  settings: BoardSettings;
  position: string;
  role: "owner" | "editor" | "viewer";
  version: number;
  createdAt: string;
  updatedAt: string;
};

export type BoardSettings = {
  defaultCardState: "collapsed" | "expanded";
  showLabels: boolean;
  showDueDate: boolean;
  showChecklist: boolean;
  hideColumnCount: boolean;
};

export const LABEL_COLORS = ["green", "lime", "yellow", "orange", "red", "pink", "purple", "blue", "sky", "gray"] as const;
export type LabelColor = (typeof LABEL_COLORS)[number];
export type Label = { id: string; boardId: string; name: string; color: LabelColor; position: string; version: number };

/** Starting columns for a new board (§3.12); offered, never forced. */
export type BoardTemplate = "blank" | "simple" | "metamorphosis";

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
  labelIds: string[];
  cover: Cover;
  startAt: string | null; // "YYYY-MM-DD"
  dueAt: string | null; // "YYYY-MM-DD", or a UTC timestamp when dueHasTime
  dueHasTime: boolean;
  checklistDone: number;
  checklistTotal: number;
  hasDescription: boolean;
  completedAt: string | null;
  archivedAt: string | null;
  deletedAt?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
};

export type BoardView = {
  defaultCardState: "collapsed" | "expanded";
  cards: Record<string, boolean>; // card id -> expanded
  columns: Record<string, boolean>; // column id -> collapsed
  focus: Focus;
  hideCompleted: boolean;
};

/** Focus mode (§3.4): one column, or only the cards matching a filter. */
export type Focus =
  | { type: "none" }
  | { type: "column"; columnId: string }
  | { type: "filter"; query?: string; labelIds?: string[] };

export type ViewChange = {
  cards?: Record<string, boolean>;
  columns?: Record<string, boolean>;
  focus?: Focus;
  hideCompleted?: boolean;
  opened?: boolean;
};

export type ChecklistItem = { id: string; checklistId: string; text: string; doneAt: string | null; position: string; version: number };
export type Checklist = { id: string; cardId: string; title: string; position: string; items: ChecklistItem[]; version: number };

/** ProseMirror JSON, or null when empty. */
export type Description = { doc: unknown; version: number; updatedAt?: string };

export type BoardDetail = { board: Board; columns: Column[]; cards: Card[]; labels: Label[]; view: BoardView };
export type Trash = { columns: Column[]; cards: Card[] };

export type ColumnPatch = Partial<Pick<Column, "name" | "color" | "colorMode" | "isDone" | "position">> & {
  softLimit?: number; // 0 clears
};
/** A card cover: a palette color, or one of the card's image attachments. */
export type Cover = { type: "none" } | { type: "color"; value: LabelColor } | { type: "image"; value: string };

export type Attachment = {
  id: string;
  cardId?: string;
  kind: "file" | "image" | "link";
  filename: string;
  mime?: string;
  sizeBytes?: number;
  url?: string;
  width?: number;
  height?: number;
  hasThumb: boolean;
  createdAt: string;
};

/** Where an attachment (or its thumbnail) is served. Same origin, so the session cookie applies. */
export const attachmentUrl = (id: string, thumb = false) =>
  `/api/v1/attachments/${encodeURIComponent(id)}${thumb ? "?thumb=1" : ""}`;

export type CardPatch = {
  title?: string;
  columnId?: string;
  position?: string;
  completed?: boolean;
  archived?: boolean;
  labelIds?: string[];
  startAt?: string | null; // null clears
  dueAt?: string | null; // null clears
  dueHasTime?: boolean;
  cover?: Cover | { type: "color" | "image"; value: string };
};

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    /** The parsed JSON error body, e.g. the current description on a 409. */
    readonly body?: unknown,
  ) {
    super(message);
  }
}

// Marks requests with this tab's id, so live-sync notices it causes are
// skipped here (it already shows the change).
const CLIENT_HEADER = { "X-Chrysalis-Client": CLIENT_ID };

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    credentials: "same-origin",
    headers: body === undefined ? CLIENT_HEADER : { ...CLIENT_HEADER, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    let message = res.statusText;
    let body: unknown;
    try {
      body = await res.json();
      message = (body as { error?: string }).error ?? message;
    } catch {
      // not JSON; keep the status text
    }
    throw new ApiError(res.status, message, body);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  branding: () => request<{ displayName: string; themes: Theme[]; hasIcon: boolean }>("GET", "/branding"),
  updatePrefs: (patch: Partial<Prefs>) => request<Prefs>("PATCH", "/me/prefs", patch),
  me: () => request<User>("GET", "/me"),
  listBoards: () => request<{ boards: Board[] }>("GET", "/boards").then((r) => r.boards),
  getBoard: (id: string) => request<Board>("GET", `/boards/${encodeURIComponent(id)}`),
  createBoard: (name: string, template: BoardTemplate = "blank") => request<Board>("POST", "/boards", { name, template }),

  boardDetail: (id: string) => request<BoardDetail>("GET", `/boards/${encodeURIComponent(id)}/detail`),
  boardTrash: (id: string) => request<Trash>("GET", `/boards/${encodeURIComponent(id)}/trash`),
  setView: (boardId: string, view: ViewChange) =>
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

  uploadBackground: async (boardId: string, file: File): Promise<Attachment> => {
    const form = new FormData();
    form.append("file", file, file.name);
    const res = await fetch(`/api/v1/boards/${encodeURIComponent(boardId)}/background`, {
      method: "POST",
      credentials: "same-origin",
      headers: CLIENT_HEADER,
      body: form,
    });
    const body = await res.json().catch(() => undefined);
    if (!res.ok) throw new ApiError(res.status, (body as { error?: string })?.error ?? res.statusText, body);
    return body as Attachment;
  },
  updateBoard: (id: string, patch: { name?: string; settings?: BoardSettings; background?: Board["background"] }) =>
    request<Board>("PATCH", `/boards/${encodeURIComponent(id)}`, patch),
  archivedCards: (boardId: string) =>
    request<{ cards: Card[] }>("GET", `/boards/${encodeURIComponent(boardId)}/archived`).then((r) => r.cards),
  createLabel: (boardId: string, name: string, color: LabelColor) =>
    request<Label>("POST", `/boards/${encodeURIComponent(boardId)}/labels`, { name, color }),
  updateLabel: (id: string, patch: { name?: string; color?: LabelColor }) =>
    request<Label>("PATCH", `/labels/${encodeURIComponent(id)}`, patch),
  trashLabel: (id: string) => request<Label>("DELETE", `/labels/${encodeURIComponent(id)}`),
  restoreLabel: (id: string) => request<Label>("POST", `/labels/${encodeURIComponent(id)}/restore`),

  description: (cardId: string) => request<Description>("GET", `/cards/${encodeURIComponent(cardId)}/description`),
  setDescription: (cardId: string, doc: unknown, baseVersion: number) =>
    request<Description>("PUT", `/cards/${encodeURIComponent(cardId)}/description`, { doc, baseVersion }),

  attachments: (cardId: string) =>
    request<{ attachments: Attachment[] }>("GET", `/cards/${encodeURIComponent(cardId)}/attachments`).then((r) => r.attachments),
  uploadAttachment: async (cardId: string, file: File): Promise<Attachment> => {
    const form = new FormData();
    form.append("file", file, file.name);
    const res = await fetch(`/api/v1/cards/${encodeURIComponent(cardId)}/attachments`, {
      method: "POST",
      credentials: "same-origin",
      headers: CLIENT_HEADER,
      body: form,
    });
    const body = await res.json().catch(() => undefined);
    if (!res.ok) throw new ApiError(res.status, (body as { error?: string })?.error ?? res.statusText, body);
    return body as Attachment;
  },
  addLink: (cardId: string, url: string, title: string) =>
    request<Attachment>("POST", `/cards/${encodeURIComponent(cardId)}/links`, { url, title }),
  trashAttachment: (id: string) => request<Attachment>("DELETE", `/attachments/${encodeURIComponent(id)}`),
  restoreAttachment: (id: string) => request<Attachment>("POST", `/attachments/${encodeURIComponent(id)}/restore`),

  checklists: (cardId: string) =>
    request<{ checklists: Checklist[] }>("GET", `/cards/${encodeURIComponent(cardId)}/checklists`).then((r) => r.checklists),
  createChecklist: (cardId: string, title: string) =>
    request<Checklist>("POST", `/cards/${encodeURIComponent(cardId)}/checklists`, { title }),
  updateChecklist: (id: string, patch: { title?: string; position?: string }) =>
    request<Checklist>("PATCH", `/checklists/${encodeURIComponent(id)}`, patch),
  trashChecklist: (id: string) => request<Checklist>("DELETE", `/checklists/${encodeURIComponent(id)}`),
  restoreChecklist: (id: string) => request<Checklist>("POST", `/checklists/${encodeURIComponent(id)}/restore`),
  createItem: (checklistId: string, text: string, position?: string) =>
    request<ChecklistItem>("POST", `/checklists/${encodeURIComponent(checklistId)}/items`, { text, position }),
  updateItem: (id: string, patch: { text?: string; done?: boolean; position?: string; checklistId?: string }) =>
    request<ChecklistItem>("PATCH", `/checklist-items/${encodeURIComponent(id)}`, patch),
  trashItem: (id: string) => request<ChecklistItem>("DELETE", `/checklist-items/${encodeURIComponent(id)}`),
  restoreItem: (id: string) => request<ChecklistItem>("POST", `/checklist-items/${encodeURIComponent(id)}/restore`),
};
