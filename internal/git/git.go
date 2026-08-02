package git

import (
	"os/exec"
	"strings"
)

// Branch returns the current git branch for the repository containing dir, or an
// empty string if dir is not inside a git repository, HEAD is detached, or git
// is unavailable. It is a single, fast subprocess so callers can poll it to keep
// a status display live.
func Branch(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" || branch == "" {
		return ""
	}
	return branch
}
