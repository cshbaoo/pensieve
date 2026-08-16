// Package ops 跨包的高阶操作：重建索引等
package ops

import (
	"context"
	"log"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/core/retrieve"
	"github.com/cshbaoo/pensieve/internal/llm"
)

// RebuildResult 索引重建结果
type RebuildResult struct {
	IndexedTotal int // 索引内现有记忆总数
	Upserted     int // 本轮重新写入的条数
	Removed      int // 清理掉的"文件已删索引还在"残留
}

// Rebuild 重建索引。
// full=true 时强制全量重嵌；否则按文件指纹(mtime+size)增量。
// 同时清理"文件已删但索引残留"的脏条目。
func Rebuild(ctx context.Context, store *memory.Store, idx *index.Index, client *llm.Client, embedModel string, full bool) (*RebuildResult, error) {
	seen := map[string]bool{}
	upserted := 0

	err := store.WalkWithStat(func(m *memory.Memory, size, modUnix int64) error {
		seen[m.ID] = true
		if !full {
			changed, err := idx.FileChanged(ctx, m.ID, size, modUnix)
			if err == nil && !changed {
				return nil // 未变动,跳过 embedding 调用,省一大块时间
			}
		}
		var vec []float32
		if client.Enabled() {
			v, err := client.Embed(ctx, embedModel, retrieve.MakeEmbedText(m.Title, m.Body, m.Tags, m.Entities))
			if err == nil {
				vec = v
			} else {
				log.Printf("[rebuild] embedding失败 %s: %v", m.ID, err)
			}
		}
		if err := idx.Upsert(ctx, m, vec, embedModel); err != nil {
			return err
		}
		if err := idx.MarkFile(ctx, m.ID, size, modUnix); err != nil {
			return err
		}
		upserted++
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 清理已删文件的索引残留
	ids, err := idx.AllIDs(ctx)
	if err != nil {
		return nil, err
	}
	removed := 0
	for _, id := range ids {
		if !seen[id] {
			if err := idx.Delete(ctx, id); err != nil {
				return nil, err
			}
			removed++
		}
	}

	n, err := idx.Count(ctx)
	if err != nil {
		return nil, err
	}
	return &RebuildResult{IndexedTotal: n, Upserted: upserted, Removed: removed}, nil
}
