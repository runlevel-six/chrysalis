// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Personal settings: how the app looks and moves, the same on every device.

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type User } from "./api";
import { BUILT_IN_THEMES, DEFAULT_PREFS, deviceTimeZone, type Prefs, type Theme } from "./appearance";
import { useUndo } from "./undo";
import { BackLink } from "./BackLink";
import { PushSettings, QuietHours } from "./PushSettings";
import { CenteringSection } from "./CenteringSettings";

export function useBrandingThemes(): Theme[] {
  const b = useQuery({ queryKey: ["branding"], queryFn: api.branding, staleTime: Infinity });
  return [...BUILT_IN_THEMES, ...(b.data?.themes ?? [])];
}

/** Current prefs, and a setter that applies instantly and saves in the background. */
export function usePrefs() {
  const qc = useQueryClient();
  const { notify } = useUndo();
  // Subscribe (not a one-off read) so this updates when the prefs arrive
  // after the page has already rendered, e.g. right after a reload.
  const me = useQuery({ queryKey: ["me"], queryFn: api.me }).data;
  const prefs: Prefs = { ...DEFAULT_PREFS, ...(me?.prefs ?? {}) };
  const save = useMutation({
    mutationFn: (patch: Partial<Prefs>) => api.updatePrefs(patch),
    onMutate: (patch) => {
      const before = qc.getQueryData<User>(["me"]);
      qc.setQueryData<User>(["me"], (u) => (u ? { ...u, prefs: { ...DEFAULT_PREFS, ...u.prefs, ...patch } } : u));
      return { before };
    },
    onError: (_e, _p, ctx) => {
      qc.setQueryData(["me"], ctx?.before);
      notify("That setting didn't save. Check your connection and try again.");
    },
    onSuccess: (p) => qc.setQueryData<User>(["me"], (u) => (u ? { ...u, prefs: p } : u)),
  });
  return { prefs, set: (patch: Partial<Prefs>) => save.mutate(patch) };
}

function Choice<T extends string>({
  name,
  legend,
  value,
  options,
  onChange,
}: {
  name: string;
  legend: string;
  value: T;
  options: [T, string][];
  onChange: (v: T) => void;
}) {
  return (
    <fieldset className="settings-group">
      <legend>{legend}</legend>
      {options.map(([v, text]) => (
        <label key={v} className="check">
          <input type="radio" name={name} checked={value === v} onChange={() => onChange(v)} />
          {text}
        </label>
      ))}
    </fieldset>
  );
}

export function SettingsPage() {
  const { prefs: saved, set: save } = usePrefs();
  // Local copy so controls respond on the click itself; the shared cache
  // (and the server) catch up a tick later.
  const [prefs, setPrefs] = useState(saved);
  useEffect(() => setPrefs(saved), [saved.theme, saved.mode, saved.density, saved.motion, saved.completionFlutter, saved.highlightOverdue, saved.remindersEnabled, saved.quietStart, saved.quietEnd]); // eslint-disable-line react-hooks/exhaustive-deps
  const set = (patch: Partial<Prefs>) => {
    setPrefs((p) => ({ ...p, ...patch }));
    save(patch);
  };
  const themes = useBrandingThemes();
  useEffect(() => {
    document.title = "Settings · Chrysalis";
  }, []);

  return (
    <section className="page">
      <BackLink page="settings" escape />
      <h1>Settings</h1>
      <p className="muted">These follow you to every device you sign in on.</p>

      <div className="settings-section">
        <h2>Theme</h2>
        <div className="theme-choices" role="radiogroup" aria-label="Theme">
          {themes.map((t) => {
            const tokens = prefs.mode === "dark" ? t.dark : t.light;
            return (
              <button
                key={t.id}
                type="button"
                role="radio"
                aria-checked={prefs.theme === t.id}
                className="theme-choice"
                onClick={() => set({ theme: t.id })}
              >
                <span className="theme-preview" aria-hidden="true" style={{ background: tokens.surface }}>
                  <span style={{ background: tokens["column-bg"] }} />
                  <span style={{ background: tokens.accent }} />
                  <span style={{ background: tokens["label-orange"] }} />
                  <span style={{ background: tokens["label-pink"] }} />
                  <span style={{ background: tokens["label-sky"] }} />
                </span>
                {t.name}
              </button>
            );
          })}
        </div>
        <Choice
          name="mode"
          legend="Light or dark"
          value={prefs.mode}
          options={[
            ["system", "Match this device"],
            ["light", "Always light"],
            ["dark", "Always dark"],
          ]}
          onChange={(mode) => set({ mode })}
        />
        <Choice
          name="density"
          legend="Spacing"
          value={prefs.density}
          options={[
            ["comfortable", "Comfortable"],
            ["compact", "Compact (more fits on screen)"],
          ]}
          onChange={(density) => set({ density })}
        />
      </div>

      <div className="settings-section">
        <h2>Motion</h2>
        <Choice
          name="motion"
          legend="Animations"
          value={prefs.motion}
          options={[
            ["system", "Match this device"],
            ["reduce", "Keep things still"],
            ["allow", "Allow gentle motion"],
          ]}
          onChange={(motion) => set({ motion })}
        />
        <label className="check">
          <input type="checkbox" checked={prefs.completionFlutter} onChange={(e) => set({ completionFlutter: e.target.checked })} />
          A small butterfly flutters off a card when you finish it
        </label>
      </div>

      <div className="settings-section">
        <h2>Due dates</h2>
        <label className="check">
          <input type="checkbox" checked={prefs.highlightOverdue} onChange={(e) => set({ highlightOverdue: e.target.checked })} />
          Mark cards that are past their due date (off unless you want it)
        </label>
      </div>

      <div className="settings-section">
        <h2>Reminders</h2>
        <label className="check">
          <input
            type="checkbox"
            checked={prefs.remindersEnabled}
            onChange={(e) => set({ remindersEnabled: e.target.checked, timeZone: deviceTimeZone() })}
          />
          Remind me about cards I choose
        </label>
        <p className="muted">
          Nothing reminds you unless you ask: once this is on, choose <strong>Remind me</strong> in a card's dates.
          Reminders show under the bell at the top of the page.
        </p>
        {prefs.remindersEnabled && (
          <>
            <PushSettings />
            <QuietHours start={prefs.quietStart} end={prefs.quietEnd} onChange={(quietStart, quietEnd) => set({ quietStart, quietEnd })} />
          </>
        )}
      </div>

      <CenteringSection />
    </section>
  );
}
