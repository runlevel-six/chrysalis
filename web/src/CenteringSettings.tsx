// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Settings for the centering line (docs/DESIGN.md §3.13): a section on the
// Settings page for how it behaves, and its own page for the collections
// and entries that feed it.

import { useEffect, useRef, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { api, ApiError, type CenteringSettings, type QuoteCollection, type QuoteEntry, type QuoteEntryFields } from "./api";
import { centeringKeys, entrySource } from "./centering";
import { byPosition, keyBetween } from "./board/useBoard";
import { Link, navigate } from "./router";
import { Icon } from "./ui";
import { useUndo } from "./undo";

const DEFAULTS: CenteringSettings = { enabled: false, mode: "rotate", interval: "daily", order: "sequential", collectionIds: [] };

const message = (err: unknown) =>
  err instanceof ApiError && err.status < 500 ? err.message : "That didn't save. Check your connection and try again.";

function useCollections() {
  return useQuery({ queryKey: centeringKeys.collections, queryFn: api.quoteCollections });
}

/** The section on the Settings page: on or off, how it changes, what feeds it. */
export function CenteringSection() {
  const qc = useQueryClient();
  const { notify } = useUndo();
  const saved = useQuery({ queryKey: centeringKeys.settings, queryFn: api.centeringSettings }).data;
  const collections = useCollections().data ?? [];
  // Local copy so controls respond on the click itself; the server answers a
  // moment later.
  const [s, setS] = useState<CenteringSettings>(saved ?? DEFAULTS);
  useEffect(() => {
    if (saved) setS(saved);
  }, [saved]);

  const set = async (patch: Partial<CenteringSettings>) => {
    const before = s;
    setS({ ...s, ...patch });
    try {
      const next = await api.updateCenteringSettings(patch);
      qc.setQueryData(centeringKeys.settings, next);
      setS(next);
      // What the line shows may have changed (pinned, the pool, on/off).
      void qc.invalidateQueries({ queryKey: centeringKeys.line });
    } catch (err) {
      setS(before);
      notify(message(err));
    }
  };
  const radio = <K extends "mode" | "interval" | "order">(key: K, legend: string, options: [CenteringSettings[K], string][]) => (
    <fieldset className="settings-group">
      <legend>{legend}</legend>
      {options.map(([v, text]) => (
        <label key={v} className="check">
          <input type="radio" name={`centering-${key}`} checked={s[key] === v} onChange={() => void set({ [key]: v })} />
          {text}
        </label>
      ))}
    </fieldset>
  );

  return (
    <div className="settings-section">
      <h2>Centering line</h2>
      <p className="muted">One quiet line of Scripture or a quote at the top of your boards, from collections you keep.</p>
      <label className="check">
        <input type="checkbox" checked={s.enabled} onChange={(e) => void set({ enabled: e.target.checked })} />
        Show the centering line on my boards
      </label>
      {s.enabled && (
        <>
          {radio("mode", "Which entry", [
            ["rotate", "Change it from time to time"],
            ["pinned", "Keep the one showing now (pinned)"],
          ])}
          {s.mode === "rotate" && (
            <>
              {radio("interval", "How often it changes", [
                ["page_load", "Each time I open the app"],
                ["hourly", "Every hour"],
                ["daily", "Every day, at midnight"],
                ["weekly", "Every week, on Monday"],
              ])}
              {radio("order", "In what order", [
                ["sequential", "In order"],
                ["shuffle", "Shuffled (no repeats until every entry has shown)"],
              ])}
            </>
          )}
          <fieldset className="settings-group">
            <legend>Entries come from</legend>
            {collections.length === 0 && <p className="muted">No collections yet. Add one below.</p>}
            {collections.map((c) => (
              <label key={c.id} className="check">
                <input
                  type="checkbox"
                  checked={s.collectionIds.includes(c.id)}
                  onChange={(e) =>
                    void set({
                      collectionIds: e.target.checked ? [...s.collectionIds, c.id] : s.collectionIds.filter((id) => id !== c.id),
                    })
                  }
                />
                {c.name} <span className="muted">({c.entryCount === 1 ? "1 entry" : `${c.entryCount} entries`})</span>
              </label>
            ))}
          </fieldset>
        </>
      )}
      <p>
        <Link to="/settings/centering">Collections and entries</Link>
      </p>
    </div>
  );
}

/** /settings/centering: collections, their entries, import, export and the trash. */
export function CenteringPage() {
  const qc = useQueryClient();
  const { push, notify } = useUndo();
  const collections = useCollections();
  const list = [...(collections.data ?? [])].sort(byPosition);
  const [openId, setOpenId] = useState<string | null>(null);
  const [newName, setNewName] = useState("");

  useEffect(() => {
    document.title = "Centering line · Chrysalis";
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !e.defaultPrevented && !(e.target instanceof HTMLElement && e.target.closest("dialog, input, textarea"))) {
        navigate("/settings");
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const refresh = () => void qc.invalidateQueries({ queryKey: ["centering"] });
  const attempt = async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
    try {
      return await fn();
    } catch (err) {
      notify(message(err));
      return undefined;
    } finally {
      refresh();
    }
  };

  const create = async (e: FormEvent) => {
    e.preventDefault();
    const name = newName.trim();
    if (!name) return;
    const c = await attempt(() => api.createQuoteCollection(name));
    if (c) {
      setNewName("");
      setOpenId(c.id);
    }
  };
  const move = (c: QuoteCollection, dir: -1 | 1) => {
    const i = list.findIndex((x) => x.id === c.id);
    const j = i + dir;
    if (j < 0 || j >= list.length) return;
    const position = dir < 0 ? keyBetween(list[j - 1]?.position ?? null, list[j].position) : keyBetween(list[j].position, list[j + 1]?.position ?? null);
    void attempt(() => api.updateQuoteCollection(c.id, { position }));
  };
  const trash = async (c: QuoteCollection) => {
    if (await attempt(() => api.trashQuoteCollection(c.id))) push(`Deleted “${c.name}”`, () => api.restoreQuoteCollection(c.id).finally(refresh));
  };

  return (
    <section className="page">
      <Link to="/settings" className="back-link">
        <span aria-hidden="true">←</span> Back to settings
      </Link>
      <h1>Centering line</h1>
      <p className="muted">
        Collections of short entries: Scripture, quotes, words of your own. Choose which ones feed your line in{" "}
        <Link to="/settings">Settings</Link>.
      </p>

      <div className="settings-section">
        <h2>Collections</h2>
        {collections.isSuccess && list.length === 0 && <p className="muted">No collections yet. Start one, or import a file.</p>}
        <ul className="collection-list">
          {list.map((c, i) => (
            <li key={c.id} className="collection">
              <div className="collection-head">
                <button
                  type="button"
                  className="text-button collection-toggle"
                  aria-expanded={openId === c.id}
                  onClick={() => setOpenId(openId === c.id ? null : c.id)}
                >
                  {openId === c.id ? <Icon.ChevronDown /> : <Icon.ChevronRight />}
                  {c.name}
                  <span className="muted">{c.entryCount === 1 ? "1 entry" : `${c.entryCount} entries`}</span>
                </button>
                {c.role === "owner" && (
                  <span className="collection-actions">
                    <button type="button" className="icon-button" aria-label={`Move ${c.name} up`} disabled={i === 0} onClick={() => move(c, -1)}>
                      <Icon.ChevronUp />
                    </button>
                    <button
                      type="button"
                      className="icon-button"
                      aria-label={`Move ${c.name} down`}
                      disabled={i === list.length - 1}
                      onClick={() => move(c, 1)}
                    >
                      <Icon.ChevronDown />
                    </button>
                    <button type="button" className="icon-button" aria-label={`Delete ${c.name}`} onClick={() => void trash(c)}>
                      <Icon.Trash />
                    </button>
                  </span>
                )}
              </div>
              {openId === c.id && <CollectionEditor collection={c} attempt={attempt} />}
            </li>
          ))}
        </ul>
        <form className="row new-collection" onSubmit={(e) => void create(e)}>
          <input aria-label="New collection name" placeholder="New collection name" value={newName} maxLength={100} onChange={(e) => setNewName(e.target.value)} />
          <button type="submit" disabled={!newName.trim()}>
            Add collection
          </button>
        </form>
      </div>

      <ImportSection collections={list} onDone={(c) => setOpenId(c.id)} />
      <CenteringTrashSection attempt={attempt} />
    </section>
  );
}

type Attempt = <T>(fn: () => Promise<T>) => Promise<T | undefined>;

const tagsText = (tags: string[]) => tags.join(", ");
const parseTags = (s: string) =>
  s
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);

function CollectionEditor({ collection, attempt }: { collection: QuoteCollection; attempt: Attempt }) {
  const { push } = useUndo();
  const qc = useQueryClient();
  const entries = useQuery({ queryKey: centeringKeys.entries(collection.id), queryFn: () => api.quoteEntries(collection.id) });
  const list = [...(entries.data ?? [])].sort(byPosition);
  const canEdit = collection.role !== "viewer";
  const [name, setName] = useState(collection.name);
  useEffect(() => setName(collection.name), [collection.name]);
  const [editing, setEditing] = useState<string | null>(null);

  const move = (e: QuoteEntry, dir: -1 | 1) => {
    const i = list.findIndex((x) => x.id === e.id);
    const j = i + dir;
    if (j < 0 || j >= list.length) return;
    const position = dir < 0 ? keyBetween(list[j - 1]?.position ?? null, list[j].position) : keyBetween(list[j].position, list[j + 1]?.position ?? null);
    // Show the new order straight away; the server agrees a moment later.
    qc.setQueryData<QuoteEntry[]>(centeringKeys.entries(collection.id), (old) => old?.map((x) => (x.id === e.id ? { ...x, position } : x)));
    void attempt(() => api.updateQuoteEntry(e.id, { position }));
  };
  const trash = async (e: QuoteEntry) => {
    if (await attempt(() => api.trashQuoteEntry(e.id)))
      push("Deleted an entry", () => api.restoreQuoteEntry(e.id).finally(() => void qc.invalidateQueries({ queryKey: ["centering"] })));
  };

  return (
    <div className="collection-body">
      {collection.role === "owner" && (
        <form
          className="row"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim() && name.trim() !== collection.name) void attempt(() => api.updateQuoteCollection(collection.id, { name }));
          }}
        >
          <input aria-label="Collection name" value={name} maxLength={100} onChange={(e) => setName(e.target.value)} />
          <button type="submit" disabled={!name.trim() || name.trim() === collection.name}>
            Rename
          </button>
        </form>
      )}
      <p className="collection-export">
        Download: <a href={api.exportQuotesUrl(collection.id, "yaml")}>YAML</a> · <a href={api.exportQuotesUrl(collection.id, "json")}>JSON</a>
      </p>
      {entries.isSuccess && list.length === 0 && <p className="muted">No entries yet.</p>}
      <ol className="entry-list" aria-label={`Entries in ${collection.name}`}>
        {list.map((e, i) =>
          editing === e.id ? (
            <li key={e.id} className="entry">
              <EntryForm
                initial={e}
                submitLabel="Save"
                onCancel={() => setEditing(null)}
                onSubmit={async (fields) => {
                  if (await attempt(() => api.updateQuoteEntry(e.id, fields))) {
                    setEditing(null);
                    const before: Partial<QuoteEntryFields> = { text: e.text, attribution: e.attribution, reference: e.reference, translation: e.translation, tags: e.tags };
                    push("Edited an entry", () => api.updateQuoteEntry(e.id, before).finally(() => void qc.invalidateQueries({ queryKey: ["centering"] })));
                    return true;
                  }
                  return false;
                }}
              />
            </li>
          ) : (
            <li key={e.id} className="entry">
              <div className="entry-text">
                <span className="entry-quote">{e.text}</span>
                {entrySource(e) && <span className="muted"> — {entrySource(e)}</span>}
                {e.tags.length > 0 && <span className="entry-tags muted"> {tagsText(e.tags)}</span>}
              </div>
              {canEdit && (
                <span className="entry-actions">
                  <button type="button" className="icon-button" aria-label="Move up" disabled={i === 0} onClick={() => move(e, -1)}>
                    <Icon.ChevronUp />
                  </button>
                  <button type="button" className="icon-button" aria-label="Move down" disabled={i === list.length - 1} onClick={() => move(e, 1)}>
                    <Icon.ChevronDown />
                  </button>
                  <button type="button" className="text-button" onClick={() => setEditing(e.id)}>
                    Edit
                  </button>
                  <button type="button" className="icon-button" aria-label="Delete entry" onClick={() => void trash(e)}>
                    <Icon.Trash />
                  </button>
                </span>
              )}
            </li>
          ),
        )}
      </ol>
      {canEdit && (
        <details className="add-entry">
          <summary>Add an entry</summary>
          <EntryForm
            submitLabel="Add entry"
            onSubmit={async (fields) => !!(await attempt(() => api.createQuoteEntry(collection.id, fields)))}
          />
        </details>
      )}
    </div>
  );
}

