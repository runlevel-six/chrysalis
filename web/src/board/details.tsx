// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Card details: labels, dates, archived cards and board settings.

import { useEffect, useRef, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { api, ApiError, attachmentUrl, LABEL_COLORS, type Board, type BoardSettings, type Card, type Label, type LabelColor } from "../api";
import { BACKGROUND_COLORS, GRADIENTS } from "./backgrounds";
import { attachmentsKey } from "./Attachments";
import { Dialog, Icon } from "../ui";
import { useUndo } from "../undo";
import { detailKey, type useBoardActions } from "./useBoard";

type Actions = ReturnType<typeof useBoardActions>;

export const labelName = (l: Label) => l.name || `${l.color[0].toUpperCase()}${l.color.slice(1)} label`;

export function LabelChip({ label }: { label: Label }) {
  return <span className={`label-chip label-${label.color}`}>{label.name || " "}</span>;
}

/** Colored dots for collapsed cards. The names are there for screen readers. */
export function LabelDots({ labels }: { labels: Label[] }) {
  if (!labels.length) return null;
  return (
    <span className="label-dots" aria-label={`Labels: ${labels.map(labelName).join(", ")}`} role="img">
      {labels.map((l) => (
        <span key={l.id} className={`label-dot label-${l.color}`} />
      ))}
    </span>
  );
}

const sameYear = (d: Date) => d.getFullYear() === new Date().getFullYear();

/** "May 31", "May 31, 2027" or "May 31, 5:00 PM", in the viewer's own time zone. */
export function formatDate(value: string, hasTime: boolean): string {
  if (hasTime) {
    const d = new Date(value);
    return d.toLocaleString(undefined, {
      month: "short", day: "numeric", year: sameYear(d) ? undefined : "numeric", hour: "numeric", minute: "2-digit",
    });
  }
  // Date-only values are calendar days, not instants: build a local date so
  // they never shift by a day across time zones.
  const [y, m, d] = value.split("-").map(Number);
  const date = new Date(y, m - 1, d);
  return date.toLocaleDateString(undefined, { month: "short", day: "numeric", year: sameYear(date) ? undefined : "numeric" });
}

/**
 * Past due and not done. A date-only due date is due all day, so it's
 * overdue from the next local day.
 */
export function isOverdue(card: Card, now = new Date()): boolean {
  if (!card.dueAt || card.completedAt) return false;
  if (card.dueHasTime) return new Date(card.dueAt) < now;
  const [y, m, d] = card.dueAt.split("-").map(Number);
  return new Date(y, m - 1, d + 1) <= now;
}

/** The due date. With highlightOverdue (off unless the person turns it on, §3.6), a quiet dot marks a past-due card. */
export function DueText({ card, highlightOverdue = false }: { card: Card; highlightOverdue?: boolean }) {
  if (!card.dueAt) return null;
  const overdue = highlightOverdue && isOverdue(card);
  return (
    <span className={`due-text${overdue ? " is-overdue" : ""}`}>
      <Icon.Calendar width={14} height={14} /> {formatDate(card.dueAt, card.dueHasTime)}
      {overdue && <span className="visually-hidden"> (past due)</span>}
    </span>
  );
}

/** Everything about a card's dates and labels, for the expanded and full views. */
export function CardMeta({ card, labels }: { card: Card; labels: Label[] }) {
  const mine = labels.filter((l) => card.labelIds.includes(l.id));
  if (!mine.length && !card.startAt && !card.dueAt) return null;
  return (
    <div className="card-meta">
      {mine.length > 0 && (
        <ul className="label-list" aria-label="Labels">
          {mine.map((l) => (
            <li key={l.id}>
              <LabelChip label={l} />
            </li>
          ))}
        </ul>
      )}
      {(card.startAt || card.dueAt) && (
        <p className="date-line">
          <Icon.Calendar width={14} height={14} />
          {card.startAt && <span>Starts {formatDate(card.startAt, false)}</span>}
          {card.startAt && card.dueAt && <span aria-hidden="true"> · </span>}
          {card.dueAt && <span>Due {formatDate(card.dueAt, card.dueHasTime)}</span>}
        </p>
      )}
    </div>
  );
}

function ColorPicker({ value, onChange }: { value: LabelColor; onChange: (c: LabelColor) => void }) {
  return (
    <div className="swatches" role="radiogroup" aria-label="Color">
      {LABEL_COLORS.map((c) => (
        <button
          key={c}
          type="button"
          role="radio"
          aria-checked={value === c}
          aria-label={c}
          className={`swatch label-${c}${value === c ? " is-selected" : ""}`}
          onClick={() => onChange(c)}
        />
      ))}
    </div>
  );
}

/** Pick a card's labels, or make a new one. */
export function LabelsDialog({ card, labels, actions, onClose }: { card: Card; labels: Label[]; actions: Actions; onClose: () => void }) {
  const [selected, setSelected] = useState<string[]>(card.labelIds);
  const [name, setName] = useState("");
  const [color, setColor] = useState<LabelColor>("green");

  const toggle = (id: string) => {
    const next = selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id];
    setSelected(next);
    void actions.updateCard(card, { labelIds: next }, `Changed labels on “${card.title}”`);
  };
  const create = async (e: FormEvent) => {
    e.preventDefault();
    const l = await actions.createLabel(name, color);
    if (l) {
      setName("");
      toggle(l.id);
    }
  };

  return (
    <Dialog open onClose={onClose} title="Labels">
      {labels.length === 0 && <p className="muted">This board has no labels yet. Make one below.</p>}
      <ul className="choice-list">
        {labels.map((l) => (
          <li key={l.id}>
            <label className="check">
              <input type="checkbox" checked={selected.includes(l.id)} onChange={() => toggle(l.id)} />
              <LabelChip label={l} />
              {!l.name && <span className="muted">{labelName(l)}</span>}
            </label>
          </li>
        ))}
      </ul>
      <form className="new-label" onSubmit={create}>
        <p className="field-label">New label</p>
        <input aria-label="Label name" placeholder="Name (optional)" value={name} maxLength={50} onChange={(e) => setName(e.target.value)} />
        <ColorPicker value={color} onChange={setColor} />
        <button type="submit">Add label</button>
      </form>
    </Dialog>
  );
}

