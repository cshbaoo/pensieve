package memory

import (
	"strings"
	"testing"
)

func TestLinksBackwardCompatible(t *testing.T) {
	// 旧格式: ["id1","id2"]
	oldYAML := `---
id: mem_t
type: pattern
title: t
project: p
status: active
confidence: human
source: t
sensitivity: normal
created: 2026-08-13T00:00:00Z
links:
  - mem_old_a
  - mem_old_b
---

body
`
	m, err := Parse([]byte(oldYAML))
	if err != nil {
		t.Fatalf("旧格式解析失败: %v", err)
	}
	if len(m.Links) != 2 || m.Links[0].ID != "mem_old_a" || m.Links[0].Rel != "" {
		t.Fatalf("旧格式 Links: %+v", m.Links)
	}

	// 新格式: [{id, rel}]
	newYAML := strings.Replace(oldYAML, `- mem_old_a`, `- id: mem_old_a
    rel: contains`, 1)
	m2, err := Parse([]byte(newYAML))
	if err != nil {
		t.Fatalf("新格式解析失败: %v", err)
	}
	if m2.Links[0].Rel != "contains" {
		t.Fatalf("rel 未解析: %+v", m2.Links[0])
	}

	// 混合往返:序列化再有 rel 的写新格式,无 rel 的写标量
	m3 := &Memory{ID: "mem_t", Links: Links{{ID: "a"}, {ID: "b", Rel: "fixes"}}}
	data, _ := Marshal(m3)
	s := string(data)
	if strings.Contains(s, "rel") && !strings.Contains(s, "fixes") {
		t.Fatalf("序列化 rel 丢失: %s", s)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Links[1].Rel != "fixes" || got.Links[0].Rel != "" {
		t.Fatalf("往返: %+v", got.Links)
	}
}
