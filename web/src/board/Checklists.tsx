// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// A card's checklists (docs/DESIGN.md §3.1). Loaded when the card is
// expanded or opened; the board only knows each card's done/total counts.

import { useEffect, useRef, useState, type FormEvent, type KeyboardEventHandler, type SyntheticEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  DndContext,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import { api, ApiError, type BoardDetail, type Card, type Checklist, type ChecklistItem } from "../api";
import { Icon } from "../ui";
import { useUndo } from "../undo";
import { byPosition, detailKey, keyBetween } from "./useBoard";
import { TitleEditor } from "./CardItem";

const checklistsKey = (cardId: string) => ["card", cardId, "checklists"] as const;

/**
 * Keep a pointer event away from the card's own drag listener. Only for
 * mouse/touch starts: stopping a keydown would also hide later keys from
 * dnd-kit, which listens on window during a keyboard move.
 */
const contain = (e: SyntheticEvent) => e.stopPropagation();

function useChecklists(card: Card) {
  const qc = useQueryClient();
  const { push, notify } = useUndo();
  const key = checklistsKey(card.id);
  const query = useQuery({ queryKey: key, queryFn: () => api.checklists(card.id) });

  const set = (fn: (lists: Checklist[]) => Checklist[]) => {
    const next = qc.setQueryData<Checklist[]>(key, (lists) => (lists ? fn(lists) : lists));
    if (!next) return;
    // Keep the board's "2/5" in step.
    const items = next.flatMap((l) => l.items);
    qc.setQueryData<BoardDetail>(detailKey(card.boardId), (d) =>
      d
        ? {
            ...d,
            cards: d.cards.map((c) =>
              c.id === card.id
                ? { ...c, checklistTotal: items.length, checklistDone: items.filter((i) => i.doneAt).length }
                : c,
            ),
          }
        : d,
    );
  };
  const putItem = (it: ChecklistItem) =>
    set((lists) =>
      lists.map((l) => ({
        ...l,
        items: l.id === it.checklistId ? [...l.items.filter((x) => x.id !== it.id), it] : l.items.filter((x) => x.id !== it.id),
      })),
    );
  const dropItem = (id: string) => set((lists) => lists.map((l) => ({ ...l, items: l.items.filter((x) => x.id !== id) })));
  const putList = (cl: Checklist) => set((lists) => [...lists.filter((x) => x.id !== cl.id), { ...cl, items: lists.find((x) => x.id === cl.id)?.items ?? cl.items }]);

  async function run<T>(local: () => void, remote: () => Promise<T>, settle: (v: T) => void) {
    local();
    try {
      const v = await remote();
      settle(v);
      return v;
    } catch (err) {
      void qc.invalidateQueries({ queryKey: key });
      void qc.invalidateQueries({ queryKey: detailKey(card.boardId) });
      notify(err instanceof ApiError && err.status === 400 ? err.message : "That didn't save. Check your connection and try again.");
      return undefined;
    }
  }

  return {
    query,
    addChecklist: () => run(() => {}, () => api.createChecklist(card.id, ""), putList),
    renameChecklist: (cl: Checklist, title: string) =>
      run(() => putList({ ...cl, title }), () => api.updateChecklist(cl.id, { title }), putList),
    trashChecklist: async (cl: Checklist) => {
      const done = await run(() => set((lists) => lists.filter((x) => x.id !== cl.id)), () => api.trashChecklist(cl.id), () => {});
      if (done) {
        push(`Deleted checklist “${cl.title}”`, async () => {
          await api.restoreChecklist(cl.id);
          await qc.invalidateQueries({ queryKey: key });
          void qc.invalidateQueries({ queryKey: detailKey(card.boardId) });
        });
      }
    },
    addItem: (cl: Checklist, text: string) => {
      const position = keyBetween(cl.items.length ? [...cl.items].sort(byPosition).at(-1)!.position : null, null);
      const temp: ChecklistItem = { id: `tmp-${crypto.randomUUID()}`, checklistId: cl.id, text: text.trim(), doneAt: null, position, version: 0 };
      return run(
        () => putItem(temp),
        () => api.createItem(cl.id, text, position),
        (it) => {
          dropItem(temp.id);
          putItem(it);
        },
      );
    },
    // Ticking a box is undone by ticking it again, so it doesn't add a toast.
    toggleItem: (it: ChecklistItem) =>
      run(() => putItem({ ...it, doneAt: it.doneAt ? null : new Date().toISOString() }), () => api.updateItem(it.id, { done: !it.doneAt }), putItem),
    renameItem: (it: ChecklistItem, text: string) =>
      run(() => putItem({ ...it, text }), () => api.updateItem(it.id, { text }), putItem),
    moveItem: (it: ChecklistItem, position: string) =>
      run(() => putItem({ ...it, position }), () => api.updateItem(it.id, { position }), putItem),
    trashItem: async (it: ChecklistItem) => {
      const done = await run(() => dropItem(it.id), () => api.trashItem(it.id), () => {});
      if (done) push(`Deleted “${it.text}”`, async () => putItem(await api.restoreItem(it.id)));
    },
  };
}

export function Checklists({ card, readOnly }: { card: Card; readOnly: boolean }) {
  const c = useChecklists(card);
  // Only a checklist made just now opens its "add an item" field; expanding a
  // card should never move focus on its own.
  const [justMade, setJustMade] = useState<string | null>(null);
  const lists = c.query.data ? [...c.query.data].sort(byPosition) : [];
  if (c.query.isPending) return null;
  return (
    <div className="checklists">
      {lists.map((cl) => (
        <ChecklistView key={cl.id} checklist={cl} readOnly={readOnly} actions={c} startAdding={cl.id === justMade} />
      ))}
      {!readOnly && (
        <button
          type="button"
          className="text-button add-checklist"
          onClick={async () => {
            const cl = await c.addChecklist();
            if (cl) setJustMade(cl.id);
          }}
        >
          <Icon.Plus /> {lists.length ? "Add another checklist" : "Add a checklist"}
        </button>
      )}
    </div>
  );
}

function ChecklistView({
  checklist,
  readOnly,
  actions,
  startAdding,
}: {
  checklist: Checklist;
  readOnly: boolean;
  actions: ReturnType<typeof useChecklists>;
  startAdding: boolean;
}) {
  const [renaming, setRenaming] = useState(false);
  const [adding, setAdding] = useState(startAdding && !readOnly);
  const items = [...checklist.items].sort(byPosition);
  const done = items.filter((i) => i.doneAt).length;

  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 4 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return;
    const ids = items.map((i) => i.id);
    const next = arrayMove(ids, ids.indexOf(String(active.id)), ids.indexOf(String(over.id)));
    const i = next.indexOf(String(active.id));
    const byId = new Map(items.map((it) => [it.id, it]));
    const before = i > 0 ? byId.get(next[i - 1])!.position : null;
    const after = i < next.length - 1 ? byId.get(next[i + 1])!.position : null;
    void actions.moveItem(byId.get(String(active.id))!, keyBetween(before, after));
  };

  return (
    <section className="checklist" aria-label={checklist.title}>
      <header className="checklist-head">
        <Icon.Check />
        {renaming ? (
          <TitleEditor
            initial={checklist.title}
            onDone={(t) => {
              setRenaming(false);
              if (t !== null && t.trim() && t.trim() !== checklist.title) void actions.renameChecklist(checklist, t);
            }}
          />
        ) : (
          <h3 className="checklist-title" onClick={() => !readOnly && setRenaming(true)}>
            {checklist.title}
          </h3>
        )}
        <span className="muted checklist-progress" aria-label={`${done} of ${items.length} done`}>
          {done}/{items.length}
        </span>
        {!readOnly && (
          <button
            type="button"
            className="icon-button"
            aria-label={`Delete checklist ${checklist.title}`}
            onClick={() => void actions.trashChecklist(checklist)}
          >
            <Icon.Trash />
          </button>
        )}
      </header>
      {items.length > 0 && (
        <div className="checklist-bar" aria-hidden="true">
          <span style={{ width: `${(done / items.length) * 100}%` }} />
        </div>
      )}
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
        <SortableContext items={items.map((i) => i.id)} strategy={verticalListSortingStrategy}>
          <ul className="checklist-items">
            {items.map((it) => (
              <ItemRow key={it.id} item={it} readOnly={readOnly} actions={actions} />
            ))}
          </ul>
        </SortableContext>
      </DndContext>
      {!readOnly &&
        (adding ? (
          <AddItem onAdd={(text) => void actions.addItem(checklist, text)} onClose={() => setAdding(false)} />
        ) : (
          <button type="button" className="text-button add-item" onClick={() => setAdding(true)}>
            <Icon.Plus /> Add an item
          </button>
        ))}
    </section>
  );
}

