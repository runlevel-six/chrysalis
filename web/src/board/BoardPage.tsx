// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

import { Suspense, useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties, type FormEvent, type ReactNode } from "react";
import { useIsMutating, useQuery } from "@tanstack/react-query";
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  closestCorners,
  useSensor,
  useSensors,
  type Announcements,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
  type UniqueIdentifier,
} from "@dnd-kit/core";
import { arrayMove, sortableKeyboardCoordinates } from "@dnd-kit/sortable";

import { api, ApiError, LABEL_COLORS, type BoardDetail, type Card, type Column, type Focus, type Label } from "../api";
import { Dialog, Icon, Menu } from "../ui";
import { flutterFrom } from "../flutter";
import { backgroundStyle } from "./backgrounds";
import { DEFAULT_PREFS, isDark } from "../appearance";
import { ArchivedDialog, BoardSettingsDialog, CardMeta, CoverBand, CoverDialog, DatesDialog, LabelChip, LabelsDialog } from "./details";
import { Attachments } from "./Attachments";
import { Checklists } from "./Checklists";
import { ColumnView } from "./ColumnView";
import { LazyDescription, TitleEditor, type CardActions, type CardContext } from "./CardItem";
import { byPosition, keyBetween, useBoardActions, useBoardDetail, useViewState } from "./useBoard";

type Containers = Record<string, string[]>; // column id -> card ids, in display order

function containersOf(d: BoardDetail): Containers {
  const out: Containers = {};
  for (const col of d.columns) out[col.id] = [];
  for (const c of [...d.cards].sort(byPosition)) out[c.columnId]?.push(c.id);
  return out;
}

const findContainer = (items: Containers, id: UniqueIdentifier) => {
  const s = String(id);
  if (s.startsWith("col:")) return s.slice(4);
  return Object.keys(items).find((col) => items[col].includes(s));
};

