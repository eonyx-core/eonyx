package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"eonyx/internal/llm"
)

const (
	// summarizeFraction is the share of the context window that, once the
	// estimated history exceeds it, triggers summarization.
	summarizeFraction = 0.8
	// keepRecentTurns is how many of the most recent user-led turns are kept
	// verbatim; everything older is folded into the running summary.
	keepRecentTurns = 3
)

// maybeSummarize compresses older history into the system prompt when the
// estimated token count nears the context window. It is a no-op when
// summarization is disabled, the history is small, or there are too few turns
// to compact.
func (a *Agent) maybeSummarize(ctx context.Context) error {
	if a.contextTokens <= 0 {
		return nil
	}
	if estimateTokens(a.history) <= int(float64(a.contextTokens)*summarizeFraction) {
		return nil
	}
	return a.summarize(ctx)
}

// summarize replaces the older portion of the conversation with a model-written
// summary folded into the system prompt. It cuts only on user-message
// boundaries so tool_use/tool_result pairs are never split.
func (a *Agent) summarize(ctx context.Context) error {
	start := 0
	if len(a.history) > 0 && a.history[0].Role == llm.RoleSystem {
		start = 1
	}

	var userTurns []int
	for i := start; i < len(a.history); i++ {
		if a.history[i].Role == llm.RoleUser {
			userTurns = append(userTurns, i)
		}
	}
	if len(userTurns) <= keepRecentTurns {
		return nil // not enough whole turns to compact safely
	}
	boundary := userTurns[len(userTurns)-keepRecentTurns]

	older := a.history[start:boundary]
	recent := a.history[boundary:]

	slog.Info("summarizing history", "olderMessages", len(older), "keepTurns", keepRecentTurns)
	summary, err := a.requestSummary(ctx, older)
	if err != nil {
		return err
	}

	system := a.baseSystem
	if system != "" {
		system += "\n\n"
	}
	system += "# Summary of earlier conversation\n\n" + summary

	compacted := make([]llm.Message, 0, len(recent)+1)
	compacted = append(compacted, llm.SystemText(system))
	compacted = append(compacted, recent...)
	a.history = compacted
	return nil
}

// requestSummary asks the model to summarize the given messages (plus any
// existing summary in the current system prompt) into an updated running
// summary. It makes a tool-free completion and returns the produced text.
func (a *Agent) requestSummary(ctx context.Context, older []llm.Message) (string, error) {
	var b strings.Builder
	b.WriteString("Summarize the conversation so far into a concise running summary. ")
	b.WriteString("Preserve decisions made, important facts, files created or changed, and any open tasks. ")
	b.WriteString("This summary will replace the older messages as context, so keep everything still relevant.\n\n")
	if len(a.history) > 0 && a.history[0].Role == llm.RoleSystem {
		if prior := renderMessage(a.history[0]); prior != "" {
			b.WriteString("Current context:\n")
			b.WriteString(prior)
			b.WriteString("\n\n")
		}
	}
	b.WriteString("Conversation to summarize:\n")
	for _, m := range older {
		if text := renderMessage(m); text != "" {
			b.WriteString(text)
			b.WriteString("\n")
		}
	}

	ch, err := a.provider.Stream(ctx, []llm.Message{llm.UserText(b.String())}, nil)
	if err != nil {
		return "", fmt.Errorf("agent: summarize: %w", err)
	}
	var summary strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case llm.EventTextDelta:
			summary.WriteString(e.Text)
		case llm.EventUsage:
			a.inTokens += e.InputTokens
			a.outTokens += e.OutputTokens
		case llm.EventError:
			return "", fmt.Errorf("agent: summarize: %w", e.Err)
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return strings.TrimSpace(summary.String()), nil
}

// renderMessage formats a message as plain text for the summarizer.
func renderMessage(m llm.Message) string {
	var parts []string
	for _, part := range m.Content {
		switch p := part.(type) {
		case llm.Text:
			parts = append(parts, p.Text)
		case llm.ToolCall:
			parts = append(parts, fmt.Sprintf("[called %s %s]", p.Name, string(p.Input)))
		case llm.ToolResult:
			parts = append(parts, fmt.Sprintf("[tool result: %s]", truncateText(p.Content, 500)))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return string(m.Role) + ": " + strings.Join(parts, " ")
}

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// estimateTokens approximates the token count of messages as characters / 4.
func estimateTokens(msgs []llm.Message) int {
	chars := 0
	for _, m := range msgs {
		for _, part := range m.Content {
			switch p := part.(type) {
			case llm.Text:
				chars += len(p.Text)
			case llm.ToolCall:
				chars += len(p.Name) + len(p.Input)
			case llm.ToolResult:
				chars += len(p.Content)
			}
		}
	}
	return chars / 4
}
