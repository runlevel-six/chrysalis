// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package trello reads a Trello board's JSON export and translates it into
// a Chrysalis board (docs/DESIGN.md §8). It only translates: writing the
// board, in one transaction, is service.Boards.Import.
//
// The export is untrusted input. Its size and counts are bounded, every
// field is checked again by the service, and anything without a Chrysalis
// equivalent is left out and listed in the report.
package trello

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/runlevel-six/chrysalis/internal/service"
)

// MaxExportBytes bounds the uploaded file. Real exports of large, busy
// boards are a few megabytes.
const MaxExportBytes = 50 << 20

// Report tells the person what came across and what didn't, in plain words.
type Report struct {
	Imported service.ImportCounts `json:"imported"`
	// Skipped lists what had no place in Chrysalis and was left out.
	Skipped []string `json:"skipped"`
	// Notes are things worth knowing about what did come across.
	Notes []string `json:"notes"`
}

// position is Trello's numeric "pos". Old exports sometimes carry "top" or
// "bottom", or a number as a string.
type position float64

func (p *position) UnmarshalJSON(b []byte) error {
	var f float64
	if err := json.Unmarshal(b, &f); err == nil {
		*p = position(f)
		return nil
	}
	// Anything else (a string, null, garbage) sorts last rather than failing
	// the whole import.
	var s string
	_ = json.Unmarshal(b, &s)
	switch s {
	case "top":
		*p = position(math.Inf(-1))
	default:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) {
			f = math.Inf(1)
		}
		*p = position(f)
	}
	return nil
}

type export struct {
	Name  string `json:"name"`
	Prefs struct {
		BackgroundColor string `json:"backgroundColor"`
		BackgroundImage string `json:"backgroundImage"`
	} `json:"prefs"`
	Lists        *[]list           `json:"lists"`
	Cards        *[]card           `json:"cards"`
	Labels       []label           `json:"labels"`
	Checklists   []checklist       `json:"checklists"`
	Actions      []action          `json:"actions"`
	PluginData   []json.RawMessage `json:"pluginData"`
	CustomFields []json.RawMessage `json:"customFields"`
}

type list struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Closed bool     `json:"closed"`
	Pos    position `json:"pos"`
}

type card struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Desc        string   `json:"desc"`
	Due         *string  `json:"due"`
	Start       *string  `json:"start"`
	DueComplete bool     `json:"dueComplete"`
	DueReminder *float64 `json:"dueReminder"`
	Closed      bool     `json:"closed"`
	IDList      string   `json:"idList"`
	IDLabels    []string `json:"idLabels"`
	IDMembers   []string `json:"idMembers"`
	Pos         position `json:"pos"`
	Cover       *struct {
		Color                *string `json:"color"`
		IDAttachment         *string `json:"idAttachment"`
		IDUploadedBackground *string `json:"idUploadedBackground"`
	} `json:"cover"`
	Attachments      []attachment      `json:"attachments"`
	CustomFieldItems []json.RawMessage `json:"customFieldItems"`
	PluginData       []json.RawMessage `json:"pluginData"`
	Coordinates      json.RawMessage   `json:"coordinates"`
	Address          *string           `json:"address"`
}

type attachment struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	IsUpload bool   `json:"isUpload"`
}

type label struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

type checklist struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	IDCard     string      `json:"idCard"`
	Pos        position    `json:"pos"`
	CheckItems []checkItem `json:"checkItems"`
}

type checkItem struct {
	Name     string   `json:"name"`
	State    string   `json:"state"`
	Pos      position `json:"pos"`
	Due      *string  `json:"due"`
	IDMember *string  `json:"idMember"`
}

type action struct {
	Type string `json:"type"`
	Date string `json:"date"`
	Data struct {
		Text string `json:"text"`
		Card struct {
			ID string `json:"id"`
		} `json:"card"`
	} `json:"data"`
	MemberCreator struct {
		FullName string `json:"fullName"`
		Username string `json:"username"`
	} `json:"memberCreator"`
}

