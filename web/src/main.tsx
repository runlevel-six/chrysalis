// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { App } from "./App";
import { ApiError } from "./api";
import { UndoProvider } from "./undo";
import { whenBackOnline } from "./offline";
import "./styles.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Don't hammer the server retrying requests that can't succeed.
      retry: (count, err) => !(err instanceof ApiError && err.status < 500) && count < 2,
      // Run reads even when the browser thinks it's offline: they fall back
      // to this device's last copy (src/offline.ts).
      networkMode: "always",
    },
    mutations: { networkMode: "always" }, // fail visibly offline; never queue silently
  },
});

// Back online: refresh everything shown from the offline copy.
whenBackOnline(() => void queryClient.invalidateQueries());
window.addEventListener("online", () => void queryClient.invalidateQueries());

// The service worker keeps the app itself available offline (§5.6).
if (import.meta.env.PROD && "serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch(() => {
      // Not fatal: the app works online without it.
    });
  });
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <UndoProvider>
        <App />
      </UndoProvider>
    </QueryClientProvider>
  </StrictMode>,
);
