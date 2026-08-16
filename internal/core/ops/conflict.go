package ops

import (
	"context"

	"github.com/cshbaoo/pensieve/internal/core/extract"
	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/core/retrieve"
	"github.com/cshbaoo/pensieve/internal/llm"
)

// ConflictScan 写入侧冲突捕获(双阈值查重):
//   - ≥ hi:已由查重拦截路径另行提示,此函数忽略;
//   - [lo,hi)"像但不同"带宽:逐条交 LLM 判实质关系,返回判定为 conflict(取代性更新)的裁决。
//
// 设计约束:冲突检测是增强,任何失败(embedding/LLM/网络)都静默降级为空——
// 绝不阻塞写入主流程。红线:这里只产出"建议",是否标记取代永远由用户确认时决定。
func ConflictScan(ctx context.Context, idx *index.Index, client *llm.Client, chatModel, embedModel string, lo, hi float64, m *memory.Memory) []extract.Verdict {
	if client == nil || !client.Enabled() || lo <= 0 || lo >= hi {
		return nil
	}
	vec, err := client.Embed(ctx, embedModel, retrieve.MakeEmbedText(m.Title, m.Body, m.Tags, m.Entities))
	if err != nil {
		return nil
	}
	hits, err := retrieve.DedupScan(ctx, idx, vec, lo, 5)
	if err != nil {
		return nil
	}
	var out []extract.Verdict
	for _, h := range hits {
		if h.Score >= hi {
			continue // 重复拦截由查重路径提示,不归冲突判定
		}
		body, _ := idx.GetBody(ctx, h.ID)
		v, err := extract.JudgeRelation(ctx, client, chatModel, m, memory.Memory{ID: h.ID, Title: h.Title, Body: body})
		if err != nil {
			continue
		}
		v.Score = h.Score
		if v.Relation == "conflict" {
			out = append(out, *v)
		}
	}
	return out
}

// ConflictNote 把裁决列表渲染为给确认者看的提示文案(空裁决返回空串)。
// hint 为调用方特定的下一步指引(CLI 建议加 flag;MCP 建议附带 supersedes 参数)。
func ConflictNote(vs []extract.Verdict, hint string) string {
	if len(vs) == 0 {
		return ""
	}
	s := "⚠️ 冲突检测:发现已有记忆与本条结论冲突:\n"
	for _, v := range vs {
		s += "- " + v.OldID + " 「" + v.OldTitle + "」: " + v.Reason + "\n"
	}
	return s + hint
}
