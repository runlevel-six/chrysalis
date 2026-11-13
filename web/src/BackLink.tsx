// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// "Back to <board>" for pages you visit from a board, such as Settings and
// the calendar. The page you came from is remembered per tab (surviving a
// reload), so Back returns to the same board.

import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";

import { api } from "./api";
import { Link, navigate } from "./router";

type Page = "settings" | "calendar";
const key = (page: Page) => `chrysalis.${page}Return`;

/** Call just before going to page, to remember where to come back to. */
export function rememberReturn(page: Page) {
  const here = window.location.pathname;
  if (here.startsWith(`/${page}`)) return;
  try {
    sessionStorage.setItem(key(page), here);
  } catch {
    // storage blocked: Back goes to the fallback
  }
}

function returnPath(page: Page, fallback: string) {
  try {
    const p = sessionStorage.getItem(key(page));
    if (p && /^\/(b\/[^/]+)?$/.test(p)) return p;
  } catch {
    // fall through
  }
  return fallback;
}

/**
 * The link back. escape also makes the Escape key go back (not on pages
 * where Escape already means something, like cancelling a drag).
 */
export function BackLink({ page, fallback = "/", escape = false }: { page: Page; fallback?: string; escape?: boolean }) {
  const to = returnPath(page, fallback);
  const boards = useQuery({ queryKey: ["boards"], queryFn: api.listBoards, enabled: to !== "/" });
  const id = to.startsWith("/b/") ? decodeURIComponent(to.slice(3)) : "";
  const name = boards.data?.find((b) => b.id === id)?.name;
  useEffect(() => {
    if (!escape) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !e.defaultPrevented && !(e.target instanceof HTMLElement && e.target.closest("dialog, [role=dialog]"))) navigate(to);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [to, escape]);
  return (
    <Link to={to} className="back-link">
      <span aria-hidden="true">←</span> {to === "/" ? "Back to your boards" : name ? `Back to ${name}` : "Back to your board"}
    </Link>
  );
}
