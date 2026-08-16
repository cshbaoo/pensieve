package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	home, _ := HomeDir()
	if cfg.Core.RepoDir != filepath.Join(home, "repo") {
		t.Fatalf("默认 repoDir: %v", cfg.Core.RepoDir)
	}
	if cfg.Dedup.Threshold != 0.85 {
		t.Fatalf("默认阈值: %v", cfg.Dedup.Threshold)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	content := `
[llm]
api_key = "${PENSIEVE_TEST_KEY}"
chat_model = "my-model"
[sync]
mode = "manual"
`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PENSIEVE_TEST_KEY", "gitin-test-secret")

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.LLM.APIKey != "gitin-test-secret" {
		t.Fatalf("环境变量展开失败: %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.ChatModel != "my-model" || cfg.Sync.Mode != "manual" {
		t.Fatalf("字段: %+v", cfg)
	}
}

func TestExpandEnvPassthrough(t *testing.T) {
	if got := expandEnv("plain-value"); got != "plain-value" {
		t.Fatalf("非环境变量应原样通过: %q", got)
	}
	t.Setenv("EXP_X", "expanded")
	if got := expandEnv("${EXP_X}"); got != "expanded" {
		t.Fatalf("展开失败: %q", got)
	}
}
