// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Typed wrappers around the /api/v1 endpoints.

import { CLIENT_ID } from "./live";
import { forgetAll, isNetworkError, recall, remember, setOffline } from "./offline";
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
  archived: boolean;
  deletedAt?: string;
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

/** What a Trello import created, and what it left out (§8). */
export type ImportReport = {
  imported: {
    columns: number;
    cards: number;
    descriptions: number;
    labels: number;
    checklists: number;
    checklistItems: number;
    comments: number;
    links: number;
  };
  skipped: string[];
  notes: string[];
};
export type ImportResult = { boardId: string; board: Board; report: ImportReport };

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
  /** Custom field values by field id: string, number, true, or a choice id. */
  fields: Record<string, string | number | boolean>;
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
  /** Show (true) or hide (false) the centering line here; null follows the person's setting. */
  centeringLine: boolean | null;
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
  centeringLine?: boolean;
  opened?: boolean;
};

/** The centering line (§3.13): collections of short entries, one shown at a time. */
export type QuoteCollection = {
  id: string;
  name: string;
  position: string;
  role: "owner" | "editor" | "viewer";
  entryCount: number;
  deletedAt?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
};
export type QuoteEntry = {
  id: string;
  collectionId: string;
  text: string;
  attribution: string;
  reference: string;
  translation: string;
  tags: string[];
  position: string;
  deletedAt?: string;
  version: number;
  updatedAt: string;
};
export type QuoteEntryFields = Pick<QuoteEntry, "text" | "attribution" | "reference" | "translation" | "tags">;
export type CenteringSettings = {
  enabled: boolean;
  mode: "rotate" | "pinned";
  interval: "page_load" | "hourly" | "daily" | "weekly";
  order: "sequential" | "shuffle";
  collectionIds: string[];
};
export type CenteringLine = {
  enabled: boolean;
  mode: "rotate" | "pinned";
  hidden: boolean;
  hiddenUntil?: string;
  entry: QuoteEntry | null;
  pinned: boolean;
  poolSize: number;
};
export type QuoteImportResult = { collection: QuoteCollection; created: boolean; added: number; replaced: number };
export type CenteringTrash = { collections: QuoteCollection[]; entries: QuoteEntry[] };

/** This device's IANA time zone, so "today" and "midnight" are the person's own. */
export const timeZone = () => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
};

export type ChecklistItem = { id: string; checklistId: string; text: string; doneAt: string | null; position: string; version: number };
export type Checklist = { id: string; cardId: string; title: string; position: string; items: ChecklistItem[]; version: number };

/** ProseMirror JSON, or null when empty. */
export type Description = { doc: unknown; version: number; updatedAt?: string };

export type BoardDetail = { board: Board; columns: Column[]; cards: Card[]; labels: Label[]; fields: CustomField[]; view: BoardView };

