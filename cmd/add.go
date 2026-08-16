package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/core/export"
	"github.com/cshbaoo/pensieve/internal/core/extract"
	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/core/ops"
	"github.com/cshbaoo/pensieve/internal/core/retrieve"
	"github.com/cshbaoo/pensieve/internal/core/secrets"
	"github.com/cshbaoo/pensieve/internal/core/stats"
	pensievesync "github.com/cshbaoo/pensieve/internal/core/sync"
	"github.com/cshbaoo/pensieve/internal/llm"
)

var (
	addType       string
	addProject    string
	addTags       []string
	addEntities   []string
	addYes        bool
	addAuto       bool
	addForce      bool
	addBodyFile   string
	addLocalOnly  bool
	addSupersedes []string
	addReviewAt   string
)

var addCmd = &cobra.Command{
	Use:   `add <title> [body...]`,
	Short: "写入一条记忆（--auto 时用 LLM 从原文自动提炼；正文也支持 --body-file 或 stdin 管道,避免暴露在进程列表）",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		input := strings.Join(args, " ")
		client := llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Timeout)

		// 正文来源优先级:命令行参数 > --body-file > stdin 管道
		piped := ""
		if addBodyFile != "" {
			data, err := os.ReadFile(addBodyFile)
			if err != nil {
				return fmt.Errorf("读取 --body-file 失败: %w", err)
			}
			piped = string(data)
		} else if st, err := os.Stdin.Stat(); err == nil && (st.Mode()&os.ModeCharDevice) == 0 {
			data, _ := io.ReadAll(os.Stdin)
			piped = strings.TrimSpace(string(data))
		}

		// 密钥扫描(写入前必过,覆盖参数/管道/文件/title/tags/entities 全部提供字段,
		// 密钥可能藏在 tags/锚点等任何角落)
		if hits := secrets.ScanParts(input, piped, strings.Join(addTags, " "), strings.Join(addEntities, " ")); len(hits) > 0 {
			return secrets.Err(hits)
		}

		m := &memory.Memory{
			Type:        addType,
			Project:     addProject,
			Tags:        addTags,
			Entities:    addEntities,
			Status:      "active",
			Confidence:  "human",
			Source:      "manual",
			Sensitivity: "normal",
			Created:     time.Now(),
		}
		if addLocalOnly {
			m.Sensitivity = "local-only"
		}

		if addAuto {
			// LLM 提炼模式:参数或管道输入都当作原文
			if piped != "" {
				input = piped
			}
			if input == "" {
				return fmt.Errorf("--auto 需要原文(参数、--body-file 或 stdin 管道)")
			}
			if !client.Enabled() {
				return fmt.Errorf("--auto 需要配置 llm.api_key")
			}
			draft, skipped, err := extract.Extract(ctx, client, cfg.LLM.ChatModel, input)
			if err != nil {
				return fmt.Errorf("提炼失败: %w", err)
			}
			if skipped {
				fmt.Println("LLM 判断该内容无可沉淀的长期价值,未生成记忆。")
				return nil
			}
			m.Title = draft.Title
			m.Type = draft.Type
			m.Body = draft.Body
			if len(m.Tags) == 0 {
				m.Tags = draft.Tags
			}
			if len(m.Entities) == 0 {
				m.Entities = draft.Entities
			}
			for _, a := range draft.Anchors {
				m.Anchors = append(m.Anchors, memory.Anchor{Kind: "code", Target: a})
			}
			for _, lk := range draft.Links {
				if lk.ID != "" {
					m.Links = append(m.Links, memory.Link{ID: lk.ID, Rel: lk.Rel})
				}
			}
			m.Confidence = "ai-inferred"
			m.Source = "cli-auto"
		} else {
			if len(args) == 0 {
				return fmt.Errorf("缺少标题(或用 --body-file/stdin 提供正文时仍需标题)")
			}
			m.Title = args[0]
			if len(args) > 1 {
				m.Body = strings.Join(args[1:], " ")
			} else if piped != "" {
				m.Body = piped
			}
		}
		if m.Title == "" {
			return fmt.Errorf("标题不能为空")
		}
		// 决策类型:补默认复核期(--review-at 可覆盖),GET 巡检据此浮出超期决策
		if addReviewAt != "" {
			t, err := time.Parse(time.RFC3339, addReviewAt)
			if err != nil {
				return fmt.Errorf("--review-at 需 RFC3339(如 2026-10-15T00:00:00+08:00): %w", err)
			}
			m.ReviewAt = t
		}
		memory.ApplyReviewPolicy(m)
		m.ID = memory.NewID(m.Title, m.Created)

		// 查重
		var embedErr error
		var vec []float32
		if client.Enabled() {
			text := retrieve.MakeEmbedText(m.Title, m.Body, m.Tags, m.Entities)
			vec, embedErr = client.Embed(ctx, cfg.LLM.EmbedModel, text)
			if embedErr == nil && !addForce {
				if idx, err := index.Open(cfg.Core.IndexDir); err == nil {
					if dup, derr := retrieve.DedupCheck(ctx, idx, vec, cfg.Dedup.Threshold); derr == nil && dup != nil {
						idx.Close()
						fmt.Printf("⚠ 检测到相似记忆(余弦 %.2f):\n  %s: %s\n", dup.Score, dup.ID, dup.Title)
						fmt.Println("如是修正请用 update --by 关联;确认并存请加 --force 重试。")
						return nil
					}
					idx.Close()
				}
			}
		}

		// 冲突带检测(增强,失败静默):草稿预览里提示可能取代的旧记忆
		if vec != nil && embedErr == nil {
			if idx, err := index.Open(cfg.Core.IndexDir); err == nil {
				vs := ops.ConflictScan(ctx, idx, client, cfg.LLM.ChatModel, cfg.LLM.EmbedModel,
					cfg.Dedup.ConflictThreshold, cfg.Dedup.Threshold, m)
				idx.Close()
				if len(vs) > 0 {
					ids := make([]string, 0, len(vs))
					for _, v := range vs {
						ids = append(ids, v.OldID)
					}
					fmt.Println(ops.ConflictNote(vs,
						fmt.Sprintf("若本条是取代性更新,请加 --supersedes %s 重跑,旧记忆将被标记为 superseded。\n", strings.Join(ids, ","))))
				}
			}
		}

		if !addYes {
			fmt.Println("── 草稿预览 ──")
			fmt.Printf("id: %s\ntype: %s | project: %s | tags: %v\n", m.ID, m.Type, m.Project, m.Tags)
			fmt.Println("\n" + m.Title)
			if m.Body != "" {
				fmt.Println(m.Body)
			}
			fmt.Print("\n确认写入? [y/N] ")
			var answer string
			fmt.Scanln(&answer)
			if !strings.EqualFold(answer, "y") {
				fmt.Println("已取消")
				return nil
			}
		}

		store := memory.NewStore(cfg.Core.RepoDir)
		if addLocalOnly {
			ensureLocalGuardCommitted(cfg.Core.RepoDir)
		}
		if err := store.Write(m); err != nil {
			return err
		}
		fmt.Println("✔ 已写入事实源:", m.Path)

		if embedErr != nil {
			fmt.Println("⚠ embedding 失败，仅建文本索引:", embedErr)
		}
		idx, err := index.Open(cfg.Core.IndexDir)
		if err != nil {
			return err
		}
		defer idx.Close()
		if err := idx.Upsert(ctx, m, vec, cfg.LLM.EmbedModel); err != nil {
			return err
		}
		fmt.Println("✔ 索引已更新")

		// 取代语义:与新记忆同一 commit,完成「新记忆 + 旧记忆 superseded」统一动作
		for _, oldID := range addSupersedes {
			if err := ops.MarkSuperseded(ctx, store, idx, client, cfg.LLM.EmbedModel, oldID, m.ID); err != nil {
				return err
			}
			fmt.Printf("✔ 已标记取代: %s → %s(旧记忆默认不再被召回)\n", oldID, m.ID)
		}

		gitCommit(cfg.Core.RepoDir, "memory: "+m.Title)
		if addLocalOnly {
			fmt.Println("✔ local-only:仅本机,不进 git 历史也不推送")
		} else {
			fmt.Println("✔ 已 git commit")
		}
		if cfg.Sync.Mode == "auto" {
			pensievesync.New(cfg.Core.RepoDir).PushAsync()
		}
		stats.Track(cfg.Core.IndexDir, "save", "cli", m.Project, map[string]any{"id": m.ID, "type": m.Type})
		// 托管区自动刷新(与 MCP serve 行为对齐; 尽力而为,失败仅告警)
		if wd, err := os.Getwd(); err == nil {
			if changed, err := export.RefreshManaged(wd, memory.NewStore(cfg.Core.RepoDir), export.DefaultOptions(), time.Now()); err != nil {
				fmt.Fprintln(os.Stderr, "⚠ AGENTS.md 托管区自动刷新失败:", err)
			} else if changed {
				fmt.Println("✔ 已同步 AGENTS.md 托管区")
			}
		}
		fmt.Printf("\n记忆已保存: %s\n", m.ID)
		return nil
	},
}

