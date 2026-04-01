package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/oladapodev/claudecode-go/internal/domain"
	"github.com/oladapodev/claudecode-go/internal/provider/shared"
	"github.com/oladapodev/claudecode-go/internal/secret"
)

type Adapter struct {
	client  shared.HTTPClient
	secrets secret.Store
}

func New(client shared.HTTPClient, secrets secret.Store) *Adapter {
	return &Adapter{client: client, secrets: secrets}
}

func (a *Adapter) ValidateProfile(ctx context.Context, profile domain.ProviderProfile) error {
	if strings.TrimSpace(profile.BaseURL) == "" {
		return fmt.Errorf("openai-compatible base URL is required")
	}
	_, err := a.secrets.Get(ctx, profile)
	return err
}

func (a *Adapter) ListModels(ctx context.Context, profile domain.ProviderProfile) ([]domain.Model, error) {
	key, err := a.secrets.Get(ctx, profile)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(profile.BaseURL, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+key)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	models := make([]domain.Model, 0, len(payload.Data))
	for _, item := range payload.Data {
		models = append(models, domain.Model{ID: item.ID, DisplayName: item.ID})
	}
	return models, nil
}

func (a *Adapter) StreamTurn(ctx context.Context, req domain.TurnRequest) (<-chan domain.StreamEvent, <-chan error) {
	events := make(chan domain.StreamEvent)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		key, err := a.secrets.Get(ctx, req.Profile)
		if err != nil {
			errs <- err
			return
		}

		body, err := openAIRequest(req)
		if err != nil {
			errs <- err
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(req.Profile.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			errs <- err
			return
		}
		httpReq.Header.Set("content-type", "application/json")
		httpReq.Header.Set("accept", "text/event-stream")
		httpReq.Header.Set("authorization", "Bearer "+key)
		if req.Profile.Organization != "" {
			httpReq.Header.Set("openai-organization", req.Profile.Organization)
		}

		buffers := map[int]*toolBuffer{}
		var usage domain.Usage

		err = shared.StreamSSE(ctx, a.client, httpReq, func(event shared.SSEEvent) error {
			if bytes.Equal(event.Data, []byte("[DONE]")) {
				events <- domain.StreamEvent{Type: domain.EventDone, Usage: &usage}
				return nil
			}

			var payload streamPayload
			if err := json.Unmarshal(event.Data, &payload); err != nil {
				return err
			}

			if len(payload.Choices) == 0 {
				return nil
			}

			choice := payload.Choices[0]
			if payload.Usage != nil {
				usage = *payload.Usage
			}

			if choice.Delta.Content != "" {
				events <- domain.StreamEvent{Type: domain.EventTextDelta, Text: choice.Delta.Content}
			}

			for _, item := range choice.Delta.ToolCalls {
				buffer := buffers[item.Index]
				if buffer == nil {
					buffer = &toolBuffer{}
					buffers[item.Index] = buffer
				}
				if item.ID != "" {
					buffer.id = item.ID
				}
				if item.Function.Name != "" {
					buffer.name = item.Function.Name
				}
				buffer.args.WriteString(item.Function.Arguments)
			}

			if choice.FinishReason == "tool_calls" {
				for _, buffer := range buffers {
					call := buffer.toolCall()
					events <- domain.StreamEvent{Type: domain.EventToolCall, Call: &call}
				}
				clear(buffers)
			}

			if choice.FinishReason == "stop" {
				events <- domain.StreamEvent{Type: domain.EventDone, Usage: &usage}
			}

			return nil
		})
		if err != nil {
			errs <- err
		}
	}()

	return events, errs
}

type toolBuffer struct {
	id   string
	name string
	args bytes.Buffer
}

func (b *toolBuffer) toolCall() domain.ToolCall {
	args := bytes.TrimSpace(b.args.Bytes())
	if len(args) == 0 {
		args = []byte(`{}`)
	}
	return domain.ToolCall{
		ID:    b.id,
		Name:  b.name,
		Input: append([]byte(nil), args...),
	}
}

type streamPayload struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *domain.Usage `json:"usage"`
}

func openAIRequest(req domain.TurnRequest) ([]byte, error) {
	type wireMessage struct {
		Role       string `json:"role"`
		Content    any    `json:"content,omitempty"`
		ToolCallID string `json:"tool_call_id,omitempty"`
		ToolCalls  []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls,omitempty"`
	}

	body := map[string]any{
		"model":       req.Model,
		"stream":      true,
		"messages":    []wireMessage{},
		"temperature": req.Temperature,
	}

	messages := make([]wireMessage, 0, len(req.Messages))
	if req.System != "" {
		messages = append(messages, wireMessage{Role: "system", Content: req.System})
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case domain.RoleSystem:
			continue
		case domain.RoleTool:
			messages = append(messages, wireMessage{
				Role:       "tool",
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			})
		case domain.RoleAssistant:
			if msg.ToolName != "" {
				wire := wireMessage{Role: "assistant"}
				wire.ToolCalls = []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				}{
					{
						ID:   msg.ToolCallID,
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      msg.ToolName,
							Arguments: string(msg.ToolInput),
						},
					},
				}
				messages = append(messages, wire)
			} else {
				messages = append(messages, wireMessage{Role: "assistant", Content: msg.Content})
			}
		default:
			messages = append(messages, wireMessage{Role: string(msg.Role), Content: msg.Content})
		}
	}

	body["messages"] = messages

	if len(req.Tools) > 0 {
		type function struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			Parameters  map[string]any `json:"parameters"`
		}
		type tool struct {
			Type     string   `json:"type"`
			Function function `json:"function"`
		}
		tools := make([]tool, 0, len(req.Tools))
		for _, item := range req.Tools {
			var schema map[string]any
			if err := json.Unmarshal(item.Schema, &schema); err != nil {
				return nil, err
			}
			tools = append(tools, tool{
				Type: "function",
				Function: function{
					Name:        item.Name,
					Description: item.Description,
					Parameters:  schema,
				},
			})
		}
		body["tools"] = tools
	}

	return json.Marshal(body)
}
