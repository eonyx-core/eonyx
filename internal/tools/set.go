package tools

import (
	"net/http"

	"eonyx/internal/shell"
)

// DefaultSet builds eonyx's standard tool registry: file and search tools
// rooted at workdir, a bash tool backed by the shared shell session, and a
// fetch tool using httpClient. When sandboxRoot is non-empty, the file tools
// (read/write/edit/multi_edit/ls) refuse paths outside it. Registration order
// is the order tools are advertised to the model.
func DefaultSet(workdir, sandboxRoot string, sh *shell.Shell, httpClient *http.Client) *Registry {
	r := NewRegistry()
	r.Register(sandboxed(sandboxRoot, workdir, newReadTool(workdir)))
	r.Register(sandboxed(sandboxRoot, workdir, newWriteTool(workdir)))
	r.Register(sandboxed(sandboxRoot, workdir, newEditTool(workdir)))
	r.Register(sandboxed(sandboxRoot, workdir, newMultiEditTool(workdir)))
	r.Register(sandboxed(sandboxRoot, workdir, newLSTool(workdir)))
	r.Register(newPickerTool(workdir))
	r.Register(newGlobTool(workdir))
	r.Register(newGrepTool(workdir))
	r.Register(newBashTool(sh))
	r.Register(newFetchTool(httpClient))
	r.Register(newGitStatusTool(workdir))
	r.Register(newGitDiffTool(workdir))
	r.Register(newGitLogTool(workdir))
	r.Register(newGitCommitTool(workdir))
	r.Register(newTodoTool())
	return r
}

// ReadOnlySet builds a side-effect-free registry (read, ls, glob, grep, fetch)
// for a sub-agent: enough to explore the codebase and the web without any tool
// that writes, runs commands, or spawns further sub-agents. File tools are
// confined to sandboxRoot when it is non-empty.
func ReadOnlySet(workdir, sandboxRoot string, httpClient *http.Client) *Registry {
	r := NewRegistry()
	r.Register(sandboxed(sandboxRoot, workdir, newReadTool(workdir)))
	r.Register(sandboxed(sandboxRoot, workdir, newLSTool(workdir)))
	r.Register(newGlobTool(workdir))
	r.Register(newGrepTool(workdir))
	r.Register(newFetchTool(httpClient))
	return r
}
