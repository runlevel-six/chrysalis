// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// One card, in its collapsed or expanded-inline state (docs/DESIGN.md §3.2).
//
// The card itself is one tab stop. When focused: Space toggles expanded,
// Enter opens the full view, E edits the title, M picks it up to move with
// the arrow keys (M, Space or Enter drops; Esc cancels). Every shortcut also
// has a visible button, and touch uses long-press to drag.

import { lazy, Suspense, useEffect, useRef, useState, type KeyboardEvent, type MouseEventHandler, type TouchEventHandler } from "react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import type { BoardSettings, Card, Label } from "../api";
import { Icon, Menu } from "../ui";
import { CardMeta, CoverBand, DueText, LabelDots } from "./details";
import { Attachments } from "./Attachments";
import { Checklists } from "./Checklists";

// The editor is heavy; load it only when a description is first shown.
export const LazyDescription = lazy(() => import("./Description"));

export type CardActions = {
  toggle: (card: Card) => void;
  openFull: (card: Card) => void;
  rename: (card: Card, title: string) => void;
  setCompleted: (card: Card, done: boolean) => void;
  moveTo: (card: Card) => void;
  labels: (card: Card) => void;
  dates: (card: Card) => void;
  cover: (card: Card) => void;
  setCover: (card: Card, cover: Card["cover"]) => void;
  archive: (card: Card) => void;
  trash: (card: Card) => void;
};

/** What the board shares with every card: its labels and display settings. */
export type CardContext = { labels: Label[]; settings: BoardSettings; highlightOverdue: boolean };

