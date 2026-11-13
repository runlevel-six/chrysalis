// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package branding

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// BuiltIn lists the themes shipped in the web app (web/src/themes/*.json).
var BuiltIn = []string{"default", "butterfly-garden"}

// Branding is what a deployment adds on top of the built-in look.
type Branding struct {
	DisplayName string  `json:"displayName,omitempty"`
	Themes      []Theme `json:"themes"`
	Icon        []byte  `json:"-"`
	IconMIME    string  `json:"-"`
}

var themeID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// Load reads an optional branding directory:
//
//	theme.json         one theme, or a list of themes (same schema as the built-ins)
//	icon.svg|icon.png  the app icon
//	display-name.txt   the product name to show instead of "Chrysalis"
//
// Missing files are fine. Themes are checked against the same contrast rules
// as the built-ins; problems are logged as warnings but the theme still
// loads, because branding is the deployer's decision. A theme that can't be
// used at all (bad JSON, no id, an id that collides) is skipped.
func Load(dir string, log *slog.Logger) (Branding, error) {
	var b Branding
	if dir == "" {
		return b, nil
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return b, fmt.Errorf("branding: %s isn't a readable directory", dir)
	}

	//nolint:gosec // G304: the deployer's configured branding dir plus a fixed file name
	if raw, err := os.ReadFile(filepath.Join(dir, "theme.json")); err == nil {
		themes, err := parseThemes(raw)
		if err != nil {
			log.Warn("branding: theme.json ignored", "err", err)
		}
		seen := map[string]bool{}
		for _, id := range BuiltIn {
			seen[id] = true
		}
		for _, t := range themes {
			if !themeID.MatchString(t.ID) || seen[t.ID] {
				log.Warn("branding: theme skipped (its id is missing, invalid or already used)", "id", t.ID)
				continue
			}
			seen[t.ID] = true
			for _, p := range Validate(t) {
				log.Warn("branding: theme contrast or token problem", "theme", t.ID, "problem", p)
			}
			b.Themes = append(b.Themes, t)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return b, err
	}

	for _, f := range []struct{ name, mime string }{{"icon.svg", "image/svg+xml"}, {"icon.png", "image/png"}} {
		raw, err := os.ReadFile(filepath.Join(dir, f.name)) //nolint:gosec // G304: configured dir, fixed name
		if err == nil {
			if len(raw) > 512<<10 {
				log.Warn("branding: icon larger than 512 KB ignored", "file", f.name)
				continue
			}
			b.Icon, b.IconMIME = raw, f.mime
			break
		}
	}

	//nolint:gosec // G304: the deployer's configured branding dir plus a fixed file name
	if raw, err := os.ReadFile(filepath.Join(dir, "display-name.txt")); err == nil {
		name := strings.TrimSpace(string(raw))
		if utf8.RuneCountInString(name) > 60 {
			log.Warn("branding: display name longer than 60 characters ignored")
		} else {
			b.DisplayName = name
		}
	}
	return b, nil
}

func parseThemes(raw []byte) ([]Theme, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '[' {
		var ts []Theme
		return ts, json.Unmarshal(raw, &ts)
	}
	var t Theme
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, err
	}
	return []Theme{t}, nil
}

// Has reports whether a theme id is built in or provided by this branding.
func (b Branding) Has(id string) bool {
	if slices.Contains(BuiltIn, id) {
		return true
	}
	for _, t := range b.Themes {
		if t.ID == id {
			return true
		}
	}
	return false
}
