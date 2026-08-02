package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"eonyx/internal/config"
)

func TestFooterShowsTokenRate(t *testing.T) {
	m := newTestModel()
	m.isRunning = true
	m.runStart = time.Now().Add(-2 * time.Second)
	m.streamChars = 800 // ~200 tokens over 2s ≈ 100 tok/s
	got := m.footerStatus()
	if !strings.Contains(got, "tok/s") {
		t.Errorf("footer should show a token rate, got %q", got)
	}
	if !strings.Contains(got, "100") {
		t.Errorf("footer rate = %q, want ~100 tok/s", got)
	}
}

func TestFooterNoRateBeforeSignal(t *testing.T) {
	m := newTestModel()
	m.isRunning = true
	m.runStart = time.Now()
	m.streamChars = 0
	if strings.Contains(m.footerStatus(), "tok/s") {
		t.Error("no token rate expected before any text streams")
	}
}

func TestCompactHeaderFitsNarrowWidth(t *testing.T) {
	m := newTestModel()
	m.cfg = &config.Config{}
	m.width = 40
	m.title = "Session 1"
	m.modelID = "a-fairly-long-model-name"
	for _, line := range strings.Split(m.renderHeader(), "\n") {
		if w := ansi.StringWidth(line); w > m.width {
			t.Errorf("compact header line too wide (%d > %d): %q", w, m.width, line)
		}
	}
}

func TestCompactWelcomeFitsNarrowWidth(t *testing.T) {
	m := newTestModel()
	m.cfg = &config.Config{}
	m.width = 40
	for _, line := range strings.Split(m.renderWelcome(), "\n") {
		if w := ansi.StringWidth(line); w > m.width {
			t.Errorf("compact welcome line too wide (%d > %d): %q", w, m.width, line)
		}
	}
}
