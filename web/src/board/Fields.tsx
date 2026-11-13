// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Custom fields (docs/DESIGN.md §4, Phase 2): defined per board in Board
// settings, filled in on expanded cards, and optionally shown on collapsed
// cards as small chips (off by default, like everything else there, §3.2).

import { useEffect, useState, type FormEvent } from "react";

import { LABEL_COLORS, type Card, type CustomField, type FieldOption, type FieldType } from "../api";
import { Icon } from "../ui";
import { keyBetween } from "./useBoard";

type Value = string | number | boolean;

const TYPE_NAMES: Record<FieldType, string> = {
  text: "Text",
  number: "Number",
  date: "Date",
  select: "Choice",
  checkbox: "Checkbox",
};

const shortDate = (s: string) => {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, m - 1, d).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
};

/** A value as text, or "" if it has none. */
export function formatValue(field: CustomField, value: Value | undefined): string {
  if (value === undefined || value === null || value === "") return "";
  switch (field.type) {
    case "number":
      return typeof value === "number" ? value.toLocaleString() : String(value);
    case "date":
      return shortDate(String(value));
    case "select":
      return field.options.find((o) => o.id === value)?.name ?? "";
    case "checkbox":
      return value === true ? "Yes" : "";
    default:
      return String(value);
  }
}

/** Fields shown on collapsed cards, if any have a value. */
export const chipFields = (card: Card, fields: CustomField[]) =>
  fields.filter((f) => f.showOnCollapsed && formatValue(f, card.fields?.[f.id]) !== "");

export function FieldChips({ card, fields }: { card: Card; fields: CustomField[] }) {
  return (
    <>
      {chipFields(card, fields).map((f) => {
        const v = card.fields[f.id];
        const opt = f.type === "select" ? f.options.find((o) => o.id === v) : undefined;
        return (
          <span key={f.id} className={`field-chip${opt?.color ? ` label-${opt.color} has-color` : ""}`} title={f.name}>
            {f.type === "checkbox" ? (
              <>
                <Icon.Check width={12} height={12} /> {f.name}
              </>
            ) : (
              <>
                <span className="visually-hidden">{f.name}: </span>
                {formatValue(f, v)}
              </>
            )}
          </span>
        );
      })}
    </>
  );
}

/** The fields on an expanded card or in full view. */
export function CardFields({
  card,
  fields,
  readOnly,
  onSet,
}: {
  card: Card;
  fields: CustomField[];
  readOnly: boolean;
  onSet: (field: CustomField, value: Value | null) => void;
}) {
  if (fields.length === 0) return null;
  if (readOnly) {
    const shown = fields.filter((f) => formatValue(f, card.fields[f.id]) !== "");
    if (!shown.length) return null;
    return (
      <dl className="card-fields is-readonly">
        {shown.map((f) => (
          <div key={f.id}>
            <dt>{f.name}</dt>
            <dd>{formatValue(f, card.fields[f.id])}</dd>
          </div>
        ))}
      </dl>
    );
  }
  return (
    <div className="card-fields" role="group" aria-label="Fields">
      {fields.map((f) => (
        <FieldEditor key={f.id} card={card} field={f} onSet={onSet} />
      ))}
    </div>
  );
}

function FieldEditor({ card, field, onSet }: { card: Card; field: CustomField; onSet: (f: CustomField, v: Value | null) => void }) {
  const value = card.fields[field.id];
  const id = `field-${card.id}-${field.id}`;
  const [draft, setDraft] = useState(value === undefined ? "" : String(value));
  useEffect(() => setDraft(value === undefined ? "" : String(value)), [value]);

  const commit = () => {
    if (draft === (value === undefined ? "" : String(value))) return;
    if (field.type === "number") {
      if (draft.trim() === "") return onSet(field, null);
      const n = Number(draft);
      if (!Number.isFinite(n)) return setDraft(value === undefined ? "" : String(value));
      return onSet(field, n);
    }
    onSet(field, draft.trim() === "" ? null : draft);
  };

  if (field.type === "checkbox") {
    return (
      <label className="check card-field">
        <input type="checkbox" checked={value === true} onChange={(e) => onSet(field, e.target.checked)} />
        {field.name}
      </label>
    );
  }
  return (
    <div className="card-field">
      <label htmlFor={id}>{field.name}</label>
      {field.type === "select" ? (
        <select id={id} value={typeof value === "string" ? value : ""} onChange={(e) => onSet(field, e.target.value || null)}>
          <option value="">—</option>
          {field.options.map((o) => (
            <option key={o.id} value={o.id}>
              {o.name}
            </option>
          ))}
        </select>
      ) : (
        <input
          id={id}
          type={field.type === "number" ? "number" : field.type === "date" ? "date" : "text"}
          inputMode={field.type === "number" ? "decimal" : undefined}
          maxLength={field.type === "text" ? 500 : undefined}
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value);
            if (field.type === "date") onSet(field, e.target.value || null);
          }}
          onBlur={field.type === "date" ? undefined : commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") (e.target as HTMLInputElement).blur();
            if (e.key === "Escape") setDraft(value === undefined ? "" : String(value));
          }}
        />
      )}
    </div>
  );
}

// ---------- Board settings ----------

type FieldActions = {
  createField: (name: string, type: FieldType, options?: FieldOption[]) => Promise<CustomField | undefined>;
  updateField: (field: CustomField, p: { name?: string; options?: FieldOption[]; showOnCollapsed?: boolean; position?: string }) => Promise<void>;
  trashField: (field: CustomField) => Promise<void>;
};

