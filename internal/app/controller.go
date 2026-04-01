package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/oladapodev/claudecode-go/internal/config"
	"github.com/oladapodev/claudecode-go/internal/domain"
	"github.com/oladapodev/claudecode-go/internal/history"
	"github.com/oladapodev/claudecode-go/internal/provider/anthropic"
	"github.com/oladapodev/claudecode-go/internal/provider/openai"
	"github.com/oladapodev/claudecode-go/internal/secret"
	"github.com/oladapodev/claudecode-go/internal/session"
	"github.com/oladapodev/claudecode-go/internal/tools"
	"github.com/oladapodev/claudecode-go/internal/transcript"
)

type TurnUpdateType string

const (
	UpdateAssistantDelta TurnUpdateType = "assistant_delta"
	UpdateToolCall       TurnUpdateType = "tool_call"
	UpdateToolResult     TurnUpdateType = "tool_result"
	UpdateDone           TurnUpdateType = "done"
	UpdateError          TurnUpdateType = "error"
)

type TurnUpdate struct {
	Type       TurnUpdateType
	Text       string
	ToolCall   *domain.ToolCall
	ToolResult *domain.ToolResult
	Usage      *domain.Usage
	SessionID  string
	Permission *domain.PermissionLog
}

type Controller struct {
	config      *domain.Config
	configStore config.Store
	providers   map[domain.ProviderName]domain.Provider
	history     history.Store
	transcripts transcript.Store
	tools       *tools.Manager
}

func NewController(cfg *domain.Config, configStore config.Store, historyStore history.Store, transcriptStore transcript.Store, permissions *tools.PermissionService, secrets secret.Store) *Controller {
	httpClient := &http.Client{Timeout: 2 * time.Minute}

	providers := map[domain.ProviderName]domain.Provider{
		domain.ProviderAnthropic: anthropic.New(httpClient, secrets),
		domain.ProviderOpenAI:    openai.New(httpClient, secrets),
	}

	return NewControllerWithProviders(cfg, configStore, historyStore, transcriptStore, permissions, providers)
}

func NewControllerWithProviders(cfg *domain.Config, configStore config.Store, historyStore history.Store, transcriptStore transcript.Store, permissions *tools.PermissionService, providers map[domain.ProviderName]domain.Provider) *Controller {

	return &Controller{
		config:      cfg,
		configStore: configStore,
		providers:   providers,
		history:     historyStore,
		transcripts: transcriptStore,
		tools:       tools.NewManager(transcriptStore.Paths, permissions),
	}
}

func (c *Controller) ToolDefinitions() []domain.ToolDefinition {
	return c.tools.Definitions()
}