/** A custom field on a board (§4, Phase 2). */
export type FieldType = "text" | "number" | "date" | "select" | "checkbox";
export type FieldOption = { id: string; name: string; color: LabelColor | "" };
export type CustomField = {
  id: string;
  boardId: string;
  name: string;
  type: FieldType;
  options: FieldOption[];
  showOnCollapsed: boolean;
  position: string;
  version: number;
};
export type FieldPatch = { name?: string; options?: FieldOption[]; showOnCollapsed?: boolean; position?: string };
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
  let res: Response;
  try {
    res = await fetch(`/api/v1${path}`, {
      method,
      credentials: "same-origin",
      headers: body === undefined ? CLIENT_HEADER : { ...CLIENT_HEADER, "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (err) {
    // No network: reads fall back to the last copy this device saw (§5.6).
    if (method === "GET" && isNetworkError(err)) {
      const copy = await recall(path);
      if (copy !== undefined) {
        setOffline(true);
        return copy as T;
      }
    }
    // A failed write isn't "offline mode": the caller keeps the edit (a
    // draft, or an undo-able toast) and the board stays editable.
    throw err;
  }
  setOffline(false);
  if (res.status === 401) void forgetAll(); // signed out: don't keep their boards on this device
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
  const data = (await res.json()) as T;
  if (method === "GET") void remember(path, data);
  return data;
}

/** The calendar view's data (GET /calendar). */
export type Calendar = {
  boards: Board[];
  columns: Column[];
  labels: Label[];
  cards: Card[];
  undated: Card[];
  undatedMore: boolean;
};

/** One person's reminder on a card (§3.9). */
export type Reminder = {
  cardId: string;
  kind: "due" | "before" | "at";
  offsetMinutes?: number;
  at?: string;
  timeZone: string;
  remindAt: string | null;
  firedAt?: string;
};
export type ReminderSpec = { kind: Reminder["kind"]; offsetMinutes?: number; at?: string; timeZone: string };

export type Notification = {
  id: string;
  kind: "reminder";
  text: string;
  cardId?: string;
  boardId?: string;
  createdAt: string;
  readAt: string | null;
};

export const api = {
  createField: (boardId: string, name: string, type: FieldType, options: FieldOption[] = []) =>
    request<CustomField>("POST", `/boards/${encodeURIComponent(boardId)}/fields`, { name, type, options }),
  updateField: (id: string, patch: FieldPatch) => request<CustomField>("PATCH", `/fields/${encodeURIComponent(id)}`, patch),
  trashField: (id: string) => request<CustomField>("DELETE", `/fields/${encodeURIComponent(id)}`),
  restoreField: (id: string) => request<CustomField>("POST", `/fields/${encodeURIComponent(id)}/restore`),
  setCardField: (cardId: string, fieldId: string, value: string | number | boolean | null) =>
    request<Card>("PUT", `/cards/${encodeURIComponent(cardId)}/fields/${encodeURIComponent(fieldId)}`, { value }),
  pushInfo: () => request<{ enabled: boolean; publicKey?: string }>("GET", "/push"),
  pushSubscribe: (subscription: PushSubscriptionJSON, label: string) =>
    request<void>("POST", "/me/push-subscriptions", { subscription, label }),
  pushUnsubscribe: (endpoint: string) => request<void>("POST", "/me/push-subscriptions/remove", { endpoint }),
  pushStatus: (endpoint: string) =>
    request<{ subscribed: boolean; devices: number }>("POST", "/me/push-subscriptions/status", { endpoint }),
  pushTest: () => request<{ devices: number }>("POST", "/me/push-test"),
  reminder: (cardId: string) =>
    request<{ reminder: Reminder | null }>("GET", `/cards/${encodeURIComponent(cardId)}/reminder`).then((r) => r.reminder),
  setReminder: (cardId: string, spec: ReminderSpec) =>
    request<{ reminder: Reminder }>("PUT", `/cards/${encodeURIComponent(cardId)}/reminder`, spec).then((r) => r.reminder),
  clearReminder: (cardId: string) => request<void>("DELETE", `/cards/${encodeURIComponent(cardId)}/reminder`),
  notifications: () => request<{ notifications: Notification[]; unread: number }>("GET", "/me/notifications"),
  readNotifications: () => request<void>("POST", "/me/notifications/read"),
  dismissAllNotifications: () => request<void>("POST", "/me/notifications/dismiss"),
  dismissNotification: (id: string) => request<void>("POST", `/notifications/${encodeURIComponent(id)}/dismiss`),
  snoozeNotification: (id: string, until: Date) =>
    request<void>("POST", `/notifications/${encodeURIComponent(id)}/snooze`, { until: until.toISOString() }),
  calendar: (q: { board?: string; from: string; to: string; undated?: boolean }) => {
    const p = new URLSearchParams({ from: q.from, to: q.to });
    if (q.board) p.set("board", q.board);
    if (q.undated) p.set("undated", "1");
    return request<Calendar>("GET", `/calendar?${p}`);
  },
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

  importTrello: async (file: File): Promise<ImportResult> => {
    const form = new FormData();
    form.append("file", file, file.name);
    const res = await fetch("/api/v1/import/trello", { method: "POST", credentials: "same-origin", body: form });
    const body = await res.json().catch(() => undefined);
    if (!res.ok) throw new ApiError(res.status, (body as { error?: string })?.error ?? res.statusText, body);
    return body as ImportResult;
  },
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
  updateBoard: (
    id: string,
    patch: { name?: string; icon?: string; position?: string; archived?: boolean; settings?: BoardSettings; background?: Board["background"] },
  ) => request<Board>("PATCH", `/boards/${encodeURIComponent(id)}`, patch),
  trashBoard: (id: string) => request<Board>("DELETE", `/boards/${encodeURIComponent(id)}`),
  restoreBoard: (id: string) => request<Board>("POST", `/boards/${encodeURIComponent(id)}/restore`),
  archivedBoards: () => request<{ boards: Board[] }>("GET", "/me/boards/archived").then((r) => r.boards),
  trashedBoards: () => request<{ boards: Board[] }>("GET", "/me/boards/trash").then((r) => r.boards),
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

  centeringSettings: () => request<CenteringSettings>("GET", "/me/centering"),
  updateCenteringSettings: (patch: Partial<CenteringSettings>) => request<CenteringSettings>("PATCH", "/me/centering", patch),
  // The tab's id (sent as a header) tells the server which page load this
  // is, for the "every page load" interval.
  centeringLine: () => request<CenteringLine>("GET", `/me/centering/line?tz=${encodeURIComponent(timeZone())}`),
  centeringNext: () => request<CenteringLine>("POST", "/me/centering/line/next"),
  centeringPrevious: () => request<CenteringLine>("POST", "/me/centering/line/previous"),
  centeringPin: (entryId?: string) => request<CenteringLine>("POST", "/me/centering/line/pin", { entryId }),
  centeringUnpin: () => request<CenteringLine>("POST", "/me/centering/line/unpin"),
  centeringHide: () => request<CenteringLine>("POST", "/me/centering/line/hide", { tz: timeZone() }),
  centeringShow: () => request<CenteringLine>("POST", "/me/centering/line/show"),
  quoteCollections: () =>
    request<{ collections: QuoteCollection[] }>("GET", "/centering/collections").then((r) => r.collections),
  createQuoteCollection: (name: string) => request<QuoteCollection>("POST", "/centering/collections", { name }),
  updateQuoteCollection: (id: string, patch: { name?: string; position?: string }) =>
    request<QuoteCollection>("PATCH", `/centering/collections/${encodeURIComponent(id)}`, patch),
  trashQuoteCollection: (id: string) => request<QuoteCollection>("DELETE", `/centering/collections/${encodeURIComponent(id)}`),
  restoreQuoteCollection: (id: string) =>
    request<QuoteCollection>("POST", `/centering/collections/${encodeURIComponent(id)}/restore`),
  quoteEntries: (collectionId: string) =>
    request<{ entries: QuoteEntry[] }>("GET", `/centering/collections/${encodeURIComponent(collectionId)}/entries`).then(
      (r) => r.entries,
    ),
  createQuoteEntry: (collectionId: string, fields: Partial<QuoteEntryFields> & { position?: string }) =>
    request<QuoteEntry>("POST", `/centering/collections/${encodeURIComponent(collectionId)}/entries`, fields),
  updateQuoteEntry: (id: string, patch: Partial<QuoteEntryFields> & { position?: string }) =>
    request<QuoteEntry>("PATCH", `/centering/entries/${encodeURIComponent(id)}`, patch),
  trashQuoteEntry: (id: string) => request<QuoteEntry>("DELETE", `/centering/entries/${encodeURIComponent(id)}`),
  restoreQuoteEntry: (id: string) => request<QuoteEntry>("POST", `/centering/entries/${encodeURIComponent(id)}/restore`),
  centeringTrash: () => request<CenteringTrash>("GET", "/centering/trash"),
  /** Where a collection downloads, as YAML or JSON. */
  exportQuotesUrl: (collectionId: string, format: "yaml" | "json" = "yaml") =>
    `/api/v1/centering/collections/${encodeURIComponent(collectionId)}/export?format=${format}`,
  importQuotes: async (file: File, opts: { collectionId?: string; replace?: boolean }): Promise<QuoteImportResult> => {
    const form = new FormData();
    form.append("file", file, file.name);
    const q = new URLSearchParams();
    if (opts.collectionId) q.set("collectionId", opts.collectionId);
    if (opts.replace) q.set("replace", "1");
    const res = await fetch(`/api/v1/centering/import?${q}`, {
      method: "POST",
      credentials: "same-origin",
      headers: CLIENT_HEADER,
      body: form,
    });
    const body = await res.json().catch(() => undefined);
    if (!res.ok) throw new ApiError(res.status, (body as { error?: string })?.error ?? res.statusText, body);
    return body as QuoteImportResult;
  },

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