function EntryForm({
  initial,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  initial?: QuoteEntryFields;
  submitLabel: string;
  /** Resolves true when saved, so the form can clear. */
  onSubmit: (fields: QuoteEntryFields) => Promise<boolean>;
  onCancel?: () => void;
}) {
  const [text, setText] = useState(initial?.text ?? "");
  const [attribution, setAttribution] = useState(initial?.attribution ?? "");
  const [reference, setReference] = useState(initial?.reference ?? "");
  const [translation, setTranslation] = useState(initial?.translation ?? "");
  const [tags, setTags] = useState(tagsText(initial?.tags ?? []));
  const textRef = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    if (initial) textRef.current?.focus();
  }, [initial]);

  return (
    <form
      className="entry-form"
      onSubmit={async (e) => {
        e.preventDefault();
        if (!text.trim()) return;
        const ok = await onSubmit({ text, attribution, reference, translation, tags: parseTags(tags) });
        if (ok && !initial) {
          setText("");
          setAttribution("");
          setReference("");
          setTranslation("");
          setTags("");
          textRef.current?.focus();
        }
      }}
    >
      <label className="field">
        Text
        <textarea ref={textRef} rows={3} maxLength={2000} value={text} onChange={(e) => setText(e.target.value)} />
      </label>
      <div className="entry-form-row">
        <label className="field">
          Said or written by <span className="muted">(optional)</span>
          <input value={attribution} maxLength={200} onChange={(e) => setAttribution(e.target.value)} />
        </label>
        <label className="field">
          Reference <span className="muted">(optional)</span>
          <input value={reference} maxLength={200} onChange={(e) => setReference(e.target.value)} />
        </label>
        <label className="field">
          Translation <span className="muted">(optional)</span>
          <input value={translation} maxLength={50} onChange={(e) => setTranslation(e.target.value)} />
        </label>
      </div>
      <label className="field">
        Tags <span className="muted">(optional, separated by commas)</span>
        <input value={tags} onChange={(e) => setTags(e.target.value)} />
      </label>
      <div className="row">
        <button type="submit" disabled={!text.trim()}>
          {submitLabel}
        </button>
        {onCancel && (
          <button type="button" className="text-button" onClick={onCancel}>
            Cancel
          </button>
        )}
      </div>
    </form>
  );
}

