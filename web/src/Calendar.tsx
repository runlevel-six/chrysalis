// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// The calendar view (docs/DESIGN.md §3.8; docs/adr/0001): month and week views
// of cards with due dates, on one board or all of them. Drag a card to another
// day to reschedule it (keyboard too: M to pick up, arrows move a day or a
// week, M to drop); click it to open it on its board. Undated cards can sit in
// a side panel and be dragged onto a day.

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  closestCenter,
  pointerWithin,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type Announcements,
  type CollisionDetection,
  type DragEndEvent,
  type DragStartEvent,
  type KeyboardCoordinateGetter,
} from "@dnd-kit/core";

import { api, ApiError, type Calendar as CalendarData, type Card } from "./api";
import { BackLink } from "./BackLink";
import { useLiveSync } from "./live";
import { navigate } from "./router";
import { Icon } from "./ui";
import { useUndo } from "./undo";

type View = "month" | "week";

// ---------- Dates (local time throughout) ----------

const pad = (n: number) => String(n).padStart(2, "0");
export const ymd = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const parse = (s: string) => {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, m - 1, d);
};
const addDays = (d: Date, n: number) => new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
const daysBetween = (a: string, b: string) => Math.round((parse(b).getTime() - parse(a).getTime()) / 86_400_000);

/** The locale's first day of the week, as getDay() numbers it (0 = Sunday). */
function weekStartDay(): number {
  try {
    const loc = new Intl.Locale(navigator.language) as Intl.Locale & {
      getWeekInfo?: () => { firstDay: number };
      weekInfo?: { firstDay: number };
    };
    const first = loc.getWeekInfo?.().firstDay ?? loc.weekInfo?.firstDay;
    if (first) return first % 7;
  } catch {
    // older browsers: fall through
  }
  return 0;
}

const startOfWeek = (d: Date, first: number) => addDays(d, -((d.getDay() - first + 7) % 7));

/** The local day a card is due on. */
export const dueDay = (c: Card) => (c.dueAt ? (c.dueHasTime ? ymd(new Date(c.dueAt)) : c.dueAt.slice(0, 10)) : null);

const longDate = (s: string) => parse(s).toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric" });

/** The change that moves a card to day, keeping its time and its start date's distance. */
export function reschedule(c: Card, day: string): Pick<Card, "dueAt" | "dueHasTime" | "startAt"> {
  const from = dueDay(c);
  let dueAt = day;
  if (c.dueAt && c.dueHasTime) {
    const old = new Date(c.dueAt);
    const [y, m, d] = day.split("-").map(Number);
    dueAt = new Date(y, m - 1, d, old.getHours(), old.getMinutes()).toISOString();
  }
  let startAt = c.startAt;
  if (startAt && from) startAt = ymd(addDays(parse(startAt), daysBetween(from, day)));
  else if (startAt && startAt > day) startAt = day; // undated with a start after the new day
  return { dueAt, dueHasTime: !!c.dueAt && c.dueHasTime, startAt };
}

// ---------- URL state ----------

function readParams() {
  const p = new URLSearchParams(window.location.search);
  const v = p.get("view");
  let saved: View | null = null;
  try {
    saved = localStorage.getItem("chrysalis.calendar.view") as View | null;
  } catch {
    // fine
  }
  const phone = window.matchMedia("(max-width: 640px)").matches;
  return {
    board: p.get("board") ?? "",
    view: (v === "week" || v === "month" ? v : saved === "week" || saved === "month" ? saved : phone ? "week" : "month") as View,
    date: /^\d{4}-\d{2}-\d{2}$/.test(p.get("date") ?? "") ? p.get("date")! : ymd(new Date()),
    undated: p.get("undated") === "1",
  };
}

