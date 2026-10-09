package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"wobble/internal/config"
)

type MessageHistory struct {
	Role    string `json:"role"` // "user" | "assistant" | "system"
	Content string `json:"content"`
}

type Client interface {
	GenerateReply(ctx context.Context, systemPrompt string, history []MessageHistory, newMsg string) (string, error)
	Enabled() bool
}

type openAIRequest struct {
	Model    string           `json:"model"`
	Messages []MessageHistory `json:"messages"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type client struct {
	endpoint   string
	apiKey     string
	model      string
	timeout    time.Duration
	httpClient *http.Client
}

func NewClient(cfg *config.Config) Client {
	return &client{
		endpoint: cfg.AIEndpoint,
		apiKey:   cfg.AIAPIKey,
		model:    cfg.AIModel,
		timeout:  cfg.AITimeout,
		httpClient: &http.Client{
			Timeout: cfg.AITimeout,
		},
	}
}

func (c *client) Enabled() bool {
	return c.apiKey != ""
}

func (c *client) GenerateReply(ctx context.Context, systemPrompt string, history []MessageHistory, newMsg string) (string, error) {
	if !c.Enabled() {
		return "", errors.New("ai service is disabled (no API key configured)")
	}

	messages := make([]MessageHistory, 0, len(history)+2)
	if systemPrompt != "" {
		messages = append(messages, MessageHistory{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	messages = append(messages, history...)
	messages = append(messages, MessageHistory{
		Role:    "user",
		Content: newMsg,
	})

	reqBody, err := json.Marshal(openAIRequest{
		Model:    c.model,
		Messages: messages,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal ai request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ai api request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read ai response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ai api returned status %d: %s", resp.StatusCode, string(body))
	}

	var aiResp openAIResponse
	if err := json.Unmarshal(body, &aiResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal ai response: %w", err)
	}

	if aiResp.Error != nil && aiResp.Error.Message != "" {
		return "", fmt.Errorf("ai api error: %s", aiResp.Error.Message)
	}

	if len(aiResp.Choices) == 0 || aiResp.Choices[0].Message.Content == "" {
		return "", errors.New("ai api returned empty reply")
	}

	return aiResp.Choices[0].Message.Content, nil
}
