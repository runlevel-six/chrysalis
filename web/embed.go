// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package web embeds the built single-page app (web/dist) into the binary.
//
// dist always contains .gitkeep (copied from public/ on every build), so the
// Go code compiles even before the frontend has been built; the server then
// shows a "run make web" message instead of the app.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built app with dist/ stripped from paths.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // only fails if the embed directive above is wrong
	}
	return sub
}
