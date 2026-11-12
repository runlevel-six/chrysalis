// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, ApiError, type Board } from "./api";
import { Link, navigate, usePath } from "./router";

export function App() {
  const path = usePath();
  const me = useQuery({ queryKey: ["me"], queryFn: api.me });

  if (me.error instanceof ApiError && me.error.status === 401) {
    return <SignedOut />;
  }

  const boardMatch = path.match(/^\/b\/([^/]+)$/);
  let page;
  if (path === "/") page = <BoardList />;
  else if (boardMatch) page = <BoardPage id={decodeURIComponent(boardMatch[1])} />;
  else page = <NotFound />;

  return (
    <>
      <header className="app-header">
        <Link to="/" className="app-name">
          <img src="/icon-192.png" alt="" width="28" height="28" className="app-mark" />
          Chrysalis
        </Link>
        {me.data && <span className="muted">{me.data.displayName}</span>}
      </header>
      <main>{page}</main>
    </>
  );
}

function usePageTitle(title: string) {
  useEffect(() => {
    document.title = title ? `${title} · Chrysalis` : "Chrysalis";
  }, [title]);
}

function BoardList() {
  usePageTitle("");
  const boards = useQuery({ queryKey: ["boards"], queryFn: api.listBoards });

  return (
    <section className="page">
      <h1>Your boards</h1>
      {boards.isPending && <p className="muted">Loading…</p>}
      {boards.error && <LoadError />}
      {boards.data?.length === 0 && <p className="empty">A fresh start. Create your first board below.</p>}
      {boards.data && boards.data.length > 0 && (
        <ul className="board-list">
          {boards.data.map((b) => (
            <li key={b.id}>
              <Link to={`/b/${b.id}`} className="board-tile">
                {b.name}
              </Link>
            </li>
          ))}
        </ul>
      )}
      <NewBoardForm />
    </section>
  );
}

// Quick capture: type a name, press Enter, and the field clears and stays
// focused for the next one.
function NewBoardForm() {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const create = useMutation({
    mutationFn: api.createBoard,
    onSuccess: (board) => {
      qc.setQueryData<Board[]>(["boards"], (old) => [...(old ?? []), board]);
      setName("");
      input.current?.focus();
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (name.trim() && !create.isPending) create.mutate(name);
  };

  return (
    <form className="new-board" onSubmit={submit}>
      <label htmlFor="new-board-name">New board</label>
      <div className="row">
        <input
          id="new-board-name"
          ref={input}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. Garden plans"
          maxLength={200}
          autoComplete="off"
        />
        <button type="submit" disabled={!name.trim() || create.isPending}>
          Create
        </button>
      </div>
      {create.error && (
        <p className="note" role="status">
          {create.error instanceof ApiError && create.error.status === 400
            ? create.error.message
            : "That didn't save. Check your connection and try again."}
        </p>
      )}
    </form>
  );
}

function BoardPage({ id }: { id: string }) {
  const board = useQuery({ queryKey: ["board", id], queryFn: () => api.getBoard(id) });
  usePageTitle(board.data?.name ?? "");

  if (board.isPending) return <p className="page muted">Loading…</p>;
  if (board.error instanceof ApiError && board.error.status === 404) return <NotFound />;
  if (board.error) return <LoadError />;

  return (
    <section className="page">
      <h1>{board.data.name}</h1>
      <p className="empty">A fresh start. Add your first card.</p>
    </section>
  );
}

function NotFound() {
  usePageTitle("Not found");
  return (
    <section className="page">
      <p>There's nothing here.</p>
      <button type="button" onClick={() => navigate("/")}>
        Back to your boards
      </button>
    </section>
  );
}

function LoadError() {
  return <p className="note">Couldn't load this right now. It'll be here when the connection's back.</p>;
}

function SignedOut() {
  const next = encodeURIComponent(window.location.pathname + window.location.search);
  return (
    <main className="page signed-out">
      <h1>Chrysalis</h1>
      <p>You're signed out.</p>
      <a className="button" href={`/auth/login?next=${next}`}>
        Sign in
      </a>
    </main>
  );
}
