package extract

import (
	"context"
	"strings"
	"testing"

	"github.com/cshbaoo/pensieve/internal/core/memory"
)

func TestJudgeRelationConflict(t *testing.T) {
	c, closer := newTestServer(t, `{"relation":"conflict","reason":"旧断言功能不存在,新断言已实现,同一对象结论相反"}`)
	defer closer()

	newM := &memory.Memory{ID: "mem_new", Title: "export 已实现", Body: "pensieve export 支持 AGENTS.md"}
	old := memory.Memory{ID: "mem_old", Title: "战略 v2", Body: "export 功能不存在"}
	v, err := JudgeRelation(context.Background(), c, "m", newM, old)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if v.Relation != "conflict" || v.Reason == "" {
		t.Fatalf("verdict: %+v", v)
	}
	if v.OldID != "mem_old" || v.OldTitle != "战略 v2" {
		t.Fatalf("候选回填: %+v", v)
	}
}

func TestJudgeRelationComplementAndFences(t *testing.T) {
	c, closer := newTestServer(t, "```json\n{\"relation\":\"complement\",\"reason\":\"互补\"}\n```")
	defer closer()
	v, err := JudgeRelation(context.Background(), c, "m", &memory.Memory{Title: "t", Body: "b"}, memory.Memory{ID: "x", Title: "x"})
	if err != nil || v.Relation != "complement" {
		t.Fatalf("fence 容错: %v %+v", err, v)
	}
}

func TestJudgeRelationBadRelation(t *testing.T) {
	c, closer := newTestServer(t, `{"relation":"maybe","reason":"r"}`)
	defer closer()
	_, err := JudgeRelation(context.Background(), c, "m", &memory.Memory{Title: "t", Body: "b"}, memory.Memory{ID: "x"})
	if err == nil || !strings.Contains(err.Error(), "未知 relation") {
		t.Fatalf("非法 relation 应报错: %v", err)
	}
}

func TestJudgeRelationGarbage(t *testing.T) {
	c, closer := newTestServer(t, "这不是 JSON")
	defer closer()
	_, err := JudgeRelation(context.Background(), c, "m", &memory.Memory{Title: "t", Body: "b"}, memory.Memory{ID: "x"})
	if err == nil {
		t.Fatal("纯噪音应报错(调用方静默降级)")
	}
}
