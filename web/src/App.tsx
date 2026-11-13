// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, ApiError, type Board, type BoardTemplate, type ImportResult } from "./api";
import markUrl from "./art/mark-small.svg";
import emptyStateSvg from "./art/empty-state.svg?raw";
import { Link, navigate, usePath } from "./router";
import { BoardPage } from "./board/BoardPage";
import { BoardDrawers, BoardTile } from "./Boards";
import { CalendarPage } from "./Calendar";
import { NotificationsButton } from "./Notifications";
import { SettingsPage, useBrandingThemes } from "./Settings";
import { rememberReturn } from "./BackLink";
import { CenteringPage } from "./CenteringSettings";
import { applyAppearance, DEFAULT_PREFS, deviceTimeZone, watchSystem } from "./appearance";
import { Dialog, Icon } from "./ui";
import { useLiveSync } from "./live";
import { useOffline } from "./offline";

// Opening the app lands where you left off (§3.3): the first time a tab
// loads "/", it goes to the last board you had open. Reloads and later
// visits to "/" (the board list link) stay put; sessionStorage is per tab and
// survives reloads, so it marks "this tab already landed".
const LANDED = "chrysalis.landed";
const alreadyLanded = () => {
  try {
    return sessionStorage.getItem(LANDED) === "1";
  } catch {
    return true; // storage blocked: never redirect unexpectedly
  }
};
const markLanded = () => {
  try {
    sessionStorage.setItem(LANDED, "1");
  } catch {
    // fine: worst case we don't redirect
  }
};

export function App() {
  const path = usePath();
  const me = useQuery({ queryKey: ["me"], queryFn: api.me });
  const offline = useOffline();
  const boardMatch = path.match(/^\/b\/([^/]+)$/);
  // Changes from other devices and tabs: this board's, and this user's own.
  // (The calendar subscribes to its own boards.)
  useLiveSync(boardMatch ? decodeURIComponent(boardMatch[1]) : undefined, !!me.data && path !== "/calendar");

  useEffect(() => {
    if (!me.data) return;
    if (!alreadyLanded() && window.location.pathname === "/" && me.data.lastBoardId) {
      window.history.replaceState(null, "", `/b/${me.data.lastBoardId}`);
      window.dispatchEvent(new PopStateEvent("popstate"));
    }
    markLanded();
  }, [me.data]);

  // Reminders use the person's time zone; keep it current as they travel.
  useEffect(() => {
    const p = me.data?.prefs;
    const tz = deviceTimeZone();
    if (p?.remindersEnabled && tz && p.timeZone !== tz) void api.updatePrefs({ timeZone: tz }).catch(() => {});
  }, [me.data?.prefs?.remindersEnabled, me.data?.prefs?.timeZone]); // eslint-disable-line react-hooks/exhaustive-deps

  // Appearance follows the person's preferences, and the system for "match
  // this device" (§3.5, §3.12).
  const themes = useBrandingThemes();
  const prefs = me.data?.prefs;
  useEffect(() => {
    const apply = () => applyAppearance({ ...DEFAULT_PREFS, ...(prefs ?? {}) }, themes);
    apply();
    return watchSystem(apply);
  }, [prefs, themes]);

  // Branding: the deployment's name and icon replace Chrysalis's own.
  const branding = useQuery({ queryKey: ["branding"], queryFn: api.branding, staleTime: Infinity });
  const appName = me.data?.appName || "Chrysalis";
  const iconUrl = branding.data?.hasIcon ? "/branding/icon" : markUrl;
  useEffect(() => {
    if (!branding.data?.hasIcon) return;
    document.querySelectorAll<HTMLLinkElement>('link[rel="icon"]').forEach((l) => (l.href = "/branding/icon"));
  }, [branding.data?.hasIcon]);

  if (me.error instanceof ApiError && me.error.status === 401) {
    return <SignedOut />;
  }

  let page;
  if (path === "/") page = <BoardList />;
  else if (path === "/settings") page = <SettingsPage />;
  else if (path === "/calendar") page = <CalendarPage />;
  else if (path === "/settings/centering") page = <CenteringPage />;
  else if (boardMatch)
    page = (
      <BoardPage
        id={decodeURIComponent(boardMatch[1])}
        empty={(text) => <EmptyState>{text}</EmptyState>}
        notFound={<NotFound />}
      />
    );
  else page = <NotFound />;

  return (
    <>
      <header className="app-header">
        <Link to="/" className="app-name">
          <img src={iconUrl} alt="" width="28" height="28" className="app-mark" />
          {appName}
        </Link>
        {me.data && (
          <span className="header-right">
            <NotificationsButton enabled={!!me.data.prefs?.remindersEnabled} />
            <Link to="/calendar" className="header-link" aria-label="Calendar" title="Calendar" onClick={() => rememberReturn("calendar")}>
              <Icon.Calendar />
            </Link>
            <Link to="/settings" className="header-link" aria-label="Settings" onClick={() => rememberReturn("settings")}>
              <Icon.Settings />
            </Link>
            <span className="muted">{me.data.displayName}</span>
            {/* Which build is running, to confirm a deploy at a glance. */}
            {me.data.version && (
              <span className="app-version muted" title="App version">
                {/^\d/.test(me.data.version) ? `v${me.data.version}` : me.data.version}
              </span>
            )}
          </span>
        )}
      </header>
      {offline && (
        <p className="offline-banner" role="status">
          Offline — viewing last synced copy
        </p>
      )}
      <main className={boardMatch ? "is-board" : undefined}>{page}</main>
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
      {boards.data?.length === 0 && <EmptyState>A fresh start. Create your first board below.</EmptyState>}
      {boards.data && boards.data.length > 0 && (
        <ul className="board-list">
          {boards.data.map((b, i) => (
            <BoardTile key={b.id} board={b} boards={boards.data} index={i} />
          ))}
        </ul>
      )}
      <NewBoardForm />
      <TrelloImport />
      <BoardDrawers />
    </section>
  );
}

