package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/oladapodev/claudecode-go/internal/app"
	"github.com/oladapodev/claudecode-go/internal/domain"
	"github.com/oladapodev/claudecode-go/internal/session"
	itools "github.com/oladapodev/claudecode-go/internal/tools"
	"github.com/oladapodev/claudecode-go/internal/transcript"
)

type message struct {
	Role      string
	Content   string
	Timestamp time.Time
}

type overlayKind string

const (
	overlayNone       overlayKind = ""
	overlayPermission overlayKind = "permission"
	overlayResume     overlayKind = "resume"
)

type overlayState struct {
	kind              overlayKind
	permissionRequest *permissionEnvelope
	resumeList        []domain.SessionSummary
	selected          int
}

type turnUpdateMsg struct{ update app.TurnUpdate }
type turnErrMsg struct{ err error }
type permissionRequestMsg struct{ envelope permissionEnvelope }

type model struct {
	services *app.Services
	prompter *ChannelPrompter

	viewport viewport.Model
	input    textinput.Model

	state       session.State
	messages    []message
	status      string
	errText     string
	width       int
	height      int
	overlay     overlayState
	inFlight    bool
	profileName string

	turnUpdates <-chan app.TurnUpdate
	turnErrs    <-chan error
	cancelTurn  context.CancelFunc

	userStyle      lipgloss.Style
	assistantStyle lipgloss.Style
	metaStyle      lipgloss.Style
	statusStyle    lipgloss.Style
	overlayStyle   lipgloss.Style
}

func Run(services *app.Services, prompter *ChannelPrompter, initial *session.State) error {
	var state session.State
	if initial != nil {
		state = *initial
	} else {
		next, err := session.New(transcript.NewSessionID(), services.Config.DefaultProvider, services.Config.DefaultModel)
		if err != nil {
			return err
		}
		if err := services.Transcript.StartSession(next.Summary); err != nil {
			return err
		}
		state = next
	}

	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Ask for help or use /help"
	input.Focus()

	vp := viewport.New(0, 0)
	m := &model{
		services:       services,
		prompter:       prompter,
		viewport:       vp,
		input:          input,
		state:          state,
		messages:       messagesFromSession(state),
		status:         fmt.Sprintf("session=%s model=%s", shortID(state.Summary.SessionID), state.Summary.Model),
		userStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true),
		assistantStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("230")),
		metaStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		statusStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		overlayStyle:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255")),
	}
	m.syncViewport()

	program := tea.NewProgram(m, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func (m *model) Init() tea.Cmd {
	return waitForPermission(m.prompter.Requests())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = max(5, msg.Height-4)
		m.syncViewport()
	case tea.KeyMsg:
		if m.overlay.kind != overlayNone {
			return m.handleOverlayKey(msg)
		}
		switch msg.String() {
		case "ctrl+c":
			if m.inFlight && m.cancelTurn != nil {
				m.cancelTurn()
				m.status = "cancelling current turn..."
				return m, nil
			}
			return m, tea.Quit
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			if value == "" || m.inFlight {
				return m, nil
			}
			m.input.SetValue("")
			if strings.HasPrefix(value, "/") {
				return m.handleSlashCommand(value)
			}
			return m.submitPrompt(value)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case turnUpdateMsg:
		return m.handleTurnUpdate(msg.update)
	case turnErrMsg:
		if msg.err != nil {
			m.errText = msg.err.Error()
			m.inFlight = false
			m.cancelTurn = nil
			m.status = "turn failed"
		}
	case permissionRequestMsg:
		m.overlay = overlayState{
			kind:              overlayPermission,
			permissionRequest: &msg.envelope,
		}
		m.status = "permission required"
	}

	return m, nil
}

func (m *model) View() string {
	base := lipgloss.JoinVertical(lipgloss.Left,
		m.viewport.View(),
		m.statusStyle.Width(m.width).Render(m.footer()),
		m.input.View(),
	)
	if m.overlay.kind == overlayNone {
		return base
	}
	return lipgloss.JoinVertical(lipgloss.Left, base, "", m.overlayStyle.Render(m.overlayContent()))
}

func (m *model) submitPrompt(value string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, message{Role: "user", Content: value, Timestamp: time.Now()})
	m.messages = append(m.messages, message{Role: "assistant", Content: "", Timestamp: time.Now()})
	m.syncViewport()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelTurn = cancel
	m.inFlight = true
	m.status = "thinking..."

	updates, errs := m.services.Controller.RunTurn(ctx, &m.state, value, m.profileName)
	m.turnUpdates = updates
	m.turnErrs = errs
	return m, tea.Batch(waitForTurnUpdate(updates), waitForTurnErr(errs))
}

