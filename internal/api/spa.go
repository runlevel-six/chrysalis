// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// The SPA is a Vite build: index.html plus content-hashed files under
// assets/. It's embedded in the binary (see web/embed.go).
//
// - /assets/* is public and cached forever: the hash in each filename
//   changes whenever the content does, and the code is open source anyway.
// - Every other GET (behind auth) serves the file if it exists, otherwise
//   index.html, so client-side routes like /b/<id> work on reload. index.html
//   is never cached, so a deploy takes effect on the next load.

func assetsHandler(static fs.FS) http.Handler {
	files := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") { // no directory listings
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		files.ServeHTTP(w, r)
	})
}

func spaHandler(static fs.FS) http.Handler {
	files := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Registered without a method (a "GET /" pattern would conflict with
		// "/api/" in ServeMux), so check it here.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if st, err := fs.Stat(static, name); err == nil && !st.IsDir() {
				w.Header().Set("Cache-Control", "no-cache")
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(static, "index.html")
		if errors.Is(err, fs.ErrNotExist) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("The web app hasn't been built. Run `make web`, then rebuild the server.\n"))
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}
