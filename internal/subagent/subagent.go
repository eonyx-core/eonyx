// Package subagent provides the "task" tool: it delegates a self-contained
// piece of work to a scoped child agent that runs its own loop with a
// read-only tool set and returns a concise result. The child's registry omits
// the task tool itself, so sub-agents cannot spawn further sub-agents.
package subagent

import (
	"context"
	"encoding/json"
	"fmt"

	"eonyx/internal/agent"
	"eonyx/internal/llm"
	"eonyx/internal/tools"
)

// taskSystemSuffix is appended to the child's system prompt so it works
// autonomously and returns a final answer rather than asking questions.
const taskSystemSuffix = "\n\nYou are a sub-agent handling a single delegated task. Work autonomously with the read-only tools available (read, ls, glob, grep, fetch); you cannot write files, run commands, or delegate further. When done, reply with a concise, self-contained answer that fully addresses the task — it is returned verbatim to the agent that delegated it."

const schema = `{
  "type": "object",
  "properties": {
    "description": {"type": "string", "description": "The task for the sub-agent, self-contained with all context it needs. The sub-agent cannot ask follow-up questions."}
  },
  "required": ["description"]
}`

// NewTaskTool builds the task tool. The child agent uses prov, the read-only
// childTools registry, and baseSystem (plus a sub-agent directive) as its system
// prompt; contextTokens and maxSteps bound its context and loop.
func NewTaskTool(prov llm.Provider, childTools *tools.Registry, baseSystem string, contextTokens, maxSteps int) tools.Tool {
	system := baseSystem + taskSystemSuffix
	return tools.Tool{
		Name:        "task",
		Description: "Delegate a self-contained research or exploration task to a sub-agent that works autonomously with read-only tools (read, ls, glob, grep, fetch) and returns a concise result. Use it to investigate the codebase or gather information without cluttering the main conversation.",
		Schema:      json.RawMessage(schema),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Description string `json:"description"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if args.Description == "" {
				return "", fmt.Errorf("description is required")
			}
			child := agent.New(prov, childTools, agent.Options{
				History:       []llm.Message{llm.SystemText(system)},
				MaxSteps:      maxSteps,
				ContextTokens: contextTokens,
			})
			return child.Send(ctx, args.Description)
		},
	}
}
