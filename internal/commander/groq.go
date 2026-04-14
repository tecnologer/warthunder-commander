package commander

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

const (
	groqEndpoint = "https://api.groq.com/openai/v1/chat/completions"
	groqModel    = "llama-3.3-70b-versatile"
)

type groqBackend struct {
	apiKey string
	http   *http.Client
}

func newGroqBackend(envVar string) *groqBackend {
	log.Printf("[commander] using Groq backend (model: %s, env: %s)", groqModel, envVar)

	return &groqBackend{
		apiKey: os.Getenv(envVar),
		http:   &http.Client{},
	}
}

func (g *groqBackend) complete(ctx context.Context, systemPrompt, prompt string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model": groqModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"max_tokens": 100,
	})
	if err != nil {
		return "", fmt.Errorf("groq: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, groqEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("groq: build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("groq: request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("groq: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("groq: status %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("groq: parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", nil
	}

	return result.Choices[0].Message.Content, nil
}
