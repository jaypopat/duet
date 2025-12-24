package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client communicates with the Duet CF Worker AI endpoints
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a new AI client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// MessageRequest is the request body for /message endpoint
type MessageRequest struct {
	Text   string `json:"text"`
	UserID string `json:"userId,omitempty"`
}

// ChatMessage represents a message in the conversation history
type ChatMessage struct {
	Role   string `json:"role"`
	UserID string `json:"userId,omitempty"`
	Text   string `json:"text"`
	Ts     int64  `json:"ts"`
}

// MessageResponse is the response from /message endpoint
type MessageResponse struct {
	Reply    string        `json:"reply"`
	Messages []ChatMessage `json:"messages"`
	Error    string        `json:"error,omitempty"`
}

// ExecRequest is the request body for /sandbox/exec endpoint
type ExecRequest struct {
	Cmd string `json:"cmd"`
}

// ExecResult contains stdout/stderr from sandbox execution
type ExecResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// ExecResponse is the response from /sandbox/exec endpoint
type ExecResponse struct {
	Result      ExecResult `json:"result"`
	SandboxName string     `json:"sandboxName"`
	Error       string     `json:"error,omitempty"`
}

// SendMessage sends a message to the AI and returns the response
func (c *Client) SendMessage(ctx context.Context, roomID, text, userID string) (*MessageResponse, error) {
	url := fmt.Sprintf("%s/api/rooms/%s/message", c.baseURL, roomID)

	body := MessageRequest{
		Text:   text,
		UserID: userID,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	var result MessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("api error: %s", result.Error)
	}

	return &result, nil
}

// ExecCommand executes a command in the room's sandbox
func (c *Client) ExecCommand(ctx context.Context, roomID, cmd string) (*ExecResponse, error) {
	url := fmt.Sprintf("%s/api/rooms/%s/sandbox/exec", c.baseURL, roomID)

	body := ExecRequest{
		Cmd: cmd,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	var result ExecResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("sandbox error: %s", result.Error)
	}

	return &result, nil
}

