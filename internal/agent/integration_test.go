package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"eonyx/internal/llm"
	"eonyx/internal/llm/openai"
	"eonyx/internal/shell"
	"eonyx/internal/tools"
)

// TestEndToEndWritesFile drives the real OpenAI client and tool set: the model
// calls the write tool on the first turn, the agent executes it (creating a
// file on disk), and the model replies on the second turn.
func TestEndToEndWritesFile(t *testing.T) {
	const turn1 = `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"write","arguments":"{\"path\":\"out.txt\",\"content\":\"hi\"}"}}]}}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`
	const turn2 = `data: {"choices":[{"delta":{"content":"wrote it"},"finish_reason":"stop"}]}

data: [DONE]

`
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(turn1))
		} else {
			_, _ = w.Write([]byte(turn2))
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	sh, err := shell.New(dir)
	if err != nil {
		t.Fatalf("shell: %v", err)
	}
	registry := tools.DefaultSet(dir, "", sh, srv.Client())
	prov := openai.New(srv.URL, "", "test-model", srv.Client())

	a := New(prov, registry, Options{History: []llm.Message{llm.SystemText("be brief")}})
	final, err := a.Send(context.Background(), "create out.txt")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if final != "wrote it" {
		t.Errorf("final = %q, want %q", final, "wrote it")
	}

	data, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "hi" {
		t.Errorf("file content = %q, want hi", data)
	}
}
