package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Core struct {
		RepoDir  string `toml:"repo_dir"`
		IndexDir string `toml:"index_dir"`
	} `toml:"core"`
	LLM struct {
		BaseURL       string `toml:"base_url"`
		APIKey        string `toml:"api_key"`
		ChatModel     string `toml:"chat_model"`
		EmbedModel    string `toml:"embed_model"`
		RerankModel   string `toml:"rerank_model"`
		RerankEnabled bool   `toml:"rerank_enabled"`
		Timeout       string `toml:"timeout"`
	} `toml:"llm"`
	Sync struct {
		Mode     string `toml:"mode"`
		Interval string `toml:"interval"`
		Remote   string `toml:"remote"`
	} `toml:"sync"`
	Dedup struct {
		Threshold         float64 `toml:"threshold"`
		ConflictThreshold float64 `toml:"conflict_threshold"`
	} `toml:"dedup"`
}

// HomeDir 返回 pensieve 的家目录（~/.pensieve）
func HomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pensieve"), nil
}

func DefaultPath() (string, error) {
	home, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config.toml"), nil
}

func (c *Config) SetDefaults(home string) {
	c.Core.RepoDir = filepath.Join(home, "repo")
	c.Core.IndexDir = filepath.Join(home, "index")
	// LLM 为通用 OpenAI 兼容端点：默认留空，由用户在 config.toml 中配置
	c.LLM.Timeout = "30s"
	c.Sync.Mode = "auto"
	c.Sync.Interval = "30m"
	c.Dedup.Threshold = 0.85
	// 冲突带宽下限:相似度落在 [conflict_threshold, threshold) 的候选交给 LLM 判定关系
	c.Dedup.ConflictThreshold = 0.60
}

func Load(path string) (*Config, error) {
	home, _ := HomeDir()
	cfg := &Config{}
	cfg.SetDefaults(home)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil // 未初始化时用默认值
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("解析配置 %s 失败: %w", path, err)
	}
	cfg.LLM.APIKey = expandEnv(cfg.LLM.APIKey)
	return cfg, nil
}

func expandEnv(v string) string {
	if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
		return os.Getenv(strings.TrimSuffix(strings.TrimPrefix(v, "${"), "}"))
	}
	return v
}

// WriteDefault 写入默认配置
func WriteDefault(path, home string) error {
	cfg := &Config{}
	cfg.SetDefaults(home)
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
