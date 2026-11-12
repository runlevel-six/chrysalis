// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Small shared UI pieces: a modal built on the native <dialog> element (which
// handles focus trapping, Esc and restoring focus on close), and icons.

import { useEffect, useRef, type ReactNode, type SVGProps } from "react";

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
};
