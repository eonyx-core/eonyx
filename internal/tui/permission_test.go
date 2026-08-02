package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"eonyx/internal/config"
	"eonyx/internal/permission"
	"eonyx/internal/tools"
)

func TestAuthorizeToolStaticAllow(t *testing.T) {
	m := newTestModel()
	pol := permission.New([]string{"write"}, false)

	if err := m.authorizeTool(pol, false, "read", nil); err != nil {
		t.Errorf("safe tool read should pass: %v", err)
	}
	if err := m.authorizeTool(pol, false, "write", nil); err != nil {
		t.Errorf("allowlisted write should pass: %v", err)
	}
	// Not statically allowed and no UI attached → falls back to denial.
	if err := m.authorizeTool(pol, false, "bash", nil); err == nil {
		t.Error("bash should be denied when there is no UI to prompt")
	}
}

func TestPlanModeBlocksMutatingTools(t *testing.T) {
	m := newTestModel()
	// yolo would normally allow everything, but plan mode overrides it.
	pol := permission.New(nil, true)

	// Read-only planning tools stay allowed.
	for _, name := range []string{"read", "grep", "git_status", "fetch", "task", "todo_write"} {
		if err := m.authorizeTool(pol, true, name, nil); err != nil {
			t.Errorf("plan mode should allow %q: %v", name, err)
		}
	}
	// Writing and command tools are blocked despite yolo.
	for _, name := range []string{"write", "edit", "multi_edit", "bash", "git_commit"} {
		if err := m.authorizeTool(pol, true, name, nil); err == nil {
			t.Errorf("plan mode should block %q", name)
		}
	}
}

func TestTogglePlanMode(t *testing.T) {
	m := newTestModel()
	m.cfg = &config.Config{}
	m.registry = tools.NewRegistry()
	m.togglePlan()
	if !m.planMode {
		t.Fatal("/plan should turn plan mode on")
	}
	m.togglePlan()
	if m.planMode {
		t.Error("/plan should turn plan mode off again")
	}
}

func TestPermissionPromptAllowOnce(t *testing.T) {
	m := newTestModel()
	respond := make(chan permDecision, 1)
	m.openPermissionPrompt(permissionRequestMsg{name: "bash", input: []byte(`{"command":"ls"}`), respond: respond})
	if m.dialog != dialogPermission || len(m.dialogOptions) != 3 {
		t.Fatalf("permission dialog not shown: dialog=%d options=%d", m.dialog, len(m.dialogOptions))
	}

	m.dialogCursor = 0 // Allow once
	m.selectDialogOption()
	if got := <-respond; got != permAllowOnce {
		t.Errorf("decision = %v, want allow once", got)
	}
	if m.dialog != dialogNone {
		t.Error("dialog should close after answering")
	}
	if m.sessionAllowed["bash"] {
		t.Error("allow once must not grant a session-wide allowance")
	}
}

func TestPermissionPromptAllowAlways(t *testing.T) {
	m := newTestModel()
	respond := make(chan permDecision, 1)
	m.openPermissionPrompt(permissionRequestMsg{name: "write", respond: respond})

	m.dialogCursor = 1 // Allow always
	m.selectDialogOption()
	if got := <-respond; got != permAllowAlways {
		t.Errorf("decision = %v, want allow always", got)
	}
	if !m.sessionAllowed["write"] {
		t.Error("allow always should grant write for the session")
	}
}

func TestPermissionPromptEscDenies(t *testing.T) {
	m := newTestModel()
	respond := make(chan permDecision, 1)
	m.openPermissionPrompt(permissionRequestMsg{name: "bash", respond: respond})

	m.handleDialogKey(tea.KeyMsg{Type: tea.KeyEsc})
	if got := <-respond; got != permDeny {
		t.Errorf("esc should deny, got %v", got)
	}
	if m.dialog != dialogNone {
		t.Error("esc should close the dialog")
	}
}

func TestPermissionAutoAllowWhenAlwaysGranted(t *testing.T) {
	m := newTestModel()
	m.sessionAllowed = map[string]bool{"bash": true}
	respond := make(chan permDecision, 1)

	m.Update(permissionRequestMsg{name: "bash", respond: respond})
	if got := <-respond; got != permAllowOnce {
		t.Errorf("an already-granted tool should auto-answer, got %v", got)
	}
	if m.dialog != dialogNone {
		t.Error("no dialog should open for an already-granted tool")
	}
}
