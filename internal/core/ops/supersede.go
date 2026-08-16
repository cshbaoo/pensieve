package ops

import (
	"context"
	"fmt"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/core/retrieve"
	"github.com/cshbaoo/pensieve/internal/llm"
)

// MarkSuperseded 把 oldID 原子地替换语义化:置 status=superseded + 追加 superseded-by 链接,
// 重写事实源文件并同步索引。供「新记忆取代旧记忆」的统一动作使用——
// 处置过期不和保存分家,一次确认完成双方更新。
// client 可为未启用(离线):此时仅更新文本/元数据索引,旧记忆的向量行会被清掉
// (superseded 默认不召回,影响可忽略)。
func MarkSuperseded(ctx context.Context, store *memory.Store, idx *index.Index, client *llm.Client, embedModel, oldID, newID string) error {
	old, err := store.GetByID(oldID)
	if err != nil {
		return fmt.Errorf("标记取代 %s: %w", oldID, err)
	}
	if old.ID == newID {
		return fmt.Errorf("记忆不能取代自身: %s", oldID)
	}
	old.Status = "superseded"
	hasLink := false
	for _, lk := range old.Links {
		if lk.Rel == "superseded-by" && lk.ID == newID {
			hasLink = true
		}
	}
	if !hasLink {
		old.Links = append(old.Links, memory.Link{ID: newID, Rel: "superseded-by"})
	}
	if err := store.Write(old); err != nil {
		return err
	}
	var vec []float32
	if client != nil && client.Enabled() {
		if v, err := client.Embed(ctx, embedModel, retrieve.MakeEmbedText(old.Title, old.Body, old.Tags, old.Entities)); err == nil {
			vec = v
		}
	}
	return idx.Upsert(ctx, old, vec, embedModel)
}