export function CalendarPage() {
  const [state, setState] = useState(readParams);
  const { board, view, date, undated } = state;
  const set = (p: Partial<typeof state>) => {
    const next = { ...state, ...p };
    setState(next);
    const q = new URLSearchParams();
    if (next.board) q.set("board", next.board);
    q.set("view", next.view);
    q.set("date", next.date);
    if (next.undated) q.set("undated", "1");
    window.history.replaceState(null, "", `/calendar?${q}`);
    try {
      localStorage.setItem("chrysalis.calendar.view", next.view);
    } catch {
      // fine
    }
  };

  const first = useMemo(weekStartDay, []);
  const anchor = parse(date);
  const start = view === "month" ? startOfWeek(new Date(anchor.getFullYear(), anchor.getMonth(), 1), first) : startOfWeek(anchor, first);
  const days = Array.from({ length: view === "month" ? 42 : 7 }, (_, i) => ymd(addDays(start, i)));
  const from = days[0];
  const to = days[days.length - 1];

  const key = ["calendar", board || "all", from, to, undated] as const;
  const cal = useQuery({ queryKey: key, queryFn: () => api.calendar({ board: board || undefined, from, to, undated }) });
  const boards = useQuery({ queryKey: ["boards"], queryFn: api.listBoards });
  // Changes on any of these boards, from other tabs and devices.
  useLiveSync(cal.data?.boards.map((b) => b.id) ?? (board ? [board] : []));

  const boardName = board ? (boards.data?.find((b) => b.id === board)?.name ?? "this board") : "";
  useEffect(() => {
    document.title = `Calendar${boardName ? ` · ${boardName}` : ""} · Chrysalis`;
  }, [boardName]);

  const step = (dir: -1 | 1) =>
    set({ date: ymd(view === "month" ? new Date(anchor.getFullYear(), anchor.getMonth() + dir, 1) : addDays(anchor, 7 * dir)) });
  const title =
    view === "month"
      ? anchor.toLocaleDateString(undefined, { month: "long", year: "numeric" })
      : `${parse(from).toLocaleDateString(undefined, { month: "short", day: "numeric" })} – ${parse(to).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}`;

  return (
    <section className="page calendar-page">
      <BackLink page="calendar" fallback={board ? `/b/${encodeURIComponent(board)}` : "/"} />
      <div className="calendar-head">
        <h1>Calendar</h1>
        <div className="calendar-controls">
          <div className="row" role="group" aria-label="Move through time">
            <button type="button" className="icon-button" aria-label={view === "month" ? "Previous month" : "Previous week"} onClick={() => step(-1)}>
              <Icon.ChevronLeft />
            </button>
            <button type="button" className="text-button" onClick={() => set({ date: ymd(new Date()) })}>
              Today
            </button>
            <button type="button" className="icon-button" aria-label={view === "month" ? "Next month" : "Next week"} onClick={() => step(1)}>
              <Icon.ChevronRight />
            </button>
            <h2 className="calendar-title" aria-live="polite">
              {title}
            </h2>
          </div>
          <div className="row">
            <div className="segmented" role="radiogroup" aria-label="Calendar view">
              {(["month", "week"] as const).map((v) => (
                <button key={v} type="button" role="radio" aria-checked={view === v} onClick={() => set({ view: v })}>
                  {v === "month" ? "Month" : "Week"}
                </button>
              ))}
            </div>
            <label className="calendar-board">
              <span className="visually-hidden">Boards shown</span>
              <select value={board} onChange={(e) => set({ board: e.target.value })}>
                <option value="">All boards</option>
                {boards.data?.map((b) => (
                  <option key={b.id} value={b.id}>
                    {b.icon ? `${b.icon} ` : ""}
                    {b.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="check">
              <input type="checkbox" checked={undated} onChange={(e) => set({ undated: e.target.checked })} />
              Undated cards
            </label>
          </div>
        </div>
      </div>

      {cal.isPending && <p className="muted">Loading…</p>}
      {cal.error && <p className="note">Couldn't load the calendar right now.</p>}
      {cal.data && (
        <CalendarBody
          data={cal.data}
          queryKey={key}
          view={view}
          days={days}
          month={anchor.getMonth()}
          showBoard={!board}
          undated={undated}
          onPickWeek={(day) => set({ view: "week", date: day })}
        />
      )}
    </section>
  );
}

// ---------- Body: grid, drag and drop, undated panel ----------

function CalendarBody({
  data,
  queryKey,
  view,
  days,
  month,
  showBoard,
  undated,
  onPickWeek,
}: {
  data: CalendarData;
  queryKey: readonly unknown[];
  view: View;
  days: string[];
  month: number;
  showBoard: boolean;
  undated: boolean;
  onPickWeek: (day: string) => void;
}) {
  const qc = useQueryClient();
  const { push, notify } = useUndo();
  const [activeId, setActiveId] = useState<string | null>(null);

  const all = useMemo(() => new Map([...data.cards, ...data.undated].map((c) => [c.id, c])), [data]);
  const byDay = useMemo(() => {
    const m = new Map<string, Card[]>();
    for (const c of data.cards) {
      const d = dueDay(c);
      if (!d) continue;
      if (!m.has(d)) m.set(d, []);
      m.get(d)!.push(c);
    }
    for (const list of m.values()) list.sort((a, b) => (a.dueHasTime ? 1 : 0) - (b.dueHasTime ? 1 : 0) || (a.dueAt ?? "").localeCompare(b.dueAt ?? ""));
    return m;
  }, [data.cards]);
  const boards = useMemo(() => new Map(data.boards.map((b) => [b.id, b])), [data.boards]);
  const labels = useMemo(() => new Map(data.labels.map((l) => [l.id, l])), [data.labels]);
  const columns = useMemo(() => new Map(data.columns.map((c) => [c.id, c])), [data.columns]);

  // Arrow keys move a dragged card a whole day (or week), not a few pixels.
  const byDays: KeyboardCoordinateGetter = (event, { currentCoordinates, context }) => {
    const rect = [...context.droppableRects.values()].find((r) => r.width > 0);
    const w = rect?.width ?? 120;
    const h = rect?.height ?? 100;
    const { x, y } = currentCoordinates;
    switch (event.code) {
      case "ArrowRight":
        return { x: x + w, y };
      case "ArrowLeft":
        return { x: x - w, y };
      case "ArrowDown":
        return { x, y: y + h };
      case "ArrowUp":
        return { x, y: y - h };
    }
    return undefined;
  };
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 5 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 8 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: byDays,
      keyboardCodes: { start: ["KeyM"], cancel: ["Escape"], end: ["KeyM", "Space", "Enter"] },
    }),
  );

  const title = (id: unknown) => all.get(String(id).replace(/^card:/, ""))?.title ?? "card";
  const where = (id: unknown) => (String(id).startsWith("day:") ? longDate(String(id).slice(4)) : "the undated cards");
  const announcements: Announcements = {
    onDragStart: ({ active }) => `Picked up ${title(active.id)}. Arrow keys move a day or a week; M or Space to drop, Escape to cancel.`,
    onDragOver: ({ active, over }) => (over ? `${title(active.id)} is over ${where(over.id)}.` : `${title(active.id)} is not over a day.`),
    onDragEnd: ({ active, over }) => (over ? `${title(active.id)} moved to ${where(over.id)}.` : `${title(active.id)} was put back.`),
    onDragCancel: ({ active }) => `Move cancelled. ${title(active.id)} is back where it was.`,
  };

  const put = (card: Card, patch: Partial<Card>) =>
    qc.setQueryData<CalendarData>(queryKey, (d) => {
      if (!d) return d;
      const next = { ...card, ...patch };
      const cards = d.cards.filter((c) => c.id !== card.id);
      const undatedCards = d.undated.filter((c) => c.id !== card.id);
      if (next.dueAt) cards.push(next);
      else undatedCards.push(next);
      return { ...d, cards, undated: undatedCards };
    });

  const move = async (card: Card, day: string) => {
    if (dueDay(card) === day) return;
    const patch = reschedule(card, day);
    const before = { dueAt: card.dueAt, dueHasTime: card.dueHasTime, startAt: card.startAt };
    put(card, patch);
    try {
      await api.updateCard(card.id, patch);
      push(`Moved “${card.title}” to ${longDate(day)}`, async () => {
        await api.updateCard(card.id, before);
        await qc.invalidateQueries({ queryKey: ["calendar"] });
      });
    } catch (err) {
      notify(err instanceof ApiError ? err.message : "That didn't save. Check your connection and try again.");
    } finally {
      void qc.invalidateQueries({ queryKey: ["calendar"] });
    }
  };

  const onDragStart = ({ active }: DragStartEvent) => setActiveId(String(active.id).replace(/^card:/, ""));
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    setActiveId(null);
    const card = all.get(String(active.id).replace(/^card:/, ""));
    if (!card || !over || !String(over.id).startsWith("day:")) return;
    void move(card, String(over.id).slice(4));
  };

  const chip = (c: Card) => (
    <CardChip
      key={c.id}
      card={c}
      color={c.cover.type === "color" ? c.cover.value : c.labelIds.map((id) => labels.get(id)?.color).find(Boolean)}
      board={showBoard ? boards.get(c.boardId)?.name : undefined}
      done={c.completedAt !== null || !!columns.get(c.columnId)?.isDone}
    />
  );
  const active = activeId ? all.get(activeId) : undefined;
  const today = ymd(new Date());

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={underPointerOrNearest}
      accessibility={{ announcements }}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragCancel={() => setActiveId(null)}
    >
      <div className={`calendar-layout${undated ? " with-panel" : ""}`}>
        <div className={`calendar-grid is-${view}`}>
          <div className="calendar-weekdays" aria-hidden="true">
            {days.slice(0, 7).map((d) => (
              <span key={d}>{parse(d).toLocaleDateString(undefined, { weekday: "short" })}</span>
            ))}
          </div>
          <div className="calendar-days">
            {days.map((d) => (
              <Day
                key={d}
                day={d}
                view={view}
                outside={view === "month" && parse(d).getMonth() !== month}
                today={d === today}
                cards={byDay.get(d) ?? []}
                chip={chip}
                onPickWeek={onPickWeek}
              />
            ))}
          </div>
        </div>
        {undated && (
          <aside className="calendar-undated" aria-label="Undated cards">
            <h2>Undated cards</h2>
            <p className="muted">Drag one onto a day to give it a due date.</p>
            {data.undated.length === 0 ? (
              <p className="muted">Every open card has a date.</p>
            ) : (
              <ul className="chip-list">
                {data.undated.map((c) => (
                  <li key={c.id}>{chip(c)}</li>
                ))}
              </ul>
            )}
            {data.undatedMore && <p className="muted">There are more; pick one board to see them all.</p>}
          </aside>
        )}
      </div>
      <DragOverlay>{active && <div className="cal-chip is-overlay">{active.title}</div>}</DragOverlay>
    </DndContext>
  );
}

