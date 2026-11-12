// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// One card, in its collapsed or expanded-inline state (docs/DESIGN.md §3.2).
//
// The card itself is one tab stop. When focused: Space toggles expanded,
// Enter opens the full view, E edits the title, M picks it up to move with
// the arrow keys (M, Space or Enter drops; Esc cancels). Every shortcut also
// has a visible button, and touch uses long-press to drag.

import { useEffect, useRef, useState, type KeyboardEvent, type MouseEventHandler, type TouchEventHandler } from "react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import type { Card } from "../api";
import { Icon } from "../ui";

export type CardActions = {
  toggle: (card: Card) => void;
  openFull: (card: Card) => void;
  rename: (card: Card, title: string) => void;
  setCompleted: (card: Card, done: boolean) => void;
  moveTo: (card: Card) => void;
  trash: (card: Card) => void;
};

export function CardItem({
  card,
  expanded,
  readOnly,
  actions,
}: {
  card: Card;
  expanded: boolean;
  readOnly: boolean;
  actions: CardActions;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: card.id,
    data: { type: "card", card },
    disabled: readOnly || card.id.startsWith("tmp-"),
  });
  const [editing, setEditing] = useState(false);
  const done = card.completedAt !== null;

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.target !== e.currentTarget || isDragging) {
      listeners?.onKeyDown?.(e);
      return;
    }
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
      onMouseDown={listeners?.onMouseDown as MouseEventHandler<HTMLDivElement> | undefined}
      onTouchStart={listeners?.onTouchStart as TouchEventHandler<HTMLDivElement> | undefined}
      onKeyDown={onKeyDown}
      onDoubleClick={(e) => {
        if ((e.target as HTMLElement).closest("button, input, textarea")) return;
        actions.openFull(card);
      }}
    >
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
      {expanded && !readOnly && (
        <div className="card-actions">
          <button type="button" className="text-button" onClick={() => actions.setCompleted(card, !done)}>
            {done ? "Mark not done" : "Mark done"}
          </button>
          <button type="button" className="text-button" onClick={() => actions.moveTo(card)}>
            Move to…
          </button>
          <button type="button" className="text-button" onClick={() => actions.trash(card)}>
            Delete
          </button>
        </div>
      )}
    </div>
  );
}

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
