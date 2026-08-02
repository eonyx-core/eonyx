package tui

import (
	"errors"
	"testing"

	"eonyx/internal/agent"
	"eonyx/internal/config"
	"eonyx/internal/tools"
)

func TestPlanApprovalOpensAfterPlanTurn(t *testing.T) {
	m := newTestModel()
	m.planMode = true
	m.Update(agentDoneMsg{text: "here is the plan"})
	if m.dialog != dialogPlanApproval {
		t.Fatalf("plan approval should open after a plan-mode turn, dialog=%d", m.dialog)
	}
}

func TestPlanApprovalNotShownOnError(t *testing.T) {
	m := newTestModel()
	m.planMode = true
	m.Update(agentDoneMsg{err: errors.New("boom")})
	if m.dialog == dialogPlanApproval {
		t.Error("no approval modal should appear for a failed turn")
	}
}

func TestPlanApprovalImplementExitsPlanMode(t *testing.T) {
	m := newTestModel()
	m.cfg = &config.Config{}
	m.registry = tools.NewRegistry()
	m.agent = agent.New(nil, tools.NewRegistry(), agent.Options{})
	m.planMode = true

	m.openPlanApproval()
	m.dialogCursor = 0 // Approve & implement
	cmd := m.selectDialogOption()

	if m.planMode {
		t.Error("approve should exit plan mode")
	}
	if cmd == nil {
		t.Error("approve & implement should start a turn")
	}
	if m.dialog != dialogNone {
		t.Error("dialog should close after approving")
	}
}

func TestPlanApprovalKeepPlanningStays(t *testing.T) {
	m := newTestModel()
	m.cfg = &config.Config{}
	m.registry = tools.NewRegistry()
	m.planMode = true

	m.openPlanApproval()
	m.dialogCursor = 2 // Keep planning
	m.selectDialogOption()

	if !m.planMode {
		t.Error("keep planning should stay in plan mode")
	}
	if m.dialog != dialogNone {
		t.Error("dialog should close")
	}
}
