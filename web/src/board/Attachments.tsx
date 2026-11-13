// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// A card's attachments (docs/DESIGN.md §3.1, §7.3): images as thumbnails,
// files and links as rows. Files go up through the server's cleaning step
// (metadata and location data removed) before anyone can see them.

import { useRef, useState, type DragEvent, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { api, ApiError, attachmentUrl, type Attachment, type Card } from "../api";
import { Icon } from "../ui";
import { useUndo } from "../undo";

export const attachmentsKey = (cardId: string) => ["card", cardId, "attachments"] as const;

export function formatSize(bytes = 0): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Upload files to a card, keeping its attachment list in step. Returns what was stored. */
export function useUploader(card: Card) {
  const qc = useQueryClient();
  const { notify } = useUndo();
  const [pending, setPending] = useState<string[]>([]);

  const upload = async (files: File[]): Promise<Attachment[]> => {
    const done: Attachment[] = [];
    for (const f of files) {
      setPending((p) => [...p, f.name]);
      try {
        const a = await api.uploadAttachment(card.id, f);
        done.push(a);
        qc.setQueryData<Attachment[]>(attachmentsKey(card.id), (list) => [...(list ?? []), a]);
      } catch (err) {
        notify(
          err instanceof ApiError && (err.status === 400 || err.status === 413)
            ? err.message
            : `“${f.name}” didn't upload. Check your connection and try again.`,
        );
      } finally {
        // Drop one "Uploading…" line for this name (names can repeat).
        setPending((p) => {
          const i = p.indexOf(f.name);
          return i < 0 ? p : [...p.slice(0, i), ...p.slice(i + 1)];
        });
      }
    }
    return done;
  };
  return { upload, pending };
}

export function Attachments({
  card,
  readOnly,
  onMakeCover,
}: {
  card: Card;
  readOnly: boolean;
  onMakeCover: (a: Attachment) => void;
}) {
  const qc = useQueryClient();
  const { push, notify } = useUndo();
  const query = useQuery({ queryKey: attachmentsKey(card.id), queryFn: () => api.attachments(card.id) });
  const { upload, pending } = useUploader(card);
  const fileInput = useRef<HTMLInputElement>(null);
  const [linking, setLinking] = useState(false);
  const [dragOver, setDragOver] = useState(false);

  const list = query.data ?? [];
  const images = list.filter((a) => a.kind === "image");
  const others = list.filter((a) => a.kind !== "image");

  const remove = async (a: Attachment) => {
    qc.setQueryData<Attachment[]>(attachmentsKey(card.id), (l) => (l ?? []).filter((x) => x.id !== a.id));
    try {
      await api.trashAttachment(a.id);
      push(`Removed “${a.filename}”`, async () => {
        const back = await api.restoreAttachment(a.id);
        qc.setQueryData<Attachment[]>(attachmentsKey(card.id), (l) => [...(l ?? []), back]);
      });
    } catch {
      void qc.invalidateQueries({ queryKey: attachmentsKey(card.id) });
      notify("That didn't save. Check your connection and try again.");
    }
  };

  const onDrop = (e: DragEvent) => {
    if (readOnly || !e.dataTransfer.files.length) return;
    e.preventDefault();
    e.stopPropagation();
    setDragOver(false);
    void upload([...e.dataTransfer.files]);
  };

  if (query.isPending) return null;
  if (readOnly && list.length === 0) return null;

  return (
    <section
      className={`attachments${dragOver ? " is-drop-target" : ""}`}
      aria-label="Attachments"
      onDragOver={(e) => {
        if (readOnly || !e.dataTransfer.types.includes("Files")) return;
        e.preventDefault();
        setDragOver(true);
      }}
      onDragLeave={() => setDragOver(false)}
      onDrop={onDrop}
    >
      {images.length > 0 && (
        <ul className="attachment-images">
          {images.map((a) => (
            <li key={a.id} className="attachment-image">
              <a href={attachmentUrl(a.id)} target="_blank" rel="noopener" aria-label={`Open ${a.filename}`}>
                <img src={attachmentUrl(a.id, true)} alt={a.filename} loading="lazy" width={a.width} height={a.height} />
              </a>
              {!readOnly && (
                <div className="attachment-image-actions">
                  <button
                    type="button"
                    className="text-button"
                    disabled={card.cover.type === "image" && card.cover.value === a.id}
                    onClick={() => onMakeCover(a)}
                  >
                    {card.cover.type === "image" && card.cover.value === a.id ? "Cover" : "Make cover"}
                  </button>
                  <button type="button" className="icon-button" aria-label={`Remove ${a.filename}`} onClick={() => void remove(a)}>
                    <Icon.Close />
                  </button>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      {others.length > 0 && (
        <ul className="attachment-rows">
          {others.map((a) => (
            <li key={a.id} className="attachment-row">
              {a.kind === "link" ? (
                <a href={a.url} target="_blank" rel="noopener noreferrer nofollow" className="attachment-name">
                  <Icon.Link width={16} height={16} /> {a.filename}
                </a>
              ) : (
                <a href={attachmentUrl(a.id)} download={a.filename} className="attachment-name">
                  {a.filename}
                </a>
              )}
              {a.kind === "file" && <span className="muted attachment-size">{formatSize(a.sizeBytes)}</span>}
              {!readOnly && (
                <button type="button" className="icon-button" aria-label={`Remove ${a.filename}`} onClick={() => void remove(a)}>
                  <Icon.Close />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {pending.map((name, i) => (
        <p key={`${name}-${i}`} className="muted attachment-pending" role="status">
          Uploading {name}…
        </p>
      ))}
      {!readOnly && (
        <div className="attachment-add">
          <input
            ref={fileInput}
            type="file"
            multiple
            hidden
            onChange={(e) => {
              void upload([...(e.target.files ?? [])]);
              e.target.value = "";
            }}
          />
          <button type="button" className="text-button" onClick={() => fileInput.current?.click()}>
            <Icon.Plus /> Attach a file
          </button>
          <button type="button" className="text-button" onClick={() => setLinking((v) => !v)}>
            <Icon.Link /> Add a link
          </button>
          {/* About both buttons, so on its own line rather than beside one. */}
          <span className="muted attachment-hint">You can also drop files onto the card.</span>
        </div>
      )}
      {linking && <LinkForm card={card} onDone={() => setLinking(false)} />}
    </section>
  );
}

function LinkForm({ card, onDone }: { card: Card; onDone: () => void }) {
  const qc = useQueryClient();
  const [url, setUrl] = useState("");
  const [title, setTitle] = useState("");
  const [error, setError] = useState("");
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const full = /^[a-z]+:/i.test(url.trim()) ? url.trim() : `https://${url.trim()}`;
    try {
      const a = await api.addLink(card.id, full, title);
      qc.setQueryData<Attachment[]>(attachmentsKey(card.id), (l) => [...(l ?? []), a]);
      onDone();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "That didn't save. Check your connection and try again.");
    }
  };
  return (
    <form className="link-attach-form" onSubmit={submit}>
      <input autoFocus aria-label="Link address" placeholder="example.com/page" value={url} onChange={(e) => setUrl(e.target.value)} />
      <input aria-label="Link name (optional)" placeholder="Name (optional)" value={title} onChange={(e) => setTitle(e.target.value)} />
      <div className="row">
        <button type="submit" disabled={!url.trim()}>
          Add link
        </button>
        <button type="button" className="text-button" onClick={onDone}>
          Cancel
        </button>
      </div>
      {error && (
        <p className="note" role="alert">
          {error}
        </p>
      )}
    </form>
  );
}
