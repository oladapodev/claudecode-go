package anthropic

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

const apiVersion = "2023-06-01"

type Adapter struct {
	client  shared.HTTPClient
	secrets secret.Store
}

func New(client shared.HTTPClient, secrets secret.Store) *Adapter {
	return &Adapter{client: client, secrets: secrets}
}

func (a *Adapter) ValidateProfile(ctx context.Context, profile domain.ProviderProfile) error {
	if strings.TrimSpace(profile.BaseURL) == "" {
		return fmt.Errorf("anthropic base URL is required")
	}
	_, err := a.secrets.Get(ctx, profile)
	return err
}

func (a *Adapter) ListModels(ctx context.Context, profile domain.ProviderProfile) ([]domain.Model, error) {
	key, err := a.secrets.Get(ctx, profile)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(profile.BaseURL, "/")+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", apiVersion)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	models := make([]domain.Model, 0, len(payload.Data))
	for _, item := range payload.Data {
		models = append(models, domain.Model{ID: item.ID, DisplayName: item.DisplayName})
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

		body, err := anthropicRequest(req)
		if err != nil {
			errs <- err
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(req.Profile.BaseURL, "/")+"/v1/messages", bytes.NewReader(body))
		if err != nil {
			errs <- err
			return
		}

		httpReq.Header.Set("content-type", "application/json")
		httpReq.Header.Set("accept", "text/event-stream")
		httpReq.Header.Set("x-api-key", key)
		httpReq.Header.Set("anthropic-version", apiVersion)

		toolBuffers := map[int]*toolBuffer{}
		var usage domain.Usage

		err = shared.StreamSSE(ctx, a.client, httpReq, func(event shared.SSEEvent) error {
			if bytes.Equal(event.Data, []byte("[DONE]")) {
				return nil
			}

			var payload streamPayload
			if err := json.Unmarshal(event.Data, &payload); err != nil {
				return err
			}

			switch event.Event {
			case "message_start":
				usage.InputTokens = payload.Message.Usage.InputTokens
			case "content_block_start":
				if payload.ContentBlock.Type == "tool_use" {
					buffer := &toolBuffer{id: payload.ContentBlock.ID, name: payload.ContentBlock.Name}
					if len(payload.ContentBlock.Input) > 0 && string(payload.ContentBlock.Input) != "null" && string(payload.ContentBlock.Input) != "{}" {
						buffer.args.Write(payload.ContentBlock.Input)
					}
					toolBuffers[payload.Index] = buffer
				}
			case "content_block_delta":
				switch payload.Delta.Type {
				case "text_delta":
					events <- domain.StreamEvent{Type: domain.EventTextDelta, Text: payload.Delta.Text}
				case "input_json_delta":
					if buffer := toolBuffers[payload.Index]; buffer != nil {
						buffer.args.WriteString(payload.Delta.PartialJSON)
					}
				}
			case "content_block_stop":
				if buffer := toolBuffers[payload.Index]; buffer != nil {
					call := buffer.toolCall()
					events <- domain.StreamEvent{Type: domain.EventToolCall, Call: &call}
					delete(toolBuffers, payload.Index)
				}
			case "message_delta":
				usage.OutputTokens = payload.Usage.OutputTokens
			case "message_stop":
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
	input := bytes.TrimSpace(b.args.Bytes())
	if len(input) == 0 {
		input = []byte(`{}`)
	}
	return domain.ToolCall{
		ID:    b.id,
		Name:  b.name,
		Input: append([]byte(nil), input...),
	}
}

type streamPayload struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
	ContentBlock struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content_block"`
	Message struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func anthropicRequest(req domain.TurnRequest) ([]byte, error) {
	type wireTool struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"input_schema"`
	}

	type wireMessage struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}

	body := map[string]any{
		"model":       req.Model,
		"max_tokens":  4096,
		"stream":      true,
		"messages":    []wireMessage{},
		"temperature": req.Temperature,
	}

	if req.System != "" {
		body["system"] = req.System
	}

	messages := make([]wireMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case domain.RoleSystem:
			continue
		case domain.RoleTool:
			messages = append(messages, wireMessage{
				Role: "user",
				Content: []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": msg.ToolCallID,
					"content":     msg.Content,
				}},
			})
		case domain.RoleAssistant:
			if msg.ToolName != "" {
				var input map[string]any
				if len(msg.ToolInput) > 0 {
					if err := json.Unmarshal(msg.ToolInput, &input); err != nil {
						return nil, err
					}
				} else {
					input = map[string]any{}
				}
				messages = append(messages, wireMessage{
					Role: "assistant",
					Content: []map[string]any{{
						"type":  "tool_use",
						"id":    msg.ToolCallID,
						"name":  msg.ToolName,
						"input": input,
					}},
				})
			} else {
				messages = append(messages, wireMessage{Role: "assistant", Content: msg.Content})
			}
		default:
			messages = append(messages, wireMessage{Role: string(msg.Role), Content: msg.Content})
		}
	}
	body["messages"] = messages

	if len(req.Tools) > 0 {
		tools := make([]wireTool, 0, len(req.Tools))
		for _, tool := range req.Tools {
			var schema map[string]any
			if err := json.Unmarshal(tool.Schema, &schema); err != nil {
				return nil, err
			}
			tools = append(tools, wireTool{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: schema,
			})
		}
		body["tools"] = tools
	}

	return json.Marshal(body)
}
