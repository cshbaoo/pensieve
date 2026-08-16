package retrieve

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/llm"
)

// 向量召回的"相关度地板"：低于阈值的纯向量结果视为噪音。
// ⚠️ 与 embedding 模型强相关：qwen3-embedding 实测 相关≈0.53 / 无关≤0.36，
// 默认 0.45 卡在中间。换 embedding 模型后应重新校准（见 retrieve/diag_test.go）。
const MinCosineRelevant = 0.45

type Request struct {
	Query    string
	Project  string
	Type     string
	Limit    int
	QueryVec []float32

	// 可选 rerank（v1）
	Reranker    *llm.Client
	RerankModel string

	// 默认不召回已被取代的记忆(防漂移:旧结论不再干扰新结论);
	// 需要溯源时调用方显式置 true(superseded 以 0.2 权重降权召回)
	IncludeSuperseded bool
}

type Result struct {
	ID       string
	Title    string
	Type     string
	Project  string
	Status   string
	Path     string
	Score    float64
	Children []string // topic 卷宗: 子记忆标题列表
}

var statusWeight = map[string]float64{
	"active": 1.0, "stale": 0.3, "superseded": 0.2,
}

// Search 混合检索：FTS(BM25) + 向量余弦 + 实体倒排，过滤噪音，可选 rerank 精排
func Search(ctx context.Context, idx *index.Index, req Request) ([]Result, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}

	// 三路并行召回
	var (
		ftsHits, likeHits map[string]float64
		entBoost          map[string]float64
		vecHits           map[string]float64
		ftsErr, entErr    error
		vecErr            error
		wg                sync.WaitGroup
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		ftsHits, ftsErr = idx.FTSSearch(ctx, req.Query, req.Project, req.Type, 20)
	}()
	go func() {
		defer wg.Done()
		entBoost, entErr = idx.EntityBoost(ctx, req.Query)
	}()
	go func() {
		defer wg.Done()
		if len(req.QueryVec) > 0 {
			vecHits, vecErr = idxVecSearch(ctx, idx, req)
		}
	}()
	wg.Wait()

	for _, err := range []error{ftsErr, entErr, vecErr} {
		if err != nil {
			return nil, err
		}
	}
	if len(ftsHits) == 0 && len(vecHits) == 0 {
		// trigram 全失配时 LIKE 兜底
		likeHits, _ = idx.LikeSearch(ctx, req.Query, req.Project, 20)
	}

	// 噪音过滤：纯向量命中且余弦低于地板 → 丢弃（避免幻觉式"总能搜出点东西"）
	if len(vecHits) > 0 && len(ftsHits) == 0 && len(likeHits) == 0 && len(entBoost) == 0 {
		for id, s := range vecHits {
			if s < MinCosineRelevant {
				delete(vecHits, id)
			}
		}
	}

	// 归一化并融合
	ftsN := normalize(ftsHits)
	vecN := normalize(vecHits)
	scores := map[string]float64{}
	collect := func(m map[string]float64, w float64) {
		for id, s := range m {
			scores[id] += w * s
		}
	}
	if len(vecHits) > 0 {
		collect(vecN, 0.45)
		collect(ftsN, 0.35)
	} else {
		collect(ftsN, 0.75)
	}
	collect(likeHits, 0.15)
	collect(entBoost, 0.15)

	out := []Result{}
	for id, s := range scores {
		row, err := idx.GetRow(ctx, id)
		if err != nil || row == nil {
			continue
		}
		// superseded 默认不召回(防止旧结论漂移污染);stale 保留但降权
		if row.Status == "superseded" && !req.IncludeSuperseded {
			continue
		}
		w := statusWeight[row.Status]
		if w == 0 {
			continue
		}
		out = append(out, Result{
			ID: row.ID, Title: row.Title, Type: row.Type,
			Project: row.Project, Status: row.Status, Path: row.Path,
			Score: s * w,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })

	// 候选进行 rerank 精排（启用时）。
	// ⚠️ 采用经验: qwen3-reranker 实测在该混合融合之上反而劣化排序(Recall@1 93%→46%),
	// 因为 reranker 对"短 query vs 长文档"的偏好与 BM25+向量的融合权重不同。
	// 保留开关供换 reranker 模型后重新评测;默认 rerank_enabled=false。
	if req.Reranker != nil && req.Reranker.Enabled() && req.RerankModel != "" && len(out) > 1 {
		candN := limit * 4
		if candN > len(out) {
			candN = len(out)
		}
		if reranked, err := rerankResults(ctx, idx, req, out[:candN]); err == nil && len(reranked) > 0 {
			out = reranked
		}
		// rerank 失败静默回退到融合排序,不阻塞
	}

	if len(out) > limit {
		out = out[:limit]
	}
	// topic 卷宗扩展:命中 topic 时自动带上子记忆清单(导航感)
	for i := range out {
		if out[i].Type != "topic" {
			continue
		}
		children, err := idx.Children(ctx, out[i].ID)
		if err != nil || len(children) == 0 {
			continue
		}
		for _, c := range children {
			out[i].Children = append(out[i].Children, fmt.Sprintf("%s (%s)", c.Title, c.ID))
		}
	}
	return out, nil
}