func (m *model) handleTurnUpdate(update app.TurnUpdate) (tea.Model, tea.Cmd) {
	switch update.Type {
	case app.UpdateAssistantDelta:
		m.status = "streaming..."
		if len(m.messages) == 0 || m.messages[len(m.messages)-1].Role != "assistant" {
			m.messages = append(m.messages, message{Role: "assistant", Timestamp: time.Now()})
		}
		m.messages[len(m.messages)-1].Content += update.Text
	case app.UpdateToolCall:
		if update.ToolCall != nil {
			m.messages = append(m.messages, message{
				Role:      "meta",
				Content:   fmt.Sprintf("[tool] %s %s", update.ToolCall.Name, string(update.ToolCall.Input)),
				Timestamp: time.Now(),
			})
		}
	case app.UpdateToolResult:
		if update.ToolResult != nil {
			text := update.ToolResult.Content
			if update.Permission != nil && !update.Permission.Allowed {
				text = fmt.Sprintf("%s\n%s", update.Permission.Reason, text)
			}
			m.messages = append(m.messages, message{
				Role:      "meta",
				Content:   "[tool-result] " + text,
				Timestamp: time.Now(),
			})
		}
	case app.UpdateDone:
		m.inFlight = false
		m.cancelTurn = nil
		m.status = "idle"
		if update.Usage != nil {
			m.status = fmt.Sprintf("idle input=%d output=%d", update.Usage.InputTokens, update.Usage.OutputTokens)
		}
		m.syncViewport()
		return m, nil
	}

	m.syncViewport()
	return m, tea.Batch(waitForTurnUpdate(m.turnUpdates), waitForTurnErr(m.turnErrs))
}

func (m *model) handleSlashCommand(value string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(value)
	cmd := strings.TrimPrefix(fields[0], "/")
	arg := ""
	if len(fields) > 1 {
		arg = strings.Join(fields[1:], " ")
	}

	switch cmd {
	case "help":
		m.messages = append(m.messages, message{Role: "meta", Content: helpText(), Timestamp: time.Now()})
	case "clear":
		next, err := session.New(transcript.NewSessionID(), m.services.Config.DefaultProvider, m.state.Summary.Model)
		if err != nil {
			m.errText = err.Error()
			return m, nil
		}
		if err := m.services.Transcript.StartSession(next.Summary); err != nil {
			m.errText = err.Error()
			return m, nil
		}
		m.state = next
		m.messages = nil
		m.status = "started new session " + shortID(next.Summary.SessionID)
	case "resume":
		list, err := m.services.Transcript.List(arg)
		if err != nil {
			m.errText = err.Error()
			return m, nil
		}
		if len(list) == 0 {
			m.messages = append(m.messages, message{Role: "meta", Content: "no matching sessions", Timestamp: time.Now()})
			break
		}
		if len(list) == 1 {
			if err := m.loadSession(list[0].SessionID); err != nil {
				m.errText = err.Error()
			}
			break
		}
		m.overlay = overlayState{kind: overlayResume, resumeList: list}
	case "model":
		if arg == "" {
			m.messages = append(m.messages, message{Role: "meta", Content: "current model: " + m.state.Summary.Model, Timestamp: time.Now()})
		} else {
			m.state.Summary.Model = arg
			m.status = "model set to " + arg
		}
	case "config":
		m.messages = append(m.messages, message{
			Role:      "meta",
			Content:   fmt.Sprintf("config=%s\nprofile=%s\nprovider=%s\nmodel=%s", m.services.Paths.ConfigFile, m.services.Config.DefaultProfile, m.state.Summary.Provider, m.state.Summary.Model),
			Timestamp: time.Now(),
		})
	case "permissions":
		rules := m.services.Permissions.Rules()
		if len(rules) == 0 {
			m.messages = append(m.messages, message{Role: "meta", Content: "no permission rules configured", Timestamp: time.Now()})
			break
		}
		lines := make([]string, 0, len(rules))
		for _, rule := range rules {
			lines = append(lines, fmt.Sprintf("%s %s -> %s", rule.Tool, rule.Pattern, rule.Action))
		}
		m.messages = append(m.messages, message{Role: "meta", Content: strings.Join(lines, "\n"), Timestamp: time.Now()})
	case "history":
		entries, err := m.services.History.Recent(10)
		if err != nil {
			m.errText = err.Error()
			return m, nil
		}
		if len(entries) == 0 {
			m.messages = append(m.messages, message{Role: "meta", Content: "history is empty", Timestamp: time.Now()})
			break
		}
		lines := make([]string, 0, len(entries))
		for _, entry := range entries {
			lines = append(lines, entry.Prompt)
		}
		m.messages = append(m.messages, message{Role: "meta", Content: strings.Join(lines, "\n"), Timestamp: time.Now()})
	case "quit", "exit":
		return m, tea.Quit
	default:
		m.messages = append(m.messages, message{Role: "meta", Content: "unknown command: " + cmd, Timestamp: time.Now()})
	}

	m.syncViewport()
	return m, nil
}

func (m *model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay.kind {
	case overlayPermission:
		return m.handlePermissionKey(msg)
	case overlayResume:
		return m.handleResumeKey(msg)
	default:
		return m, nil
	}
}

