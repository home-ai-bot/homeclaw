// Package llm provides LLM chat utilities for semantic recognition.
package llm

import (
	"context"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/providers"
)

type LLM struct {
	Provider providers.LLMProvider
	Model    string
}

// Chat performs a simple LLM chat without tools for semantic recognition.
// It uses the smallProvider and smallModel configured in the factory.
// systemPrompt: the system instruction for the LLM
// userMessage: the user input to process
// Returns the LLM response content or error.
func (f *LLM) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	if f.Provider == nil {
		return "", fmt.Errorf("chat failed: provider is null")
	}

	messages := []providers.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	resp, err := f.Provider.Chat(ctx, messages, nil, f.Model, nil)
	if err != nil {
		return "", fmt.Errorf("chat failed: %w", err)
	}

	if resp == nil || resp.Content == "" {
		return "", fmt.Errorf("empty response from LLM")
	}

	return resp.Content, nil
}

// ChatWithMessages performs a simple LLM chat with custom messages without tools.
// Useful when you need multi-turn conversation or custom message structure.
func (f *LLM) ChatWithMessages(ctx context.Context, messages []providers.Message) (string, error) {
	if f.Provider == nil {
		return "", fmt.Errorf("chat failed: provider is null")
	}
	resp, err := f.Provider.Chat(ctx, messages, nil, f.Model, nil)
	if err != nil {
		return "", fmt.Errorf("chat failed: %w", err)
	}

	if resp == nil || resp.Content == "" {
		return "", fmt.Errorf("empty response from LLM")
	}

	return resp.Content, nil
}