// The day under the pointer; a keyboard drag has no pointer, so the nearest.
const underPointerOrNearest: CollisionDetection = (args) => (args.pointerCoordinates ? pointerWithin(args) : closestCenter(args));

const MONTH_LIMIT = 3;

function Day({
  day,
  view,
  outside,
  today,
  cards,
  chip,
  onPickWeek,
}: {
  day: string;
  view: View;
  outside: boolean;
  today: boolean;
  cards: Card[];
  chip: (c: Card) => ReactNode;
  onPickWeek: (day: string) => void;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: `day:${day}` });
  const [all, setAll] = useState(false);
  const d = parse(day);
  const shown = view === "month" && !all ? cards.slice(0, MONTH_LIMIT) : cards;
  const label = `${longDate(day)}${today ? ", today" : ""}: ${cards.length ? `${cards.length} ${cards.length === 1 ? "card" : "cards"} due` : "nothing due"}`;
  return (
    <section
      ref={setNodeRef}
      className={`cal-day${outside ? " is-outside" : ""}${today ? " is-today" : ""}${isOver ? " is-over" : ""}`}
      aria-label={label}
    >
      <h3 className="cal-date">
        {view === "week" && <span className="cal-weekday">{d.toLocaleDateString(undefined, { weekday: "short" })} </span>}
        {view === "month" ? (
          <button type="button" className="cal-date-button" onClick={() => onPickWeek(day)} aria-label={`Show the week of ${longDate(day)}`}>
            {d.getDate()}
          </button>
        ) : (
          d.toLocaleDateString(undefined, { month: "short", day: "numeric" })
        )}
      </h3>
      {/* Phones show a quiet dot per card in month view; the week shows them. */}
      {view === "month" && cards.length > 0 && (
        <span className="cal-dots" aria-hidden="true">
          {cards.slice(0, 4).map((c) => (
            <span key={c.id} className="cal-dot" />
          ))}
        </span>
      )}
      {shown.length > 0 && (
        <ul className="chip-list">
          {shown.map((c) => (
            <li key={c.id}>{chip(c)}</li>
          ))}
        </ul>
      )}
      {view === "month" && cards.length > MONTH_LIMIT && (
        <button type="button" className="text-button cal-more" onClick={() => setAll((v) => !v)}>
          {all ? "Show fewer" : `${cards.length - MONTH_LIMIT} more`}
        </button>
      )}
    </section>
  );
}

