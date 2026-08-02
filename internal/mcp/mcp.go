package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/eonyx-core/eonyx/internal/tools"
	"github.com/eonyx-core/eonyx/internal/version"
)

// ServerConfig describes how to reach one MCP server.
type ServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	URL     string            `json:"url,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// headerRoundTripper adds fixed headers (with environment references expanded)
// to every outgoing request, used to authenticate HTTP MCP servers.
type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	for k, v := range h.headers {
		req.Header.Set(k, os.ExpandEnv(v))
	}
	return h.base.RoundTrip(req)
}

// Client wraps an MCP client session.
type Client struct {
	name    string
	session *mcp.ClientSession
}

// DialStdio starts an MCP server command and connects over stdio.
func DialStdio(ctx context.Context, cfg ServerConfig) (*Client, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("mcp server %q: command is required", cfg.Name)
	}
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	transport := &mcp.CommandTransport{Command: cmd}
	return connect(ctx, cfg.Name, transport)
}

// DialHTTP connects to an MCP server over HTTP/SSE.
func DialHTTP(ctx context.Context, cfg ServerConfig) (*Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("mcp server %q: url is required", cfg.Name)
	}
	transport := &mcp.SSEClientTransport{Endpoint: cfg.URL}
	if len(cfg.Headers) > 0 {
		transport.HTTPClient = &http.Client{Transport: headerRoundTripper{base: http.DefaultTransport, headers: cfg.Headers}}
	}
	return connect(ctx, cfg.Name, transport)
}

func connect(ctx context.Context, name string, transport mcp.Transport) (*Client, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "eonyx", Version: version.String()}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect %s: %w", name, err)
	}
	return &Client{name: name, session: session}, nil
}

// DiscoverTools returns eonyx tools for every tool advertised by the server.
func (c *Client) DiscoverTools(ctx context.Context) ([]tools.Tool, error) {
	res, err := c.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("mcp list tools %s: %w", c.name, err)
	}
	var out []tools.Tool
	for _, t := range res.Tools {
		out = append(out, toolFromMCP(c.name, t, c.handler(t.Name)))
	}
	return out, nil
}

// DiscoverResourceTools returns tools for browsing and reading the server's
// resources, but only when the server supports resources — it probes with a
// ListResources call and returns the probe error (so the caller skips) when the
// capability is absent.
func (c *Client) DiscoverResourceTools(ctx context.Context) ([]tools.Tool, error) {
	if _, err := c.session.ListResources(ctx, &mcp.ListResourcesParams{}); err != nil {
		return nil, err
	}
	return []tools.Tool{c.listResourcesTool(), c.readResourceTool()}, nil
}

func (c *Client) listResourcesTool() tools.Tool {
	return tools.Tool{
		Name:        toolName(c.name, "list_resources"),
		Description: fmt.Sprintf("List the resources exposed by the %q MCP server (URI, name, description).", c.name),
		Schema:      json.RawMessage(`{"type":"object","properties":{}}`),
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			res, err := c.session.ListResources(ctx, &mcp.ListResourcesParams{})
			if err != nil {
				return "", fmt.Errorf("mcp list resources %s: %w", c.name, err)
			}
			return formatResources(res.Resources), nil
		},
	}
}

func (c *Client) readResourceTool() tools.Tool {
	const schema = `{"type":"object","properties":{"uri":{"type":"string","description":"The resource URI to read."}},"required":["uri"]}`
	return tools.Tool{
		Name:        toolName(c.name, "read_resource"),
		Description: fmt.Sprintf("Read a resource from the %q MCP server by its URI.", c.name),
		Schema:      json.RawMessage(schema),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				URI string `json:"uri"`
			}
			if len(input) > 0 {
				if err := json.Unmarshal(input, &args); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}
			}
			if args.URI == "" {
				return "", fmt.Errorf("uri is required")
			}
			res, err := c.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: args.URI})
			if err != nil {
				return "", fmt.Errorf("mcp read resource %s: %w", c.name, err)
			}
			return formatResourceContents(res.Contents), nil
		},
	}
}

// DiscoverPromptTools returns tools for listing and rendering the server's
// prompt templates, gated on the server supporting prompts (probed with
// ListPrompts).
func (c *Client) DiscoverPromptTools(ctx context.Context) ([]tools.Tool, error) {
	if _, err := c.session.ListPrompts(ctx, &mcp.ListPromptsParams{}); err != nil {
		return nil, err
	}
	return []tools.Tool{c.listPromptsTool(), c.getPromptTool()}, nil
}

func (c *Client) listPromptsTool() tools.Tool {
	return tools.Tool{
		Name:        toolName(c.name, "list_prompts"),
		Description: fmt.Sprintf("List the prompt templates exposed by the %q MCP server (name, arguments, description).", c.name),
		Schema:      json.RawMessage(`{"type":"object","properties":{}}`),
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			res, err := c.session.ListPrompts(ctx, &mcp.ListPromptsParams{})
			if err != nil {
				return "", fmt.Errorf("mcp list prompts %s: %w", c.name, err)
			}
			return formatPrompts(res.Prompts), nil
		},
	}
}

func (c *Client) getPromptTool() tools.Tool {
	const schema = `{"type":"object","properties":{"name":{"type":"string","description":"The prompt name."},"arguments":{"type":"object","description":"String arguments for the prompt template.","additionalProperties":{"type":"string"}}},"required":["name"]}`
	return tools.Tool{
		Name:        toolName(c.name, "get_prompt"),
		Description: fmt.Sprintf("Get a prompt template from the %q MCP server, rendered with the given arguments.", c.name),
		Schema:      json.RawMessage(schema),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Name      string            `json:"name"`
				Arguments map[string]string `json:"arguments"`
			}
			if len(input) > 0 {
				if err := json.Unmarshal(input, &args); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}
			}
			if args.Name == "" {
				return "", fmt.Errorf("name is required")
			}
			res, err := c.session.GetPrompt(ctx, &mcp.GetPromptParams{Name: args.Name, Arguments: args.Arguments})
			if err != nil {
				return "", fmt.Errorf("mcp get prompt %s: %w", c.name, err)
			}
			return formatPromptMessages(res), nil
		},
	}
}

// formatResources renders a resource listing one per line.
func formatResources(list []*mcp.Resource) string {
	if len(list) == 0 {
		return "(no resources)"
	}
	var b strings.Builder
	for _, r := range list {
		name := r.Name
		if r.Title != "" {
			name = r.Title
		}
		fmt.Fprintf(&b, "%s\t%s", r.URI, name)
		if r.Description != "" {
			fmt.Fprintf(&b, " — %s", r.Description)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatResourceContents joins the text parts of a read resource, noting any
// binary blobs rather than dumping them.
func formatResourceContents(contents []*mcp.ResourceContents) string {
	var parts []string
	for _, rc := range contents {
		switch {
		case rc.Text != "":
			parts = append(parts, rc.Text)
		case len(rc.Blob) > 0:
			parts = append(parts, fmt.Sprintf("(binary resource %s, %d bytes)", rc.URI, len(rc.Blob)))
		}
	}
	if len(parts) == 0 {
		return "(empty resource)"
	}
	return strings.Join(parts, "\n")
}

// formatPrompts renders a prompt listing one per line with argument names
// (required ones marked with "*").
func formatPrompts(list []*mcp.Prompt) string {
	if len(list) == 0 {
		return "(no prompts)"
	}
	var b strings.Builder
	for _, p := range list {
		b.WriteString(p.Name)
		if len(p.Arguments) > 0 {
			names := make([]string, 0, len(p.Arguments))
			for _, a := range p.Arguments {
				n := a.Name
				if a.Required {
					n += "*"
				}
				names = append(names, n)
			}
			fmt.Fprintf(&b, "(%s)", strings.Join(names, ", "))
		}
		if p.Description != "" {
			fmt.Fprintf(&b, " — %s", p.Description)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatPromptMessages renders a fetched prompt as role-tagged text.
func formatPromptMessages(res *mcp.GetPromptResult) string {
	var b strings.Builder
	if res.Description != "" {
		b.WriteString(res.Description)
		b.WriteString("\n\n")
	}
	for _, m := range res.Messages {
		text := ""
		if tc, ok := m.Content.(*mcp.TextContent); ok {
			text = tc.Text
		} else if m.Content != nil {
			text = fmt.Sprintf("%v", m.Content)
		}
		fmt.Fprintf(&b, "[%s] %s\n", m.Role, text)
	}
	return strings.TrimRight(b.String(), "\n")
}

// toolFromMCP converts one MCP tool into a eonyx tool, binding h as the
// handler that forwards calls to the server.
func toolFromMCP(server string, t *mcp.Tool, h tools.Handler) tools.Tool {
	schema, _ := json.Marshal(t.InputSchema)
	return tools.Tool{
		Name:        toolName(server, t.Name),
		Description: t.Description,
		Schema:      schema,
		Handler:     h,
	}
}

// Name returns the configured server name.
func (c *Client) Name() string { return c.name }

// Close closes the underlying MCP session.
func (c *Client) Close() error {
	if c.session == nil {
		return nil
	}
	return c.session.Close()
}

// DialAll connects to all configured servers best-effort: a server that fails
// to connect is skipped and its error collected, so one bad server does not stop
// eonyx from starting. It returns the clients that connected and any errors.
func DialAll(ctx context.Context, configs []ServerConfig) ([]*Client, []error) {
	var clients []*Client
	var errs []error
	for _, cfg := range configs {
		var client *Client
		var err error
		if cfg.URL != "" {
			client, err = DialHTTP(ctx, cfg)
		} else {
			client, err = DialStdio(ctx, cfg)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		clients = append(clients, client)
	}
	return clients, errs
}

// CloseAll closes all clients, ignoring errors.
func CloseAll(clients []*Client) {
	for _, c := range clients {
		_ = c.Close()
	}
}

// RegisterFromClient discovers tools from the MCP client and registers them
// in the eonyx registry with a server-name prefix.
func RegisterFromClient(ctx context.Context, reg *tools.Registry, client interface {
	DiscoverTools(context.Context) ([]tools.Tool, error)
	Close() error
}) error {
	discovered, err := client.DiscoverTools(ctx)
	if err != nil {
		return err
	}
	for _, t := range discovered {
		reg.Register(t)
	}

	// Additionally expose resources and prompts as tools when the client (and
	// its server) support them. A probe error means the capability is absent, so
	// those tools are simply skipped.
	if ext, ok := client.(interface {
		DiscoverResourceTools(context.Context) ([]tools.Tool, error)
		DiscoverPromptTools(context.Context) ([]tools.Tool, error)
	}); ok {
		for _, discover := range []func(context.Context) ([]tools.Tool, error){ext.DiscoverResourceTools, ext.DiscoverPromptTools} {
			extra, err := discover(ctx)
			if err != nil {
				continue
			}
			for _, t := range extra {
				reg.Register(t)
			}
		}
	}
	return nil
}

// toolName prefixes an MCP tool name with its server name to avoid collisions.
func toolName(server, name string) string {
	return server + "_" + name
}

func (c *Client) handler(toolName string) tools.Handler {
	return func(ctx context.Context, input json.RawMessage) (string, error) {
		var args map[string]any
		if len(input) > 0 {
			if err := json.Unmarshal(input, &args); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
		}
		res, err := c.session.CallTool(ctx, &mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		})
		if err != nil {
			return "", fmt.Errorf("mcp call %s: %w", toolName, err)
		}
		return formatResult(res), nil
	}
}

func formatResult(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var parts []string
	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcp.TextContent:
			parts = append(parts, v.Text)
		default:
			parts = append(parts, fmt.Sprintf("%v", c))
		}
	}
	return strings.Join(parts, "\n")
}