func (c *Controller) RunTurn(ctx context.Context, state *session.State, prompt string, profileName string) (<-chan TurnUpdate, <-chan error) {
	updates := make(chan TurnUpdate)
	errs := make(chan error, 1)

	go func() {
		defer close(updates)
		defer close(errs)

		profile, err := c.resolveProfile(profileName)
		if err != nil {
			errs <- err
			return
		}

		if state.Summary.Provider == "" {
			state.Summary.Provider = profile.Provider
		}
		if state.Summary.Model == "" {
			state.Summary.Model = profile.Model
		}

		userMessage := domain.Message{Role: domain.RoleUser, Content: prompt}
		state.Messages = append(state.Messages, userMessage)

		if err := c.transcripts.Append(state.Summary.SessionID, domain.TranscriptEvent{
			Type:      "user_message",
			Timestamp: time.Now().UTC(),
			SessionID: state.Summary.SessionID,
			Message:   &userMessage,
		}); err != nil {
			errs <- err
			return
		}

		if err := c.history.Append(history.Entry{
			SessionID: state.Summary.SessionID,
			Prompt:    prompt,
		}); err != nil {
			errs <- err
			return
		}

		for {
			request := domain.TurnRequest{
				SessionID:   state.Summary.SessionID,
				Profile:     profile,
				Model:       state.Summary.Model,
				System:      systemPrompt(),
				Messages:    state.Messages,
				Tools:       c.ToolDefinitions(),
				Temperature: profile.Temperature,
			}

			stream, providerErrs := c.providers[profile.Provider].StreamTurn(ctx, request)
			var assistantText string
			var usage *domain.Usage
			var toolCalls []domain.ToolCall

			for stream != nil || providerErrs != nil {
				select {
				case <-ctx.Done():
					_ = c.transcripts.Append(state.Summary.SessionID, domain.TranscriptEvent{
						Type:      "turn_cancelled",
						Timestamp: time.Now().UTC(),
						SessionID: state.Summary.SessionID,
						Error:     ctx.Err().Error(),
					})
					errs <- ctx.Err()
					return
				case event, ok := <-stream:
					if !ok {
						stream = nil
						continue
					}
					switch event.Type {
					case domain.EventTextDelta:
						assistantText += event.Text
						updates <- TurnUpdate{Type: UpdateAssistantDelta, Text: event.Text, SessionID: state.Summary.SessionID}
					case domain.EventToolCall:
						if event.Call != nil {
							call := *event.Call
							toolCalls = append(toolCalls, call)
							updates <- TurnUpdate{Type: UpdateToolCall, ToolCall: &call, SessionID: state.Summary.SessionID}
						}
					case domain.EventDone:
						usage = event.Usage
					}
				case err, ok := <-providerErrs:
					if !ok {
						providerErrs = nil
						continue
					}
					if err != nil {
						errs <- err
						return
					}
				}
			}

			if assistantText != "" {
				message := domain.Message{Role: domain.RoleAssistant, Content: assistantText}
				state.Messages = append(state.Messages, message)
				if err := c.transcripts.Append(state.Summary.SessionID, domain.TranscriptEvent{
					Type:      "assistant_message",
					Timestamp: time.Now().UTC(),
					SessionID: state.Summary.SessionID,
					Message:   &message,
				}); err != nil {
					errs <- err
					return
				}
			}

			if len(toolCalls) == 0 {
				_ = c.transcripts.Append(state.Summary.SessionID, domain.TranscriptEvent{
					Type:      "turn_completed",
					Timestamp: time.Now().UTC(),
					SessionID: state.Summary.SessionID,
					Usage:     usage,
				})
				updates <- TurnUpdate{Type: UpdateDone, Usage: usage, SessionID: state.Summary.SessionID}
				return
			}

			for _, call := range toolCalls {
				callMessage := domain.Message{
					Role:       domain.RoleAssistant,
					ToolCallID: call.ID,
					ToolName:   call.Name,
					ToolInput:  json.RawMessage(call.Input),
				}
				state.Messages = append(state.Messages, callMessage)
				_ = c.transcripts.Append(state.Summary.SessionID, domain.TranscriptEvent{
					Type:      "tool_call",
					Timestamp: time.Now().UTC(),
					SessionID: state.Summary.SessionID,
					ToolCall:  &call,
				})

				result, permissionLog, toolErr := c.tools.Invoke(ctx, domain.ToolInvocation{
					Name:      call.Name,
					Input:     json.RawMessage(call.Input),
					CWD:       state.Summary.CWD,
					SessionID: state.Summary.SessionID,
				})

				if permissionLog != nil {
					_ = c.transcripts.Append(state.Summary.SessionID, domain.TranscriptEvent{
						Type:               "permission_decision",
						Timestamp:          time.Now().UTC(),
						SessionID:          state.Summary.SessionID,
						PermissionDecision: permissionLog,
					})
				}

				if toolErr != nil {
					if errors.Is(toolErr, tools.ErrPermissionDenied) {
						result.Content = fmt.Sprintf("permission denied for tool %s", call.Name)
					} else {
						result.Content = fmt.Sprintf("tool %s failed: %v\n%s", call.Name, toolErr, result.Content)
					}
				}

				toolMessage := domain.Message{
					Role:       domain.RoleTool,
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Content:    result.Content,
				}
				state.Messages = append(state.Messages, toolMessage)
				_ = c.transcripts.Append(state.Summary.SessionID, domain.TranscriptEvent{
					Type:       "tool_result",
					Timestamp:  time.Now().UTC(),
					SessionID:  state.Summary.SessionID,
					ToolResult: &toolMessage,
				})
				updates <- TurnUpdate{
					Type:       UpdateToolResult,
					ToolCall:   &call,
					ToolResult: &result,
					SessionID:  state.Summary.SessionID,
					Permission: permissionLog,
				}
			}
		}
	}()

	return updates, errs
}

func (c *Controller) resolveProfile(name string) (domain.ProviderProfile, error) {
	if name == "" {
		name = c.config.DefaultProfile
	}

	profile, ok := c.config.Profiles[name]
	if !ok {
		return domain.ProviderProfile{}, fmt.Errorf("unknown profile %q", name)
	}

	if profile.Model == "" {
		profile.Model = c.config.DefaultModel
	}
	if profile.Provider == "" {
		profile.Provider = c.config.DefaultProvider
	}
	if profile.Temperature == 0 {
		profile.Temperature = 0.2
	}
	if _, ok := c.providers[profile.Provider]; !ok {
		return domain.ProviderProfile{}, fmt.Errorf("unsupported provider %q", profile.Provider)
	}
	return profile, nil
}

func systemPrompt() string {
	return "You are a coding assistant for a local Unix terminal workflow. Use the available tools carefully, favor direct action, explain concise outcomes, and respect permission boundaries."
}
