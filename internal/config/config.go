// Package config loads eonyx's layered configuration. Three layers are
// merged top to bottom — global config, global data, then the project config
// found by walking up from the working directory — with lower layers
// overriding higher ones. Provider credentials may reference environment
// variables (e.g. "$OPENAI_API_KEY"), which are expanded after merging.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strconv"
	"strings"

	"github.com/eonyx-core/eonyx/internal/hooks"
)

// Config is the fully merged, environment-resolved configuration.
type Config struct {
	Providers    map[string]Provider `json:"providers"`
	Models       []Model             `json:"models"`
	DefaultModel string              `json:"default_model"`
	// MaxSteps bounds tool-calling round-trips per turn. Zero uses the agent's
	// default (50).
	MaxSteps    int               `json:"max_steps,omitempty"`
	Permissions Permissions       `json:"permissions"`
	Skills      []string          `json:"skills,omitempty"`
	MCP         []MCPServerConfig `json:"mcp,omitempty"`
	Hooks       hooks.Hooks       `json:"hooks,omitempty"`
	Report      ReportConfig      `json:"report"`
	// SystemPrompt overrides the generated system prompt when non-empty. The
	// --system-prompt flag takes precedence over it.
	SystemPrompt string `json:"system_prompt,omitempty"`
	// LogFile directs the log to this exact path instead of the per-project
	// default. It may reference an environment variable; --log-file overrides it.
	LogFile string `json:"log_file,omitempty"`
	// Retry tunes the provider's request retry and response-header timeout.
	Retry RetryConfig `json:"retry,omitempty"`

	// projectRoot is the directory holding the project config, if any. It is
	// not serialized; it is set during Load and used to locate project data.
	projectRoot string
}

// RetryConfig tunes how the provider retries transient request failures and how
// long it waits for a response to begin. Zero/omitted fields use built-in
// defaults.
type RetryConfig struct {
	// MaxRetries is the number of extra attempts after a transient failure
	// (transport error or 429/5xx). Nil uses the default (3); 0 disables retries.
	MaxRetries *int `json:"max_retries,omitempty"`
	// BaseDelayMS is the first backoff step in milliseconds; each further retry
	// doubles it, capped at 30s. Zero uses the default (500).
	BaseDelayMS int `json:"base_delay_ms,omitempty"`
	// ResponseHeaderTimeoutSec bounds the wait for the first response header,
	// before streaming begins. Zero uses the default (60).
	ResponseHeaderTimeoutSec int `json:"response_header_timeout_sec,omitempty"`
}

