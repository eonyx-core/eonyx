package tui

import (
	"strings"
	"testing"
	"time"

	"eonyx/internal/llm"
)

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Second: "30s",
		5 * time.Minute:  "5m",
		90 * time.Minute: "1h30m",
		50 * time.Hour:   "2d",
	}
	for d, want := range cases {
		if got := formatDuration(d); got != want {
			t.Errorf("formatDuration(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestTimestampPrefixTogglesWithFlag(t *testing.T) {
	m := newTestModel()
	item := messageItem{role: llm.RoleUser, text: "hi", done: true,
		createdAt: time.Date(2026, 7, 3, 14, 5, 6, 0, time.Local)}

	if strings.Contains(m.renderItem(&item), "14:05:06") {
		t.Error("timestamp shown while the toggle is off")
	}
	m.showTimestamps = true
	if !strings.Contains(m.renderItem(&item), "14:05:06") {
		t.Error("timestamp not shown while the toggle is on")
	}

	// A turn restored from history (zero time) shows no timestamp even when on.
	restored := messageItem{role: llm.RoleUser, text: "old", done: true}
	if m.renderItem(&restored) != m.renderItemBody(&restored) {
		t.Error("restored turn with zero time should not get a timestamp prefix")
	}
}

func TestTimestampsSlashCommandToggles(t *testing.T) {
	m := newTestModel()
	m.runSlashCommand("/timestamps")
	if !m.showTimestamps {
		t.Fatal("/timestamps should turn timestamps on")
	}
	m.runSlashCommand("/timestamps")
	if m.showTimestamps {
		t.Error("/timestamps should turn timestamps off again")
	}
}
