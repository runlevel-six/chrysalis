// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// MaxDescriptionBytes bounds a description's stored JSON. Images are
// attachments referenced by ID, so text alone rarely comes close.
const MaxDescriptionBytes = 256 << 10

// Description is a card's rich-text description: ProseMirror JSON, or null
// when empty.
type Description struct {
	Doc       json.RawMessage `json:"doc"`
	Version   int64           `json:"version"`
	UpdatedAt string          `json:"updatedAt,omitempty"`
}

// ConflictError means the description changed since the client loaded it
// (docs/DESIGN.md §5.5). Current is what's stored now, so the client can
// offer "keep yours / use theirs / view both".
type ConflictError struct{ Current Description }

func (e *ConflictError) Error() string { return "description was edited elsewhere" }

func descriptionFromRow(r db.CardDescription) Description {
	d := Description{Doc: json.RawMessage("null"), Version: r.Version, UpdatedAt: r.UpdatedAt}
	if r.DocJson != "" {
		d.Doc = json.RawMessage(r.DocJson)
	}
	return d
}

func (s *Boards) Description(ctx context.Context, userID, cardID string) (Description, error) {
	if _, err := liveCard(ctx, s.st.Queries, userID, cardID, false); err != nil {
		return Description{}, err
	}
	r, err := s.st.GetCardDescription(ctx, cardID)
	if errors.Is(err, sql.ErrNoRows) {
		return Description{Doc: json.RawMessage("null")}, nil
	}
	if err != nil {
		return Description{}, err
	}
	return descriptionFromRow(r), nil
}

// cleanDoc checks that doc is null or a ProseMirror document, and compacts it.
// The server doesn't interpret the content beyond that: it's rendered by
// the editor's schema on the client, which drops anything it doesn't know.
func cleanDoc(doc json.RawMessage) (string, error) {
	doc = bytes.TrimSpace(doc)
	if len(doc) == 0 || string(doc) == "null" {
		return "", nil
	}
	if len(doc) > MaxDescriptionBytes {
		return "", &ValidationError{fmt.Sprintf("That description is too long to save (over %d KB).", MaxDescriptionBytes>>10)}
	}
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(doc, &head); err != nil || head.Type != "doc" {
		return "", &ValidationError{"The description must be an editor document."}
	}
	var out bytes.Buffer
	if err := json.Compact(&out, doc); err != nil {
		return "", &ValidationError{"The description must be valid JSON."}
	}
	return out.String(), nil
}

// SetDescription saves a new description if baseVersion is still current;
// otherwise it returns a *ConflictError and changes nothing.
func (s *Boards) SetDescription(ctx context.Context, userID, cardID string, doc json.RawMessage, baseVersion int64) (Description, error) {
	clean, err := cleanDoc(doc)
	if err != nil {
		return Description{}, err
	}
	var out Description
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := liveCard(ctx, q, userID, cardID, true); err != nil {
			return err
		}
		cur, err := q.GetCardDescription(ctx, cardID)
		var current int64
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return err
		default:
			current = cur.Version
		}
		if current != baseVersion {
			return &ConflictError{Current: descriptionFromRow(cur)}
		}
		row, err := q.UpsertCardDescription(ctx, db.UpsertCardDescriptionParams{
			CardID: cardID, DocJson: clean, Version: current + 1,
			UpdatedAt: store.FormatTime(s.now()), UpdatedBy: userID,
		})
		out = descriptionFromRow(row)
		return err
	})
	return out, err
}