function CardChip({ card, color, board, done }: { card: Card; color?: string; board?: string; done: boolean }) {
  const { setNodeRef, listeners, attributes, isDragging } = useDraggable({ id: `card:${card.id}` });
  const time = card.dueAt && card.dueHasTime ? new Date(card.dueAt).toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" }) : "";
  const open = () => navigate(`/b/${card.boardId}?card=${encodeURIComponent(card.id)}`);
  return (
    <div
      ref={setNodeRef}
      {...attributes}
      {...listeners}
      role="button"
      aria-roledescription="draggable card"
      aria-label={`${card.title}${time ? `, ${time}` : ""}${board ? `, on ${board}` : ""}${done ? ", done" : ""}. Enter opens it; M moves it to another day.`}
      className={`cal-chip${done ? " is-done" : ""}${isDragging ? " is-dragging" : ""}${color ? ` label-${color}` : ""}`}
      onClick={open}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget) return;
        if (e.key === "Enter") {
          e.preventDefault();
          open();
          return;
        }
        listeners?.onKeyDown?.(e);
      }}
    >
      {done && <Icon.Check />}
      <span className="cal-chip-title">{card.title}</span>
      {(time || board) && <span className="cal-chip-meta">{[time, board].filter(Boolean).join(" · ")}</span>}
    </div>
  );
}