function ItemRow({ item, readOnly, actions }: { item: ChecklistItem; readOnly: boolean; actions: ReturnType<typeof useChecklists> }) {
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({
    id: item.id,
    disabled: readOnly || item.id.startsWith("tmp-"),
    // Short (§3.6), and short enough that quick arrow-key moves don't arrive
    // mid-animation and get measured against stale positions.
    transition: { duration: 120, easing: "ease-out" },
  });
  const [editing, setEditing] = useState(false);
  // Local copy so the box responds on the click itself (the cache updates a tick later).
  const [checked, setChecked] = useState(!!item.doneAt);
  useEffect(() => setChecked(!!item.doneAt), [item.doneAt]);
  const handle = listeners ?? {};

  return (
    <li
      ref={setNodeRef}
      className={`checklist-item${item.doneAt ? " is-done" : ""}${isDragging ? " is-dragging" : ""}`}
      style={{ transform: CSS.Translate.toString(transform), transition }}
    >
      {!readOnly && (
        <button
          ref={setActivatorNodeRef}
          type="button"
          className="icon-button drag-handle"
          {...attributes}
          aria-label={`Reorder ${item.text}`}
          aria-roledescription="sortable item"
          onMouseDown={(e) => {
            contain(e);
            handle.onMouseDown?.(e);
          }}
          onTouchStart={(e) => {
            contain(e);
            handle.onTouchStart?.(e);
          }}
          onKeyDown={handle.onKeyDown as KeyboardEventHandler<HTMLButtonElement> | undefined}
        >
          <span aria-hidden="true">⋮⋮</span>
        </button>
      )}
      <input
        type="checkbox"
        checked={checked}
        disabled={readOnly}
        aria-label={item.text}
        onChange={() => {
          setChecked(!checked);
          void actions.toggleItem(item);
        }}
      />
      {editing ? (
        <span>
          <TitleEditor
            initial={item.text}
            onDone={(t) => {
              setEditing(false);
              if (t !== null && t.trim() && t.trim() !== item.text) void actions.renameItem(item, t);
            }}
          />
        </span>
      ) : (
        <span className="checklist-text" onClick={() => !readOnly && setEditing(true)}>
          {item.text}
        </span>
      )}
      {!readOnly && (
        <button type="button" className="icon-button" aria-label={`Delete ${item.text}`} onClick={() => void actions.trashItem(item)}>
          <Icon.Close />
        </button>
      )}
    </li>
  );
}

/** Quick add: Enter adds the item and keeps the field open; Esc closes. */
function AddItem({ onAdd, onClose }: { onAdd: (text: string) => void; onClose: () => void }) {
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
    <form className="add-item-form" onSubmit={submit}>
      <input
        ref={ref}
        autoFocus
        aria-label="New checklist item"
        placeholder="Add an item"
        value={value}
        maxLength={1000}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => e.key === "Escape" && onClose()}
        onBlur={() => !value.trim() && onClose()}
      />
    </form>
  );
}
