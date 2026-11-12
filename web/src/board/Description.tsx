// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// A card's rich-text description (docs/DESIGN.md §3.1, §3.6, §5.5).
//
// - Autosaves as you type; there's no Save button.
// - Every change goes to IndexedDB first, so a reload, a crash or a dropped
//   connection never loses it. The draft is removed once the server has it.
// - Saves carry the version they were based on. If another device saved in
//   between, the server says so and we ask calmly: keep yours, use theirs, or
//   view both.
//
// This module (and the editor library) loads only when a description is
// shown, so the board itself stays light.

import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { EditorContent, useEditor, type Editor, type JSONContent } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { Placeholder } from "@tiptap/extensions";
import Image from "@tiptap/extension-image";

import { api, ApiError, attachmentUrl, type Attachment, type BoardDetail, type Card, type Description as Desc } from "../api";
import { attachmentsKey } from "./Attachments";
import { drafts } from "../drafts";
import { Icon } from "../ui";
import { detailKey } from "./useBoard";

const descKey = (cardId: string) => ["card", cardId, "description"] as const;

// Images in descriptions are this app's own attachments, stored in the doc
// as "attachment:<id>" (docs/DESIGN.md §7.3) and turned into the
// authenticated URL only when displayed. Nothing else is accepted: a pasted
// image from another site, or a doc pointing at an outside URL, renders with
// no source, so a description can never load a tracking pixel or contact a
// third party.
const ATTACHMENT_SRC = /^attachment:([0-9A-Z]{26})$/;
const SERVED_SRC = /^\/api\/v1\/attachments\/([0-9A-Z]{26})(?:\?.*)?$/;

const toStored = (src: string): string | null => {
  const served = src.match(SERVED_SRC);
  if (served) return `attachment:${served[1]}`;
  return ATTACHMENT_SRC.test(src) ? src : null;
};

const AttachmentImage = Image.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      src: {
        default: null,
        parseHTML: (el: HTMLElement) => toStored(el.getAttribute("src") ?? ""),
        renderHTML: (attrs: { src?: string | null }) => {
          const m = String(attrs.src ?? "").match(ATTACHMENT_SRC);
          return m ? { src: attachmentUrl(m[1]) } : {};
        },
      },
    };
  },
  parseHTML() {
    return [{ tag: "img[src]", getAttrs: (el: HTMLElement) => (toStored(el.getAttribute("src") ?? "") ? null : false) }];
  },
}).configure({ allowBase64: false });

const extensions = [
  StarterKit.configure({
    heading: { levels: [2, 3] },
    link: {
      openOnClick: false,
      autolink: true,
      // Only ordinary web and mail links; never javascript: or data:.
      protocols: ["http", "https", "mailto"],
      isAllowedUri: (url) => /^(https?:|mailto:)/i.test(url),
      HTMLAttributes: { rel: "noopener noreferrer nofollow", target: "_blank" },
    },
  }),
  Placeholder.configure({ placeholder: "Add a description…" }),
  AttachmentImage,
];

const imageFiles = (list: FileList | null | undefined) =>
  [...(list ?? [])].filter((f) => f.type.startsWith("image/"));

type Status = "idle" | "saving" | "saved" | "unsent" | "conflict";

/**
 * compact: inside an inline card, where space is tight. The toolbar then
 * appears below the text once you start editing, and stays until the card
 * closes. It never hides again on blur: hiding would shift content under
 * the pointer mid-click (Safari doesn't focus buttons on click).
 */