// Import from Trello (§8): pick the board's JSON export; a new board is
// made from it, then a short report says what came across and what didn't.
function TrelloImport() {
  const qc = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [result, setResult] = useState<ImportResult | null>(null);
  const run = useMutation({
    mutationFn: api.importTrello,
    onSuccess: (r) => {
      qc.setQueryData<Board[]>(["boards"], (old) => [...(old ?? []), r.board]);
      void qc.invalidateQueries({ queryKey: ["boards"] });
      setResult(r);
    },
  });

  const n = result?.report.imported;
  const counts = n
    ? (
        [
          [n.columns, "column", "columns"],
          [n.cards, "card", "cards"],
          [n.labels, "label", "labels"],
          [n.checklists, "checklist", "checklists"],
          [n.checklistItems, "checklist item", "checklist items"],
          [n.comments, "comment", "comments"],
          [n.links, "link", "links"],
        ] as const
      )
        .filter(([count]) => count > 0)
        .map(([count, one, many]) => `${count} ${count === 1 ? one : many}`)
    : [];

  return (
    <div className="trello-import">
      <input
        ref={input}
        type="file"
        accept=".json,application/json"
        hidden
        aria-label="Trello export file"
        onChange={(e) => {
          const file = e.target.files?.[0];
          e.target.value = "";
          if (file) run.mutate(file);
        }}
      />
      <button type="button" className="text-button" disabled={run.isPending} onClick={() => input.current?.click()}>
        {run.isPending ? "Importing…" : "Import from Trello"}
      </button>
      {run.error && (
        <p className="note" role="status">
          {run.error instanceof ApiError && (run.error.status === 400 || run.error.status === 413)
            ? run.error.message
            : "That didn't import. Check your connection and try again."}
        </p>
      )}
      <Dialog open={result !== null} onClose={() => setResult(null)} title="Imported from Trello">
        {result && (
          <div className="import-report">
            <p>
              <strong>{result.board.name}</strong> is ready{counts.length > 0 ? `: ${counts.join(", ")}.` : "."}
            </p>
            {result.report.notes.length > 0 && (
              <>
                <h3>Good to know</h3>
                <ul>
                  {result.report.notes.map((t) => (
                    <li key={t}>{t}</li>
                  ))}
                </ul>
              </>
            )}
            {result.report.skipped.length > 0 && (
              <>
                <h3>Left out</h3>
                <ul>
                  {result.report.skipped.map((t) => (
                    <li key={t}>{t}</li>
                  ))}
                </ul>
              </>
            )}
            <div className="dialog-actions">
              <Link to={`/b/${result.boardId}`} className="button" onClick={() => setResult(null)}>
                Open the board
              </Link>
            </div>
          </div>
        )}
      </Dialog>
    </div>
  );
}

// Quick capture: type a name, press Enter, and the field clears and stays
// focused for the next one.
function NewBoardForm() {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [template, setTemplate] = useState<BoardTemplate>("blank");
  const input = useRef<HTMLInputElement>(null);
  const create = useMutation({
    mutationFn: (n: string) => api.createBoard(n, template),
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
      <fieldset className="template-choice">
        <legend>Start with</legend>
        {(
          [
            ["blank", "An empty board"],
            ["simple", "To do, Doing, Done"],
            ["metamorphosis", "Egg, Caterpillar, Chrysalis, Imago"],
          ] as const
        ).map(([value, text]) => (
          <label key={value} className="check">
            <input type="radio" name="template" checked={template === value} onChange={() => setTemplate(value)} />
            {text}
          </label>
        ))}
      </fieldset>
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

// A soft line drawing with one gentle sentence (docs/DESIGN.md §3.12). The
// SVG is inlined (it's our own static file) so its strokes can use
// currentColor and follow the theme.
function EmptyState({ children }: { children: string }) {
  return (
    <div className="empty-state">
      <span className="empty-art" aria-hidden="true" dangerouslySetInnerHTML={{ __html: emptyStateSvg }} />
      <p className="empty">{children}</p>
    </div>
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
