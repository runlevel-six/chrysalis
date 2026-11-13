// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package branding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// docs/DESIGN.md §3.12: every built-in theme passes WCAG AA for text on
// surfaces and for text on every label color, computed from the token files.
func TestBuiltInThemesMeetContrast(t *testing.T) {
	files, err := filepath.Glob("../../web/src/themes/*.json")
	if err != nil || len(files) < 2 {
		t.Fatalf("theme files: %v %v", files, err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var th Theme
		if err := json.Unmarshal(raw, &th); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, p := range Validate(th) {
			t.Errorf("%s: %s", filepath.Base(f), p)
		}
	}
}

func TestContrastMath(t *testing.T) {
	black, white := RGB{0, 0, 0}, RGB{255, 255, 255}
	if c := Contrast(black, white); c < 20.99 || c > 21.01 {
		t.Errorf("black/white = %.3f, want 21", c)
	}
	grey, _ := ParseColor("#767676") // the classic just-passing grey on white
	if c := Contrast(grey, white); c < 4.5 || c > 4.6 {
		t.Errorf("#767676/white = %.3f, want ~4.54", c)
	}
	if _, err := ParseColor("red"); err == nil {
		t.Error("named colors aren't accepted")
	}
}

func TestValidateCatchesProblems(t *testing.T) {
	th := Theme{ID: "x", Name: "X", Light: map[string]string{}, Dark: map[string]string{}}
	if len(Validate(th)) == 0 {
		t.Fatal("empty theme passed")
	}
	raw, _ := os.ReadFile("../../web/src/themes/default.json")
	if err := json.Unmarshal(raw, &th); err != nil {
		t.Fatal(err)
	}
	th.Light["text-muted"] = "#cccccc" // far too light on a light surface
	if probs := Validate(th); len(probs) == 0 {
		t.Fatal("low-contrast muted text passed")
	}
}

func TestBuiltInListMatchesFiles(t *testing.T) {
	for _, id := range BuiltIn {
		if _, err := os.Stat("../../web/src/themes/" + id + ".json"); err != nil {
			t.Errorf("built-in theme %q has no token file", id)
		}
	}
}