export function BoardPage({
  id,
  empty,
  notFound,
}: {
  id: string;
  empty: (text: string) => ReactNode;
  notFound: ReactNode;
}) {
  // Subscribed (not read once), so preference changes show immediately.
  const me = useQuery({ queryKey: ["me"], queryFn: api.me });
  const detail = useBoardDetail(id);
  const d = detail.data;
  const actions = useBoardActions(id);
  const view = useViewState(id, d);
  const saving = useIsMutating() > 0;

  const [fullCardId, setFullCardId] = useState<string | null>(null);
  const [moveCardId, setMoveCardId] = useState<string | null>(null);
  const [optionsColumnId, setOptionsColumnId] = useState<string | null>(null);
  const [trashOpen, setTrashOpen] = useState(false);
  const [archivedOpen, setArchivedOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [labelsCardId, setLabelsCardId] = useState<string | null>(null);
  const [datesCardId, setDatesCardId] = useState<string | null>(null);
  const [coverCardId, setCoverCardId] = useState<string | null>(null);
  const [filterOpen, setFilterOpen] = useState(false);
  const [capturingIn, setCapturingIn] = useState<string | null>(null);
  const lastColumn = useRef<string | null>(null);

  useEffect(() => {
    document.title = d ? `${d.board.name} · Chrysalis` : "Chrysalis";
  }, [d]);

  const columns = useMemo(() => (d ? [...d.columns].sort(byPosition) : []), [d]);
  const cardsById = useMemo(() => new Map(d?.cards.map((c) => [c.id, c]) ?? []), [d]);
  const readOnly = d?.board.role === "viewer";

  // Drag state: while dragging, the layout comes from `dragItems` so cards
  // can preview their new column before the drop is committed.
  const [dragItems, setDragItems] = useState<Containers | null>(null);
  const [activeId, setActiveId] = useState<string | null>(null);
  const items = dragItems ?? (d ? containersOf(d) : {});

  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    // Long-press to drag on touch, so scrolling still works (§3.2).
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 8 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
      keyboardCodes: { start: ["KeyM"], cancel: ["Escape"], end: ["KeyM", "Space", "Enter"] },
    }),
  );

  // One width for every column, from the board's width and how many columns
  // it has, counting collapsed ones as if open. Collapsing a column then
  // leaves space instead of resizing the others: things don't move unless
  // you moved them (§3.6). Phones and tablets use their own CSS widths.
  const columnsEl = useRef<HTMLDivElement>(null);
  const [colWidth, setColWidth] = useState<number | null>(null);
  const columnCount = d ? (view.focus.type === "column" ? 1 : d.columns.length) : 0;
  const hasAddColumn = !!d && d.board.role !== "viewer" && view.focus.type !== "column";
  useLayoutEffect(() => {
    const el = columnsEl.current;
    if (!el || !columnCount) return;
    const measure = () => {
      const rem = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
      const gap = 0.75 * rem;
      const padding = 2 * rem;
      const addColumn = hasAddColumn ? 14 * rem + gap : 0;
      const each = (el.clientWidth - padding - addColumn - gap * (columnCount - 1)) / columnCount;
      setColWidth(Math.round(Math.min(40 * rem, Math.max(17 * rem, each))));
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [columnCount, hasAddColumn]);

  // Remember this as the board to come back to (§3.3).
  useEffect(() => {
    void api.setView(id, { opened: true }).catch(() => {});
  }, [id]);

  // Board shortcuts (§3.10): N quick capture, F focus mode, / filter, Esc
  // leaves focus. Ignored while typing or inside a dialog.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t.closest("input, textarea, select, [contenteditable], dialog")) return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const here = () =>
        t.closest<HTMLElement>("[data-column-id]")?.dataset.columnId ?? lastColumn.current ?? columns.find((c) => !view.isCollapsed(c.id))?.id;
      const key = e.key.toLowerCase();
      if (key === "n" && !readOnly) {
        const target = here();
        if (!target) return;
        e.preventDefault();
        if (view.isCollapsed(target)) view.setCollapsed(target, false);
        setCapturingIn(target);
      } else if (key === "f") {
        e.preventDefault();
        if (view.focus.type !== "none") view.setFocus({ type: "none" });
        else {
          const target = here();
          if (target) view.setFocus({ type: "column", columnId: target });
        }
      } else if (e.key === "/") {
        e.preventDefault();
        setFilterOpen(true);
      } else if (e.key === "Escape" && view.focus.type !== "none") {
        view.setFocus({ type: "none" });
        setFilterOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [columns, view, readOnly]);

  if (detail.isPending) return <p className="page muted">Loading…</p>;
  if (detail.error instanceof ApiError && detail.error.status === 404) return <>{notFound}</>;
  if (detail.error || !d) return <p className="page note">Couldn't load this board right now.</p>;

  const cardActions: CardActions = {
    toggle: (c) => view.setExpanded([c.id], !view.isExpanded(c.id)),
    openFull: (c) => setFullCardId(c.id),
    rename: (c, title) => void actions.updateCard(c, { title }, `Renamed “${c.title}”`),
    setCompleted: (c, done) => {
      if (done) flutterFrom(c.id, me.data?.prefs);
      void actions.updateCard(c, { completed: done }, done ? `Marked “${c.title}” done` : `Marked “${c.title}” not done`);
    },
    moveTo: (c) => setMoveCardId(c.id),
    labels: (c) => setLabelsCardId(c.id),
    dates: (c) => setDatesCardId(c.id),
    cover: (c) => setCoverCardId(c.id),
    setCover: (c, cover) => void actions.updateCard(c, { cover }, `Changed the cover on “${c.title}”`),
    archive: (c) => {
      setFullCardId(null);
      void actions.updateCard(c, { archived: true }, `Archived “${c.title}”`);
    },
    trash: (c) => void actions.trashCard(c),
  };
  const ctx: CardContext = {
    labels: [...d.labels].sort(byPosition),
    settings: d.board.settings,
    highlightOverdue: me.data?.prefs?.highlightOverdue ?? false,
  };

  const columnName = (colId: string) => d.columns.find((c) => c.id === colId)?.name ?? "a column";
  // Moving a card into a done column from one that isn't finishes it (§3.12).
  const isDoneColumn = (colId: string) => !!d.columns.find((c) => c.id === colId)?.isDone;
  const finishes = (card: Card, toColumn: string) => isDoneColumn(toColumn) && !isDoneColumn(card.columnId);
  const announcements: Announcements = {
    onDragStart: ({ active }) => `Picked up ${cardsById.get(String(active.id))?.title ?? "card"}. Use arrow keys to move, M or Space to drop, Escape to cancel.`,
    onDragOver: ({ active, over }) => {
      if (!over) return `${cardsById.get(String(active.id))?.title ?? "Card"} is no longer over a column.`;
      const col = findContainer(items, over.id);
      const pos = col ? items[col].indexOf(String(active.id)) + 1 : 0;
      return `${cardsById.get(String(active.id))?.title ?? "Card"} is in ${col ? columnName(col) : "a column"}${pos > 0 ? `, position ${pos}` : ""}.`;
    },
    onDragEnd: ({ active, over }) =>
      over ? `Dropped ${cardsById.get(String(active.id))?.title ?? "card"}.` : "Move cancelled.",
    onDragCancel: () => "Move cancelled. The card is back where it was.",
  };

  const onDragStart = ({ active }: DragStartEvent) => {
    setActiveId(String(active.id));
    setDragItems(containersOf(d));
  };

  const onDragOver = ({ active, over }: DragOverEvent) => {
    if (!over || !dragItems) return;
    const from = findContainer(dragItems, active.id);
    const to = findContainer(dragItems, over.id);
    if (!from || !to || from === to) return;
    setDragItems((cur) => {
      if (!cur) return cur;
      const src = cur[from].filter((x) => x !== active.id);
      const dst = [...cur[to]];
      const overIndex = dst.indexOf(String(over.id));
      dst.splice(overIndex >= 0 ? overIndex : dst.length, 0, String(active.id));
      return { ...cur, [from]: src, [to]: dst };
    });
  };

  const onDragEnd = ({ active, over }: DragEndEvent) => {
    const card = cardsById.get(String(active.id));
    let final = dragItems;
    setActiveId(null);
    setDragItems(null);
    if (!card || !over || !final) return;
    const col = findContainer(final, active.id);
    if (!col) return;
    const overCol = findContainer(final, over.id);
    if (overCol === col && !String(over.id).startsWith("col:")) {
      const ids = final[col];
      final = { ...final, [col]: arrayMove(ids, ids.indexOf(String(active.id)), ids.indexOf(String(over.id))) };
    }
    const ids = final[col];
    const i = ids.indexOf(card.id);
    // Dropped where it started: nothing to save.
    if (card.columnId === col && containersOf(d)[col].indexOf(card.id) === i) return;
    // A new key between the neighbors it landed next to; only this card's row changes (§5.4).
    const before = i > 0 ? cardsById.get(ids[i - 1])?.position ?? null : null;
    const after = i < ids.length - 1 ? cardsById.get(ids[i + 1])?.position ?? null : null;
    if (finishes(card, col)) flutterFrom(card.id, me.data?.prefs);
    void actions.moveCard(card, col, keyBetween(before, after));
  };

  const activeCard = activeId ? cardsById.get(activeId) : undefined;
  const fullCard = fullCardId ? cardsById.get(fullCardId) : undefined;
  const moveCard = moveCardId ? cardsById.get(moveCardId) : undefined;
  const optionsColumn = optionsColumnId ? d.columns.find((c) => c.id === optionsColumnId) : undefined;
  const bgStyle = backgroundStyle(d.board, isDark({ ...DEFAULT_PREFS, ...(me.data?.prefs ?? {}) }));

  // What's on screen (§3.4): hide-completed drops done cards and cards in
  // done columns; focus mode shows one column, or only matching cards.
  const focus = view.focus;
  const doneColumns = new Set(d.columns.filter((c) => c.isDone).map((c) => c.id));
  const query = focus.type === "filter" ? (focus.query ?? "").trim().toLowerCase() : "";
  const wantLabels = focus.type === "filter" ? (focus.labelIds ?? []) : [];
  const visible = (c: Card) =>
    !(view.hideCompleted && (c.completedAt || doneColumns.has(c.columnId))) &&
    (!query || c.title.toLowerCase().includes(query)) &&
    (!wantLabels.length || c.labelIds.some((l) => wantLabels.includes(l)));
  const shownColumns = focus.type === "column" ? columns.filter((c) => c.id === focus.columnId) : columns;
  const focusedColumn = focus.type === "column" ? columns.find((c) => c.id === focus.columnId) : undefined;
  const allCardIds = d.cards.map((c) => c.id);

  return (
    <div className={`board${bgStyle ? " has-background" : ""}`} style={bgStyle}>
      <div className="board-head">
        <h1>{d.board.name}</h1>
        <span className="save-state muted" aria-live="polite">
          {saving ? "Saving…" : "Saved"}
        </span>
        <Menu
          label="View"
          icon={<Icon.Eye />}
          items={[
            { label: "Collapse all cards", onSelect: () => view.setExpanded(allCardIds, false) },
            { label: "Expand all cards", onSelect: () => view.setExpanded(allCardIds, true) },
            {
              label: view.hideCompleted ? "Show completed cards" : "Hide completed cards",
              onSelect: () => view.setHideCompleted(!view.hideCompleted),
            },
            { label: "Filter cards…", onSelect: () => setFilterOpen(true) },
          ]}
        />
        <Menu
          label="Board"
          icon={<Icon.More />}
          items={[
            ...(readOnly ? [] : [{ label: "Board settings", onSelect: () => setSettingsOpen(true) }]),
            { label: "Archived cards", onSelect: () => setArchivedOpen(true) },
            { label: "Trash", onSelect: () => setTrashOpen(true) },
          ]}
        />
      </div>

      {columns.length > 1 && (
        <nav className="column-tabs" aria-label="Columns">
          {columns.map((c) => (
            <button
              key={c.id}
              type="button"
              className="column-tab"
              onClick={() =>
                document.querySelector(`[data-column-id="${c.id}"]`)?.scrollIntoView({ behavior: "smooth", inline: "start", block: "nearest" })
              }
            >
              {c.name}
            </button>
          ))}
        </nav>
      )}

      {(filterOpen || focus.type === "filter") && (
        <FilterBar
          focus={focus}
          labels={ctx.labels}
          onChange={(f) => view.setFocus(f)}
          onClose={() => {
            setFilterOpen(false);
            view.setFocus({ type: "none" });
          }}
        />
      )}
      {(focusedColumn || view.hideCompleted) && (
        <div className="focus-banner" role="status">
          {focusedColumn && <span>Focused on {focusedColumn.name}.</span>}
          {view.hideCompleted && <span>Completed cards are hidden.</span>}
          {focusedColumn && (
            <button type="button" className="text-button" onClick={() => view.setFocus({ type: "none" })}>
              Show all columns
            </button>
          )}
          {view.hideCompleted && (
            <button type="button" className="text-button" onClick={() => view.setHideCompleted(false)}>
              Show completed
            </button>
          )}
        </div>
      )}

      <p id="card-shortcuts" hidden>
        Space to expand or collapse, Enter to open, E to edit the title, M to move with the arrow keys.
      </p>

      {columns.length === 0 && empty("A fresh start. Add your first column.")}

      <DndContext
        sensors={sensors}
        collisionDetection={closestCorners}
        accessibility={{ announcements }}
        onDragStart={onDragStart}
        onDragOver={onDragOver}
        onDragEnd={onDragEnd}
        onDragCancel={() => {
          setActiveId(null);
          setDragItems(null);
        }}
      >
        <div
          ref={columnsEl}
          className="columns"
          style={colWidth ? ({ "--col-width": `${colWidth}px` } as CSSProperties) : undefined}
          onFocus={(e) => {
          const col = (e.target as HTMLElement).closest<HTMLElement>("[data-column-id]")?.dataset.columnId;
          if (col) lastColumn.current = col;
        }}>
          {shownColumns.map((col) => (
            <ColumnView
              key={col.id}
              column={col}
              cards={(items[col.id] ?? []).map((cid) => cardsById.get(cid)).filter((c): c is Card => !!c && visible(c))}
              collapsed={view.isCollapsed(col.id)}
              readOnly={readOnly}
              isExpanded={view.isExpanded}
              cardActions={cardActions}
              ctx={ctx}
              onToggleCollapsed={() => view.setCollapsed(col.id, !view.isCollapsed(col.id))}
              onRename={(name) => void actions.updateColumn(col, { name }, `Renamed column “${col.name}”`)}
              onOptions={() => setOptionsColumnId(col.id)}
              onAddCard={(title) => {
                lastColumn.current = col.id;
                void actions.addCard(col.id, title);
              }}
              capturing={capturingIn === col.id}
              setCapturing={(on) => setCapturingIn(on ? col.id : null)}
            />
          ))}
          {!readOnly && focus.type !== "column" && <AddColumn onAdd={(name) => void actions.addColumn(name)} />}
        </div>
        <DragOverlay>
          {activeCard && <div className="card is-overlay">{activeCard.title}</div>}
        </DragOverlay>
      </DndContext>

      {fullCard && (
        <CardFull
          card={fullCard}
          column={d.columns.find((c) => c.id === fullCard.columnId)}
          readOnly={readOnly}
          actions={cardActions}
          ctx={ctx}
          onClose={() => setFullCardId(null)}
        />
      )}
      {labelsCardId && cardsById.get(labelsCardId) && (
        <LabelsDialog card={cardsById.get(labelsCardId)!} labels={ctx.labels} actions={actions} onClose={() => setLabelsCardId(null)} />
      )}
      {datesCardId && cardsById.get(datesCardId) && (
        <DatesDialog card={cardsById.get(datesCardId)!} actions={actions} onClose={() => setDatesCardId(null)} />
      )}
      {coverCardId && cardsById.get(coverCardId) && (
        <CoverDialog card={cardsById.get(coverCardId)!} actions={actions} onClose={() => setCoverCardId(null)} />
      )}
      {archivedOpen && <ArchivedDialog boardId={id} onClose={() => setArchivedOpen(false)} />}
      {settingsOpen && (
        <BoardSettingsDialog
          boardId={id}
          name={d.board.name}
          settings={d.board.settings}
          background={d.board.background}
          labels={ctx.labels}
          actions={actions}
          onClose={() => setSettingsOpen(false)}
        />
      )}
      {moveCard && (
        <MoveDialog
          card={moveCard}
          currentBoardId={id}
          onMove={(columnId) => {
            setMoveCardId(null);
            setFullCardId(null);
            if (finishes(moveCard, columnId)) flutterFrom(moveCard.id, me.data?.prefs);
            void actions.updateCard(moveCard, { columnId }, `Moved “${moveCard.title}”`);
          }}
          onClose={() => setMoveCardId(null)}
        />
      )}
      {optionsColumn && (
        <ColumnOptions
          column={optionsColumn}
          columns={columns}
          cardCount={(items[optionsColumn.id] ?? []).length}
          actions={actions}
          onFocus={() => view.setFocus({ type: "column", columnId: optionsColumn.id })}
          onExpandAll={(expanded) =>
            view.setExpanded(d.cards.filter((c) => c.columnId === optionsColumn.id).map((c) => c.id), expanded)
          }
          onClose={() => setOptionsColumnId(null)}
        />
      )}
      {trashOpen && <TrashDialog boardId={id} actions={actions} onClose={() => setTrashOpen(false)} />}
    </div>
  );
}

function AddColumn({ onAdd }: { onAdd: (name: string) => void }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    onAdd(name);
    setName("");
  };
  if (!open) {
    return (
      <button type="button" className="add-column" onClick={() => setOpen(true)}>
        <Icon.Plus /> Add a column
      </button>
    );
  }
  return (
    <form className="add-column is-open" onSubmit={submit}>
      <input
        autoFocus
        aria-label="New column name"
        placeholder="Column name"
        value={name}
        maxLength={100}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => e.key === "Escape" && setOpen(false)}
        onBlur={() => !name.trim() && setOpen(false)}
      />
    </form>
  );
}

