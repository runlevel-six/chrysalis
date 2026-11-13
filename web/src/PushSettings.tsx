// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Reminders on this device (§3.9): Web Push, turned on per browser or
// installed app, by the person, from Settings. Nothing ever asks for
// notification permission except this button.

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { api, ApiError } from "./api";
import { useUndo } from "./undo";

type State = "checking" | "unsupported" | "needs-install" | "blocked" | "off" | "on";

const supported = () => "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;
const isAppleMobile = () => /iPhone|iPad|iPod/.test(navigator.userAgent) || (navigator.userAgent.includes("Mac") && navigator.maxTouchPoints > 1);
const standalone = () =>
  window.matchMedia("(display-mode: standalone)").matches || (navigator as Navigator & { standalone?: boolean }).standalone === true;

/** A name for this device the person will recognise in a list later. */
function deviceLabel() {
  const ua = navigator.userAgent;
  const browser = /Edg\//.test(ua) ? "Edge" : /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "Browser";
  const os = /iPhone/.test(ua) ? "iPhone" : /iPad/.test(ua) || isAppleMobile() ? "iPad" : /Android/.test(ua) ? "Android" : /Mac/.test(ua) ? "Mac" : /Windows/.test(ua) ? "Windows" : /Linux/.test(ua) ? "Linux" : "";
  return os ? `${browser} on ${os}` : browser;
}

const keyBytes = (b64: string) => {
  const s = atob(b64.replace(/-/g, "+").replace(/_/g, "/") + "===".slice((b64.length + 3) % 4));
  return Uint8Array.from(s, (c) => c.charCodeAt(0));
};

export function PushSettings() {
  const { notify } = useUndo();
  const info = useQuery({ queryKey: ["push"], queryFn: api.pushInfo, staleTime: Infinity });
  const [state, setState] = useState<State>("checking");
  const [busy, setBusy] = useState(false);

  const check = async () => {
    if (!supported()) {
      setState(isAppleMobile() && !standalone() ? "needs-install" : "unsupported");
      return;
    }
    if (Notification.permission === "denied") {
      setState("blocked");
      return;
    }
    const reg = await navigator.serviceWorker.getRegistration();
    const sub = await reg?.pushManager.getSubscription();
    if (!sub) {
      setState("off");
      return;
    }
    const s = await api.pushStatus(sub.endpoint).catch(() => ({ subscribed: false }));
    setState(s.subscribed ? "on" : "off");
  };
  useEffect(() => {
    void check();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  if (!info.data?.enabled) return null;

  const fail = (err: unknown) =>
    notify(err instanceof ApiError ? err.message : "That didn't work. Check your connection and try again.");

  const turnOn = async () => {
    setBusy(true);
    try {
      const permission = await Notification.requestPermission();
      if (permission !== "granted") {
        setState(permission === "denied" ? "blocked" : "off");
        return;
      }
      const reg = await navigator.serviceWorker.ready;
      const sub =
        (await reg.pushManager.getSubscription()) ??
        (await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: keyBytes(info.data!.publicKey!) }));
      await api.pushSubscribe(sub.toJSON(), deviceLabel());
      setState("on");
      notify("This device will get your reminders.");
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  };

  const turnOff = async () => {
    setBusy(true);
    try {
      const reg = await navigator.serviceWorker.getRegistration();
      const sub = await reg?.pushManager.getSubscription();
      if (sub) {
        await api.pushUnsubscribe(sub.endpoint);
        await sub.unsubscribe();
      }
      setState("off");
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  };

  const test = async () => {
    try {
      const r = await api.pushTest();
      notify(r.devices ? "Sent. It should appear in a moment." : "No device took it; try turning this off and on again.");
    } catch (err) {
      fail(err);
    }
  };

  return (
    <div className="push-settings">
      <h3>On this device</h3>
      {state === "checking" && <p className="muted">Checking…</p>}
      {state === "unsupported" && <p className="muted">This browser can't show notifications from Chrysalis. Reminders still show under the bell.</p>}
      {state === "needs-install" && (
        <p className="muted">
          On iPhone and iPad, add Chrysalis to your Home Screen first (the Share button, then <strong>Add to Home Screen</strong>), open it
          from there, and turn this on.
        </p>
      )}
      {state === "blocked" && (
        <p className="muted">Notifications are blocked for this site. You can allow them in your browser's site settings, then come back here.</p>
      )}
      {state === "off" && (
        <>
          <p className="muted">Get reminders as notifications here, even when Chrysalis isn't open.</p>
          <button type="button" disabled={busy} onClick={() => void turnOn()}>
            Send reminders to this device
          </button>
        </>
      )}
      {state === "on" && (
        <>
          <p className="muted">This device gets your reminders.</p>
          <div className="row">
            <button type="button" className="text-button" onClick={() => void test()}>
              Send a test
            </button>
            <button type="button" className="text-button" disabled={busy} onClick={() => void turnOff()}>
              Stop on this device
            </button>
          </div>
        </>
      )}
    </div>
  );
}

/** Quiet hours: nothing is pushed between these local times. */
export function QuietHours({
  start,
  end,
  onChange,
}: {
  start: string;
  end: string;
  onChange: (start: string, end: string) => void;
}) {
  const on = !!start && !!end;
  return (
    <div className="quiet-hours">
      <label className="check">
        <input type="checkbox" checked={on} onChange={(e) => (e.target.checked ? onChange("21:00", "07:00") : onChange("", ""))} />
        Quiet hours: no notifications on devices
      </label>
      {on && (
        <div className="row">
          <label className="field">
            From
            <input type="time" value={start} onChange={(e) => e.target.value && onChange(e.target.value, end)} />
          </label>
          <label className="field">
            Until
            <input type="time" value={end} onChange={(e) => e.target.value && onChange(start, e.target.value)} />
          </label>
        </div>
      )}
      {on && <p className="muted">Reminders during quiet hours still wait for you under the bell.</p>}
    </div>
  );
}
