// Package permission decides whether a tool may run. The decision here is
// static: read-only tools and those on the allowlist run freely, everything
// else is refused unless yolo mode is on. The TUI layers interactive prompting
// ("allow once / always / deny") on top of this for the refused case.
package permission

import "fmt"

// safeTools are read-only and never require permission.
var safeTools = map[string]bool{
	"read":       true,
	"ls":         true,
	"glob":       true,
	"grep":       true,
	"git_status": true,
	"git_diff":   true,
	"git_log":    true,
	"todo_write": true,
}

// Policy is a static allow/deny decision maker.
type Policy struct {
	yolo    bool
	allowed map[string]bool
}

// New builds a policy from the configured allowlist and the yolo flag. When
// yolo is true every tool is permitted.
func New(allowed []string, yolo bool) *Policy {
	set := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		set[name] = true
	}
	return &Policy{yolo: yolo, allowed: set}
}

// Authorize returns nil if the named tool may run, or an error explaining why
// it is refused and how to permit it.
func (p *Policy) Authorize(name string) error {
	if p.yolo || safeTools[name] || p.allowed[name] {
		return nil
	}
	return fmt.Errorf("permission denied for tool %q: re-run with --yolo or add %q to permissions.allowed_tools", name, name)
}