const pad = (n: number) => String(n).padStart(2, "0");
const localDate = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const localTime = (d: Date) => `${pad(d.getHours())}:${pad(d.getMinutes())}`;

/** Start and due dates. Native date and time inputs work well on phones and with screen readers. */
export function DatesDialog({ card, actions, onClose }: { card: Card; actions: Actions; onClose: () => void }) {
  const due = card.dueAt && card.dueHasTime ? new Date(card.dueAt) : null;
  const [start, setStart] = useState(card.startAt ?? "");
  const [dueDate, setDueDate] = useState(due ? localDate(due) : (card.dueAt ?? ""));
  const [withTime, setWithTime] = useState(card.dueHasTime);
  const [time, setTime] = useState(due ? localTime(due) : "17:00");
  const [error, setError] = useState("");

  const save = (e: FormEvent) => {
    e.preventDefault();
    if (start && dueDate && start > dueDate) {
      setError("The start date is after the due date.");
      return;
    }
    let dueAt: string | null = dueDate || null;
    if (dueDate && withTime) {
      const [y, m, d] = dueDate.split("-").map(Number);
      const [hh, mm] = time.split(":").map(Number);
      dueAt = new Date(y, m - 1, d, hh, mm).toISOString(); // the viewer's local time, sent as UTC
    }
    void actions.updateCard(
      card,
      { startAt: start || null, dueAt, dueHasTime: !!dueAt && withTime },
      `Changed dates on “${card.title}”`,
    );
    onClose();
  };

  return (
    <Dialog open onClose={onClose} title="Dates">
      <form className="dates-form" onSubmit={save}>
        <label className="field">
          Start date
          <input type="date" value={start} onChange={(e) => setStart(e.target.value)} />
        </label>
        <label className="field">
          Due date
          <input type="date" value={dueDate} onChange={(e) => setDueDate(e.target.value)} />
        </label>
        <label className="check">
          <input type="checkbox" checked={withTime} disabled={!dueDate} onChange={(e) => setWithTime(e.target.checked)} />
          Add a time
        </label>
        {withTime && dueDate && (
          <label className="field">
            Due time
            <input type="time" value={time} onChange={(e) => setTime(e.target.value)} />
          </label>
        )}
        {error && <p className="note" role="alert">{error}</p>}
        <div className="row">
          <button type="submit">Save</button>
          <button
            type="button"
            className="text-button"
            onClick={() => {
              setStart("");
              setDueDate("");
              setWithTime(false);
            }}
          >
            Clear dates
          </button>
        </div>
      </form>
    </Dialog>
  );
}

