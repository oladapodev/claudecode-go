package tools

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oladapodev/claudecode-go/internal/domain"
)

type PromptRequest struct {
	Tool   string
	Target string
	Reason string
}

type PromptResponse struct {
	Action domain.PermissionAction
	Scope  string
}

type Prompter interface {
	PromptPermission(context.Context, PromptRequest) (PromptResponse, error)
}

type PermissionService struct {
	mu           sync.RWMutex
	config       *domain.Config
	saveConfig   func(domain.Config) error
	sessionRules []domain.PermissionRule
	prompter     Prompter
}

func NewPermissionService(cfg *domain.Config, save func(domain.Config) error, prompter Prompter) *PermissionService {
	return &PermissionService{
		config:     cfg,
		saveConfig: save,
		prompter:   prompter,
	}
}

func (s *PermissionService) SetPrompter(prompter Prompter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompter = prompter
}

func (s *PermissionService) Resolve(ctx context.Context, tool, target, reason string, fallback domain.PermissionAction) (domain.PermissionDecision, error) {
	s.mu.RLock()
	for _, rule := range s.sessionRules {
		if matchesRule(rule, tool, target) {
			s.mu.RUnlock()
			return domain.PermissionDecision{Action: rule.Action, Scope: "session", Reason: "matched session rule"}, nil
		}
	}

	for _, rule := range s.config.Permissions.Rules {
		if matchesRule(rule, tool, target) {
			s.mu.RUnlock()
			return domain.PermissionDecision{Action: rule.Action, Scope: "config", Reason: "matched config rule"}, nil
		}
	}

	defaultAction := s.config.Permissions.DefaultAction
	prompter := s.prompter
	s.mu.RUnlock()

	if fallback == domain.PermissionAllow {
		return domain.PermissionDecision{Action: domain.PermissionAllow, Scope: "builtin", Reason: "safe built-in rule"}, nil
	}

	if defaultAction == domain.PermissionAllow && fallback != domain.PermissionDeny {
		return domain.PermissionDecision{Action: domain.PermissionAllow, Scope: "default", Reason: "default allow"}, nil
	}

	if defaultAction == domain.PermissionDeny || fallback == domain.PermissionDeny {
		return domain.PermissionDecision{Action: domain.PermissionDeny, Scope: "default", Reason: "default deny"}, nil
	}

	if prompter == nil {
		return domain.PermissionDecision{Action: domain.PermissionDeny, Scope: "none", Reason: "permission prompt unavailable"}, nil
	}

	response, err := prompter.PromptPermission(ctx, PromptRequest{
		Tool:   tool,
		Target: target,
		Reason: reason,
	})
	if err != nil {
		return domain.PermissionDecision{}, err
	}

	decision := domain.PermissionDecision{
		Action: response.Action,
		Scope:  response.Scope,
		Reason: reason,
	}

	s.persist(tool, target, response)

	return decision, nil
}

func (s *PermissionService) Rules() []domain.PermissionRule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rules := make([]domain.PermissionRule, 0, len(s.sessionRules)+len(s.config.Permissions.Rules))
	rules = append(rules, s.config.Permissions.Rules...)
	rules = append(rules, s.sessionRules...)
	return rules
}

func (s *PermissionService) persist(tool, target string, response PromptResponse) {
	if response.Scope != "session" && response.Scope != "config" {
		return
	}

	rule := domain.PermissionRule{
		Tool:       tool,
		Pattern:    target,
		Action:     response.Action,
		Persistent: response.Scope == "config",
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if response.Scope == "session" {
		s.sessionRules = append(s.sessionRules, rule)
		return
	}

	s.config.Permissions.Rules = append(s.config.Permissions.Rules, rule)
	if s.saveConfig != nil {
		_ = s.saveConfig(*s.config)
	}
}

func matchesRule(rule domain.PermissionRule, tool, target string) bool {
	if rule.Tool != "*" && rule.Tool != tool {
		return false
	}

	if rule.Pattern == "*" {
		return true
	}

	if strings.HasSuffix(rule.Pattern, string(filepath.Separator)) {
		return strings.HasPrefix(target, rule.Pattern)
	}

	if ok, err := filepath.Match(rule.Pattern, target); err == nil && ok {
		return true
	}

	return target == rule.Pattern || strings.HasPrefix(target, rule.Pattern+string(filepath.Separator))
}