// MCPServerConfig describes one MCP server to connect to.
type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	URL     string            `json:"url,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	// Headers are added to every request to an HTTP MCP server, e.g. an
	// "Authorization" bearer token. Values may reference environment variables.
	Headers map[string]string `json:"headers,omitempty"`
}

// ReportConfig configures the workload report tools' database access. When
// DatabaseURL is empty the report tools are not registered.
type ReportConfig struct {
	// DatabaseURL is the PostgreSQL connection string. It may reference an
	// environment variable like "$DATABASE_URL", expanded during Load.
	DatabaseURL string `json:"database_url,omitempty"`
}

// Hook describes a before/after shell command for a named tool.
type Hook = hooks.Hook

// Hooks maps tool names to before/after shell commands.
type Hooks = hooks.Hooks

// Provider describes how to reach one LLM backend.
type Provider struct {
	// Type selects the client implementation. Currently only "openai", which
	// covers any OpenAI-compatible endpoint (OpenAI, OpenRouter, DeepSeek,
	// Moonshot, and local servers like Ollama, llama.cpp, or LM Studio).
	Type string `json:"type"`
	// BaseURL is the API base URL. Required.
	BaseURL string `json:"base_url,omitempty"`
	// APIKey may contain an environment reference like "$OPENAI_API_KEY",
	// which is expanded during Load.
	APIKey string `json:"api_key,omitempty"`
}

// Model binds a model id to one of the configured providers.
type Model struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	// ContextWindow is the model's usable context size in tokens. When set, it
	// enables automatic summarization of older history as the conversation
	// approaches the limit. Zero leaves summarization disabled.
	ContextWindow int `json:"context_window,omitempty"`
	// InputPrice is the cost per 1M input tokens in USD. Zero means free.
	InputPrice float64 `json:"input_price,omitempty"`
	// OutputPrice is the cost per 1M output tokens in USD. Zero means free.
	OutputPrice float64 `json:"output_price,omitempty"`
}

// Permissions configures the tool permission gate.
type Permissions struct {
	// AllowedTools lists tools that may run without prompting.
	AllowedTools []string `json:"allowed_tools,omitempty"`
	// Sandbox confines the file tools (read/write/edit/multi_edit/ls) to the
	// project root. It is off by default; set it true to opt in.
	Sandbox bool `json:"sandbox,omitempty"`
}

// FileSandboxRoot returns the directory the file tools are confined to, or ""
// when sandboxing is disabled. It is the project root when one was found, else
// the working directory.
func (c *Config) FileSandboxRoot(workdir string) string {
	if !c.Permissions.Sandbox {
		return ""
	}
	if c.projectRoot != "" {
		return c.projectRoot
	}
	return workdir
}

// ProjectRoot returns the directory of the loaded project config, or "" if no
// project config was found.
func (c *Config) ProjectRoot() string { return c.projectRoot }

// Model returns the configured model with the given id.
func (c *Config) Model(id string) (Model, bool) {
	for _, m := range c.Models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// Load reads and merges all configuration layers, resolving the project layer
// by walking up from startDir. Missing files are skipped. Environment
// references in provider fields are expanded, and the result is validated.
func Load(startDir string) (*Config, error) {
	cfg := &Config{Providers: map[string]Provider{}}

	if err := cfg.mergeFile(GlobalConfigPath()); err != nil {
		return nil, err
	}
	if err := cfg.mergeFile(GlobalDataPath()); err != nil {
		return nil, err
	}
	if path, root, found := FindProjectConfig(startDir); found {
		if err := cfg.mergeFile(path); err != nil {
			return nil, err
		}
		cfg.projectRoot = root
	}

	cfg.expandEnv()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// mergeFile reads one config file and merges it into c. A missing file is
// skipped silently. An unreadable or incompatible file is a fatal error:
// eonyx refuses to start on a config it cannot honor rather than silently
// running with a different configuration than intended.
func (c *Config) mergeFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read config %s: %w", path, err)
	}

	// The config must match this version's schema exactly: unknown fields,
	// wrong types, and malformed JSON are all rejected so eonyx never runs
	// against a config it cannot fully honor.
	var layer Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&layer); err != nil {
		return decodeError(path, data, err)
	}
	c.merge(layer)
	return nil
}

// decodeError turns a config decode failure into an actionable message. The
// common case is an unknown field — a typo or a field written for a newer
// eonyx — which the standard decoder reports without a location; we name the
// field, point to its line, and say how to check the supported fields. Other
// failures keep the generic incompatibility message.
func decodeError(path string, data []byte, err error) error {
	field, ok := unknownField(err)
	if !ok {
		return fmt.Errorf("config %s is incompatible with this version of eonyx: %w", path, err)
	}
	where := fmt.Sprintf("config %s: unknown field %q", path, field)
	if line, ok := fieldLine(data, field); ok {
		where = fmt.Sprintf("%s (line %d)", where, line)
	}
	return fmt.Errorf("%s — check for a typo or a field this eonyx version doesn't support; run 'eonyx config' or see eonyx.example.json for the supported fields", where)
}

// unknownField extracts the field name from a DisallowUnknownFields decode
// error ("json: unknown field \"name\""), returning false for any other error.
func unknownField(err error) (string, bool) {
	const prefix = "json: unknown field "
	msg := err.Error()
	i := strings.Index(msg, prefix)
	if i < 0 {
		return "", false
	}
	name, uerr := strconv.Unquote(msg[i+len(prefix):])
	if uerr != nil {
		return "", false
	}
	return name, true
}

// fieldLine returns the 1-based line of the first occurrence of the given JSON
// key in the raw config, best-effort, for locating an unknown field.
func fieldLine(data []byte, field string) (int, bool) {
	idx := bytes.Index(data, []byte(`"`+field+`"`))
	if idx < 0 {
		return 0, false
	}
	return bytes.Count(data[:idx], []byte{'\n'}) + 1, true
}

// merge applies layer on top of c with lower-overrides-higher semantics: the
// providers map is unioned by key, while models, default_model, and
// permissions are replaced when the layer sets them.
func (c *Config) merge(layer Config) {
	maps.Copy(c.Providers, layer.Providers)
	if len(layer.Models) > 0 {
		c.Models = layer.Models
	}
	if layer.DefaultModel != "" {
		c.DefaultModel = layer.DefaultModel
	}
	if layer.MaxSteps != 0 {
		c.MaxSteps = layer.MaxSteps
	}
	if layer.Permissions.AllowedTools != nil {
		c.Permissions.AllowedTools = layer.Permissions.AllowedTools
	}
	if len(layer.Skills) > 0 {
		c.Skills = layer.Skills
	}
	if len(layer.MCP) > 0 {
		c.MCP = layer.MCP
	}
	if len(layer.Hooks) > 0 {
		c.Hooks = layer.Hooks
	}
	if layer.Report.DatabaseURL != "" {
		c.Report = layer.Report
	}
	if layer.SystemPrompt != "" {
		c.SystemPrompt = layer.SystemPrompt
	}
	if layer.LogFile != "" {
		c.LogFile = layer.LogFile
	}
	if layer.Retry.MaxRetries != nil {
		c.Retry.MaxRetries = layer.Retry.MaxRetries
	}
	if layer.Retry.BaseDelayMS != 0 {
		c.Retry.BaseDelayMS = layer.Retry.BaseDelayMS
	}
	if layer.Retry.ResponseHeaderTimeoutSec != 0 {
		c.Retry.ResponseHeaderTimeoutSec = layer.Retry.ResponseHeaderTimeoutSec
	}
}

// expandEnv resolves environment references in provider credential fields.
func (c *Config) expandEnv() {
	for name, p := range c.Providers {
		p.APIKey = os.ExpandEnv(p.APIKey)
		p.BaseURL = os.ExpandEnv(p.BaseURL)
		c.Providers[name] = p
	}
	c.Report.DatabaseURL = os.ExpandEnv(c.Report.DatabaseURL)
	c.LogFile = os.ExpandEnv(c.LogFile)
}

// validate checks cross-references and required fields.
func (c *Config) validate() error {
	for name, p := range c.Providers {
		switch p.Type {
		case "openai":
		case "":
			return fmt.Errorf("provider %q: missing type", name)
		default:
			return fmt.Errorf("provider %q: unknown type %q", name, p.Type)
		}
		if p.BaseURL == "" {
			return fmt.Errorf("provider %q: openai type requires base_url", name)
		}
	}
	for _, m := range c.Models {
		if m.ID == "" {
			return fmt.Errorf("model with empty id")
		}
		if _, ok := c.Providers[m.Provider]; !ok {
			return fmt.Errorf("model %q references unknown provider %q", m.ID, m.Provider)
		}
	}
	if c.DefaultModel != "" {
		if _, ok := c.Model(c.DefaultModel); !ok {
			return fmt.Errorf("default_model %q is not in models", c.DefaultModel)
		}
	}
	if c.MaxSteps < 0 {
		return fmt.Errorf("max_steps must be >= 0, got %d", c.MaxSteps)
	}
	if c.Retry.MaxRetries != nil && *c.Retry.MaxRetries < 0 {
		return fmt.Errorf("retry.max_retries must be >= 0, got %d", *c.Retry.MaxRetries)
	}
	if c.Retry.BaseDelayMS < 0 {
		return fmt.Errorf("retry.base_delay_ms must be >= 0, got %d", c.Retry.BaseDelayMS)
	}
	if c.Retry.ResponseHeaderTimeoutSec < 0 {
		return fmt.Errorf("retry.response_header_timeout_sec must be >= 0, got %d", c.Retry.ResponseHeaderTimeoutSec)
	}
	return nil
}
