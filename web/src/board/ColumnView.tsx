// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

import { useRef, useState, type FormEvent } from "react";
import { useDroppable } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";

import type { Card, Column } from "../api";
import { Icon } from "../ui";
import { CardItem, TitleEditor, type CardActions } from "./CardItem";

export function ColumnView({
  column,
  cards,
  collapsed,
  readOnly,
  isExpanded,
  cardActions,
  onToggleCollapsed,
  onRename,
  onOptions,
  onAddCard,
  capturing,
  setCapturing,
}: {
  column: Column;
  cards: Card[];
  collapsed: boolean;
  readOnly: boolean;
  isExpanded: (id: string) => boolean;
  cardActions: CardActions;
  onToggleCollapsed: () => void;
  onRename: (name: string) => void;
  onOptions: () => void;
  onAddCard: (title: string) => void;
  capturing: boolean;
  setCapturing: (on: boolean) => void;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: `col:${column.id}`, data: { type: "column", column } });
  const [renaming, setRenaming] = useState(false);
  const count = cards.length;
  const overLimit = column.softLimit !== null && count > column.softLimit;
  const tint = column.color ? { "--column-tint": column.color } : undefined;

  if (collapsed) {
    // A narrow strip with the name and count (§3.4).
    return (
      <section className="column is-collapsed" aria-label={column.name} data-column-id={column.id} style={tint as never}>
        <button type="button" className="column-strip" aria-expanded={false} onClick={onToggleCollapsed}>
          <span className="column-strip-name">{column.name}</span>
          <span className="column-count">{count}</span>
        </button>
      </section>
    );
  }

  return (
    <section
      className={`column${column.color ? ` tint-${column.colorMode}` : ""}${isOver ? " is-over" : ""}`}
      aria-label={column.name}
      data-column-id={column.id}
      style={tint as never}
    >
      <header className="column-head">
        <button
          type="button"
          className="icon-button"
          aria-label={`Collapse ${column.name}`}
          aria-expanded={true}
          onClick={onToggleCollapsed}
        >
          <Icon.ChevronDown />
        </button>
        {renaming ? (
          <TitleEditor
            initial={column.name}
            onDone={(name) => {
              setRenaming(false);
              if (name !== null && name.trim() && name.trim() !== column.name) onRename(name);
            }}
          />
        ) : (
          <h2 className="column-name" onClick={() => !readOnly && setRenaming(true)}>
            {column.name}
          </h2>
        )}
        <span className={`column-count${overLimit ? " over-limit" : ""}`}>
          {column.softLimit !== null ? `${count}/${column.softLimit}` : count}
        </span>
        {!readOnly && (
          <button type="button" className="icon-button" aria-label={`${column.name} options`} onClick={onOptions}>
            <Icon.More />
          </button>
        )}
      </header>

      <SortableContext items={cards.map((c) => c.id)} strategy={verticalListSortingStrategy}>
        <div ref={setNodeRef} className="card-list">
          {cards.map((c) => (
            <CardItem key={c.id} card={c} expanded={isExpanded(c.id)} readOnly={readOnly} actions={cardActions} />
          ))}
          {cards.length === 0 && !capturing && <p className="column-empty">Nothing here yet.</p>}
        </div>
      </SortableContext>

      {!readOnly &&
        (capturing ? (
          <QuickCapture onAdd={onAddCard} onClose={() => setCapturing(false)} />
        ) : (
          <button type="button" className="add-card" onClick={() => setCapturing(true)}>
            <Icon.Plus /> Add a card
          </button>
        ))}
    </section>
  );
}

/**
 * Quick capture (§3.6): Enter creates the card and keeps the field open for
 * the next one; Esc (or leaving it empty) closes it.
 */
function QuickCapture({ onAdd, onClose }: { onAdd: (title: string) => void; onClose: () => void }) {
  const [value, setValue] = useState("");
  const ref = useRef<HTMLInputElement>(null);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!value.trim()) return;
    onAdd(value);
    setValue("");
    ref.current?.focus();
  };
  return (
    <form className="quick-capture" onSubmit={submit}>
      <input
        ref={ref}
        autoFocus
        aria-label="New card title"
        placeholder="Card title"
        value={value}
        maxLength={500}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") onClose();
        }}
        onBlur={() => {
          if (!value.trim()) onClose();
        }}
      />
    </form>
  );
}
