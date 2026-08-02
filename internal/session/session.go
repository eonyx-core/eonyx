// Package session persists conversations so they can be resumed. History is
// stored as JSONL — one message per line — behind the Store interface, with a
// separate index of session metadata. The interface lets a different backend
// (e.g. SQLite) replace the JSONL implementation without touching callers.
package session

import (
	"errors"
	"time"

	"eonyx/internal/llm"
)

// ErrNotFound is returned when a session id does not exist.
var ErrNotFound = errors.New("session not found")

// Session is the metadata for one stored conversation.
type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Messages is the number of messages stored for the session.
	Messages int `json:"messages"`
}

// Store persists and retrieves conversations.
type Store interface {
	// Create starts a new, empty session and returns its metadata.
	Create(title string) (Session, error)
	// Fork copies an existing session's messages into a new session with the
	// given title, so a conversation can branch without affecting the original.
	Fork(id, title string) (Session, error)
	// List returns all sessions, most recently updated first.
	List() ([]Session, error)
	// Get returns one session's metadata.
	Get(id string) (Session, error)
	// Messages returns a session's messages in order.
	Messages(id string) ([]llm.Message, error)
	// Append adds a message to a session and updates its metadata.
	Append(id string, msg llm.Message) error
	// Reset clears a session's messages, keeping its id and title.
	Reset(id string) error
	// Rename changes a session's title.
	Rename(id, title string) error
	// Delete removes a session and its messages.
	Delete(id string) error
}
