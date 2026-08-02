package session

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"eonyx/internal/llm"
)

// maxMessageLine bounds a single stored message line. Tool results can be large.
const maxMessageLine = 16 << 20

// JSONLStore stores each session as a JSONL file under <dir>/sessions and keeps
// session metadata in <dir>/sessions.json. It is safe for concurrent use.
type JSONLStore struct {
	mu  sync.Mutex
	dir string
}

var _ Store = (*JSONLStore)(nil)

// NewJSONLStore creates a store rooted at dir, creating the sessions directory.
func NewJSONLStore(dir string) (*JSONLStore, error) {
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		return nil, fmt.Errorf("session: create store dir: %w", err)
	}
	return &JSONLStore{dir: dir}, nil
}

func (s *JSONLStore) indexPath() string { return filepath.Join(s.dir, "sessions.json") }
func (s *JSONLStore) sessionPath(id string) string {
	return filepath.Join(s.dir, "sessions", id+".jsonl")
}

func (s *JSONLStore) Create(title string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := newID()
	if err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	sess := Session{ID: id, Title: title, CreatedAt: now, UpdatedAt: now}

	list, err := s.readIndex()
	if err != nil {
		return Session{}, err
	}
	list = append(list, sess)
	if err := s.writeIndex(list); err != nil {
		return Session{}, err
	}

	f, err := os.OpenFile(s.sessionPath(id), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return Session{}, fmt.Errorf("session: create file: %w", err)
	}
	_ = f.Close()
	return sess, nil
}

func (s *JSONLStore) Fork(id, title string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.readIndex()
	if err != nil {
		return Session{}, err
	}
	var src *Session
	for i := range list {
		if list[i].ID == id {
			src = &list[i]
			break
		}
	}
	if src == nil {
		return Session{}, ErrNotFound
	}

	data, err := os.ReadFile(s.sessionPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return Session{}, ErrNotFound
		}
		return Session{}, err
	}

	newID, err := newID()
	if err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	forked := Session{ID: newID, Title: title, CreatedAt: now, UpdatedAt: now, Messages: src.Messages}

	if err := os.WriteFile(s.sessionPath(newID), data, 0o644); err != nil {
		return Session{}, fmt.Errorf("session: write fork: %w", err)
	}
	list = append(list, forked)
	if err := s.writeIndex(list); err != nil {
		return Session{}, err
	}
	return forked, nil
}

func (s *JSONLStore) List() ([]Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.readIndex()
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].UpdatedAt.After(list[j].UpdatedAt)
	})
	return list, nil
}

func (s *JSONLStore) Get(id string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.readIndex()
	if err != nil {
		return Session{}, err
	}
	for _, sess := range list {
		if sess.ID == id {
			return sess, nil
		}
	}
	return Session{}, ErrNotFound
}

func (s *JSONLStore) Messages(id string) ([]llm.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.sessionPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer f.Close()

	var msgs []llm.Message
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxMessageLine)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		m, err := decodeMessage(line)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("session: read messages: %w", err)
	}
	return msgs, nil
}

func (s *JSONLStore) Append(id string, msg llm.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	line, err := encodeMessage(msg)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(s.sessionPath(id), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("session: append message: %w", err)
	}

	return s.touch(id)
}

func (s *JSONLStore) Reset(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.readIndex()
	if err != nil {
		return err
	}
	found := false
	for i := range list {
		if list[i].ID == id {
			list[i].Messages = 0
			list[i].UpdatedAt = time.Now().UTC()
			found = true
			break
		}
	}
	if !found {
		return ErrNotFound
	}

	f, err := os.OpenFile(s.sessionPath(id), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("session: reset file: %w", err)
	}
	_ = f.Close()

	return s.writeIndex(list)
}

func (s *JSONLStore) Rename(id, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.readIndex()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID == id {
			list[i].Title = title
			list[i].UpdatedAt = time.Now().UTC()
			return s.writeIndex(list)
		}
	}
	return ErrNotFound
}

func (s *JSONLStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.readIndex()
	if err != nil {
		return err
	}
	kept := list[:0]
	found := false
	for _, sess := range list {
		if sess.ID == id {
			found = true
			continue
		}
		kept = append(kept, sess)
	}
	if !found {
		return ErrNotFound
	}
	if err := s.writeIndex(kept); err != nil {
		return err
	}
	if err := os.Remove(s.sessionPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// touch increments the message count and updates the timestamp of a session in
// the index. The caller holds the lock.
func (s *JSONLStore) touch(id string) error {
	list, err := s.readIndex()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID == id {
			list[i].Messages++
			list[i].UpdatedAt = time.Now().UTC()
			return s.writeIndex(list)
		}
	}
	return ErrNotFound
}

func (s *JSONLStore) readIndex() ([]Session, error) {
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("session: read index: %w", err)
	}
	var list []Session
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("session: parse index: %w", err)
	}
	return list, nil
}

// writeIndex writes the index atomically via a temp file and rename.
func (s *JSONLStore) writeIndex(list []Session) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.indexPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("session: write index: %w", err)
	}
	if err := os.Rename(tmp, s.indexPath()); err != nil {
		return fmt.Errorf("session: replace index: %w", err)
	}
	return nil
}

// newID returns a sortable, unique session id: a UTC timestamp plus random hex.
func newID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("session: generate id: %w", err)
	}
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b[:]), nil
}