export function ArchivedDialog({ boardId, onClose }: { boardId: string; onClose: () => void }) {
  const qc = useQueryClient();
  const { push } = useUndo();
  const archived = useQuery({ queryKey: ["board", boardId, "archived"], queryFn: () => api.archivedCards(boardId) });

  const unarchive = async (card: Card) => {
    await api.updateCard(card.id, { archived: false });
    void archived.refetch();
    void qc.invalidateQueries({ queryKey: detailKey(boardId) });
    push(`Put “${card.title}” back on the board`, async () => {
      await api.updateCard(card.id, { archived: true });
      void qc.invalidateQueries({ queryKey: detailKey(boardId) });
    });
  };

  return (
    <Dialog open onClose={onClose} title="Archived cards">
      <p className="muted">Archived cards are tucked away, not deleted.</p>
      {archived.isPending && <p className="muted">Loading…</p>}
      {archived.data?.length === 0 && <p className="empty">Nothing archived.</p>}
      <ul className="choice-list">
        {archived.data?.map((c) => (
          <li key={c.id} className="trash-item">
            <span>{c.title}</span>
            <button type="button" className="text-button" onClick={() => void unarchive(c)}>
              Put back
            </button>
          </li>
        ))}
      </ul>
    </Dialog>
  );
}

/** Board name, what collapsed cards show, and the board's labels. */
export function BoardSettingsDialog({
  boardId,
  name,
  settings,
  background,
  labels,
  actions,
  onClose,
}: {
  boardId: string;
  name: string;
  settings: BoardSettings;
  background: Board["background"];
  labels: Label[];
  actions: Actions;
  onClose: () => void;
}) {
  const [boardName, setBoardName] = useState(name);
  useEffect(() => setBoardName(name), [name]);
  // Local copy so controls respond on the same frame they're clicked; the
  // query cache (and server) catch up a moment later.
  const [local, setLocal] = useState(settings);
  useEffect(() => setLocal(settings), [settings]);
  const set = (p: Partial<BoardSettings>) => {
    const next = { ...local, ...p };
    setLocal(next);
    void actions.updateBoard({ settings: next }, "Changed board settings");
  };
  const toggle = (key: keyof BoardSettings, text: string, invert = false) => (
    <label className="check">
      <input
        type="checkbox"
        checked={invert ? !local[key] : (local[key] as boolean)}
        onChange={(e) => set({ [key]: invert ? !e.target.checked : e.target.checked })}
      />
      {text}
    </label>
  );

  return (
    <Dialog open onClose={onClose} title="Board settings">
      <form
        className="field"
        onSubmit={(e) => {
          e.preventDefault();
          if (boardName.trim() && boardName.trim() !== name) void actions.updateBoard({ name: boardName }, `Renamed the board`);
        }}
      >
        <label className="field">
          Board name
          <div className="row">
            <input value={boardName} maxLength={200} onChange={(e) => setBoardName(e.target.value)} />
            <button type="submit" disabled={!boardName.trim() || boardName.trim() === name}>
              Rename
            </button>
          </div>
        </label>
      </form>

      <fieldset className="settings-group">
        <legend>New and untouched cards open</legend>
        <label className="check">
          <input
            type="radio"
            name="default-card-state"
            checked={local.defaultCardState === "collapsed"}
            onChange={() => set({ defaultCardState: "collapsed" })}
          />
          Collapsed (title only)
        </label>
        <label className="check">
          <input
            type="radio"
            name="default-card-state"
            checked={local.defaultCardState === "expanded"}
            onChange={() => set({ defaultCardState: "expanded" })}
          />
          Expanded
        </label>
      </fieldset>

      <fieldset className="settings-group">
        <legend>Collapsed cards also show</legend>
        {toggle("showLabels", "Label colors")}
        {toggle("showDueDate", "Due date")}
        {toggle("showChecklist", "Checklist progress")}
      </fieldset>

      <fieldset className="settings-group">
        <legend>Collapsed columns</legend>
        {toggle("hideColumnCount", "Show the number of cards", true)}
      </fieldset>

      <BackgroundPicker boardId={boardId} background={background} actions={actions} />

      <fieldset className="settings-group">
        <legend>Labels</legend>
        {labels.length === 0 && <p className="muted">No labels yet. Add them from a card's Labels button.</p>}
        <ul className="choice-list">
          {labels.map((l) => (
            <LabelEditor key={l.id} label={l} actions={actions} />
          ))}
        </ul>
      </fieldset>
    </Dialog>
  );
}

function LabelEditor({ label, actions }: { label: Label; actions: Actions }) {
  const [name, setName] = useState(label.name);
  return (
    <li className="label-editor">
      <LabelChip label={label} />
      <input
        aria-label={`Name for ${labelName(label)}`}
        value={name}
        maxLength={50}
        onChange={(e) => setName(e.target.value)}
        onBlur={() => name.trim() !== label.name && void actions.updateLabel(label, { name })}
        onKeyDown={(e) => e.key === "Enter" && (e.target as HTMLInputElement).blur()}
      />
      <ColorPicker value={label.color} onChange={(color) => void actions.updateLabel(label, { color })} />
      <button type="button" className="icon-button" aria-label={`Delete ${labelName(label)}`} onClick={() => void actions.trashLabel(label)}>
        <Icon.Trash />
      </button>
    </li>
  );
}

/**
 * A card's cover, as a band across its top. Decorative: no text, no alt.
 * Cards on the board use the light thumbnail; the full view (large) uses the
 * original so it stays sharp.
 */
export function CoverBand({ card, large = false }: { card: Card; large?: boolean }) {
  if (card.cover.type === "color") return <div className={`card-cover label-${card.cover.value}`} aria-hidden="true" />;
  if (card.cover.type === "image") {
    return (
      <div className="card-cover is-image" aria-hidden="true">
        <img src={attachmentUrl(card.cover.value, !large)} alt="" loading="lazy" />
      </div>
    );
  }
  return null;
}

/** Pick a cover: a palette color, one of the card's images, or none. */
export function CoverDialog({ card, actions, onClose }: { card: Card; actions: Actions; onClose: () => void }) {
  const images = useQuery({
    queryKey: attachmentsKey(card.id),
    queryFn: () => api.attachments(card.id),
    select: (l) => l.filter((a) => a.kind === "image"),
  });
  const set = (cover: Card["cover"]) => {
    void actions.updateCard(card, { cover }, `Changed the cover on “${card.title}”`);
    onClose();
  };
  const current = card.cover;
  return (
    <Dialog open onClose={onClose} title="Cover">
      <p className="field-label">Color</p>
      <div className="swatches" role="radiogroup" aria-label="Cover color">
        {LABEL_COLORS.map((c) => (
          <button
            key={c}
            type="button"
            role="radio"
            aria-checked={current.type === "color" && current.value === c}
            aria-label={c}
            className={`swatch label-${c}${current.type === "color" && current.value === c ? " is-selected" : ""}`}
            onClick={() => set({ type: "color", value: c })}
          />
        ))}
      </div>
      <p className="field-label">Image</p>
      {images.data?.length === 0 && <p className="muted">Attach an image to the card to use it as a cover.</p>}
      <ul className="cover-images">
        {images.data?.map((a) => (
          <li key={a.id}>
            <button
              type="button"
              className={`cover-choice${current.type === "image" && current.value === a.id ? " is-selected" : ""}`}
              aria-pressed={current.type === "image" && current.value === a.id}
              aria-label={`Use ${a.filename}`}
              onClick={() => set({ type: "image", value: a.id })}
            >
              <img src={attachmentUrl(a.id, true)} alt="" />
            </button>
          </li>
        ))}
      </ul>
      {current.type !== "none" && (
        <div className="dialog-actions">
          <button type="button" className="text-button" onClick={() => set({ type: "none" })}>
            Remove cover
          </button>
        </div>
      )}
    </Dialog>
  );
}

/** Board background: theme default, a color, a gradient preset, or a photo (§3.5). */
function BackgroundPicker({ boardId, background, actions }: { boardId: string; background: Board["background"]; actions: Actions }) {
  const { notify } = useUndo();
  const file = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const set = (bg: Board["background"]) => void actions.updateBoard({ background: bg }, "Changed the background");
  const is = (type: string, value: string) => background.type === type && background.value === value;

  const upload = async (f: File | undefined) => {
    if (!f) return;
    setUploading(true);
    try {
      const a = await api.uploadBackground(boardId, f);
      set({ type: "image", value: a.id });
    } catch (err) {
      notify(err instanceof ApiError ? err.message : "That photo didn't upload. Check your connection and try again.");
    } finally {
      setUploading(false);
    }
  };

  return (
    <fieldset className="settings-group">
      <legend>Background</legend>
      <div className="bg-choices" role="radiogroup" aria-label="Background">
        <button
          type="button"
          role="radio"
          aria-checked={!background.value}
          className="bg-choice is-default"
          onClick={() => set({ type: "color", value: "" })}
        >
          Theme default
        </button>
        {BACKGROUND_COLORS.map((c) => (
          <button
            key={c.value}
            type="button"
            role="radio"
            aria-checked={is("color", c.value)}
            aria-label={c.name}
            title={c.name}
            className="bg-choice"
            style={{ background: c.value }}
            onClick={() => set({ type: "color", value: c.value })}
          />
        ))}
        {GRADIENTS.map((g) => (
          <button
            key={g.id}
            type="button"
            role="radio"
            aria-checked={is("gradient", g.id)}
            aria-label={`${g.name} gradient`}
            title={g.name}
            className="bg-choice"
            style={{ background: g.css }}
            onClick={() => set({ type: "gradient", value: g.id })}
          />
        ))}
      </div>
      <div className="row bg-more">
        <label className="check">
          <span>Any color</span>
          <input
            type="color"
            aria-label="Pick any background color"
            value={background.type === "color" && background.value ? background.value : "#f6efe0"}
            onChange={(e) => set({ type: "color", value: e.target.value })}
          />
        </label>
        <input ref={file} type="file" accept="image/*" hidden onChange={(e) => void upload(e.target.files?.[0])} />
        <button type="button" className="text-button" disabled={uploading} onClick={() => file.current?.click()}>
          <Icon.Image /> {uploading ? "Uploading…" : background.type === "image" ? "Use a different photo" : "Use a photo"}
        </button>
      </div>
    </fieldset>
  );
}