// Limits on the raw export, checked before any translating. They're above
// the service's own (service.MaxImport*), which apply to what's imported.
const (
	maxRawActions     = 200000
	maxRawAttachments = 1000 // per card
	maxRawLabelsCard  = 1000 // per card
)

var notTrello = &service.ValidationError{Msg: "That file doesn't look like a Trello board export. In Trello, open the board's menu, choose Print, export, and share, then Export as JSON, and upload the file it saves."}

func tooLarge() error {
	return &service.ValidationError{Msg: "This board is larger than Chrysalis can import in one go. Try archiving or removing old cards first, then export it again."}
}

// Parse reads a Trello export and translates it. The returned report has
// everything except the counts, which come from the import itself.
func Parse(data []byte) (service.ImportBoard, Report, error) {
	if len(data) > MaxExportBytes {
		return service.ImportBoard{}, Report{}, &service.ValidationError{Msg: fmt.Sprintf("That file is too big to import (the limit is %d MB).", MaxExportBytes>>20)}
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // a byte-order mark from some editors
	var ex export
	if err := json.Unmarshal(data, &ex); err != nil || ex.Lists == nil || ex.Cards == nil {
		return service.ImportBoard{}, Report{}, notTrello
	}
	if len(*ex.Lists) > service.MaxImportColumns || len(*ex.Cards) > service.MaxImportCards ||
		len(ex.Labels) > service.MaxImportLabels || len(ex.Checklists) > service.MaxImportChecklists ||
		len(ex.Actions) > maxRawActions {
		return service.ImportBoard{}, Report{}, tooLarge()
	}
	items := 0
	for _, cl := range ex.Checklists {
		items += len(cl.CheckItems)
	}
	if items > service.MaxImportItems {
		return service.ImportBoard{}, Report{}, tooLarge()
	}
	for _, c := range *ex.Cards {
		if len(c.Attachments) > maxRawAttachments || len(c.IDLabels) > maxRawLabelsCard {
			return service.ImportBoard{}, Report{}, tooLarge()
		}
	}
	var m mapper
	return m.translate(ex)
}

// tally counts things for the report.
type tally struct {
	closedLists, cardsInClosedLists, orphanCards          int
	truncated, startAfterDue, uploads, badLinks           int
	coverImages, members, reminders, locations, longDescs int
	pluginData, customFields, customValues, itemExtras    int
	emptyItems, history, comments, colorVariants          int
}

type mapper struct {
	t tally
}

func (m *mapper) translate(ex export) (service.ImportBoard, Report, error) {
	out := service.ImportBoard{
		Name:    m.name(ex.Name, service.MaxBoardNameLen, "Imported from Trello"),
		Labels:  []service.ImportLabel{},
		Columns: []service.ImportColumn{},
	}
	var rep Report

	// Background: a plain color comes across; photos don't.
	if ex.Prefs.BackgroundImage != "" {
		rep.Skipped = append(rep.Skipped, "The board's background photo. You can choose a background in the board's settings.")
	} else if hex := strings.ToLower(ex.Prefs.BackgroundColor); hexColor.MatchString(hex) {
		out.Background = &service.Background{Type: "color", Value: hex}
	}

	labelIdx := map[string]int{}
	for _, l := range ex.Labels {
		if _, dup := labelIdx[l.ID]; dup {
			continue
		}
		color, variant := LabelColor(l.Color)
		if variant {
			m.t.colorVariants++
		}
		name := strings.TrimSpace(l.Name)
		if name == "" {
			name = colorName(l.Color)
		}
		labelIdx[l.ID] = len(out.Labels)
		out.Labels = append(out.Labels, service.ImportLabel{Name: m.name(name, service.MaxLabelNameLen, ""), Color: color})
	}

	lists := append([]list(nil), *ex.Lists...)
	sort.SliceStable(lists, func(i, j int) bool { return lists[i].Pos < lists[j].Pos })
	colIdx := map[string]int{}
	for _, l := range lists {
		if _, dup := colIdx[l.ID]; dup {
			continue
		}
		colIdx[l.ID] = len(out.Columns)
		if l.Closed {
			m.t.closedLists++
		}
		out.Columns = append(out.Columns, service.ImportColumn{
			Name: m.name(l.Name, service.MaxColumnNameLen, "Untitled list"), Collapsed: l.Closed, Cards: []service.ImportCard{},
		})
	}

	checklists := map[string][]checklist{}
	for _, cl := range ex.Checklists {
		checklists[cl.IDCard] = append(checklists[cl.IDCard], cl)
	}
	comments := map[string][]action{}
	for _, a := range ex.Actions {
		if a.Type == "commentCard" {
			comments[a.Data.Card.ID] = append(comments[a.Data.Card.ID], a)
		} else {
			m.t.history++
		}
	}
	if len(ex.PluginData) > 0 {
		m.t.pluginData += len(ex.PluginData)
	}
	m.t.customFields = len(ex.CustomFields)

	cards := append([]card(nil), *ex.Cards...)
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].Pos < cards[j].Pos })
	seen := map[string]bool{}
	for _, c := range cards {
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		ci, ok := colIdx[c.IDList]
		if !ok {
			m.t.orphanCards++
			continue
		}
		col := &out.Columns[ci]
		ic := m.card(c, labelIdx, checklists[c.ID], comments[c.ID])
		if col.Collapsed {
			// Cards in an archived list stay out of sight, in the archive.
			ic.Archived = true
			m.t.cardsInClosedLists++
		}
		col.Cards = append(col.Cards, ic)
	}

	rep.Skipped = append(rep.Skipped, m.skipped()...)
	rep.Notes = m.notes(len(ex.Actions))
	rep.Imported = out.Count()
	if rep.Skipped == nil {
		rep.Skipped = []string{}
	}
	return out, rep, nil
}

