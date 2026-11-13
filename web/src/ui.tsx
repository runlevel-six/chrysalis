// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Small shared UI pieces: a modal built on the native <dialog> element (which
// handles focus trapping, Esc and restoring focus on close), and icons.

import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode, type SVGProps } from "react";
import { createPortal } from "react-dom";

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
  ChevronLeft: svg(<path d="m15 6-6 6 6 6" />),
  ChevronUp: svg(<path d="m6 15 6-6 6 6" />),
  // A pushpin: the head, the collar, the point.
  Pin: svg(<path d="M9 3h6l-1 6 3 3v2H7v-2l3-3-1-6ZM12 14v7" />),
  EyeOff: svg(
    <>
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" />
      <circle cx="12" cy="12" r="3" />
      <path d="m4 4 16 16" />
    </>,
  ),
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
  Bold: svg(<path d="M7 5h6a3.5 3.5 0 0 1 0 7H7zM7 12h7a3.5 3.5 0 0 1 0 7H7z" />),
  Italic: svg(<path d="M19 4h-9M14 20H5M15 4 9 20" />),
  BulletList: svg(<path d="M9 6h11M9 12h11M9 18h11M4.5 6h.01M4.5 12h.01M4.5 18h.01" />),
  NumberedList: svg(<path d="M10 6h10M10 12h10M10 18h10M4 6h1v4M4 10h2M6 18H4c0-1 2-2 2-3s-1-1.5-2-1" />),
  Settings: svg(
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z" />
    </>,
  ),
  Eye: svg(
    <>
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" />
      <circle cx="12" cy="12" r="3" />
    </>,
  ),
  Image: svg(
    <>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <circle cx="9" cy="10" r="1.5" />
      <path d="m21 16-5-5-9 9" />
    </>,
  ),
  Link: svg(<path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7" />),
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
 *
 * The list is rendered in a portal with fixed positioning next to its
 * button, so a scrolling column (or any overflow) can never clip it. It
 * opens below and flips above when there isn't room, and stays on screen.
 */
export function Menu({
  label,
  icon,
  items,
  iconOnly = false,
}: {
  label: string;
  icon: ReactNode;
  items: { label: string; onSelect: () => void }[];
  /** Show just the icon (the label stays as its accessible name and tooltip). */
  iconOnly?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);
  const button = useRef<HTMLButtonElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const id = useId();

  const place = useCallback(() => {
    const b = button.current?.getBoundingClientRect();
    const l = list.current;
    if (!b || !l) return;
    const w = l.offsetWidth;
    const h = l.offsetHeight;
    const gap = 4;
    const margin = 8;
    let top = b.bottom + gap;
    if (top + h > window.innerHeight - margin && b.top - gap - h > margin) top = b.top - gap - h;
    // Prefer lining up with the button's right edge (menus open toward the
    // board's middle), then keep the whole list on screen.
    let left = b.right - w;
    left = Math.max(margin, Math.min(left, window.innerWidth - w - margin));
    setPos({ top, left });
  }, []);

  useLayoutEffect(() => {
    if (!open) return;
    place();
    list.current?.querySelector<HTMLElement>("[role=menuitem]")?.focus();
    const onDown = (e: MouseEvent) => {
      if (!list.current?.contains(e.target as Node) && !button.current?.contains(e.target as Node)) setOpen(false);
    };
    // Follow the button if the page or a column scrolls; close if it's gone.
    const onMove = () => (button.current?.isConnected ? place() : setOpen(false));
    document.addEventListener("mousedown", onDown);
    window.addEventListener("scroll", onMove, true);
    window.addEventListener("resize", onMove);
    return () => {
      document.removeEventListener("mousedown", onDown);
      window.removeEventListener("scroll", onMove, true);
      window.removeEventListener("resize", onMove);
    };
  }, [open, place]);

  const close = () => {
    setOpen(false);
    button.current?.focus();
  };

  // Inside a modal <dialog>, render into the dialog: the dialog sits in the
  // top layer, above anything portalled to <body>.
  const host = open ? (button.current?.closest("dialog") ?? document.body) : null;

  return (
    <div className="menu">
      <button
        ref={button}
        type="button"
        className={iconOnly ? "icon-button" : "text-button"}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={id}
        aria-label={iconOnly ? label : undefined}
        title={iconOnly ? label : undefined}
        onClick={() => setOpen((o) => !o)}
      >
        {icon}
        {!iconOnly && ` ${label}`}
      </button>
      {open &&
        host &&
        createPortal(
          <div
            ref={list}
            id={id}
            role="menu"
            aria-label={label}
            className="menu-list"
            style={{ top: pos?.top ?? -9999, left: pos?.left ?? -9999 }}
            onKeyDown={(e) => {
              const items = [...(list.current?.querySelectorAll<HTMLElement>("[role=menuitem]") ?? [])];
              const i = items.indexOf(document.activeElement as HTMLElement);
              if (e.key === "Escape") close();
              if (e.key === "Tab") setOpen(false);
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
          </div>,
          host,
        )}
    </div>
  );
}
