package stats

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrackAndSummarize(t *testing.T) {
	dir := t.TempDir()
	Track(dir, "search", "cli", "myteam/my-api", map[string]any{"hits": 3.0})
	Track(dir, "search", "mcp", "myteam/my-api", map[string]any{"hits": 0.0})
	Track(dir, "save", "mcp", "myteam/my-api", map[string]any{"id": "mem_a", "type": "gotcha"})
	Track(dir, "get", "mcp", "", map[string]any{"id": "mem_a"})
	Track(dir, "get", "mcp", "", map[string]any{"id": "mem_a"})
	Track(dir, "context", "mcp", "myteam/my-service", nil)

	events, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(events) != 6 {
		t.Fatalf("事件数: %d", len(events))
	}

	s := Summarize(events, 0)
	if s.TotalEvents != 6 || s.SearchTotal != 2 || s.SearchWithHit != 1 {
		t.Fatalf("summary: %+v", s)
	}
	if s.BySource["cli"] != 1 || s.BySource["mcp"] != 5 {
		t.Fatalf("source分布: %v", s.BySource)
	}
	if s.SaveTopTypes["gotcha"] != 1 {
		t.Fatalf("类型分布: %v", s.SaveTopTypes)
	}
	if len(s.GetTopIDs) != 1 || s.GetTopIDs[0].Count != 2 || s.GetTopIDs[0].ID != "mem_a" {
		t.Fatalf("get排名: %+v", s.GetTopIDs)
	}
	if s.ActiveDays != 1 {
		t.Fatalf("活跃天数: %d", s.ActiveDays)
	}
}

func TestLoadEmpty(t *testing.T) {
	events, err := LoadAll(t.TempDir())
	if err != nil || len(events) != 0 {
		t.Fatalf("空目录应返回空: %v %d", err, len(events))
	}
}

func TestTrackNoIndexDir(t *testing.T) {
	Track("", "search", "cli", "", nil) // 不应 panic、不应写盘
	if _, err := os.Stat(filepath.Join("", "events.jsonl")); err == nil {
		t.Fatal("不应创建文件")
	}
}

func TestReportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	Track(dir, "search", "cli", "myteam/my-api", map[string]any{"hits": 3.0})
	Track(dir, "save", "mcp", "myteam/my-api", map[string]any{"id": "mem_a", "type": "gotcha"})

	r, err := ExportReport(dir)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if r.Version != 1 || len(r.Events) != 2 || r.ExportedAt == 0 {
		t.Fatalf("report 元信息: %+v", r)
	}

	path := filepath.Join(t.TempDir(), "report.json")
	if err := SaveReport(r, path); err != nil {
		t.Fatalf("save report: %v", err)
	}
	loaded, err := LoadReportFile(path)
	if err != nil {
		t.Fatalf("load report: %v", err)
	}
	if len(loaded.Events) != 2 || loaded.Exporter != r.Exporter {
		t.Fatalf("round trip 不一致: %+v", loaded)
	}

	if _, err := LoadReportFile(filepath.Join(t.TempDir(), "not-exist.json")); err == nil {
		t.Fatal("读取不存在文件应报错")
	}
}
