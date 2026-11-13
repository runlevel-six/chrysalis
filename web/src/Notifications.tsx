// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// The in-app notification list (docs/DESIGN.md §3.9): reminders that went
// off, under a bell in the header. A quiet dot marks new ones (no counts);
// each can be opened, snoozed or put away. When one arrives while the app is
// open, a gentle toast says so once.

import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { api, ApiError, type Notification } from "./api";
import { navigate } from "./router";
import { Dialog, Icon } from "./ui";
import { useUndo } from "./undo";

const key = ["notifications"] as const;

/** Snooze choices: an hour, this evening, tomorrow morning, or a time. */
function snoozeTimes(now = new Date()) {
  const hour = new Date(now.getTime() + 60 * 60 * 1000);
  const tonight = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 19, 0);
  const tomorrow = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, 9, 0);
  return [
    { label: "In an hour", at: hour },
    ...(tonight.getTime() - now.getTime() > 90 * 60 * 1000 ? [{ label: "This evening", at: tonight }] : []),
    { label: "Tomorrow morning", at: tomorrow },
  ];
}

const ago = (iso: string) => {
  const mins = Math.round((Date.now() - new Date(iso).getTime()) / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins} min ago`;
  const h = Math.round(mins / 60);
  if (h < 24) return `${h} h ago`;
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric" });
};

export function NotificationsButton({ enabled }: { enabled: boolean }) {
  const qc = useQueryClient();
  const { notify } = useUndo();
  const [open, setOpen] = useState(false);
  const q = useQuery({ queryKey: key, queryFn: api.notifications, enabled });
  const items = q.data?.notifications ?? [];
  const unread = q.data?.unread ?? 0;

  // A gentle toast for each reminder that arrives while the app is open.
  const seen = useRef<Set<string> | null>(null);
  useEffect(() => {
    if (!q.data) return;
    const ids = new Set(q.data.notifications.map((n) => n.id));
    if (seen.current) {
      const fresh = q.data.notifications.find((n) => !seen.current!.has(n.id) && !n.readAt);
      if (fresh && !open) notify(fresh.text);
    }
    seen.current = ids;
  }, [q.data]); // eslint-disable-line react-hooks/exhaustive-deps

  const refresh = () => qc.invalidateQueries({ queryKey: key });
  const show = () => {
    setOpen(true);
    if (unread) void api.readNotifications().then(refresh);
  };

  if (!enabled && items.length === 0) return null;
  return (
    <>
      <button
        type="button"
        className="header-link notifications-button"
        aria-label={unread ? "Notifications (new)" : "Notifications"}
        title="Notifications"
        onClick={show}
      >
        <Icon.Bell />
        {unread > 0 && <span className="notify-dot" aria-hidden="true" />}
      </button>
      <Dialog open={open} onClose={() => setOpen(false)} title="Notifications">
        {items.length === 0 ? (
          <p className="muted">Nothing here. Reminders you set on cards will show up here when they're due.</p>
        ) : (
          <>
            <ul className="notification-list">
              {items.map((n) => (
                <NotificationItem key={n.id} n={n} onChange={refresh} onOpen={() => setOpen(false)} />
              ))}
            </ul>
            <div className="dialog-actions">
              <button
                type="button"
                className="text-button"
                onClick={async () => {
                  await api.dismissAllNotifications();
                  await refresh();
                }}
              >
                Clear all
              </button>
            </div>
          </>
        )}
      </Dialog>
    </>
  );
}

function NotificationItem({ n, onChange, onOpen }: { n: Notification; onChange: () => void; onOpen: () => void }) {
  const { notify } = useUndo();
  const [snoozing, setSnoozing] = useState(false);
  const [custom, setCustom] = useState("");
  const act = async (fn: () => Promise<unknown>, done?: string) => {
    try {
      await fn();
      onChange();
      if (done) notify(done);
    } catch (err) {
      notify(err instanceof ApiError ? err.message : "That didn't save. Check your connection and try again.");
    }
  };
  const snooze = (at: Date) =>
    act(
      () => api.snoozeNotification(n.id, at),
      `I'll remind you again ${at.toLocaleString(undefined, { weekday: "short", hour: "numeric", minute: "2-digit" })}.`,
    );

  return (
    <li className={`notification${n.readAt ? "" : " is-new"}`}>
      <p className="notification-text">{n.text}</p>
      <p className="muted notification-time">{ago(n.createdAt)}</p>
      <div className="row notification-actions">
        {n.boardId && n.cardId && (
          <button
            type="button"
            className="text-button"
            onClick={() => {
              onOpen();
              navigate(`/b/${n.boardId}?card=${encodeURIComponent(n.cardId!)}`);
            }}
          >
            Open card
          </button>
        )}
        {n.cardId && (
          <button type="button" className="text-button" aria-expanded={snoozing} onClick={() => setSnoozing((v) => !v)}>
            Snooze…
          </button>
        )}
        <button type="button" className="text-button" onClick={() => void act(() => api.dismissNotification(n.id))}>
          Done
        </button>
      </div>
      {snoozing && (
        <div className="snooze-choices" role="group" aria-label="Snooze until">
          {snoozeTimes().map((s) => (
            <button key={s.label} type="button" className="text-button" onClick={() => void snooze(s.at)}>
              {s.label}
            </button>
          ))}
          <form
            className="row"
            onSubmit={(e) => {
              e.preventDefault();
              if (custom) void snooze(new Date(custom));
            }}
          >
            <label className="visually-hidden" htmlFor={`snooze-${n.id}`}>
              Pick a time
            </label>
            <input id={`snooze-${n.id}`} type="datetime-local" value={custom} onChange={(e) => setCustom(e.target.value)} />
            <button type="submit" disabled={!custom}>
              Snooze
            </button>
          </form>
        </div>
      )}
    </li>
  );
}
