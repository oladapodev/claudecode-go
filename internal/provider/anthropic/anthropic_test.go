package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oladapodev/claudecode-go/internal/domain"
)

type staticSecret struct{}

func (staticSecret) Get(context.Context, domain.ProviderProfile) (string, error) {
	return "test-key", nil
}
func (staticSecret) Set(context.Context, domain.ProviderProfile, string) error { return nil }

func TestStreamTurnParsesTextAndToolUse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\ndata: {\"message\":{\"usage\":{\"input_tokens\":5}}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n")
		fmt.Fprint(w, "event: content_block_start\ndata: {\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"call-1\",\"name\":\"read_file\",\"input\":{}}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\\\"README.md\\\"}\"}}\n\n")
		fmt.Fprint(w, "event: content_block_stop\ndata: {\"index\":1}\n\n")
		fmt.Fprint(w, "event: message_delta\ndata: {\"usage\":{\"output_tokens\":7}}\n\n")
		fmt.Fprint(w, "event: message_stop\ndata: {}\n\n")
	}))
	defer server.Close()

	adapter := New(server.Client(), staticSecret{})
	profile := domain.ProviderProfile{
		Provider: domain.ProviderAnthropic,
		BaseURL:  server.URL,
		Model:    "claude-test",
	}
	stream, errs := adapter.StreamTurn(context.Background(), domain.TurnRequest{
		Profile: profile,
		Model:   profile.Model,
		Tools: []domain.ToolDefinition{{
			Name:        "read_file",
			Description: "Read file",
			Schema:      []byte(`{"type":"object"}`),
		}},
	})

	var text string
	var call *domain.ToolCall
	for event := range stream {
		if event.Text != "" {
			text += event.Text
		}
		if event.Call != nil {
			tmp := *event.Call
			call = &tmp
		}
	}
	if err := <-errs; err != nil {
		t.Fatalf("StreamTurn() error = %v", err)
	}

	if text != "hello" {
		t.Fatalf("expected text delta, got %q", text)
	}
	if call == nil || string(call.Input) != "{\"path\":\"README.md\"}" {
		t.Fatalf("unexpected tool call: %#v", call)
	}
}
