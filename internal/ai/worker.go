package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type WorkerClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewWorkerClient(baseURL string) *WorkerClient {
	return &WorkerClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

type workerMessageRequest struct {
	UserID string `json:"userId,omitempty"`
	Text   string `json:"text"`
}

type workerMessageResponse struct {
	Reply    string `json:"reply"`
	Error    string `json:"error"`
	Messages any    `json:"messages"`
}

func (c *WorkerClient) Enabled() bool {
	return strings.TrimSpace(c.BaseURL) != ""
}

func (c *WorkerClient) SendMessage(ctx context.Context, roomID, userID, text string) (string, error) {
    if !c.Enabled() {
        return "", errors.New("worker url not configured")
    }
    if roomID == "" || text == "" {
        return "", errors.New("missing room id or text")
    }

    base, err := url.Parse(c.BaseURL)
    if err != nil {
        return "", fmt.Errorf("invalid worker url: %w", err)
    }
    base.Path = path.Join(base.Path, "/api/rooms", roomID, "message")

    body, _ := json.Marshal(workerMessageRequest{
        UserID: strings.TrimSpace(userID),
        Text:   strings.TrimSpace(text),
    })

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(body))
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.HTTP.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
    if resp.StatusCode >= 300 {
        return "", fmt.Errorf("worker error %d: %s", resp.StatusCode, string(respBody))
    }

    var out workerMessageResponse
    if err := json.Unmarshal(respBody, &out); err != nil {
        return "", fmt.Errorf("bad worker response: %w", err)
    }
    if out.Error != "" {
        return "", errors.New(out.Error)
    }

    return strings.TrimSpace(out.Reply), nil
}

