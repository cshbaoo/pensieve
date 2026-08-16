package topic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/llm"
)

func llmOf(t *testing.T, reply string) (*llm.Client, func()) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": reply}}},
		})
	}))
	return llm.New(srv.URL+"/v1", "k", "5s"), srv.Close
}

func cands() []index.Row {
	return []index.Row{
		{ID: "mem_1", Title: "多AZ 缓存设计", Type: "decision"},
		{ID: "mem_2", Title: "今天午饭吃什么", Type: "pattern"},
		{ID: "mem_3", Title: "多AZ 接口命名", Type: "api"},
	}
}

func TestGenerateOK(t *testing.T) {
	c, closer := llmOf(t, `{"title":"多AZ 方案","links":["mem_1","mem_3"],"summary":"多AZ 一期全套决策。","keywords":["多AZ"]}`)
	defer closer()
	d, skip, err := Generate(context.Background(), c, "m", "多AZ", cands())
	if err != nil || skip {
		t.Fatalf("gen: %v skip=%v", err, skip)
	}
	if d.Title != "多AZ 方案" || len(d.Links) != 2 {
		t.Fatalf("draft: %+v", d)
	}
}

func TestGenerateSkip(t *testing.T) {
	c, closer := llmOf(t, `{"skip":true}`)
	defer closer()
	_, skip, _ := Generate(context.Background(), c, "m", "不存在主题", cands())
	if !skip {
		t.Fatal("应 skip")
	}
}

func TestGenerateNoCands(t *testing.T) {
	c, closer := llmOf(t, `{"title":"x"}`)
	defer closer()
	_, skip, _ := Generate(context.Background(), c, "m", "x", nil)
	if !skip {
		t.Fatal("无候选应 skip(不浪费一次 LLM 调用)")
	}
}
