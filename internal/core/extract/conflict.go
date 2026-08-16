package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/llm"
)

// Verdict 新记忆与已有候选的关系判定
type Verdict struct {
	Relation string `json:"relation"` // conflict(结论冲突,新的取代旧的) | complement(互补可并存) | unrelated
	Reason   string `json:"reason"`
	OldID    string // 判定时对应的候选记忆 id(回填)
	OldTitle string
	Score    float64 // 向量相似度(带宽参考)
}

// conflictPrompt 冲突带宽判定提示词:
// 查重(≥0.85)拦"太像";0.60–0.85 带宽里"像但不同"恰是漂移的栖息带,由 LLM 判实质关系。
// 借鉴 mem0 的 ADD/UPDATE/NOOP 写入裁决,但收敛为 human gate 前的建议,不自动改写。
const conflictPrompt = `你是工程记忆库的关系判别器。给定一条「新记忆」和一条「已有记忆」,判断二者实质关系:

- conflict: 两者陈述同一主题的事实/方案/结论但内容不一致,新的更可能是对的(取代性更新)。
  例:旧说"export 功能不存在",新说"export 已实现"——同一断言,结论相反。
- complement: 同主题但互补或可并存(新旧都成立)。
- unrelated: 主题不同,仅表面相似,无需动作。

只比较实质结论,忽略措辞差异。拿不准时输出 complement(宁漏拦,不误标)。

输出严格 JSON(不要 markdown 代码块):
{"relation":"conflict|complement|unrelated","reason":"一句话说明"}`

// JudgeRelation 让 LLM 判定新记忆与一条候选旧记忆的关系。LLM/网络失败返回 error——
// 调用方应静默降级(冲突检测是增强,不可阻塞写入主流程)。
func JudgeRelation(ctx context.Context, client *llm.Client, model string, newM *memory.Memory, old memory.Memory) (*Verdict, error) {
	user := fmt.Sprintf("【新记忆】\n标题: %s\n正文:\n%s\n\n【已有记忆 %s】\n标题: %s\n正文:\n%s",
		newM.Title, truncRunesC(newM.Body, 800), old.ID, old.Title, truncRunesC(old.Body, 800))
	out, err := client.Chat(ctx, model, conflictPrompt, user)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if m := jsonBlock.FindStringSubmatch(out); m != nil {
		out = m[1]
	}
	lo := strings.Index(out, "{")
	hi := strings.LastIndex(out, "}")
	if lo < 0 || hi <= lo {
		return nil, fmt.Errorf("LLM 未返回 JSON: %s", trunc(out, 120))
	}
	var v Verdict
	if err := json.Unmarshal([]byte(out[lo:hi+1]), &v); err != nil {
		return nil, fmt.Errorf("判定 JSON 解析失败: %w", err)
	}
	switch v.Relation {
	case "conflict", "complement", "unrelated":
	default:
		return nil, fmt.Errorf("未知 relation: %q", v.Relation)
	}
	v.OldID, v.OldTitle = old.ID, old.Title
	return &v, nil
}

func truncRunesC(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}