func init() {
	addCmd.Flags().StringVarP(&addType, "type", "t", "pattern", "记忆类型: bugfix|gotcha|pattern|api|decision|snippet|requirement|topic")
	addCmd.Flags().StringVarP(&addProject, "project", "p", "global", "项目（如 myteam/my-api）或 global")
	addCmd.Flags().StringSliceVar(&addTags, "tags", nil, "标签（逗号分隔）")
	addCmd.Flags().StringSliceVar(&addEntities, "entities", nil, "实体（接口名/文件名等，逗号分隔）")
	addCmd.Flags().BoolVarP(&addYes, "yes", "y", false, "跳过确认直接写入")
	addCmd.Flags().BoolVar(&addAuto, "auto", false, "用 LLM 从原文自动提炼结构化记忆")
	addCmd.Flags().BoolVar(&addForce, "force", false, "查重命中后仍强制并存")
	addCmd.Flags().StringVar(&addBodyFile, "body-file", "", "从文件读取正文(避免长文本暴露在进程列表)")
	addCmd.Flags().BoolVar(&addLocalOnly, "local-only", false, "敏感记忆:仅本机,永不推送远程")
	addCmd.Flags().StringSliceVar(&addSupersedes, "supersedes", nil, "本条记忆取代的旧记忆 id(逗号分隔);写入后自动将其标记为 superseded")
	addCmd.Flags().StringVar(&addReviewAt, "review-at", "", "复核期(RFC3339);decision 类型缺省自动设为 60 天后")
}
