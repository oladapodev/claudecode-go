package ui

import (
	"context"

	itools "github.com/oladapodev/claudecode-go/internal/tools"
)

type permissionEnvelope struct {
	Request  itools.PromptRequest
	Response chan itools.PromptResponse
}

type ChannelPrompter struct {
	requests chan permissionEnvelope
}

func NewChannelPrompter() *ChannelPrompter {
	return &ChannelPrompter{requests: make(chan permissionEnvelope)}
}

func (p *ChannelPrompter) PromptPermission(ctx context.Context, req itools.PromptRequest) (itools.PromptResponse, error) {
	resp := make(chan itools.PromptResponse, 1)

	select {
	case p.requests <- permissionEnvelope{Request: req, Response: resp}:
	case <-ctx.Done():
		return itools.PromptResponse{}, ctx.Err()
	}

	select {
	case decision := <-resp:
		return decision, nil
	case <-ctx.Done():
		return itools.PromptResponse{}, ctx.Err()
	}
}

func (p *ChannelPrompter) Requests() <-chan permissionEnvelope {
	return p.requests
}