export function CardItem({
  card,
  expanded,
  readOnly,
  actions,
  ctx,
}: {
  card: Card;
  expanded: boolean;
  readOnly: boolean;
  actions: CardActions;
  ctx: CardContext;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: card.id,
    data: { type: "card", card },
    disabled: readOnly || card.id.startsWith("tmp-"),
  });
  const [editing, setEditing] = useState(false);
  const done = card.completedAt !== null;

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    // Shortcuts apply only when the card itself has focus. Keys from controls
    // inside it (checklists, buttons, fields) are theirs. Once a keyboard
    // move is underway, dnd-kit handles keys at the window level.
    if (e.target !== e.currentTarget || isDragging) return;
    if (e.key === " ") {
      e.preventDefault();
      actions.toggle(card);
    } else if (e.key === "Enter") {
      e.preventDefault();
      actions.openFull(card);
    } else if ((e.key === "e" || e.key === "E") && !readOnly && !e.metaKey && !e.ctrlKey) {
      e.preventDefault();
      if (!expanded) actions.toggle(card);
      setEditing(true);
    } else {
      listeners?.onKeyDown?.(e); // M starts a keyboard move
    }
  };

  return (
    <div
      ref={setNodeRef}
      {...attributes}
      // dnd-kit's attributes make this a role="button"; it's really a group
      // of controls with keyboard shortcuts.
      role="group"
      aria-roledescription="card"
      aria-label={card.title}
      aria-describedby="card-shortcuts"
      tabIndex={0}
      data-card-id={card.id}
      className={`card${expanded ? " is-expanded" : ""}${done ? " is-done" : ""}${isDragging ? " is-dragging" : ""}`}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      // Mouse and touch can start a drag anywhere on the card; the keyboard
      // sensor runs from onKeyDown below, only on M.
      onMouseDown={(e) => !fromField(e.target) && (listeners?.onMouseDown as MouseEventHandler<HTMLDivElement> | undefined)?.(e)}
      onTouchStart={(e) => !fromField(e.target) && (listeners?.onTouchStart as TouchEventHandler<HTMLDivElement> | undefined)?.(e)}
      onKeyDown={onKeyDown}
      onDoubleClick={(e) => {
        if ((e.target as HTMLElement).closest("button, input, textarea, [contenteditable]")) return;
        actions.openFull(card);
      }}
    >
      <CoverBand card={card} />
      <div className="card-row">
        <button
          type="button"
          tabIndex={-1}
          className="icon-button card-chevron"
          aria-label={expanded ? "Collapse card" : "Expand card"}
          aria-expanded={expanded}
          onClick={() => actions.toggle(card)}
        >
          {expanded ? <Icon.ChevronDown /> : <Icon.ChevronRight />}
        </button>
        {editing ? (
          <TitleEditor
            initial={card.title}
            onDone={(title) => {
              setEditing(false);
              if (title !== null && title.trim() && title.trim() !== card.title) actions.rename(card, title);
            }}
          />
        ) : (
          <span
            className="card-title"
            onClick={() => {
              // Click the title of an expanded card to edit it; otherwise toggle.
              if (expanded && !readOnly) setEditing(true);
              else actions.toggle(card);
            }}
          >
            {done && <Icon.Check className="card-done-mark" aria-label="Done" />}
            {card.title}
          </span>
        )}
        <button
          type="button"
          tabIndex={-1}
          className="icon-button card-open"
          aria-label="Open card"
          onClick={() => actions.openFull(card)}
        >
          <Icon.Expand />
        </button>
      </div>
      {!expanded && hasExtras(card, ctx) && (
        // Optional extras on collapsed cards; all off by default (§3.2).
        <div className="card-extras">
          {ctx.settings.showLabels && <LabelDots labels={ctx.labels.filter((l) => card.labelIds.includes(l.id))} />}
          {ctx.settings.showDueDate && <DueText card={card} highlightOverdue={ctx.highlightOverdue} />}
          {ctx.settings.showChecklist && card.checklistTotal > 0 && (
            <span className="checklist-count" aria-label={`Checklist: ${card.checklistDone} of ${card.checklistTotal} done`}>
              <Icon.Check width={14} height={14} /> {card.checklistDone}/{card.checklistTotal}
            </span>
          )}
        </div>
      )}
      {expanded && (
        <div className="card-expanded">
          {/* Actions sit right under the title so they stay in reach however long
              the description and checklists get. */}
          {!readOnly && (
            <div className="card-actions">
              <button type="button" className="text-button" onClick={() => actions.setCompleted(card, !done)}>
                {done ? "Mark not done" : "Mark done"}
              </button>
              <button type="button" className="text-button" onClick={() => actions.labels(card)}>
                Labels…
              </button>
              <button type="button" className="text-button" onClick={() => actions.dates(card)}>
                Dates…
              </button>
              {/* Less frequent actions stay out of the way (§3.6). */}
              <Menu
                label="More"
                iconOnly
                icon={<Icon.More />}
                items={[
                  { label: "Cover…", onSelect: () => actions.cover(card) },
                  { label: "Move to…", onSelect: () => actions.moveTo(card) },
                  { label: "Archive", onSelect: () => actions.archive(card) },
                  { label: "Delete", onSelect: () => actions.trash(card) },
                ]}
              />
            </div>
          )}
          <CardMeta card={card} labels={ctx.labels} />
          {(card.hasDescription || !readOnly) && (
            <Suspense fallback={<p className="muted description-loading">Loading description…</p>}>
              <LazyDescription card={card} readOnly={readOnly} compact />
            </Suspense>
          )}
          <Checklists card={card} readOnly={readOnly} />
          <Attachments card={card} readOnly={readOnly} onMakeCover={(a) => actions.setCover(card, { type: "image", value: a.id })} />
        </div>
      )}
    </div>
  );
}

const hasExtras = (card: Card, ctx: CardContext) =>
  (ctx.settings.showLabels && card.labelIds.length > 0) ||
  (ctx.settings.showDueDate && !!card.dueAt) ||
  (ctx.settings.showChecklist && card.checklistTotal > 0);

/** Text fields inside a card shouldn't start a card drag (selecting text would move the card). */
const fromField = (t: EventTarget) => t instanceof Element && !!t.closest("input, textarea, select, [contenteditable]");

/** Inline title editor: Enter saves, Esc cancels, blur saves. */
export function TitleEditor({ initial, onDone }: { initial: string; onDone: (value: string | null) => void }) {
  const [value, setValue] = useState(initial);
  const ref = useRef<HTMLInputElement>(null);
  const finished = useRef(false);
  useEffect(() => {
    ref.current?.focus();
    ref.current?.select();
  }, []);
  const finish = (v: string | null) => {
    if (finished.current) return;
    finished.current = true;
    onDone(v);
  };
  return (
    <input
      ref={ref}
      className="title-input"
      aria-label="Title"
      value={value}
      maxLength={500}
      onChange={(e) => setValue(e.target.value)}
      onKeyDown={(e) => {
        e.stopPropagation(); // keep Space/Enter/E from reaching the card
        if (e.key === "Enter") finish(value);
        if (e.key === "Escape") finish(null);
      }}
      onBlur={() => finish(value)}
    />
  );
}
