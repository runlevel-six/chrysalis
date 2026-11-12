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
  return (await res.json()) as T;
}

export const api = {
  me: () => request<User>("GET", "/me"),
  listBoards: () => request<{ boards: Board[] }>("GET", "/boards").then((r) => r.boards),
  getBoard: (id: string) => request<Board>("GET", `/boards/${encodeURIComponent(id)}`),
  createBoard: (name: string) => request<Board>("POST", "/boards", { name }),
};
