// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Small shared UI pieces: a modal built on the native <dialog> element (which
// handles focus trapping, Esc and restoring focus on close), and icons.

import { useEffect, useId, useRef, useState, type ReactNode, type SVGProps } from "react";

export function Dialog({
  open,
  onClose,
  title,
  children,
  className = "",
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  className?: string;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      className={`dialog ${className}`}
      aria-label={title}
      onClose={onClose}
      onClick={(e) => {
        if (e.target === ref.current) onClose(); // click on the backdrop
      }}
    >
      {open && (
        <div className="dialog-body">
          <div className="dialog-head">
            <h2>{title}</h2>
            <button type="button" className="icon-button" aria-label="Close" onClick={onClose}>
              <Icon.Close />
            </button>
          </div>
          {children}
        </div>
      )}
    </dialog>
  );
}

const svg = (path: ReactNode) =>
  function IconSvg(props: SVGProps<SVGSVGElement>) {
    return (
      <svg
        viewBox="0 0 24 24"
        width="20"
        height="20"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        focusable="false"
        {...props}
      >
        {path}
      </svg>
    );
  };

export const Icon = {
  ChevronRight: svg(<path d="m9 6 6 6-6 6" />),
  ChevronDown: svg(<path d="m6 9 6 6 6-6" />),
  Expand: svg(<path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7" />),
  Plus: svg(<path d="M12 5v14M5 12h14" />),
  More: svg(
    <>
      <circle cx="5" cy="12" r="1" />
      <circle cx="12" cy="12" r="1" />
      <circle cx="19" cy="12" r="1" />
    </>,
  ),
  Close: svg(<path d="M18 6 6 18M6 6l12 12" />),
  Check: svg(<path d="M20 6 9 17l-5-5" />),
  Trash: svg(<path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14" />),
  Calendar: svg(
    <>
      <rect x="3" y="5" width="18" height="16" rx="2" />
      <path d="M16 3v4M8 3v4M3 10h18" />
    </>,
  ),
  Tag: svg(
    <>
      <path d="M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0L3 13V3h10l7.6 7.6a2 2 0 0 1 0 2.8Z" />
      <circle cx="7.5" cy="7.5" r="1.5" />
    </>,
  ),
};

/**
 * A small dropdown menu: a button that opens a list of actions. Arrow keys
 * move between items, Esc or a click outside closes it, and focus returns to
 * the button.
 */
export function Menu({ label, icon, items }: { label: string; icon: ReactNode; items: { label: string; onSelect: () => void }[] }) {
  const [open, setOpen] = useState(false);
  const button = useRef<HTMLButtonElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const id = useId();

  useEffect(() => {
    if (!open) return;
    list.current?.querySelector<HTMLElement>("[role=menuitem]")?.focus();
    const onDown = (e: MouseEvent) => {
      if (!list.current?.contains(e.target as Node) && !button.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  const close = () => {
    setOpen(false);
    button.current?.focus();
  };

  return (
    <div className="menu">
      <button
        ref={button}
        type="button"
        className="text-button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen((o) => !o)}
      >
        {icon} {label}
      </button>
      {open && (
        <div
          ref={list}
          id={id}
          role="menu"
          aria-label={label}
          className="menu-list"
          onKeyDown={(e) => {
            const items = [...(list.current?.querySelectorAll<HTMLElement>("[role=menuitem]") ?? [])];
            const i = items.indexOf(document.activeElement as HTMLElement);
            if (e.key === "Escape") close();
            if (e.key === "ArrowDown") items[(i + 1) % items.length]?.focus();
            if (e.key === "ArrowUp") items[(i - 1 + items.length) % items.length]?.focus();
            if (e.key === "ArrowDown" || e.key === "ArrowUp") e.preventDefault();
          }}
        >
          {items.map((it) => (
            <button
              key={it.label}
              type="button"
              role="menuitem"
              className="menu-item"
              onClick={() => {
                setOpen(false);
                it.onSelect();
              }}
            >
              {it.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
