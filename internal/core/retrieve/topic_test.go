package retrieve

import (
	"context"
	"testing"
	"time"

	"github.com/cshbaoo/pensieve/internal/core/memory"
)

// topic 命中时应自动带子上记忆清单
func TestTopicChildrenExpansion(t *testing.T) {
	idx := openTempIndex(t)
	ctx := context.Background()

	child := &memory.Memory{ID: "mem_child", Title: "子记忆A", Body: "内容", Project: "p",
		Type: "bugfix", Status: "active", Confidence: "human", Source: "t", Sensitivity: "normal",
		Created: time.Now().UTC(), Path: "x/mem_child.md", Entities: []string{"子主题X"}}
	topicM := &memory.Memory{ID: "mem_topic", Title: "主题卷宗 子主题X", Body: "汇总索引",
		Project: "p", Type: "topic", Status: "active", Confidence: "human", Source: "t",
		Sensitivity: "normal", Created: time.Now().UTC(), Path: "x/mem_topic.md",
		Links: []memory.Link{{ID: "mem_child", Rel: "contains"}}}

	if err := idx.Upsert(ctx, child, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := idx.Upsert(ctx, topicM, nil, ""); err != nil {
		t.Fatal(err)
	}

	results, err := Search(ctx, idx, Request{Query: "子主题X", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	var topicRes *Result
	for i := range results {
		if results[i].ID == "mem_topic" {
			topicRes = &results[i]
		}
	}
	if topicRes == nil {
		t.Fatalf("topic 未被召回: %+v", results)
	}
	if len(topicRes.Children) != 1 || topicRes.Children[0] != "子记忆A (mem_child)" {
		t.Fatalf("children 扩展失败: %#v", topicRes.Children)
	}

	// 反向链接也建上了
	parents, err := idx.Parents(ctx, "mem_child")
	if err != nil || len(parents) != 1 || parents[0].ID != "mem_topic" {
		t.Fatalf("backlink: %+v err=%v", parents, err)
	}
}
