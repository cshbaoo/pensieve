package memory

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Store 记忆事实源：基于 Markdown 文件的读写
type Store struct {
	RepoDir string
}

func NewStore(repoDir string) *Store {
	return &Store{RepoDir: repoDir}
}

// RelPath 计算记忆的仓库内相对路径:{project}/{yyyy}/{mm}/{id}.md
// local-only 记忆隔离到 local/ 子树,由记忆仓库根的 .gitignore fail-closed(永不推远程)
func RelPath(m *Memory) string {
	rel := filepath.Join(m.Project, m.Created.Format("2006"), m.Created.Format("01"), m.ID+".md")
	if m.Sensitivity == "local-only" {
		return filepath.Join("local", rel)
	}
	return rel
}

// Write 原子写入（tmp + rename + fsync）
// 注意:调用方在写 local-only 记忆前应先调 EnsureLocalGitignore 并单独提交守卫,
// 否则守卫会混进"memory:" 的 commit、undo 时会连带被删。
func (s *Store) Write(m *Memory) error {
	if m.Sensitivity == "local-only" {
		if _, err := s.EnsureLocalGitignore(); err != nil {
			return err
		}
	}
	rel := RelPath(m)
	abs := filepath.Join(s.RepoDir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	data, err := Marshal(m)
	if err != nil {
		return err
	}
	tmp := abs + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, abs); err != nil {
		return err
	}
	m.Path = rel
	return nil
}

// EnsureLocalGitignore 保证记忆仓库根的 .gitignore 存在且含 /local/(exist-ok 幂等)。
// 返回 created=true 表示本次新建了守卫,调用方应立刻单独 commit,
// 避免守卫混进下一次记忆写入的 commit(那样 undo 会误删守卫)。
func (s *Store) EnsureLocalGitignore() (bool, error) {
	p := filepath.Join(s.RepoDir, ".gitignore")
	data, err := os.ReadFile(p)
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == "/local/" {
				return false, nil
			}
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return true, os.WriteFile(p, append(data, []byte("/local/\n")...), 0o644)
}

// Marshal 序列化为 frontmatter + 正文
func Marshal(m *Memory) ([]byte, error) {
	fm, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(fm)
	buf.WriteString("---\n\n")
	buf.WriteString(strings.TrimLeft(m.Body, "\n"))
	if !strings.HasSuffix(m.Body, "\n") {
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}

// Parse 解析 Markdown 记忆文件
func Parse(data []byte) (*Memory, error) {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return nil, fmt.Errorf("缺少 frontmatter")
	}
	rest := s[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, fmt.Errorf("frontmatter 未闭合")
	}
	m := &Memory{}
	if err := yaml.Unmarshal([]byte(rest[:end]), m); err != nil {
		return nil, fmt.Errorf("frontmatter 解析失败: %w", err)
	}
	m.Body = strings.TrimPrefix(rest[end+4:], "\n")
	return m, nil
}

// WalkWithStat 遍历记忆文件,同时把文件的 size/mtime 传给回调(增量索引用)
func (s *Store) WalkWithStat(fn func(m *Memory, size, modUnix int64) error) error {
	return filepath.WalkDir(s.RepoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".md") || !strings.HasPrefix(name, "mem_") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		m, err := Parse(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		rel, _ := filepath.Rel(s.RepoDir, path)
		m.Path = rel
		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(m, info.Size(), info.ModTime().Unix())
	})
}

// Walk 遍历仓库内全部记忆文件（fn 返回 error 即中断）
func (s *Store) Walk(fn func(m *Memory) error) error {
	return s.WalkWithStat(func(m *Memory, _, _ int64) error { return fn(m) })
}

// GetByID 按 id 查找（MVP：直接遍历，量大了再建索引兜底）
func (s *Store) GetByID(id string) (*Memory, error) {
	var found *Memory
	err := s.Walk(func(m *Memory) error {
		if m.ID == id {
			found = m
			return fs.SkipAll
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.SkipAll) {
		return nil, err
	}
	if found == nil {
		return nil, fmt.Errorf("记忆不存在: %s", id)
	}
	return found, nil
}

// FreshnessNotice 读取侧兜底横幅:命中 superseded/stale/archived 记忆时先于正文呈现。
// superseded 时尽力加载继任者标题与日期(失败仅省略,不报错)。
func (s *Store) FreshnessNotice(m *Memory) string {
	var succTitle, succCreated string
	if id := SuccessorID(m); id != "" {
		if succ, err := s.GetByID(id); err == nil {
			succTitle = succ.Title + " [" + succ.ID + "]"
			succCreated = succ.Created.Format("2006-01-02")
		}
	}
	return FreshnessBanner(m, succTitle, succCreated)
}