func (m *mapper) card(c card, labelIdx map[string]int, cls []checklist, acts []action) service.ImportCard {
	out := service.ImportCard{
		Title:     m.name(c.Name, service.MaxCardTitleLen, "Untitled card"),
		Completed: c.DueComplete,
		Archived:  c.Closed,
	}
	if doc := Markdown(c.Desc); doc != nil {
		raw, err := json.Marshal(doc)
		if err == nil && len(raw) <= service.MaxDescriptionBytes {
			out.Description = raw
		} else {
			m.t.longDescs++
		}
	}

	var dueDate string
	if c.Due != nil {
		if t, err := time.Parse(time.RFC3339, *c.Due); err == nil {
			s := t.UTC().Format(time.RFC3339)
			out.DueAt = &s
			dueDate = t.UTC().Format(time.DateOnly)
		}
	}
	if c.Start != nil {
		if t, err := time.Parse(time.RFC3339, *c.Start); err == nil {
			s := t.UTC().Format(time.DateOnly)
			if dueDate != "" && s > dueDate {
				m.t.startAfterDue++
			} else {
				out.StartAt = &s
			}
		}
	}
	if c.DueReminder != nil && *c.DueReminder >= 0 && out.DueAt != nil {
		m.t.reminders++
	}

	seenLabel := map[int]bool{}
	for _, id := range c.IDLabels {
		if i, ok := labelIdx[id]; ok && !seenLabel[i] {
			seenLabel[i] = true
			out.Labels = append(out.Labels, i)
		}
	}

	if c.Cover != nil {
		if c.Cover.Color != nil && *c.Cover.Color != "" {
			out.CoverColor, _ = LabelColor(c.Cover.Color)
		} else if c.Cover.IDAttachment != nil || c.Cover.IDUploadedBackground != nil {
			m.t.coverImages++
		}
	}

	sort.SliceStable(cls, func(i, j int) bool { return cls[i].Pos < cls[j].Pos })
	for _, cl := range cls {
		items := append([]checkItem(nil), cl.CheckItems...)
		sort.SliceStable(items, func(i, j int) bool { return items[i].Pos < items[j].Pos })
		ic := service.ImportChecklist{Title: m.name(cl.Name, service.MaxChecklistTitleLen, "Checklist"), Items: []service.ImportItem{}}
		for _, it := range items {
			if strings.TrimSpace(it.Name) == "" {
				m.t.emptyItems++
				continue
			}
			if it.Due != nil || it.IDMember != nil {
				m.t.itemExtras++
			}
			ic.Items = append(ic.Items, service.ImportItem{Text: m.name(it.Name, service.MaxItemTextLen, ""), Done: it.State == "complete"})
		}
		out.Checklists = append(out.Checklists, ic)
	}

	// The export lists actions newest first; comments read oldest first.
	sort.SliceStable(acts, func(i, j int) bool { return acts[i].Date < acts[j].Date })
	for _, a := range acts {
		if body := commentBody(a); body != nil {
			out.Comments = append(out.Comments, service.ImportComment{Body: body, CreatedAt: a.Date})
			m.t.comments++
		}
	}

	for _, a := range c.Attachments {
		if _, err := service.CleanLinkURL(a.URL); err != nil {
			m.t.badLinks++
			continue
		}
		if a.IsUpload {
			m.t.uploads++
		}
		out.Links = append(out.Links, service.ImportLink{URL: strings.TrimSpace(a.URL), Title: m.name(a.Name, 200, "")})
	}

	if len(c.IDMembers) > 0 {
		m.t.members++
	}
	if len(c.CustomFieldItems) > 0 {
		m.t.customValues++
	}
	m.t.pluginData += len(c.PluginData)
	if (len(c.Coordinates) > 0 && string(c.Coordinates) != "null") || (c.Address != nil && *c.Address != "") {
		m.t.locations++
	}
	return out
}

