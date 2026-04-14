package commander

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

type anthropicBackend struct {
	client anthropic.Client
}

func newAnthropicBackend(envVar string) *anthropicBackend {
	log.Printf("[commander] using Anthropic backend (model: %s, env: %s)", anthropic.ModelClaudeOpus4_6, envVar)

	return &anthropicBackend{client: anthropic.NewClient()}
}

func (a *anthropicBackend) complete(ctx context.Context, systemPrompt, prompt string) (string, error) {
	stream := a.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeOpus4_6,
		MaxTokens: 100,
		Thinking:  anthropic.ThinkingConfigParamUnion{OfDisabled: new(anthropic.NewThinkingConfigDisabledParam())},
		System: []anthropic.TextBlockParam{{
			Text: systemPrompt,
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})

	msg := anthropic.Message{}
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return "", fmt.Errorf("anthropic accumulate: %w", err)
		}
	}

	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("anthropic stream: %w", err)
	}

	var builder strings.Builder

	for _, block := range msg.Content {
		if textBlock, ok := block.AsAny().(anthropic.TextBlock); ok {
			builder.WriteString(textBlock.Text)
		}
	}

	return builder.String(), nil
}
