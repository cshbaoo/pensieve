package llm

import "context"

type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

type rerankResponse struct {
	Results []struct {
		Index int     `json:"index"`
		Score float64 `json:"relevance_score"`
	} `json:"results"`
}

// RankedItem rerank 后的单项（Index 指向传入 documents 的下标）
type RankedItem struct {
	Index int
	Score float64
}

// Rerank 调用 /rerank（OpenAI/Volc/jina 兼容），失败由调用方静默回退
func (c *Client) Rerank(ctx context.Context, model, query string, documents []string, topN int) ([]RankedItem, error) {
	var resp rerankResponse
	err := c.post(ctx, "/rerank", rerankRequest{Model: model, Query: query, Documents: documents, TopN: topN}, &resp)
	if err != nil {
		return nil, err
	}
	out := make([]RankedItem, 0, len(resp.Results))
	for _, r := range resp.Results {
		out = append(out, RankedItem{Index: r.Index, Score: r.Score})
	}
	return out, nil
}
