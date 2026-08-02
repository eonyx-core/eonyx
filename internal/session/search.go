package session

import (
	"errors"
	"strings"

	"eonyx/internal/llm"
)

// SearchHit is a session that matched a query, with a short snippet showing the
// match in context.
type SearchHit struct {
	Session Session
	Snippet string
}

// Search returns the sessions whose title or any message text contains query
// (case-insensitive), most recently updated first. Each hit carries a snippet of
// the first matching message, or the title when the title itself matched. An
// empty query matches nothing.
func Search(store Store, query string) ([]SearchHit, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	list, err := store.List()
	if err != nil {
		return nil, err
	}
	var hits []SearchHit
	for _, sess := range list {
		if strings.Contains(strings.ToLower(sess.Title), q) {
			hits = append(hits, SearchHit{Session: sess, Snippet: sess.Title})
			continue
		}
		msgs, err := store.Messages(sess.ID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		if snip, ok := matchSnippet(msgs, q); ok {
			hits = append(hits, SearchHit{Session: sess, Snippet: snip})
		}
	}
	return hits, nil
}

// matchSnippet scans a conversation for the lowercased query, skipping the
// system turn, and returns a snippet around the first match.
func matchSnippet(msgs []llm.Message, q string) (string, bool) {
	for _, m := range msgs {
		if m.Role == llm.RoleSystem {
			continue
		}
		for _, part := range m.Content {
			t, ok := part.(llm.Text)
			if !ok {
				continue
			}
			if i := strings.Index(strings.ToLower(t.Text), q); i >= 0 {
				return snippetAround(t.Text, i, len(q)), true
			}
		}
	}
	return "", false
}

// snippetAround returns up to ~30 characters of context on each side of the
// match at byte offset i, collapsing newlines and marking truncation.
func snippetAround(text string, i, qlen int) string {
	const pad = 30
	start := i - pad
	if start < 0 {
		start = 0
	}
	end := i + qlen + pad
	if end > len(text) {
		end = len(text)
	}
	snip := strings.TrimSpace(strings.ReplaceAll(text[start:end], "\n", " "))
	if start > 0 {
		snip = "…" + snip
	}
	if end < len(text) {
		snip += "…"
	}
	return snip
}
