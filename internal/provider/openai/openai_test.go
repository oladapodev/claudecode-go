package openai

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

func TestStreamTurnParsesToolCalls(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"function\":{\"name\":\"shell\",\"arguments\":\"{\\\"command\\\":\\\"\"}}]},\"finish_reason\":\"\"}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"pwd\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	adapter := New(server.Client(), staticSecret{})
	profile := domain.ProviderProfile{
		Provider: domain.ProviderOpenAI,
		BaseURL:  server.URL + "/v1",
		Model:    "gpt-test",
	}
	stream, errs := adapter.StreamTurn(context.Background(), domain.TurnRequest{
		Profile: profile,
		Model:   profile.Model,
		Tools: []domain.ToolDefinition{{
			Name:        "shell",
			Description: "Run shell",
			Schema:      []byte(`{"type":"object"}`),
		}},
	})

	var calls []domain.ToolCall
	for event := range stream {
		if event.Call != nil {
			calls = append(calls, *event.Call)
		}
	}
	if err := <-errs; err != nil {
		t.Fatalf("StreamTurn() error = %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if got := string(calls[0].Input); got != "{\"command\":\"pwd\"}" {
		t.Fatalf("unexpected tool call input: %s", got)
	}
}