func (m *model) handlePermissionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.overlay.permissionRequest == nil {
		m.overlay = overlayState{}
		return m, waitForPermission(m.prompter.Requests())
	}

	var response itools.PromptResponse
	valid := true

	switch msg.String() {
	case "y":
		response = itools.PromptResponse{Action: domain.PermissionAllow, Scope: "once"}
	case "a":
		response = itools.PromptResponse{Action: domain.PermissionAllow, Scope: "session"}
	case "A":
		response = itools.PromptResponse{Action: domain.PermissionAllow, Scope: "config"}
	case "n", "esc":
		response = itools.PromptResponse{Action: domain.PermissionDeny, Scope: "once"}
	case "d":
		response = itools.PromptResponse{Action: domain.PermissionDeny, Scope: "session"}
	case "D":
		response = itools.PromptResponse{Action: domain.PermissionDeny, Scope: "config"}
	default:
		valid = false
	}

	if !valid {
		return m, nil
	}

	m.overlay.permissionRequest.Response <- response
	m.overlay = overlayState{}
	m.status = "permission decision recorded"
	return m, waitForPermission(m.prompter.Requests())
}

func (m *model) handleResumeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.overlay.selected > 0 {
			m.overlay.selected--
		}
	case "down", "j":
		if m.overlay.selected < len(m.overlay.resumeList)-1 {
			m.overlay.selected++
		}
	case "enter":
		if len(m.overlay.resumeList) > 0 {
			if err := m.loadSession(m.overlay.resumeList[m.overlay.selected].SessionID); err != nil {
				m.errText = err.Error()
			}
		}
		m.overlay = overlayState{}
	case "esc":
		m.overlay = overlayState{}
	}
	return m, nil
}

func (m *model) loadSession(sessionID string) error {
	events, err := m.services.Transcript.Load(sessionID)
	if err != nil {
		return err
	}
	m.state = session.FromEvents(events)
	m.messages = messagesFromSession(m.state)
	m.status = "resumed session " + shortID(sessionID)
	m.syncViewport()
	return nil
}

func (m *model) overlayContent() string {
	switch m.overlay.kind {
	case overlayPermission:
		req := m.overlay.permissionRequest.Request
		return fmt.Sprintf("Permission required\n\nTool: %s\nTarget: %s\nReason: %s\n\nKeys:\n y allow once\n a allow this session\n A allow and persist\n n deny once\n d deny this session\n D deny and persist", req.Tool, req.Target, req.Reason)
	case overlayResume:
		lines := []string{"Resume Session", ""}
		for i, summary := range m.overlay.resumeList {
			prefix := "  "
			if i == m.overlay.selected {
				prefix = "> "
			}
			lines = append(lines, fmt.Sprintf("%s%s  %s  %s", prefix, shortID(summary.SessionID), summary.Title, summary.Model))
		}
		lines = append(lines, "", "Use arrows and Enter to select.")
		return strings.Join(lines, "\n")
	default:
		return ""
	}
}

func (m *model) footer() string {
	parts := []string{m.status}
	if m.inFlight {
		parts = append(parts, "ctrl+c cancels current turn")
	}
	return strings.Join(parts, " | ")
}

func (m *model) syncViewport() {
	lines := make([]string, 0, len(m.messages)+1)
	for _, msg := range m.messages {
		lines = append(lines, renderMessage(msg, m.services.Config.UI.ShowTimestamps, m.userStyle, m.assistantStyle, m.metaStyle))
	}
	if m.errText != "" {
		lines = append(lines, m.metaStyle.Render("[error] "+m.errText))
	}
	m.viewport.SetContent(strings.Join(lines, "\n\n"))
	m.viewport.GotoBottom()
}

func waitForTurnUpdate(ch <-chan app.TurnUpdate) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return nil
		}
		return turnUpdateMsg{update: update}
	}
}

func waitForTurnErr(ch <-chan error) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		err, ok := <-ch
		if !ok {
			return nil
		}
		return turnErrMsg{err: err}
	}
}

func waitForPermission(ch <-chan permissionEnvelope) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		req := <-ch
		return permissionRequestMsg{envelope: req}
	}
}

func messagesFromSession(state session.State) []message {
	result := make([]message, 0, len(state.Messages))
	for _, item := range state.Messages {
		role := string(item.Role)
		content := item.Content
		if item.Role == domain.RoleAssistant && item.ToolName != "" {
			role = "meta"
			content = fmt.Sprintf("[tool] %s %s", item.ToolName, string(item.ToolInput))
		}
		result = append(result, message{
			Role:      role,
			Content:   content,
			Timestamp: state.Summary.CreatedAt,
		})
	}
	return result
}

func renderMessage(msg message, showTimestamps bool, userStyle, assistantStyle, metaStyle lipgloss.Style) string {
	prefix := "[" + msg.Role + "]"
	if showTimestamps {
		prefix = msg.Timestamp.Format("15:04:05") + " " + prefix
	}
	switch msg.Role {
	case "user":
		return userStyle.Render(prefix + " " + msg.Content)
	case "assistant":
		return assistantStyle.Render(prefix + " " + msg.Content)
	default:
		return metaStyle.Render(prefix + " " + msg.Content)
	}
}

func helpText() string {
	return strings.Join([]string{
		"/help",
		"/clear",
		"/resume [query]",
		"/model [name]",
		"/config",
		"/permissions",
		"/history",
		"/quit",
	}, "\n")
}

func shortID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