// commentBody is the comment as a document, starting with who wrote it.
func commentBody(a action) json.RawMessage {
	text := strings.TrimSpace(a.Data.Text)
	if text == "" {
		return nil
	}
	who := strings.TrimSpace(a.MemberCreator.FullName)
	if who == "" {
		who = strings.TrimSpace(a.MemberCreator.Username)
	}
	if who == "" {
		who = "someone"
	}
	who = truncate(who, 100)
	doc := Markdown(text)
	if doc == nil {
		return nil
	}
	doc.Content = append([]Node{{Type: "paragraph", Content: []Node{{Type: "text", Text: "Originally by " + who + ":"}}}}, doc.Content...)
	raw, err := json.Marshal(doc)
	if err != nil || len(raw) > service.MaxDescriptionBytes {
		return nil
	}
	return raw
}

// name trims s and fits it within max characters, or uses fallback when
// it's empty.
func (m *mapper) name(s string, max int, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	if utf8.RuneCountInString(s) > max {
		m.t.truncated++
		return truncate(s, max)
	}
	return s
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

var hexColor = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// trelloColors maps Trello's label and cover colors onto the nearest
// Chrysalis palette key (service.LabelColors). Trello's darker and lighter
// shades (green_dark, green_light, ...) use their base color.
var trelloColors = map[string]string{
	"green":  "green",
	"yellow": "yellow",
	"orange": "orange",
	"red":    "red",
	"purple": "purple",
	"blue":   "blue",
	"sky":    "sky",
	"lime":   "lime",
	"pink":   "pink",
	"black":  "gray",
}

// LabelColor maps a Trello color to a palette key; no color, or one we
// don't know, becomes gray. variant reports a darker or lighter shade.
func LabelColor(c *string) (key string, variant bool) {
	if c == nil {
		return "gray", false
	}
	base := *c
	for _, suffix := range []string{"_dark", "_light"} {
		if b, ok := strings.CutSuffix(base, suffix); ok {
			base, variant = b, true
		}
	}
	if k, ok := trelloColors[base]; ok {
		return k, variant
	}
	return "gray", false
}

// colorName is a readable name for a Trello color, for labels without one.
func colorName(c *string) string {
	if c == nil || *c == "" {
		return "gray"
	}
	base, shade := *c, ""
	if b, ok := strings.CutSuffix(base, "_dark"); ok {
		base, shade = b, "dark "
	} else if b, ok := strings.CutSuffix(base, "_light"); ok {
		base, shade = b, "light "
	}
	return shade + base
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func (m *mapper) skipped() []string {
	t := m.t
	var out []string
	add := func(n int, s string) {
		if n > 0 {
			out = append(out, s)
		}
	}
	add(t.pluginData, fmt.Sprintf("Power-Up data (%s). Power-Ups have no Chrysalis equivalent.", plural(t.pluginData, "item", "items")))
	add(t.customFields+t.customValues, fmt.Sprintf("Custom fields (%s, with values on %s).", plural(t.customFields, "field", "fields"), plural(t.customValues, "card", "cards")))
	add(t.members, fmt.Sprintf("People assigned to %s. Cards here don't have members.", plural(t.members, "card", "cards")))
	add(t.reminders, fmt.Sprintf("Due-date reminders on %s.", plural(t.reminders, "card", "cards")))
	add(t.locations, fmt.Sprintf("Locations on %s.", plural(t.locations, "card", "cards")))
	add(t.coverImages, fmt.Sprintf("Cover images on %s. The pictures themselves came across as links.", plural(t.coverImages, "card", "cards")))
	add(t.itemExtras, fmt.Sprintf("Due dates and assignees on %s.", plural(t.itemExtras, "checklist item", "checklist items")))
	add(t.emptyItems, fmt.Sprintf("%s with no text.", plural(t.emptyItems, "checklist item", "checklist items")))
	add(t.badLinks, plural(t.badLinks, "attachment whose address isn't a web link.", "attachments whose addresses aren't web links."))
	add(t.orphanCards, fmt.Sprintf("%s whose list wasn't in the export.", plural(t.orphanCards, "card", "cards")))
	add(t.longDescs, fmt.Sprintf("%s too long to bring across.", plural(t.longDescs, "card description", "card descriptions")))
	add(t.startAfterDue, fmt.Sprintf("%s that fell after the card's due date.", plural(t.startAfterDue, "start date", "start dates")))
	add(t.history, "Trello's activity history (moves, edits and so on). Only comments come across.")
	return out
}

func (m *mapper) notes(actions int) []string {
	t := m.t
	out := []string{}
	switch {
	case t.closedLists == 1:
		out = append(out, "1 list was archived in Trello. It's here as a collapsed column, and its cards are in the board's archive.")
	case t.closedLists > 1:
		out = append(out, fmt.Sprintf("%d lists were archived in Trello. They're here as collapsed columns, and their cards are in the board's archive.", t.closedLists))
	}
	if t.comments > 0 {
		out = append(out, "Comments are shown under your name, each starting with who originally wrote it, and keep their original dates.")
	}
	if actions >= 1000 {
		out = append(out, "Trello's export holds only the most recent 1,000 actions, so some older comments may be missing.")
	}
	if t.uploads > 0 {
		out = append(out, fmt.Sprintf("%s uploaded to Trello came across as links. Opening them may need you to be signed in to Trello.",
			plural(t.uploads, "file", "files")))
	}
	if t.colorVariants > 0 {
		out = append(out, "Trello's darker and lighter label colors use the nearest Chrysalis color.")
	}
	switch {
	case t.truncated == 1:
		out = append(out, "1 name was longer than Chrysalis allows, so it was shortened.")
	case t.truncated > 1:
		out = append(out, fmt.Sprintf("%d names were longer than Chrysalis allows, so they were shortened.", t.truncated))
	}
	return out
}
