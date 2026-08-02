package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// todoItem is one task in the model's working task list.
type todoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

// newTodoTool returns a task-tracking tool. The list is kept in a closure, so it
// persists across calls within one session (one tool registry) and starts empty
// for each new session. The model passes the full list each call, mirroring the
// way Claude Code's task list works.
func newTodoTool() Tool {
	var list []todoItem
	const schema = `{
  "type": "object",
  "properties": {
    "todos": {
      "type": "array",
      "description": "The full task list, replacing any previous one.",
      "items": {
        "type": "object",
        "properties": {
          "content": {"type": "string", "description": "What the task is."},
          "status": {"type": "string", "enum": ["pending", "in_progress", "completed"], "description": "Task state; defaults to pending."}
        },
        "required": ["content"]
      }
    }
  },
  "required": ["todos"]
}`
	return Tool{
		Name:        "todo_write",
		Description: "Record or update the task list for multi-step work so progress is visible. Pass the full list every time (it replaces the previous one); statuses are pending, in_progress, or completed. Returns the rendered checklist.",
		Schema:      json.RawMessage(schema),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Todos []todoItem `json:"todos"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			for i, t := range args.Todos {
				if strings.TrimSpace(t.Content) == "" {
					return "", fmt.Errorf("todo %d: content is required", i+1)
				}
				switch t.Status {
				case "pending", "in_progress", "completed":
				case "":
					args.Todos[i].Status = "pending"
				default:
					return "", fmt.Errorf("todo %d: invalid status %q (want pending, in_progress, or completed)", i+1, t.Status)
				}
			}
			list = args.Todos
			return formatTodos(list), nil
		},
	}
}

// formatTodos renders the task list as a checklist.
func formatTodos(list []todoItem) string {
	if len(list) == 0 {
		return "(no tasks)"
	}
	var b strings.Builder
	for _, t := range list {
		mark := "[ ]"
		switch t.Status {
		case "in_progress":
			mark = "[~]"
		case "completed":
			mark = "[x]"
		}
		fmt.Fprintf(&b, "%s %s\n", mark, t.Content)
	}
	return strings.TrimRight(b.String(), "\n")
}
