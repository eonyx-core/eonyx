package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eonyx-core/eonyx/internal/agent"
	"github.com/eonyx-core/eonyx/internal/config"
	"github.com/eonyx-core/eonyx/internal/hooks"
	"github.com/eonyx-core/eonyx/internal/license"
	"github.com/eonyx-core/eonyx/internal/llm"
	"github.com/eonyx-core/eonyx/internal/mcp"
	"github.com/eonyx-core/eonyx/internal/permission"
	"github.com/eonyx-core/eonyx/internal/prompt"
	"github.com/eonyx-core/eonyx/internal/provider"
	"github.com/eonyx-core/eonyx/internal/report"
	"github.com/eonyx-core/eonyx/internal/session"
	"github.com/eonyx-core/eonyx/internal/shell"
	"github.com/eonyx-core/eonyx/internal/skills"
	"github.com/eonyx-core/eonyx/internal/subagent"
	"github.com/eonyx-core/eonyx/internal/tools"
)

func newRunCmd() *cobra.Command {
	var modelID string
	var sessionID string
	var continueLast bool
	var yolo bool
	var jsonOut bool
	var systemPromptFlag string

	cmd := &cobra.Command{
		Use:   "run [prompt]",
		Short: "Run the agent headlessly on a single prompt",
		Long: "run executes the agent loop on the given prompt and streams the result,\n" +
			"without entering the interactive TUI. The prompt may be passed as arguments\n" +
			"or piped on stdin (e.g. `cat bug.md | eonyx run`). The conversation is\n" +
			"saved as a session; use --continue or --session to resume a previous one.\n" +
			"\n" +
			"Exit codes:\n" +
			"  0    success\n" +
			"  1    generic error (config, provider setup, persistence)\n" +
			"  2    usage error (bad flags or missing prompt)\n" +
			"  3    API/model error while streaming\n" +
			"  4    completed, but a tool call was blocked by the permission gate\n" +
			"  5    stopped at the max_steps tool round-trip limit\n" +
			"  130  interrupted (Ctrl-C)",
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			userPrompt, err := resolvePrompt(cmd, args)
			if err != nil {
				return err
			}
			if err := license.Ensure(cmd.Context()); err != nil {
				return err
			}

			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			if path, created, err := config.EnsureDefaultConfig(wd); err != nil {
				return err
			} else if created {
				fmt.Fprintf(cmd.ErrOrStderr(), "Created default config at %s — runs against a local Ollama model by default; edit it to switch providers.\n", path)
			}
			cfg, err := config.Load(wd)
			if err != nil {
				return err
			}
			defer initLogging(cfg, wd)()
			prov, err := provider.For(cfg, modelID, nil)
			if err != nil {
				return err
			}

			store, err := session.NewJSONLStore(sessionDir(cfg, wd))
			if err != nil {
				return err
			}

			skillDirs := skills.DefaultDirs(cfg.ProjectRoot())
			skillDirs = append(skillDirs, cfg.Skills...)
			skillSet := skills.Discover(skillDirs)

			mcpConfigs := make([]mcp.ServerConfig, len(cfg.MCP))
			for i, c := range cfg.MCP {
				mcpConfigs[i] = mcp.ServerConfig(c)
			}
			mcpClients, mcpErrs := mcp.DialAll(cmd.Context(), mcpConfigs)
			defer mcp.CloseAll(mcpClients)
			for _, e := range mcpErrs {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: mcp: %v\n", e)
			}

			sh, err := shell.New(wd)
			if err != nil {
				return err
			}
			hk := hooks.New(sh, cfg.Hooks)
			registry := tools.DefaultSet(wd, cfg.FileSandboxRoot(wd), sh, nil)
			registry.Register(skills.NewLoadTool(skillSet))
			if cfg.Report.DatabaseURL != "" {
				for _, t := range report.NewTools(cfg.Report.DatabaseURL) {
					registry.Register(t)
				}
			}
			for _, cli := range mcpClients {
				if err := mcp.RegisterFromClient(cmd.Context(), registry, cli); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: mcp: %v\n", err)
				}
			}

			systemPrompt := resolveSystemPrompt(cmd, cfg, wd, skillSet, systemPromptFlag)
			childTools := tools.ReadOnlySet(wd, cfg.FileSandboxRoot(wd), nil)
			registry.Register(subagent.NewTaskTool(prov, childTools, systemPrompt, contextWindow(cfg, modelID), cfg.MaxSteps))
			sess, history, err := resolveSession(store, sessionID, continueLast, userPrompt, systemPrompt)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			policy := permission.New(cfg.Permissions.AllowedTools, yolo)
			var denied bool

			var events agent.Events
			var jw *jsonEmitter
			reasoningSeen, reasoningClosed := false, false
			if jsonOut {
				jw = &jsonEmitter{enc: json.NewEncoder(out)}
				events = jw.events()
			} else {
				events = agent.Events{
					Text: func(delta string) {
						if reasoningSeen && !reasoningClosed {
							fmt.Fprintln(errOut)
							reasoningClosed = true
						}
						fmt.Fprint(out, delta)
					},
					Reasoning: func(delta string) {
						if !reasoningSeen {
							fmt.Fprint(errOut, "\n[thinking] ")
							reasoningSeen = true
						}
						fmt.Fprint(errOut, delta)
					},
					ToolStart: func(name string, input []byte) {
						fmt.Fprintf(errOut, "\n→ %s %s\n", name, truncate(string(input), 200))
					},
					ToolEnd: func(name, result string, isError bool) {
						if isError {
							fmt.Fprintf(errOut, "← %s error: %s\n", name, truncate(result, 200))
							return
						}
						if name == "write" || name == "edit" || name == "multi_edit" {
							fmt.Fprintf(errOut, "← %s\n", truncate(firstLine(result), 200))
						}
					},
				}
			}

			a := agent.New(prov, registry, agent.Options{
				History:       history,
				ContextTokens: contextWindow(cfg, modelID),
				MaxSteps:      cfg.MaxSteps,
				Persist:       func(m llm.Message) error { return store.Append(sess.ID, m) },
				Authorize: func(name string, _ []byte) error {
					if err := policy.Authorize(name); err != nil {
						denied = true
						return err
					}
					return nil
				},
				Hooks:  hk,
				Events: events,
			})

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			final, err := a.Send(ctx, userPrompt)
			if err != nil {
				return classifySendError(err)
			}

			in, outTok := a.Usage()
			cached := a.Cached()
			if jsonOut {
				jw.emit(jsonEvent{Type: "result", Text: final, InputTokens: in, OutputTokens: outTok, CachedTokens: cached, Session: sess.ID})
			} else {
				if !strings.HasSuffix(final, "\n") {
					fmt.Fprintln(out)
				}
				cachedNote := ""
				if cached > 0 {
					cachedNote = fmt.Sprintf(", %d cached", cached)
				}
				fmt.Fprintf(errOut, "\n[tokens: %d in%s, %d out] [session: %s]\n", in, cachedNote, outTok, sess.ID)
			}
			if denied {
				fmt.Fprintln(errOut, "note: one or more tool calls were blocked by the permission gate; pass --yolo or add them to permissions.allowed_tools")
				exitOverride = exitPermissionDenied
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&modelID, "model", "m", "", "model id to use (defaults to default_model)")
	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "resume a specific session by id")
	cmd.Flags().BoolVarP(&continueLast, "continue", "c", false, "resume the most recent session")
	cmd.Flags().BoolVar(&yolo, "yolo", false, "allow all tools without permission checks")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit newline-delimited JSON events instead of plain text")
	cmd.Flags().StringVar(&systemPromptFlag, "system-prompt", "", "override the system prompt (takes precedence over the system_prompt config field and AGENTS.md)")
	return cmd
}

