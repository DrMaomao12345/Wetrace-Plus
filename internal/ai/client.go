package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type ChatCompletionResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type Client struct {
	APIKey  string
	BaseURL string
	Model   string
	HTTP    *http.Client
}

func NewClient(apiKey, baseURL, model string) *Client {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &Client{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
		HTTP: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) Chat(messages []Message) (string, error) {
	reqBody := ChatCompletionRequest{
		Model:    c.Model,
		Messages: messages,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/chat/completions", c.BaseURL), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result ChatCompletionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("AI API returned no choices")
	}

	return result.Choices[0].Message.Content, nil
}

func (c *Client) TestConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reqBody := ChatCompletionRequest{
		Model:    c.Model,
		Messages: []Message{{Role: "user", Content: "test"}},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/chat/completions", c.BaseURL), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("API Key 无效")
	}

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("API 返回错误 (状态码 %d): 无法读取响应体", resp.StatusCode)
		}
		return fmt.Errorf("API 返回错误 (状态码 %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
