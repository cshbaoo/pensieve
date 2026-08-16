package retrieve

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/cshbaoo/pensieve/internal/config"
	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/llm"
)

// 临时诊断工具（跑法: go test -run TestDiagCosine -v ./internal/core/retrieve/）
// 看真实记忆库里指定查询的原始余弦分布，用于校准 MinCosineRelevant
func TestDiagCosine(t *testing.T) {
	t.Skip("余弦分布校准工具,日常跳过。跑法: 去掉本行后 go test -run TestDiagCosine -v ./internal/core/retrieve/")
	q := "cpu 显示为 0"
	cfgPath, _ := config.DefaultPath()
	cfg, _ := config.Load(cfgPath)
	ctx := context.Background()
	client := llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Timeout)
	v, err := client.Embed(ctx, cfg.LLM.EmbedModel, q)
	if err != nil {
		t.Fatal(err)
	}
	idx, _ := index.Open(cfg.Core.IndexDir)
	defer idx.Close()
	cands, _ := idx.VecCandidates(ctx, "", "")
	type pair struct {
		id string
		s  float64
	}
	var ps []pair
	for id, vec := range cands {
		ps = append(ps, pair{id, cosine(v, vec)})
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].s > ps[j].s })
	for _, p := range ps {
		fmt.Printf("%.4f  %s\n", p.s, p.id)
	}
}
