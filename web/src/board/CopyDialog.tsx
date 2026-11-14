// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// "Copy…" and "Save as template…" on a card (docs/DESIGN.md §4, Phase 2):
// a title, and what to bring along. Everything is ticked to start with.

import { useState, type FormEvent } from "react";

import { COPY_EVERYTHING, type Card, type CopyInclude, type CopyOptions } from "../api";
import { Dialog } from "../ui";

const PARTS: [keyof CopyInclude, string][] = [
  ["checklists", "Checklists"],
  ["labels", "Labels"],
  ["description", "Description"],
  ["dates", "Dates"],
  ["fields", "Fields"],
  ["attachments", "Attachments and cover"],
];

export function CopyDialog({
  card,
  template,
  onCopy,
  onClose,
}: {
  card: Card;
  /** Save as a template instead of a plain copy. */
  template: boolean;
  onCopy: (opts: CopyOptions) => void;
  onClose: () => void;
}) {
  const [title, setTitle] = useState(card.title);
  const [include, setInclude] = useState<CopyInclude>(template ? { ...COPY_EVERYTHING, dates: false } : COPY_EVERYTHING);
  const [fresh, setFresh] = useState(false);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;
    onCopy({ title: title.trim(), template, include, freshChecklists: template || fresh });
    onClose();
  };

  return (
    <Dialog open onClose={onClose} title={template ? "Save as a template" : "Copy card"}>
      <form className="copy-form" onSubmit={submit}>
        <label className="field">
          {template ? "Template name" : "Title"}
          <input value={title} maxLength={500} onChange={(e) => setTitle(e.target.value)} />
        </label>
        <fieldset className="settings-group">
          <legend>Bring along</legend>
          {PARTS.map(([k, text]) => (
            <label key={k} className="check">
              <input type="checkbox" checked={include[k]} onChange={(e) => setInclude({ ...include, [k]: e.target.checked })} />
              {text}
            </label>
          ))}
          {!template && include.checklists && (
            <label className="check">
              <input type="checkbox" checked={fresh} onChange={(e) => setFresh(e.target.checked)} />
              Untick every checklist item
            </label>
          )}
        </fieldset>
        {template && (
          <p className="muted">
            Templates stay out of the columns. Start a card from one with <strong>From a template</strong> at the bottom of a
            column; manage them in Board settings.
          </p>
        )}
        <div className="row">
          <button type="submit" disabled={!title.trim()}>
            {template ? "Save template" : "Copy"}
          </button>
          <button type="button" className="text-button" onClick={onClose}>
            Cancel
          </button>
        </div>
      </form>
    </Dialog>
  );
}
