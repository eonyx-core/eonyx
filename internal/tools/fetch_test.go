package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchHTMLToMarkdown(t *testing.T) {
	const page = `<html><head><title>T</title><style>x{}</style></head>
<body><h1>Title</h1><p>Hello <a href="https://example.com">world</a>.</p>
<script>ignored()</script></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	out, err := run(t, newFetchTool(srv.Client()), `{"url":"`+srv.URL+`"}`)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.Contains(out, "# Title") {
		t.Errorf("missing heading in %q", out)
	}
	if !strings.Contains(out, "[world](https://example.com)") {
		t.Errorf("missing link in %q", out)
	}
	if strings.Contains(out, "ignored") {
		t.Errorf("script content leaked into %q", out)
	}
}

func TestFetchTruncatesLargeResponse(t *testing.T) {
	huge := make([]byte, maxFetchBytes+1)
	for i := range huge {
		huge[i] = 'a'
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(huge)
	}))
	defer srv.Close()

	out, err := run(t, newFetchTool(srv.Client()), `{"url":"`+srv.URL+`"}`)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(out) <= len(huge) {
		t.Errorf("expected truncated output, got length %d", len(out))
	}
	if !strings.Contains(out, "response exceeded 5 MiB and was truncated") {
		t.Errorf("missing truncation warning in output: %q", out[:200])
	}
}

func TestFetchNon200ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := newFetchTool(srv.Client()).Handler(context.Background(), []byte(`{"url":"`+srv.URL+`"}`)); err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestFetchInvalidURL(t *testing.T) {
	if _, err := newFetchTool(nil).Handler(context.Background(), []byte(`{"url":"ftp://example.com"}`)); err == nil {
		t.Fatal("expected error for non-http url")
	}
}
