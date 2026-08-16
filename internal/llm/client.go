package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client OpenAI 兼容 API 客户端（任意提供方）
type Client struct {
	BaseURL string
	APIKey  string
	http    *http.Client
}

func New(baseURL, apiKey, timeout string) *Client {
	d, _ := time.ParseDuration(timeout)
	if d == 0 {
		d = 30 * time.Second
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		http:    &http.Client{Timeout: d},
	}
}

// Enabled 未配置 apiKey 时置为不可用（退化纯本地检索）
func (c *Client) Enabled() bool { return c != nil && c.APIKey != "" }

func (c *Client) post(ctx context.Context, path string, reqBody any, respBody any) error {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("API %s -> %d: %s", path, resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(respBody)
}

type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed 文本向量化（429/抖动自动退避重试 3 次）
func (c *Client) Embed(ctx context.Context, model, text string) ([]float32, error) {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		var vec []float32
		vec, err = c.embedOnce(ctx, model, text)
		if err == nil {
			return vec, nil
		}
		time.Sleep(time.Duration(500*(1<<attempt)) * time.Millisecond) // 500ms → 1s → 2s
	}
	return nil, err
}

func (c *Client) embedOnce(ctx context.Context, model, text string) ([]float32, error) {
	var resp embedResponse
	if err := c.post(ctx, "/embeddings", embedRequest{Model: model, Input: text}, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embedding 返回为空")
	}
	return resp.Data[0].Embedding, nil
}