export default function Description({ card, readOnly, compact = false }: { card: Card; readOnly: boolean; compact?: boolean }) {
  const qc = useQueryClient();
  const query = useQuery({ queryKey: descKey(card.id), queryFn: () => api.description(card.id) });
  const [status, setStatus] = useState<Status>("idle");
  const [theirs, setTheirs] = useState<Desc | null>(null);
  const [showBoth, setShowBoth] = useState(false);
  const [restored, setRestored] = useState(false);
  const [activated, setActivated] = useState(!compact);

  const base = useRef(0); // server version our edits are based on
  const edits = useRef(0); // local edit counter, to know if a save is current
  const dirty = useRef(false);
  const inflight = useRef(false);
  const suppress = useRef(false); // set while we change content ourselves
  const saveTimer = useRef<number | undefined>(undefined);
  const draftTimer = useRef<number | undefined>(undefined);
  const initialized = useRef(false);

  // Upload pasted or dropped images as attachments, then put them in the
  // text at pos (or the cursor).
  const insertImages = async (files: File[], pos?: number) => {
    for (const f of files) {
      try {
        const a = await api.uploadAttachment(card.id, f);
        qc.setQueryData<Attachment[]>(attachmentsKey(card.id), (l) => [...(l ?? []), a]);
        if (a.kind !== "image" || !editorRef.current) continue;
        const chain = editorRef.current.chain().focus();
        (pos !== undefined ? chain.insertContentAt(pos, imageNode(a)) : chain.insertContent(imageNode(a))).run();
      } catch (err) {
        setUploadError(err instanceof ApiError ? err.message : `“${f.name}” didn't upload. Check your connection and try again.`);
      }
    }
  };
  const editorRef = useRef<Editor | null>(null);
  const [uploadError, setUploadError] = useState("");

  const editor = useEditor({
    extensions,
    editable: !readOnly,
    injectCSS: false, // the CSP blocks injected <style>; styles live in styles.css
    immediatelyRender: true,
    editorProps: {
      attributes: { "aria-label": `Description for ${card.title}`, class: "description-editor" },
      handlePaste: (_view, event) => {
        const files = imageFiles(event.clipboardData?.files);
        if (readOnly || !files.length) return false;
        event.preventDefault();
        void insertImages(files);
        return true;
      },
      handleDrop: (view, event) => {
        const files = imageFiles(event.dataTransfer?.files);
        if (readOnly || !files.length) return false;
        event.preventDefault();
        const at = view.posAtCoords({ left: event.clientX, top: event.clientY });
        void insertImages(files, at?.pos);
        return true;
      },
    },
    onCreate: ({ editor: ed }) => {
      editorRef.current = ed;
    },
    onFocus: () => setActivated(true),
    onUpdate: () => {
      if (suppress.current) return;
      edits.current++;
      dirty.current = true;
      window.clearTimeout(draftTimer.current);
      draftTimer.current = window.setTimeout(() => void keepDraft(), 300);
      window.clearTimeout(saveTimer.current);
      saveTimer.current = window.setTimeout(() => void save(), 1000);
    },
  });

  const docOf = (ed: Editor): JSONContent | null => (ed.isEmpty ? null : ed.getJSON());

  const setContent = (ed: Editor, doc: unknown) => {
    suppress.current = true;
    ed.commands.setContent((doc as JSONContent) ?? "", { emitUpdate: false });
    suppress.current = false;
  };

  const keepDraft = async () => {
    if (!editor || !dirty.current) return;
    await drafts.put(card.id, { doc: docOf(editor), baseVersion: base.current, savedAt: Date.now() });
  };

  const settled = useCallback(
    (d: Desc) => {
      qc.setQueryData(descKey(card.id), d);
      qc.setQueryData<BoardDetail>(detailKey(card.boardId), (bd) =>
        bd ? { ...bd, cards: bd.cards.map((c) => (c.id === card.id ? { ...c, hasDescription: d.doc !== null } : c)) } : bd,
      );
    },
    [qc, card.id, card.boardId],
  );

  const save = async () => {
    if (!editor || readOnly || !dirty.current) return;
    if (inflight.current) {
      // Try again once the current save finishes.
      window.clearTimeout(saveTimer.current);
      saveTimer.current = window.setTimeout(() => void save(), 400);
      return;
    }
    inflight.current = true;
    const sentAt = edits.current;
    setStatus("saving");
    try {
      const d = await api.setDescription(card.id, docOf(editor), base.current);
      base.current = d.version;
      settled(d);
      if (edits.current === sentAt) {
        dirty.current = false;
        await drafts.remove(card.id);
        setStatus("saved");
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setTheirs((err.body as { current: Desc }).current);
        setStatus("conflict");
      } else {
        setStatus("unsent"); // the draft is safe locally; retry when we can
      }
    } finally {
      inflight.current = false;
    }
  };

  // First load: show the server's description, then reconcile with any
  // draft left over from an earlier visit.
  useEffect(() => {
    if (!editor || !query.data || initialized.current) return;
    initialized.current = true;
    const server = query.data;
    base.current = server.version;
    setContent(editor, server.doc);
    void drafts.get(card.id).then((draft) => {
      if (!draft) return;
      setContent(editor, draft.doc);
      dirty.current = true;
      edits.current++;
      if (draft.baseVersion === server.version) {
        setRestored(true);
        void save();
      } else {
        setTheirs(server);
        setStatus("conflict");
      }
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editor, query.data]);

  // A newer version arrived (another device) and we have no local changes:
  // just show it.
  useEffect(() => {
    if (!editor || !query.data || !initialized.current) return;
    if (query.data.version > base.current && !dirty.current && status !== "conflict") {
      base.current = query.data.version;
      setContent(editor, query.data.doc);
    }
  }, [editor, query.data, status]);

  // Back online: send what's waiting.
  useEffect(() => {
    const retry = () => status === "unsent" && void save();
    window.addEventListener("online", retry);
    return () => window.removeEventListener("online", retry);
  });

  // Leaving (card collapsed, dialog closed): save now rather than in a second.
  useEffect(
    () => () => {
      window.clearTimeout(saveTimer.current);
      window.clearTimeout(draftTimer.current);
      if (dirty.current) {
        void keepDraft();
        void save();
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [editor],
  );

  const keepMine = () => {
    if (!theirs) return;
    base.current = theirs.version;
    setTheirs(null);
    setShowBoth(false);
    dirty.current = true;
    void save();
  };
  const useTheirs = async () => {
    if (!editor || !theirs) return;
    // Drop the local draft first and wait for it: once the screen shows
    // their version, a reload must not bring ours back.
    dirty.current = false;
    window.clearTimeout(draftTimer.current);
    await drafts.remove(card.id);
    setContent(editor, theirs.doc);
    base.current = theirs.version;
    settled(theirs);
    setTheirs(null);
    setShowBoth(false);
    setStatus("saved");
  };

  if (query.isPending || !editor) return <p className="muted description-loading">Loading description…</p>;
  if (readOnly && editor.isEmpty) return null;

  return (
    <div className={`description${compact ? " is-compact" : ""}`}>
      {!readOnly && !compact && <Toolbar editor={editor} onImages={(f) => void insertImages(f)} />}
      <EditorContent editor={editor} />
      {!readOnly && compact && activated && <Toolbar editor={editor} onImages={(f) => void insertImages(f)} />}
      {uploadError && (
        <p className="note" role="alert">
          {uploadError}
        </p>
      )}
      {status === "conflict" && theirs && (
        <div className="conflict" role="alert">
          <p>This card was edited on another device.</p>
          <div className="row">
            <button type="button" onClick={keepMine}>
              Keep yours
            </button>
            <button type="button" className="text-button" onClick={() => void useTheirs()}>
              Use theirs
            </button>
            <button type="button" className="text-button" aria-expanded={showBoth} onClick={() => setShowBoth((v) => !v)}>
              {showBoth ? "Hide theirs" : "View both"}
            </button>
          </div>
          {showBoth && <ReadOnlyDoc doc={theirs.doc} label="The other device's version" />}
        </div>
      )}
      {!readOnly && (
        <p className="save-note muted" aria-live="polite">
          {status === "saving" && "Saving…"}
          {status === "saved" && (restored ? "Restored your unsaved changes. Saved." : "Saved")}
          {status === "unsent" && "Not saved yet. It's kept on this device and will send when you're back online."}
        </p>
      )}
    </div>
  );
}

function ReadOnlyDoc({ doc, label }: { doc: unknown; label: string }) {
  const ed = useEditor({
    extensions,
    editable: false,
    injectCSS: false,
    content: (doc as JSONContent) ?? "",
    editorProps: { attributes: { class: "description-editor is-readonly" } },
  });
  return (
    <section className="conflict-theirs" aria-label={label}>
      <p className="field-label">{label}</p>
      <EditorContent editor={ed} />
    </section>
  );
}

function Toolbar({ editor, onImages }: { editor: Editor; onImages: (files: File[]) => void }) {
  const [linking, setLinking] = useState(false);
  const imageInput = useRef<HTMLInputElement>(null);
  const [href, setHref] = useState("");
  const [, rerender] = useState(0);

  // Re-render on selection changes so the pressed states stay accurate.
  useEffect(() => {
    const on = () => rerender((n) => n + 1);
    editor.on("transaction", on);
    return () => {
      editor.off("transaction", on);
    };
  }, [editor]);

  // Icon buttons: the accessible name is the label, with nothing visible
  // that could disagree with it.
  const btn = (label: string, active: boolean, run: () => void, icon: ReactNode) => (
    <button
      type="button"
      className={`tool${active ? " is-active" : ""}`}
      aria-label={label}
      aria-pressed={active}
      onMouseDown={(e) => e.preventDefault()} // keep the editor's selection
      onClick={run}
    >
      {icon}
    </button>
  );

  const applyLink = (e: FormEvent) => {
    e.preventDefault();
    const url = href.trim();
    const chain = editor.chain().focus().extendMarkRange("link");
    if (!url) chain.unsetLink().run();
    else chain.setLink({ href: /^[a-z]+:/i.test(url) ? url : `https://${url}` }).run();
    setLinking(false);
  };

  return (
    <div className="toolbar" role="toolbar" aria-label="Formatting">
      {btn("Bold", editor.isActive("bold"), () => editor.chain().focus().toggleBold().run(), <Icon.Bold />)}
      {btn("Italic", editor.isActive("italic"), () => editor.chain().focus().toggleItalic().run(), <Icon.Italic />)}
      {btn("Bulleted list", editor.isActive("bulletList"), () => editor.chain().focus().toggleBulletList().run(), <Icon.BulletList />)}
      {btn("Numbered list", editor.isActive("orderedList"), () => editor.chain().focus().toggleOrderedList().run(), <Icon.NumberedList />)}
      {btn("Link", editor.isActive("link"), () => {
        setHref(editor.getAttributes("link").href ?? "");
        setLinking((v) => !v);
      }, <Icon.Link />)}
      {btn("Insert image", false, () => imageInput.current?.click(), <Icon.Image />)}
      <input
        ref={imageInput}
        type="file"
        accept="image/*"
        multiple
        hidden
        onChange={(e) => {
          onImages(imageFiles(e.target.files));
          e.target.value = "";
        }}
      />
      {linking && (
        <form className="link-form" onSubmit={applyLink}>
          <input
            autoFocus
            aria-label="Link address"
            placeholder="example.com"
            value={href}
            onChange={(e) => setHref(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && setLinking(false)}
          />
          <button type="submit">{href.trim() ? "Apply" : "Remove link"}</button>
        </form>
      )}
    </div>
  );
}

function imageNode(a: Attachment) {
  return { type: "image", attrs: { src: `attachment:${a.id}`, alt: a.filename } };
}