/** Full view (§3.2): a large overlay. Closing returns to the card's inline state. */
function CardFull({
  card,
  column,
  readOnly,
  actions,
  ctx,
  onClose,
}: {
  card: Card;
  column?: Column;
  readOnly: boolean;
  actions: CardActions;
  ctx: CardContext;
  onClose: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const done = card.completedAt !== null;
  return (
    <Dialog open onClose={onClose} title={card.title} className="card-full">
      <CoverBand card={card} large />
      <p className="muted">Column: {column?.name ?? "unknown"}</p>
      <CardMeta card={card} labels={ctx.labels} />
      <Suspense fallback={<p className="muted description-loading">Loading description…</p>}>
        <LazyDescription card={card} readOnly={readOnly} />
      </Suspense>
      <Checklists card={card} readOnly={readOnly} />
      <Attachments card={card} readOnly={readOnly} onMakeCover={(a) => actions.setCover(card, { type: "image", value: a.id })} />
      {editing ? (
        <TitleEditor
          initial={card.title}
          onDone={(t) => {
            setEditing(false);
            if (t !== null && t.trim() && t.trim() !== card.title) actions.rename(card, t);
          }}
        />
      ) : (
        !readOnly && (
          <button type="button" className="text-button" onClick={() => setEditing(true)}>
            Edit title
          </button>
        )
      )}
      {!readOnly && (
        <div className="dialog-actions">
          <button type="button" className="text-button" onClick={() => actions.setCompleted(card, !done)}>
            <Icon.Check /> {done ? "Mark not done" : "Mark done"}
          </button>
          <button type="button" className="text-button" onClick={() => actions.labels(card)}>
            <Icon.Tag /> Labels…
          </button>
          <button type="button" className="text-button" onClick={() => actions.dates(card)}>
            <Icon.Calendar /> Dates…
          </button>
          <button type="button" className="text-button" onClick={() => actions.cover(card)}>
            Cover…
          </button>
          <button type="button" className="text-button" onClick={() => actions.moveTo(card)}>
            Move to…
          </button>
          <button type="button" className="text-button" onClick={() => actions.archive(card)}>
            Archive
          </button>
          <button
            type="button"
            className="text-button"
            onClick={() => {
              onClose();
              actions.trash(card);
            }}
          >
            <Icon.Trash /> Delete
          </button>
        </div>
      )}
    </Dialog>
  );
}

/** "Move to…" (§3.7): the reliable alternative to dragging, including to another board. */
function MoveDialog({
  card,
  currentBoardId,
  onMove,
  onClose,
}: {
  card: Card;
  currentBoardId: string;
  onMove: (columnId: string) => void;
  onClose: () => void;
}) {
  const [boardId, setBoardId] = useState(currentBoardId);
  const boards = useQuery({ queryKey: ["boards"], queryFn: api.listBoards });
  const target = useQuery({ queryKey: ["board", boardId, "detail"], queryFn: () => api.boardDetail(boardId) });
  const editable = (boards.data ?? []).filter((b) => b.role !== "viewer");
  const cols = target.data ? [...target.data.columns].sort(byPosition) : [];

  return (
    <Dialog open onClose={onClose} title={`Move “${card.title}”`}>
      {editable.length > 1 && (
        <label className="field">
          Board
          <select value={boardId} onChange={(e) => setBoardId(e.target.value)}>
            {editable.map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </select>
        </label>
      )}
      <p className="field-label">Column</p>
      {target.isPending && <p className="muted">Loading…</p>}
      {target.data && cols.length === 0 && <p className="muted">That board has no columns yet.</p>}
      <ul className="choice-list">
        {cols.map((c) => (
          <li key={c.id}>
            <button
              type="button"
              className="choice"
              disabled={c.id === card.columnId}
              onClick={() => onMove(c.id)}
            >
              {c.name}
              {c.id === card.columnId && <span className="muted"> (here now)</span>}
            </button>
          </li>
        ))}
      </ul>
    </Dialog>
  );
}

function ColumnOptions({
  column,
  columns,
  cardCount,
  actions,
  onFocus,
  onExpandAll,
  onClose,
}: {
  column: Column;
  columns: Column[];
  cardCount: number;
  actions: ReturnType<typeof useBoardActions>;
  onFocus: () => void;
  onExpandAll: (expanded: boolean) => void;
  onClose: () => void;
}) {
  const [confirming, setConfirming] = useState(false);
  // Local copies so controls respond on the click itself; see BoardSettingsDialog.
  const [isDone, setIsDone] = useState(column.isDone);
  const [color, setColor] = useState(column.color);
  const [colorMode, setColorMode] = useState(column.colorMode);
  const i = columns.findIndex((c) => c.id === column.id);
  const move = (dir: -1 | 1) => {
    // New position between the neighbors on the side we're moving to.
    const a = dir < 0 ? columns[i - 2]?.position ?? null : columns[i + 1]?.position ?? null;
    const b = dir < 0 ? columns[i - 1]?.position ?? null : columns[i + 2]?.position ?? null;
    void actions.updateColumn(column, { position: keyBetween(a, b) }, `Moved column “${column.name}”`);
    onClose();
  };
  const trash = () => {
    onClose();
    void actions.trashColumn(column);
  };

  return (
    <Dialog open onClose={onClose} title={`${column.name} options`}>
      <div className="dialog-actions vertical">
        <button
          type="button"
          className="text-button"
          onClick={() => {
            onFocus();
            onClose();
          }}
        >
          Focus on this column
        </button>
        <button
          type="button"
          className="text-button"
          onClick={() => {
            onExpandAll(false);
            onClose();
          }}
        >
          Collapse all cards here
        </button>
        <button
          type="button"
          className="text-button"
          onClick={() => {
            onExpandAll(true);
            onClose();
          }}
        >
          Expand all cards here
        </button>
        <button type="button" className="text-button" disabled={i <= 0} onClick={() => move(-1)}>
          Move left
        </button>
        <button type="button" className="text-button" disabled={i >= columns.length - 1} onClick={() => move(1)}>
          Move right
        </button>
        <fieldset className="settings-group">
          <legend>Color</legend>
          <div className="swatches" role="radiogroup" aria-label="Column color">
            <button
              type="button"
              role="radio"
              aria-checked={!color}
              className={`swatch is-none${!color ? " is-selected" : ""}`}
              aria-label="No color"
              title="No color"
              onClick={() => {
                setColor("");
                void actions.updateColumn(column, { color: "" }, `Removed the color from “${column.name}”`);
              }}
            />
            {LABEL_COLORS.map((c) => (
              <button
                key={c}
                type="button"
                role="radio"
                aria-checked={color === c}
                aria-label={c}
                title={c}
                className={`swatch label-${c}${color === c ? " is-selected" : ""}`}
                onClick={() => {
                  setColor(c);
                  void actions.updateColumn(column, { color: c }, `Changed the color of “${column.name}”`);
                }}
              />
            ))}
          </div>
          {color && (
            <div className="row">
              {(
                [
                  ["header", "A stripe across the top"],
                  ["full", "A light tint over the column"],
                ] as const
              ).map(([mode, text]) => (
                <label key={mode} className="check">
                  <input
                    type="radio"
                    name={`color-mode-${column.id}`}
                    checked={colorMode === mode}
                    onChange={() => {
                      setColorMode(mode);
                      void actions.updateColumn(column, { colorMode: mode }, `Changed how “${column.name}” shows its color`);
                    }}
                  />
                  {text}
                </label>
              ))}
            </div>
          )}
        </fieldset>
        <label className="check">
          <input
            type="checkbox"
            checked={isDone}
            onChange={(e) => {
              const on = e.target.checked;
              setIsDone(on);
              void actions.updateColumn(column, { isDone: on }, on ? `“${column.name}” is now a done column` : `“${column.name}” is no longer a done column`);
            }}
          />
          Cards here count as done
        </label>
        {confirming ? (
          // Bulk action: the only kind that asks first (§3.6).
          <div className="confirm" role="alert">
            <p>
              Move “{column.name}” and its {cardCount} {cardCount === 1 ? "card" : "cards"} to the trash? You can restore
              them from the trash for 30 days.
            </p>
            <div className="row">
              <button type="button" onClick={trash}>
                Move to trash
              </button>
              <button type="button" className="text-button" onClick={() => setConfirming(false)}>
                Keep it
              </button>
            </div>
          </div>
        ) : (
          <button type="button" className="text-button" onClick={() => (cardCount > 0 ? setConfirming(true) : trash())}>
            <Icon.Trash /> Delete column
          </button>
        )}
      </div>
    </Dialog>
  );
}

function TrashDialog({
  boardId,
  actions,
  onClose,
}: {
  boardId: string;
  actions: ReturnType<typeof useBoardActions>;
  onClose: () => void;
}) {
  const trash = useQuery({ queryKey: ["board", boardId, "trash"], queryFn: () => api.boardTrash(boardId) });
  const restore = async (fn: () => Promise<void>) => {
    await fn();
    void trash.refetch();
  };
  const t = trash.data;
  const empty = t && t.columns.length === 0 && t.cards.length === 0;
  return (
    <Dialog open onClose={onClose} title="Trash">
      <p className="muted">Deleted items stay here for 30 days.</p>
      {trash.isPending && <p className="muted">Loading…</p>}
      {empty && <p className="empty">The trash is empty.</p>}
      <ul className="choice-list">
        {t?.columns.map((c) => (
          <li key={c.id} className="trash-item">
            <span>Column: {c.name}</span>
            <button type="button" className="text-button" onClick={() => void restore(() => actions.restoreColumn(c))}>
              Restore
            </button>
          </li>
        ))}
        {t?.cards.map((c) => (
          <li key={c.id} className="trash-item">
            <span>{c.title}</span>
            <button type="button" className="text-button" onClick={() => void restore(() => actions.restoreCard(c))}>
              Restore
            </button>
          </li>
        ))}
      </ul>
    </Dialog>
  );
}

/** Show only cards whose title contains the text, or that carry any of the chosen labels. */
function FilterBar({
  focus,
  labels,
  onChange,
  onClose,
}: {
  focus: Focus;
  labels: Label[];
  onChange: (f: Focus) => void;
  onClose: () => void;
}) {
  const current = focus.type === "filter" ? focus : { type: "filter" as const, query: "", labelIds: [] as string[] };
  const [text, setText] = useState(current.query ?? "");
  const chosen = current.labelIds ?? [];
  const update = (q: string, ls: string[]) =>
    onChange(q.trim() || ls.length ? { type: "filter", query: q, labelIds: ls } : { type: "none" });
  return (
    <div className="filter-bar" role="search">
      <input
        autoFocus
        aria-label="Filter cards"
        placeholder="Filter cards by title"
        value={text}
        maxLength={200}
        onChange={(e) => {
          setText(e.target.value);
          update(e.target.value, chosen);
        }}
        onKeyDown={(e) => e.key === "Escape" && onClose()}
      />
      {labels.length > 0 && (
        <div className="filter-labels" role="group" aria-label="Filter by label">
          {labels.map((l) => (
            <button
              key={l.id}
              type="button"
              className={`filter-label${chosen.includes(l.id) ? " is-on" : ""}`}
              aria-pressed={chosen.includes(l.id)}
              onClick={() => update(text, chosen.includes(l.id) ? chosen.filter((x) => x !== l.id) : [...chosen, l.id])}
            >
              <LabelChip label={l} />
            </button>
          ))}
        </div>
      )}
      <button type="button" className="text-button" onClick={onClose}>
        Clear filter
      </button>
    </div>
  );
}