function ImportSection({ collections, onDone }: { collections: QuoteCollection[]; onDone: (c: QuoteCollection) => void }) {
  const qc = useQueryClient();
  const [target, setTarget] = useState("");
  const [replace, setReplace] = useState(false);
  const [status, setStatus] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const file = useRef<HTMLInputElement>(null);
  const editable = collections.filter((c) => c.role !== "viewer");

  const run = async (f: File) => {
    setBusy(true);
    setStatus(null);
    try {
      const r = await api.importQuotes(f, { collectionId: target || undefined, replace: !!target && replace });
      const n = r.added === 1 ? "1 entry" : `${r.added} entries`;
      setStatus({
        ok: true,
        text: r.created
          ? `Added ${n} to a new collection, “${r.collection.name}”.`
          : `Added ${n} to “${r.collection.name}”${r.replaced ? `; the ${r.replaced} it had are in the trash` : ""}.`,
      });
      onDone(r.collection);
    } catch (err) {
      setStatus({ ok: false, text: err instanceof ApiError ? err.message : "That file didn't import. Check your connection and try again." });
    } finally {
      setBusy(false);
      if (file.current) file.current.value = "";
      void qc.invalidateQueries({ queryKey: ["centering"] });
    }
  };

  return (
    <div className="settings-section">
      <h2>Import</h2>
      <p className="muted">
        A YAML or JSON file with a collection name and a list of entries. Imports add to what's there unless you choose to replace it.
      </p>
      <label className="field">
        Into
        <select value={target} onChange={(e) => setTarget(e.target.value)}>
          <option value="">The collection named in the file (new if needed)</option>
          {editable.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
      </label>
      {target && (
        <label className="check">
          <input type="checkbox" checked={replace} onChange={(e) => setReplace(e.target.checked)} />
          Replace its entries (the old ones go to the trash)
        </label>
      )}
      <label className="field">
        File to import
        <input
          ref={file}
          type="file"
          accept=".yaml,.yml,.json,application/json,application/yaml,text/yaml"
          disabled={busy}
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) void run(f);
          }}
        />
      </label>
      {status && (
        <p role="status" className="note">
          {status.text}
        </p>
      )}
    </div>
  );
}

function CenteringTrashSection({ attempt }: { attempt: Attempt }) {
  const trash = useQuery({ queryKey: centeringKeys.trash, queryFn: api.centeringTrash });
  const t = trash.data;
  if (!t || t.collections.length + t.entries.length === 0) return null;
  return (
    <div className="settings-section">
      <h2>Trash</h2>
      <p className="muted">Deleted collections and entries stay here for a while before they're gone for good.</p>
      <ul className="choice-list">
        {t.collections.map((c) => (
          <li key={c.id} className="trash-item">
            <span>
              {c.name} <span className="muted">(collection)</span>
            </span>
            <button type="button" className="text-button" onClick={() => void attempt(() => api.restoreQuoteCollection(c.id))}>
              Restore
            </button>
          </li>
        ))}
        {t.entries.map((e) => (
          <li key={e.id} className="trash-item">
            <span className="entry-quote">{e.text}</span>
            <button type="button" className="text-button" onClick={() => void attempt(() => api.restoreQuoteEntry(e.id))}>
              Restore
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
