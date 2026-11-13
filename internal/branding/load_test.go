// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package branding

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadBranding(t *testing.T) {
	dir := t.TempDir()
	raw, _ := os.ReadFile("../../web/src/themes/default.json")
	var good Theme
	_ = json.Unmarshal(raw, &good)
	good.ID, good.Name = "meadow", "Meadow"
	weak := good
	weak.ID, weak.Name = "faded", "Faded"
	weak.Light = map[string]string{}
	for k, v := range good.Light {
		weak.Light[k] = v
	}
	weak.Light["text-muted"] = "#dddddd"
	clash := good
	clash.ID = "default" // can't replace a built-in
	themes, _ := json.Marshal([]Theme{good, weak, clash})
	_ = os.WriteFile(filepath.Join(dir, "theme.json"), themes, 0o600)
	_ = os.WriteFile(filepath.Join(dir, "icon.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "display-name.txt"), []byte("  Garden Boards \n"), 0o600)

	var logs bytes.Buffer
	b, err := Load(dir, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Themes) != 2 || b.Themes[0].ID != "meadow" || b.Themes[1].ID != "faded" {
		t.Fatalf("themes = %+v", b.Themes)
	}
	if !strings.Contains(logs.String(), "faded") || !strings.Contains(logs.String(), "text-muted") {
		t.Errorf("weak theme should load with a contrast warning; logs:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "already used") {
		t.Errorf("clashing id should be skipped with a warning")
	}
	if b.DisplayName != "Garden Boards" || b.IconMIME != "image/svg+xml" {
		t.Errorf("name/icon = %q %q", b.DisplayName, b.IconMIME)
	}
	if !b.Has("meadow") || !b.Has("butterfly-garden") || b.Has("nope") {
		t.Error("Has() wrong")
	}
}

func TestLoadBrandingOptional(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if b, err := Load("", log); err != nil || len(b.Themes) != 0 {
		t.Fatalf("no dir: %+v %v", b, err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing"), log); err == nil {
		t.Error("a configured but missing directory should be an error")
	}
	b, err := Load(t.TempDir(), log) // empty is fine
	if err != nil || len(b.Themes) != 0 || b.Icon != nil {
		t.Fatalf("empty dir: %+v %v", b, err)
	}
}
