package extract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cshbaoo/pensieve/internal/llm"
)

// newTestServer 起一个假的 OpenAI 兼容端点
func newTestServer(t *testing.T, reply string) (*llm.Client, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("错误路径: %s", r.URL.Path)
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": reply}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	c := llm.New(srv.URL+"/v1", "test-key", "5s")
	return c, srv.Close
}

func TestExtractOK(t *testing.T) {
	reply := `{"title":"缓存列表性能优化","type":"bugfix","tags":["性能"],"entities":["logcache"],"anchors":["impl.go:301"],"body":"**根因** N+1"}`
	c, closer := newTestServer(t, reply)
	defer closer()

	d, skip, err := Extract(context.Background(), c, "m", "排了很久发现 refreshCacheStats 每组都调 RPC，改成循环外一次拉取。")
	if err != nil || skip {
		t.Fatalf("extract: skip=%v err=%v", skip, err)
	}
	if d.Title != "缓存列表性能优化" || d.Type != "bugfix" || d.Tags[0] != "性能" {
		t.Fatalf("draft: %+v", d)
	}
	if d.Anchors[0] != "impl.go:301" {
		t.Fatalf("anchors: %v", d.Anchors)
	}
}

func TestExtractSkip(t *testing.T) {
	c, closer := newTestServer(t, `{"skip": true}`)
	defer closer()
	_, skip, err := Extract(context.Background(), c, "m", "今天中午吃了面")
	if err != nil || !skip {
		t.Fatalf("skip: %v %v", skip, err)
	}
}

func TestExtractMarkdownFence(t *testing.T) {
	c, closer := newTestServer(t, "好的，结果如下：\n```json\n{\"title\":\"围栏测试\",\"type\":\"pattern\",\"body\":\"b\"}\n```\n以上。")
	defer closer()
	d, skip, err := Extract(context.Background(), c, "m", "内容")
	if err != nil || skip || d.Title != "围栏测试" {
		t.Fatalf("围栏容错: %+v skip=%v err=%v", d, skip, err)
	}
}

func TestExtractGarbage(t *testing.T) {
	c, closer := newTestServer(t, "这不是JSON,完全不可用")
	defer closer()
	_, _, err := Extract(context.Background(), c, "m", "内容")
	if err == nil {
		t.Fatal("垃圾响应应报错")
	}
}

func TestExtractDefaultType(t *testing.T) {
	c, closer := newTestServer(t, `{"title":"x","body":"b"}`)
	defer closer()
	d, _, err := Extract(context.Background(), c, "m", "内容")
	if err != nil || d.Type != "pattern" {
		t.Fatalf("默认 type: %+v err=%v", d, err)
	}
}