// rerankResults 调 rerank 模型重排候选
func rerankResults(ctx context.Context, idx *index.Index, req Request, cands []Result) ([]Result, error) {
	docs := make([]string, len(cands))
	for i, c := range cands {
		body, _ := idx.GetBody(ctx, c.ID)
		docs[i] = c.Title + "\n" + truncRunes(body, 400)
	}
	ranked, err := req.Reranker.Rerank(ctx, req.RerankModel, req.Query, docs, len(cands))
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(cands))
	for _, r := range ranked {
		if r.Index < 0 || r.Index >= len(cands) {
			continue
		}
		c := cands[r.Index]
		c.Score = r.Score
		out = append(out, c)
	}
	return out, nil
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// idxVecSearch 全量候选暴力余弦（goroutine 分片）。返回 id→原始余弦值。
func idxVecSearch(ctx context.Context, idx *index.Index, req Request) (map[string]float64, error) {
	cands, err := idx.VecCandidates(ctx, req.Project, req.Type)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(cands))
	for id := range cands {
		ids = append(ids, id)
	}
	nWorkers := runtime.NumCPU()
	if len(ids) < nWorkers*4 || len(ids) == 0 {
		nWorkers = 1
	}
	results := make([][]float64, nWorkers)
	chunk := (len(ids) + nWorkers - 1) / nWorkers
	var wg sync.WaitGroup
	for w := 0; w < nWorkers; w++ {
		lo, hi := w*chunk, (w+1)*chunk
		if lo >= len(ids) {
			break
		}
		if hi > len(ids) {
			hi = len(ids)
		}
		wg.Add(1)
		go func(w, lo, hi int) {
			defer wg.Done()
			var part []float64
			for k := lo; k < hi; k++ {
				part = append(part, float64(k), cosine(req.QueryVec, cands[ids[k]]))
			}
			results[w] = part
		}(w, lo, hi)
	}
	wg.Wait()
	out := map[string]float64{}
	for _, part := range results {
		for k := 0; k+1 < len(part); k += 2 {
			out[ids[int(part[k])]] = part[k+1]
		}
	}
	return out, nil
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		na += ai * ai
		nb += bi * bi
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func normalize(m map[string]float64) map[string]float64 {
	if len(m) == 0 {
		return m
	}
	var max float64
	for _, v := range m {
		if v > max {
			max = v
		}
	}
	if max == 0 {
		max = 1
	}
	out := map[string]float64{}
	for k, v := range m {
		out[k] = v / max
	}
	return out
}

// MakeEmbedText 索引/查询时的向量化文本
func MakeEmbedText(title, body string, tags, entities []string) string {
	return title + "\n" + strings.Join(tags, " ") + " " + strings.Join(entities, " ") + "\n" + body
}
