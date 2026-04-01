package transcript

import "github.com/oladapodev/claudecode-go/internal/domain"

func RebuildConversation(events []domain.TranscriptEvent) ([]domain.Message, domain.SessionSummary) {
	messages := make([]domain.Message, 0, len(events))
	var summary domain.SessionSummary

	for _, event := range events {
		switch event.Type {
		case "session_started":
			summary = domain.SessionSummary{
				SessionID: event.SessionID,
				Title:     event.Title,
				CWD:       event.CWD,
				Provider:  event.Provider,
				Model:     event.Model,
				CreatedAt: event.Timestamp,
			}
		case "user_message", "assistant_message":
			if event.Message != nil {
				messages = append(messages, *event.Message)
			}
		case "tool_call":
			if event.ToolCall != nil {
				messages = append(messages, domain.Message{
					Role:       domain.RoleAssistant,
					ToolCallID: event.ToolCall.ID,
					ToolName:   event.ToolCall.Name,
					ToolInput:  event.ToolCall.Input,
				})
			}
		case "tool_result":
			if event.ToolResult != nil {
				messages = append(messages, *event.ToolResult)
			}
		}
	}

	return messages, summary
}
