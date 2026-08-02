package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	fetchTimeout  = 30
	maxFetchBytes = 5 << 20 // 5 MiB
)

func newFetchTool(client *http.Client) Tool {
	if client == nil {
		client = http.DefaultClient
	}
	const schema = `{
  "type": "object",
  "properties": {
    "url": {"type": "string", "description": "HTTP or HTTPS URL to fetch."}
  },
  "required": ["url"]
}`
	return Tool{
		Name:        "fetch",
		Description: "Fetch a web page or text resource. HTML is converted to markdown; other text is returned as-is. Binary content is rejected.",
		Schema:      json.RawMessage(schema),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				URL string `json:"url"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			if args.URL == "" {
				return "", fmt.Errorf("url is required")
			}
			if !strings.HasPrefix(args.URL, "http://") && !strings.HasPrefix(args.URL, "https://") {
				return "", fmt.Errorf("url must start with http:// or https://")
			}

			ctx, cancel := context.WithTimeout(ctx, fetchTimeout*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
			if err != nil {
				return "", err
			}
			req.Header.Set("User-Agent", "eonyx/0.1")
			req.Header.Set("Accept", "text/html,text/plain,*/*")

			resp, err := client.Do(req)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				return "", fmt.Errorf("fetch %s: %s", args.URL, resp.Status)
			}

			contentType := resp.Header.Get("Content-Type")
			if !isTextual(contentType) {
				return "", fmt.Errorf("unsupported content type %q (only text is fetched)", contentType)
			}

			body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
			if err != nil {
				return "", err
			}
			truncated := len(body) > maxFetchBytes
			if truncated {
				body = body[:maxFetchBytes]
			}

			if strings.Contains(contentType, "html") {
				doc, err := html.Parse(strings.NewReader(string(body)))
				if err != nil {
					return "", fmt.Errorf("parse html: %w", err)
				}
				md := htmlToMarkdown(doc)
				if truncated {
					return md + "\n\n(response exceeded 5 MiB and was truncated)", nil
				}
				return md, nil
			}
			if truncated {
				return string(body) + "\n\n(response exceeded 5 MiB and was truncated)", nil
			}
			return string(body), nil
		},
	}
}

// isTextual reports whether a Content-Type header denotes text we can render.
func isTextual(contentType string) bool {
	ct := strings.ToLower(contentType)
	if ct == "" {
		return true // assume text when the server is silent
	}
	return strings.HasPrefix(ct, "text/") ||
		strings.Contains(ct, "html") ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "xml")
}
