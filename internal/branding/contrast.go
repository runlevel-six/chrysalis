// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package branding validates themes and loads optional branding (extra
// themes, an icon, a display name) from CHRYSALIS_BRANDING_DIR
// (docs/DESIGN.md §3.12).
package branding

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Theme is a named set of color tokens with light and dark variants. The
// same shape is used by the built-in themes (web/src/themes/*.json) and by
// a branding directory's theme.json.
type Theme struct {
	ID    string            `json:"id"`
	Name  string            `json:"name"`
	Light map[string]string `json:"light"`
	Dark  map[string]string `json:"dark"`
}

// Tokens every theme variant must define.
var RequiredTokens = []string{
	"surface", "surface-raised", "column-bg", "text", "text-muted", "border", "accent", "on-accent",
	"focus-ring", "hover", "toast-bg", "toast-text", "label-text",
	"label-green", "label-lime", "label-yellow", "label-orange", "label-red",
	"label-pink", "label-purple", "label-blue", "label-sky", "label-gray",
}

var labelTokens = []string{
	"label-green", "label-lime", "label-yellow", "label-orange", "label-red",
	"label-pink", "label-purple", "label-blue", "label-sky", "label-gray",
}

// Check is one contrast requirement: foreground on background, at least Min.
type Check struct {
	FG, BG string
	Min    float64
	Why    string
}

// Checks lists every pairing a theme must satisfy (WCAG 2.2 AA): 4.5:1 for
// text, 3:1 for UI parts such as focus rings and label dots.
func Checks() []Check {
	var cs []Check
	for _, bg := range []string{"surface", "surface-raised", "column-bg"} {
		cs = append(cs,
			Check{"text", bg, 4.5, "text"},
			Check{"text-muted", bg, 4.5, "secondary text"},
			Check{"focus-ring", bg, 3, "focus ring"},
		)
	}
	cs = append(cs,
		Check{"accent", "surface", 4.5, "links in descriptions"},
		Check{"on-accent", "accent", 4.5, "button text"},
		Check{"toast-text", "toast-bg", 4.5, "notification text"},
	)
	for _, l := range labelTokens {
		cs = append(cs,
			Check{"label-text", l, 4.5, "label name on its chip"},
			Check{l, "surface-raised", 3, "label dot on a card"},
		)
	}
	return cs
}

// Problem is one failed requirement.
type Problem struct {
	Variant string
	Check   Check
	Ratio   float64
}

func (p Problem) String() string {
	return fmt.Sprintf("%s: %s on %s is %.2f:1, needs %.1f:1 (%s)", p.Variant, p.Check.FG, p.Check.BG, p.Ratio, p.Check.Min, p.Check.Why)
}

// Validate reports missing tokens, unparseable colors and failed contrast
// checks. An empty result means the theme is fine.
func Validate(t Theme) []string {
	var out []string
	if t.ID == "" || t.Name == "" {
		out = append(out, "theme needs an id and a name")
	}
	for variant, tokens := range map[string]map[string]string{"light": t.Light, "dark": t.Dark} {
		missing := false
		for _, k := range RequiredTokens {
			if _, ok := tokens[k]; !ok {
				out = append(out, fmt.Sprintf("%s: missing token %q", variant, k))
				missing = true
			}
		}
		if missing {
			continue
		}
		for _, c := range Checks() {
			fg, err1 := ParseColor(tokens[c.FG])
			bg, err2 := ParseColor(tokens[c.BG])
			if err1 != nil || err2 != nil {
				out = append(out, fmt.Sprintf("%s: %s/%s isn't a #rrggbb color", variant, c.FG, c.BG))
				continue
			}
			if r := Contrast(fg, bg); r < c.Min {
				out = append(out, Problem{Variant: variant, Check: c, Ratio: r}.String())
			}
		}
	}
	return out
}

// RGB is a color with 0-255 channels.
type RGB struct{ R, G, B uint8 }

// ParseColor accepts #rrggbb (the only form theme tokens use for colors
// that are checked).
func ParseColor(s string) (RGB, error) {
	s = strings.TrimSpace(s)
	if len(s) != 7 || s[0] != '#' {
		return RGB{}, fmt.Errorf("not #rrggbb: %q", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 24) // six hex digits: fits in 24 bits
	if err != nil {
		return RGB{}, fmt.Errorf("not #rrggbb: %q", s)
	}
	//nolint:gosec // G115: each channel is masked to one byte on purpose
	return RGB{uint8(v >> 16 & 0xff), uint8(v >> 8 & 0xff), uint8(v & 0xff)}, nil
}

func channel(c uint8) float64 {
	v := float64(c) / 255
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// Luminance is WCAG relative luminance.
func Luminance(c RGB) float64 {
	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}

// Contrast is the WCAG contrast ratio between two colors (1 to 21).
func Contrast(a, b RGB) float64 {
	la, lb := Luminance(a), Luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}
