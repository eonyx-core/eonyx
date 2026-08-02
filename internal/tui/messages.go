package tui

// Internal tea.Msg types used to bridge agent events and UI commands back into
// the Bubble Tea update loop.

type agentDeltaMsg struct {
	delta string
}

// agentReasoningMsg carries a chunk of a reasoning model's thinking stream.
type agentReasoningMsg struct {
	delta string
}

// agentStepMsg reports the start of a tool round-trip: step is 1-based and
// maxSteps the configured limit, shown as a progress counter in the footer.
type agentStepMsg struct {
	step     int
	maxSteps int
}

type agentToolStartMsg struct {
	name  string
	input []byte
}

type agentToolEndMsg struct {
	name    string
	result  string
	isError bool
}

type agentDoneMsg struct {
	text string
	err  error
}

// shellResultMsg carries the result of a "!command" shell escape back into the
// update loop.
type shellResultMsg struct {
	cmd    string
	output string
	err    error
}

// ipResultMsg carries the result of the /ip public-address lookup.
type ipResultMsg struct {
	info string
	err  error
}

// blinkTickMsg drives the winking eye on the welcome screen.
type blinkTickMsg struct{}

// branchTickMsg schedules a git-branch refresh; branchMsg carries the result
// back into the update loop so the header tracks branch switches.
type branchTickMsg struct{}

type branchMsg struct {
	branch string
}

// dialogState enumerates the modal overlays the model can show.
type dialogState int

const (
	dialogNone dialogState = iota
	dialogModelPicker
	dialogSessionPicker
	dialogCommandPalette
	dialogFilePicker
	dialogDeleteConfirm
	dialogCapabilities
	dialogHelp
	dialogPermission
	dialogPlanApproval
)

// permDecision is the outcome of an interactive permission prompt.
type permDecision int

const (
	permDeny permDecision = iota
	permAllowOnce
	permAllowAlways
)

// permissionRequestMsg asks the UI to decide whether a tool may run. The agent
// goroutine blocks on respond until the user (or an existing "always" grant)
// answers; respond is buffered so the UI never blocks delivering the decision.
type permissionRequestMsg struct {
	name    string
	input   []byte
	respond chan permDecision
}
