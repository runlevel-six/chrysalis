// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
)

// Comment is one comment on a card. Writing comments is Phase 2; for now
// they come from imports, and the API can read them.
type Comment struct {
	ID         string          `json:"id"`
	CardID     string          `json:"cardId"`
	AuthorID   string          `json:"authorId"`
	AuthorName string          `json:"authorName"`
	Body       json.RawMessage `json:"body"` // ProseMirror JSON
	CreatedAt  string          `json:"createdAt"`
	UpdatedAt  string          `json:"updatedAt"`
	Version    int64           `json:"version"`
}

// Comments lists a card's comments, oldest first, for any member of its
// board.
func (s *Boards) Comments(ctx context.Context, userID, cardID string) ([]Comment, error) {
	if _, err := liveCard(ctx, s.st.Queries, userID, cardID, false); err != nil {
		return nil, err
	}
	rows, err := s.st.ListComments(ctx, cardID)
	if err != nil {
		return nil, err
	}
	out := make([]Comment, 0, len(rows))
	for _, r := range rows {
		out = append(out, Comment{
			ID: r.ID, CardID: r.CardID, AuthorID: r.AuthorID, AuthorName: r.AuthorName,
			Body: json.RawMessage(r.BodyJson), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version,
		})
	}
	return out, nil
}