export function FieldsSettings({ fields, actions }: { fields: CustomField[]; actions: FieldActions }) {
  const [name, setName] = useState("");
  const [type, setType] = useState<FieldType>("text");
  const [editing, setEditing] = useState<string | null>(null);
  const sorted = [...fields].sort((a, b) => (a.position < b.position ? -1 : a.position > b.position ? 1 : 0));

  const add = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    const n = name.trim();
    setName(""); // at once, so typing the next one isn't wiped when this one saves
    const f = await actions.createField(n, type, type === "select" ? [{ id: "", name: "First choice", color: "" }] : []);
    if (f && type === "select") setEditing(f.id); // a choice field needs its choices
  };
  const move = (i: number, dir: -1 | 1) => {
    const j = i + dir;
    const [a, b] = dir < 0 ? [sorted[j - 1]?.position ?? null, sorted[j].position] : [sorted[j].position, sorted[j + 1]?.position ?? null];
    void actions.updateField(sorted[i], { position: keyBetween(a, b) });
  };

  return (
    <fieldset className="settings-group">
      <legend>Fields</legend>
      <p className="muted">Extra details for every card on this board, such as a cost, a room or a yes/no.</p>
      {sorted.length > 0 && (
        <ul className="field-list">
          {sorted.map((f, i) => (
            <li key={f.id}>
              <div className="field-row">
                <span className="field-name">
                  {f.name} <span className="muted">· {TYPE_NAMES[f.type]}</span>
                </span>
                <button
                  type="button"
                  className="text-button"
                  aria-expanded={editing === f.id}
                  aria-label={`Edit the field ${f.name}`}
                  onClick={() => setEditing(editing === f.id ? null : f.id)}
                >
                  Edit
                </button>
              </div>
              {editing === f.id && (
                <FieldEditorPanel
                  field={f}
                  canUp={i > 0}
                  canDown={i < sorted.length - 1}
                  onMove={(dir) => move(i, dir)}
                  actions={actions}
                  onDone={() => setEditing(null)}
                />
              )}
            </li>
          ))}
        </ul>
      )}
      <form className="row field-add" onSubmit={add}>
        <label className="visually-hidden" htmlFor="new-field-name">
          New field name
        </label>
        <input id="new-field-name" placeholder="New field" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} />
        <label className="visually-hidden" htmlFor="new-field-type">
          Kind of field
        </label>
        <select id="new-field-type" value={type} onChange={(e) => setType(e.target.value as FieldType)}>
          {(Object.keys(TYPE_NAMES) as FieldType[]).map((t) => (
            <option key={t} value={t}>
              {TYPE_NAMES[t]}
            </option>
          ))}
        </select>
        <button type="submit" disabled={!name.trim()}>
          Add field
        </button>
      </form>
    </fieldset>
  );
}

function FieldEditorPanel({
  field,
  canUp,
  canDown,
  onMove,
  actions,
  onDone,
}: {
  field: CustomField;
  canUp: boolean;
  canDown: boolean;
  onMove: (dir: -1 | 1) => void;
  actions: FieldActions;
  onDone: () => void;
}) {
  const [name, setName] = useState(field.name);
  const [options, setOptions] = useState<FieldOption[]>(field.options);
  const [show, setShow] = useState(field.showOnCollapsed);
  const changedOptions = JSON.stringify(options) !== JSON.stringify(field.options);

  const save = (e: FormEvent) => {
    e.preventDefault();
    const p: { name?: string; options?: FieldOption[] } = {};
    if (name.trim() && name.trim() !== field.name) p.name = name.trim();
    if (field.type === "select" && changedOptions) p.options = options.filter((o) => o.name.trim());
    if (Object.keys(p).length) void actions.updateField(field, p);
    onDone();
  };

  return (
    <form className="field-panel" onSubmit={save}>
      <label className="field">
        Name
        <input value={name} maxLength={60} onChange={(e) => setName(e.target.value)} />
      </label>
      {field.type === "select" && (
        <fieldset className="field-options">
          <legend>Choices</legend>
          {options.map((o, i) => (
            <div key={o.id || i} className="row field-option">
              <input
                aria-label={`Choice ${i + 1}`}
                value={o.name}
                maxLength={60}
                onChange={(e) => setOptions(options.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))}
              />
              <select
                aria-label={`Color of choice ${i + 1}`}
                value={o.color}
                onChange={(e) => setOptions(options.map((x, j) => (j === i ? { ...x, color: e.target.value as FieldOption["color"] } : x)))}
              >
                <option value="">No color</option>
                {LABEL_COLORS.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
              <button
                type="button"
                className="icon-button"
                aria-label={`Remove choice ${o.name || i + 1}`}
                onClick={() => setOptions(options.filter((_, j) => j !== i))}
              >
                <Icon.Close />
              </button>
            </div>
          ))}
          <button type="button" className="text-button" onClick={() => setOptions([...options, { id: "", name: "", color: "" }])}>
            <Icon.Plus /> Add a choice
          </button>
          {changedOptions && options.length < field.options.length && (
            <p className="muted">Cards using a removed choice will have it cleared.</p>
          )}
        </fieldset>
      )}
      <label className="check">
        <input
          type="checkbox"
          checked={show}
          onChange={(e) => {
            setShow(e.target.checked);
            void actions.updateField(field, { showOnCollapsed: e.target.checked });
          }}
        />
        Show on collapsed cards
      </label>
      <div className="row">
        <button type="submit">Save</button>
        {canUp && (
          <button type="button" className="text-button" onClick={() => onMove(-1)}>
            Move up
          </button>
        )}
        {canDown && (
          <button type="button" className="text-button" onClick={() => onMove(1)}>
            Move down
          </button>
        )}
        <button
          type="button"
          className="text-button"
          onClick={() => {
            void actions.trashField(field);
            onDone();
          }}
        >
          Delete field
        </button>
      </div>
    </form>
  );
}