// resolvePrompt returns the prompt from the arguments, or from stdin when no
// arguments are given and stdin is piped. A missing prompt on an interactive
// stdin, or an empty pipe, is a usage error.
func resolvePrompt(cmd *cobra.Command, args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && isTTY(f) {
		return "", codedf(exitUsage, "no prompt given: pass it as an argument or pipe it on stdin")
	}
	data, err := io.ReadAll(in)
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(data))
	if p == "" {
		return "", codedf(exitUsage, "no prompt given: stdin was empty")
	}
	return p, nil
}

// resolveSystemPrompt picks the system prompt: the --system-prompt flag if it
// was set, then the system_prompt config field, then the generated prompt.
func resolveSystemPrompt(cmd *cobra.Command, cfg *config.Config, wd string, skillSet []*skills.Skill, flag string) string {
	if cmd.Flags().Changed("system-prompt") {
		return flag
	}
	if cfg.SystemPrompt != "" {
		return cfg.SystemPrompt
	}
	return prompt.System(wd, cfg.ProjectRoot(), skillSet)
}

// classifySendError maps an agent turn failure to a coded error so the process
// exits with a status that tells scripts what went wrong.
func classifySendError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return coded(exitInterrupted, err)
	case errors.Is(err, agent.ErrMaxSteps):
		return coded(exitMaxSteps, err)
	default:
		return coded(exitAPIError, err)
	}
}

