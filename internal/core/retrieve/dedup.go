package retrieve

import (
	"context"
	"sort"

	"github.com/cshbaoo/pensieve/internal/core/index"
)

// DupHit 疑似重复记忆
type DupHit struct {
	ID    string
	Title string
	Score float64
}

// DedupCheck 写入前查重：候选与每条现有记忆做向量余弦，返回超过阈值的最相似记忆
func DedupCheck(ctx context.Context, idx *index.Index, vec []float32, threshold float64) (*DupHit, error) {
	hits, err := DedupScan(ctx, idx, vec, threshold, 1)
	if err != nil || len(hits) == 0 {
		return nil, err
	}
	return &hits[0], nil
}

// DedupScan 相似度扫描：返回所有 ≥ threshold 的候选(按相似度降序,最多 limit 条)。
// 与 DedupCheck 的单条拦截不同,供冲突带宽判定(conflict band)一次取回多条候选。
func DedupScan(ctx context.Context, idx *index.Index, vec []float32, threshold float64, limit int) ([]DupHit, error) {
	if len(vec) == 0 || limit <= 0 {
		return nil, nil
	}
	cands, err := idx.VecCandidates(ctx, "", "")
	if err != nil {
		return nil, err
	}
	var hits []DupHit
	for id, v := range cands {
		s := cosine(vec, v)
		if s >= threshold {
			hits = append(hits, DupHit{ID: id, Score: s})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	for k := range hits {
		if row, err := idx.GetRow(ctx, hits[k].ID); err == nil && row != nil {
			hits[k].Title = row.Title
		}
	}
	return hits, nil
}
