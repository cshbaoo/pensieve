package topic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/llm"
)

// Draft LLM 产生的 topic 草稿
type Draft struct {
	Title    string   `json:"title"`
	Links    []string `json:"links"` // 子记忆 id 列表
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords"`
}

const genPrompt = `你是编程团队的知识管理员。用户想为主题 "%s" 建一个"记忆卷宗"(topic)。
下面是候选记忆列表(id + 标题)。
请选出真正属于该主题的子记忆(宁缺毋滥),并给这个卷宗写一句导航说明。

回应严格 JSON(不要其他内容,不要用 markdown 围栏):
{"title":"主题规范名","links":["mem_..."],"summary":"一句话导航","keywords":["k1","k2"]}

若没有任何候选真正属于该主题,输出 {"skip":true}`

// Generate 用 LLM 从候选记忆里筛出属于主题的子记忆
func Generate(ctx context.Context, client *llm.Client, model, theme string, candidates []index.Row) (*Draft, bool, error) {
	if len(candidates) == 0 {
		return nil, true, nil
	}
	var sb strings.Builder
	for _, c := range candidates {
		fmt.Fprintf(&sb, "- %s: %s [%s]\n", c.ID, c.Title, c.Type)
	}
	out, err := client.Chat(ctx, model, fmt.Sprintf(genPrompt, theme), sb.String())
	if err != nil {
		return nil, false, err
	}
	out = strings.TrimSpace(out)
	lo, hi := strings.Index(out, "{"), strings.LastIndex(out, "}")
	if lo < 0 || hi <= lo {
		return nil, false, fmt.Errorf("LLM 未返回 JSON: %s", trunc(out, 200))
	}
	var d struct {
		Draft
		Skip bool `json:"skip"`
	}
	if err := json.Unmarshal([]byte(out[lo:hi+1]), &d); err != nil {
		return nil, false, fmt.Errorf("解析失败: %w", err)
	}
	if d.Skip || d.Draft.Title == "" || len(d.Draft.Links) == 0 {
		return nil, true, nil
	}
	return &d.Draft, false, nil
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