// jsonEvent is one newline-delimited JSON record emitted by `run --json`.
type jsonEvent struct {
	Type         string          `json:"type"`
	Name         string          `json:"name,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	Text         string          `json:"text,omitempty"`
	Result       string          `json:"result,omitempty"`
	IsError      bool            `json:"is_error,omitempty"`
	InputTokens  int             `json:"input_tokens,omitempty"`
	OutputTokens int             `json:"output_tokens,omitempty"`
	CachedTokens int             `json:"cached_tokens,omitempty"`
	Session      string          `json:"session,omitempty"`
}

// jsonEmitter writes jsonEvents as NDJSON. The agent invokes the Events
// callbacks serially, so the encoder needs no locking.
type jsonEmitter struct {
	enc *json.Encoder
}

func (j *jsonEmitter) emit(ev jsonEvent) { _ = j.enc.Encode(ev) }

func (j *jsonEmitter) events() agent.Events {
	return agent.Events{
		Text:      func(delta string) { j.emit(jsonEvent{Type: "text", Text: delta}) },
		Reasoning: func(delta string) { j.emit(jsonEvent{Type: "reasoning", Text: delta}) },
		ToolStart: func(name string, input []byte) {
			j.emit(jsonEvent{Type: "tool_start", Name: name, Input: json.RawMessage(input)})
		},
		ToolEnd: func(name, result string, isError bool) {
			j.emit(jsonEvent{Type: "tool_end", Name: name, Result: result, IsError: isError})
		},
	}
}

// resolveSession selects the session to run against: a specific id, the most
// recent one, or a freshly created session seeded with the system prompt. It
// returns the session and the history to seed the agent with.
func resolveSession(store session.Store, id string, continueLast bool, userPrompt, systemPrompt string) (session.Session, []llm.Message, error) {
	switch {
	case id != "":
		sess, err := store.Get(id)
		if err != nil {
			return session.Session{}, nil, err
		}
		history, err := store.Messages(id)
		if err != nil {
			return session.Session{}, nil, err
		}
		return sess, history, nil

	case continueLast:
		list, err := store.List()
		if err != nil {
			return session.Session{}, nil, err
		}
		if len(list) > 0 {
			history, err := store.Messages(list[0].ID)
			if err != nil {
				return session.Session{}, nil, err
			}
			return list[0], history, nil
		}
		// Nothing to continue; fall through to a new session.
	}

	sess, err := store.Create(truncate(userPrompt, 60))
	if err != nil {
		return session.Session{}, nil, err
	}
	sysMsg := llm.SystemText(systemPrompt)
	if err := store.Append(sess.ID, sysMsg); err != nil {
		return session.Session{}, nil, err
	}
	return sess, []llm.Message{sysMsg}, nil
}

// contextWindow returns the configured context window of the selected model
// (the default model when modelID is empty), or 0 if unset.
func contextWindow(cfg *config.Config, modelID string) int {
	if modelID == "" {
		modelID = cfg.DefaultModel
	}
	if m, ok := cfg.Model(modelID); ok {
		return m.ContextWindow
	}
	return 0
}

// sessionDir returns the directory for session storage: a single global
// location shared across all projects.
func sessionDir(_ *config.Config, _ string) string {
	return config.GlobalSessionsDir()
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
