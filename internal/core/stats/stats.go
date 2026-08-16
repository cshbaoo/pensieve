// Package stats 使用行为埋点：本地 JSONL 日志，不上传任何地方
package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event 一次使用行为
type Event struct {
	Ts      int64          `json:"ts"`
	Event   string         `json:"event"`   // search/get/save/update/context/undo
	Source  string         `json:"source"`  // cli | mcp
	Project string         `json:"project,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
}

var mu sync.Mutex

// Track 追加一条事件日志（失败静默，绝不阻塞主流程）
func Track(indexDir, event, source, project string, detail map[string]any) {
	if indexDir == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	path := filepath.Join(indexDir, "events.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	e := Event{Ts: time.Now().Unix(), Event: event, Source: source, Project: project, Detail: detail}
	data, _ := json.Marshal(e)
	f.Write(append(data, '\n'))
}

// LoadAll 读取全部事件
func LoadAll(indexDir string) ([]Event, error) {
	data, err := os.ReadFile(filepath.Join(indexDir, "events.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, line := range splitLines(string(data)) {
		var e Event
		if json.Unmarshal([]byte(line), &e) == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			if line := s[start:i]; line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// Summary 聚合统计
type Summary struct {
	TotalEvents   int
	ByEvent       map[string]int
	BySource      map[string]int
	SearchTotal   int
	SearchWithHit int // 召回率分母分子
	GetTopIDs     []IDCount
	SaveTopTypes  map[string]int
	ActiveDays    int
	FirstTs       int64
	LastTs        int64
}

type IDCount struct {
	ID    string
	Count int
}

// Summarize 聚合最近 days 天（≤0 = 全部）
func Summarize(events []Event, days int) Summary {
	var cutoff int64
	if days > 0 {
		cutoff = time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	}
	s := Summary{ByEvent: map[string]int{}, BySource: map[string]int{}, SaveTopTypes: map[string]int{}}
	getCount := map[string]int{}
	daysSeen := map[string]bool{}

	for _, e := range events {
		if e.Ts < cutoff {
			continue
		}
		s.TotalEvents++
		s.ByEvent[e.Event]++
		s.BySource[e.Source]++
		daysSeen[time.Unix(e.Ts, 0).Format("2006-01-02")] = true
		if s.FirstTs == 0 || e.Ts < s.FirstTs {
			s.FirstTs = e.Ts
		}
		if e.Ts > s.LastTs {
			s.LastTs = e.Ts
		}

		switch e.Event {
		case "search":
			s.SearchTotal++
			if n, ok := e.Detail["hits"].(float64); ok && n > 0 {
				s.SearchWithHit++
			}
		case "get":
			if id, ok := e.Detail["id"].(string); ok {
				getCount[id]++
			}
		case "save":
			if t, ok := e.Detail["type"].(string); ok {
				s.SaveTopTypes[t]++
			}
		}
	}
	s.ActiveDays = len(daysSeen)
	for id, c := range getCount {
		s.GetTopIDs = append(s.GetTopIDs, IDCount{id, c})
	}
	for i := 0; i < len(s.GetTopIDs); i++ {
		for j := i + 1; j < len(s.GetTopIDs); j++ {
			if s.GetTopIDs[j].Count > s.GetTopIDs[i].Count {
				s.GetTopIDs[i], s.GetTopIDs[j] = s.GetTopIDs[j], s.GetTopIDs[i]
			}
		}
	}
	return s
}
