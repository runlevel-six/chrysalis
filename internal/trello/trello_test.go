// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package trello

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/service"
)

var update = flag.Bool("update", false, "rewrite golden files")

// The synthetic garden board covers every mapped feature; the golden file
// is the exact translation and report. Run with -update after an
// intentional change, and read the diff.
func TestGardenBoardGolden(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "garden-board.json"))
	if err != nil {
		t.Fatal(err)
	}
	board, rep, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	enc := json.NewEncoder(&got)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{"board": board, "report": rep}); err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "garden-board.golden.json")
	if *update {
		if err := os.WriteFile(golden, got.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Errorf("translation differs from %s (run go test -update to see the diff in git):\n%s", golden, got.String())
	}
}

func TestNotATrelloExport(t *testing.T) {
	for _, in := range []string{
		``, `null`, `[]`, `"board"`, `{}`, `{"name":"x","lists":[]}`, `{"name":"x","cards":[]}`,
		`{"name":"x","lists":{},"cards":[]}`, `{"name":"x","lists":[],"cards":[{"name":5}]}`,
		`<html>not json</html>`, "\x00\x01\x02",
	} {
		_, _, err := Parse([]byte(in))
		var verr *service.ValidationError
		if !errors.As(err, &verr) || !strings.Contains(verr.Msg, "Export as JSON") {
			t.Errorf("Parse(%q) = %v, want the calm not-an-export message", in, err)
		}
	}
}

func TestMinimalExport(t *testing.T) {
	b, rep, err := Parse([]byte("\xef\xbb\xbf" + `{"lists":[{"id":"l","name":""}],"cards":[{"id":"c","name":"","idList":"l"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "Imported from Trello" || b.Background != nil || len(b.Columns) != 1 || b.Columns[0].Name != "Untitled list" ||
		len(b.Columns[0].Cards) != 1 || b.Columns[0].Cards[0].Title != "Untitled card" {
		t.Errorf("board = %+v", b)
	}
	if len(rep.Skipped) != 0 || len(rep.Notes) != 0 {
		t.Errorf("report = %+v", rep)
	}
}

func TestLimits(t *testing.T) {
	big := func(field string, n int, item string) []byte {
		items := strings.TrimSuffix(strings.Repeat(item+",", n), ",")
		return []byte(`{"name":"x","lists":[],"cards":[],"` + field + `":[` + items + `]}`)
	}
	cases := map[string][]byte{
		"lists":      []byte(`{"name":"x","cards":[],"lists":[` + strings.TrimSuffix(strings.Repeat(`{"id":"l"},`, service.MaxImportColumns+1), ",") + `]}`),
		"labels":     big("labels", service.MaxImportLabels+1, `{}`),
		"checklists": big("checklists", service.MaxImportChecklists+1, `{}`),
		"items":      big("checklists", 3, `{"checkItems":[`+strings.TrimSuffix(strings.Repeat(`{},`, service.MaxImportItems/2), ",")+`]}`),
	}
	for name, data := range cases {
		_, _, err := Parse(data)
		var verr *service.ValidationError
		if !errors.As(err, &verr) || !strings.Contains(verr.Msg, "larger than Chrysalis can import") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	_, _, err := Parse(make([]byte, MaxExportBytes+1))
	var verr *service.ValidationError
	if !errors.As(err, &verr) || !strings.Contains(verr.Msg, "too big") {
		t.Errorf("oversized file: err = %v", err)
	}
}

func TestLongNamesAreShortened(t *testing.T) {
	long := strings.Repeat("ü", service.MaxCardTitleLen+20)
	data, _ := json.Marshal(map[string]any{
		"name":  strings.Repeat("b", service.MaxBoardNameLen+1),
		"lists": []any{map[string]any{"id": "l", "name": "List"}},
		"cards": []any{map[string]any{"id": "c", "name": long, "idList": "l"}},
	})
	b, rep, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	title := b.Columns[0].Cards[0].Title
	if n := len([]rune(title)); n != service.MaxCardTitleLen || !strings.HasSuffix(title, "…") {
		t.Errorf("title has %d runes: %q", n, title)
	}
	if len([]rune(b.Name)) != service.MaxBoardNameLen {
		t.Errorf("board name = %d runes", len([]rune(b.Name)))
	}
	if len(rep.Notes) != 1 || rep.Notes[0] != "2 names were longer than Chrysalis allows, so they were shortened." {
		t.Errorf("notes = %q", rep.Notes)
	}
}

func TestLabelColor(t *testing.T) {
	s := func(v string) *string { return &v }
	cases := []struct {
		in      *string
		want    string
		variant bool
	}{
		{s("green"), "green", false}, {s("yellow"), "yellow", false}, {s("orange"), "orange", false},
		{s("red"), "red", false}, {s("purple"), "purple", false}, {s("blue"), "blue", false},
		{s("sky"), "sky", false}, {s("lime"), "lime", false}, {s("pink"), "pink", false},
		{s("black"), "gray", false}, {nil, "gray", false}, {s(""), "gray", false}, {s("chartreuse"), "gray", false},
		{s("green_dark"), "green", true}, {s("sky_light"), "sky", true}, {s("black_dark"), "gray", true},
		{s("pink_light"), "pink", true},
	}
	for _, tc := range cases {
		got, variant := LabelColor(tc.in)
		if got != tc.want || variant != tc.variant {
			t.Errorf("LabelColor(%v) = %q, %v; want %q, %v", tc.in, got, variant, tc.want, tc.variant)
		}
		if !contains(service.LabelColors, got) {
			t.Errorf("%q isn't a palette key", got)
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func TestPositionFormsKeepOrder(t *testing.T) {
	data := []byte(`{"name":"x","lists":[
		{"id":"d","name":"D","pos":"bottom"},
		{"id":"b","name":"B","pos":"20"},
		{"id":"c","name":"C","pos":30.5},
		{"id":"a","name":"A","pos":"top"},
		{"id":"e","name":"E"}
	],"cards":[]}`)
	b, _, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range b.Columns {
		names = append(names, c.Name)
	}
	// A missing pos is 0: after "top", before the rest.
	if got := strings.Join(names, ""); got != "AEBCD" {
		t.Errorf("order = %s", got)
	}
}
