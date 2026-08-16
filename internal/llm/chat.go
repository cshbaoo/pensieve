package llm

import "context"

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Chat 单轮对话（system + user）
func (c *Client) Chat(ctx context.Context, model, system, user string) (string, error) {
	var resp chatResponse
	err := c.post(ctx, "/chat/completions", chatRequest{
		Model:       model,
		Temperature: 0.2,
		Messages:    []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
	}, &resp)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", &apiError{"chat 返回为空"}
	}
	return resp.Choices[0].Message.Content, nil
}

type apiError struct{ msg string }

func (e *apiError) Error() string { return e.msg }
