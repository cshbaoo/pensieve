package sync

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Syncer 记忆仓库的 Git 同步器
type Syncer struct {
	RepoDir string
}

func New(repoDir string) *Syncer { return &Syncer{RepoDir: repoDir} }

func (s *Syncer) git(ctx context.Context, args ...string) (string, error) {
	// 进程级互斥：防 MCP 常驻进程与 CLI 并发操作撞 index.lock
	lock := filepath.Join(filepath.Dir(s.RepoDir), "sync.lock")
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		defer f.Close()
	}
	args = append([]string{"-C", s.RepoDir, "-c", "user.name=pensieve", "-c", "user.email=pensieve@local"}, args...)
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	return string(out), err
}

// HasRemote 是否配置了 origin
func (s *Syncer) HasRemote(ctx context.Context) bool {
	out, err := s.git(ctx, "remote", "get-url", "origin")
	return err == nil && strings.TrimSpace(out) != ""
}

// Push 推送（无 remote 时静默跳过）
func (s *Syncer) Push(ctx context.Context) error {
	if !s.HasRemote(ctx) {
		return nil
	}
	out, err := s.git(ctx, "push")
	if err != nil {
		return fmt.Errorf("git push: %w\n%s", err, out)
	}
	return nil
}

// Pull 拉取（rebase + autostash；冲突时回退到 merge）
func (s *Syncer) Pull(ctx context.Context) error {
	if !s.HasRemote(ctx) {
		return nil
	}
	out, err := s.git(ctx, "pull", "--rebase", "--autostash")
	if err != nil {
		return fmt.Errorf("git pull: %w\n%s", err, out)
	}
	return nil
}

// PushAsync 写入后异步推送（不阻塞主流程）
func (s *Syncer) PushAsync() {
	if s == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.Push(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ 后台 push 失败(已存本地,稍后重试): %v\n", err)
		}
	}()
}

// AutoLoop 定时拉取（供常驻进程使用；sync.mode=auto 时启用）
func (s *Syncer) AutoLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c2, cancel := context.WithTimeout(ctx, 60*time.Second)
			_ = s.Pull(c2)
			cancel()
		}
	}
}
