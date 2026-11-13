// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Keeps the app itself available offline (docs/DESIGN.md §5.6). Board data
// isn't cached here: the app keeps its own copy in IndexedDB, so the API is
// always asked first and authorization stays with the server.
//
// - Pages: network first; offline, the last copy of the app shell.
// - Hashed build assets: cache first (their names change with every build).
// - Icons: the cached copy, refreshed in the background.

const SHELL = "chrysalis-shell-v1";
const ASSETS = "chrysalis-assets-v1";

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(SHELL)
      .then((c) => c.add(new Request("/", { cache: "reload" })))
      .catch(() => {}) // e.g. signed out: the shell is cached on the next visit
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== SHELL && k !== ASSETS).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

const cacheable = (res) => res && res.ok && res.type === "basic";

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith("/api/") || url.pathname.startsWith("/auth/")) return;

  if (req.mode === "navigate") {
    event.respondWith(
      fetch(req)
        .then((res) => {
          // Only a real copy of the app; never a sign-in redirect.
          if (cacheable(res)) {
            const copy = res.clone();
            void caches.open(SHELL).then((c) => c.put("/", copy));
          }
          return res;
        })
        .catch(() => caches.match("/", { cacheName: SHELL }).then((r) => r || Response.error())),
    );
    return;
  }

  if (url.pathname.startsWith("/assets/")) {
    event.respondWith(
      caches.open(ASSETS).then((c) =>
        c.match(req).then(
          (hit) =>
            hit ||
            fetch(req).then((res) => {
              if (cacheable(res)) void c.put(req, res.clone());
              return res;
            }),
        ),
      ),
    );
    return;
  }

  if (/\.(png|svg|webmanifest)$/.test(url.pathname) || url.pathname === "/branding/icon") {
    event.respondWith(
      caches.open(SHELL).then((c) =>
        c.match(req).then((hit) => {
          const fresh = fetch(req)
            .then((res) => {
              if (cacheable(res)) void c.put(req, res.clone());
              return res;
            })
            .catch(() => hit || Response.error());
          return hit || fresh;
        }),
      ),
    );
  }
});

// ---------- Push notifications (reminders, §3.9) ----------
//
// The server sends {title, body, url, tag}, encrypted for this browser; the
// push service that carried it can't read it.

self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { body: event.data ? event.data.text() : "" };
  }
  event.waitUntil(
    self.registration.showNotification(data.title || "Chrysalis", {
      body: data.body || "",
      tag: data.tag || undefined, // a newer reminder for the same card replaces the older
      icon: "/icon-192.png",
      badge: "/icon-192.png",
      data: { url: data.url || "/" },
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = new URL((event.notification.data && event.notification.data.url) || "/", self.location.origin).href;
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((windows) => {
      // Reuse an open window of the app if there is one.
      for (const w of windows) {
        if (new URL(w.url).origin === self.location.origin && "focus" in w) {
          return w.focus().then((f) => (f && "navigate" in f ? f.navigate(url) : f));
        }
      }
      return self.clients.openWindow(url);
    }),
  );
});
