package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/cshbaoo/pensieve/internal/config"
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

// Server 是 core 的薄协议壳：只做参数解析 → 调 core → 序列化结果
type Server struct {
	cfg            *config.Config
	defaultProject string // 从 MCP 工作目录自动检测的当前项目（owner/repo）
}

func New(cfg *config.Config) *Server { return &Server{cfg: cfg} }

// Serve 启动 stdio MCP server
func (s *Server) Serve() error {
	s.defaultProject = detectProject()

	instructions := `Pensieve 是编程记忆系统:沉淀项目踩坑(bugfix/gotcha)、约定(pattern)、接口细节(api)、决策(decision)、代码片段(snippet)。
纪律:
1. 每次会话开始,先调 memory_context 拉取项目记忆简报(project 可省,自动用当前工作区)。
2. 开始任何排查/开发任务前,先用 memory_search 检索相关历史经验,避免重复踩坑。
3. 解决了问题、做了决策、发现 API 坑后,用 memory_save 沉淀(先 confirmed=false 出草稿给用户确认,用户同意后再 confirmed=true 落盘)。`
	if s.defaultProject != "" {
		instructions += fmt.Sprintf("\n当前检测到工作区项目: %s (memory_search/memory_context 不传 project 时默认用它)。", s.defaultProject)
	}

	sv := server.NewMCPServer("pensieve", "0.2.0",
		server.WithToolCapabilities(false),
		server.WithInstructions(instructions),
	)

	sv.AddTool(toolSearch(), s.handleSearch)
	sv.AddTool(toolGet(), s.handleGet)
	sv.AddTool(toolSave(), s.handleSave)
	sv.AddTool(toolUpdate(), s.handleUpdate)
	sv.AddTool(toolContext(), s.handleContext)

	return server.ServeStdio(sv)
}

func (s *Server) openIndex() (*index.Index, error) { return index.Open(s.cfg.Core.IndexDir) }

func (s *Server) llmClient() *llm.Client {
	return llm.New(s.cfg.LLM.BaseURL, s.cfg.LLM.APIKey, s.cfg.LLM.Timeout)
}

func (s *Server) handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := req.GetString("query", "")
	if query == "" {
		return mcp.NewToolResultError("query 不能为空"), nil
	}
	idx, err := s.openIndex()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	defer idx.Close()

	var qvec []float32
	if c := s.llmClient(); c.Enabled() {
		if v, err := c.Embed(ctx, s.cfg.LLM.EmbedModel, query); err == nil {
			qvec = v
		}
	}
	searchReq := retrieve.Request{
		Query:    query,
		Project:  s.resolveProject(req.GetString("project", "")),
		Type:     req.GetString("type", ""),
		Limit:    req.GetInt("limit", 5),
		QueryVec: qvec,
		// superseded 默认不召回,仅显式溯源时放开
		IncludeSuperseded: req.GetBool("include_superseded", false),
	}
	if s.cfg.LLM.RerankEnabled && s.cfg.LLM.RerankModel != "" {
		searchReq.Reranker = s.llmClient()
		searchReq.RerankModel = s.cfg.LLM.RerankModel
	}
	results, err := retrieve.Search(ctx, idx, searchReq)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	stats.Track(s.cfg.Core.IndexDir, "search", "mcp", s.resolveProject(req.GetString("project", "")), map[string]any{"hits": len(results)})
	if len(results) == 0 {
		return mcp.NewToolResultText("无相关记忆。如确认是新问题,解决后请用 memory_save 沉淀。"), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "找到 %d 条相关记忆:\n\n", len(results))
	for _, r := range results {
		marker := ""
		if r.Status != "active" {
			marker = " [" + r.Status + "]"
		}
		fmt.Fprintf(&sb, "- [%s] %s (score=%.2f, project=%s, id=%s)%s\n", r.Type, r.Title, r.Score, r.Project, r.ID, marker)
		for _, c := range r.Children {
			fmt.Fprintf(&sb, "    ↳ %s\n", c)
		}
	}
	sb.WriteString("\n用 memory_get(id) 获取某条全文。")
	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := req.GetString("id", "")
	if id == "" {
		return mcp.NewToolResultError("id 不能为空"), nil
	}
	store := memory.NewStore(s.cfg.Core.RepoDir)
	m, err := store.GetByID(id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	stats.Track(s.cfg.Core.IndexDir, "get", "mcp", m.Project, map[string]any{"id": id})
	data, _ := memory.Marshal(m)
	// 读取兜底:过时记忆先出横幅(含继任者指路),防漂移结论被采信
	out := store.FreshnessNotice(m) + string(data)

	// topic 卷宗:附子记忆目录
	if m.Type == "topic" && len(m.Links) > 0 {
		idx, err := s.openIndex()
		if err == nil {
			defer idx.Close()
			children, _ := idx.Children(ctx, m.ID)
			if len(children) > 0 {
				var sb strings.Builder
				sb.WriteString("\n---\n\n📑 本卷宗目录（%d 条子记忆）:\n\n")
				for _, c := range children {
					fmt.Fprintf(&sb, "- [%s] %s (%s)\n", c.Type, c.Title, c.ID)
				}
				out = out + fmt.Sprintf(sb.String(), len(children))
			}
		}
	}
	return mcp.NewToolResultText(out), nil
}

func (s *Server) handleSave(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 模式1: raw 文本 → LLM 自动提炼草稿
	if raw := req.GetString("raw", ""); raw != "" {
		return s.handleExtractDraft(ctx, req, raw)
	}

	title := req.GetString("title", "")
	content := req.GetString("content", "")
	if title == "" || content == "" {
		return mcp.NewToolResultError("title 和 content 不能为空(或改用 raw 让 LLM 提炼)"), nil
	}

	// 密钥扫描:任何模式写入前必过,覆盖全部用户提供字段(密钥可能藏在 tags/锚点/链接任何角落)
	if hits := secrets.ScanParts(title, content,
		strings.Join(strSliceArg(req, "tags"), " "),
		strings.Join(strSliceArg(req, "entities"), " "),
		strings.Join(strSliceArg(req, "anchors"), " "),
		strings.Join(memory.Links(linkArgs(req, "links")).IDs(), " ")); len(hits) > 0 {
		return mcp.NewToolResultError(secrets.Err(hits).Error()), nil
	}

	m := &memory.Memory{
		Title:       title,
		Body:        content,
		Type:        req.GetString("type", "pattern"),
		Project:     req.GetString("project", "global"),
		Tags:        strSliceArg(req, "tags"),
		Entities:    strSliceArg(req, "entities"),
		Status:      "active",
		Confidence:  "ai-inferred",
		Source:      harnessSource(ctx),
		Sensitivity: req.GetString("sensitivity", "normal"),
		Created:     time.Now(),
	}
	for _, a := range strSliceArg(req, "anchors") {
		m.Anchors = append(m.Anchors, memory.Anchor{Kind: "code", Target: a})
	}
	for _, lk := range linkArgs(req, "links") {
		m.Links = append(m.Links, lk)
	}
	// 复核期决策:显式 review_at 优先;decision 缺省自动补 60 天
	if ra := req.GetString("review_at", ""); ra != "" {
		t, err := time.Parse(time.RFC3339, ra)
		if err != nil {
			return mcp.NewToolResultError("review_at 需 RFC3339 格式: " + err.Error()), nil
		}
		m.ReviewAt = t
	}
	memory.ApplyReviewPolicy(m)
	m.ID = memory.NewID(title, m.Created)

	if !req.GetBool("confirmed", false) {
		// 草稿阶段做冲突带检测,把"取代旧记忆"的决策前置到确认时
		conflictNote := s.conflictNote(ctx, m)
		draft, _ := memory.Marshal(m)
		return mcp.NewToolResultText(fmt.Sprintf(
			"【草稿,尚未写入】请把以下草稿展示给用户确认。用户同意后,以同样参数并 confirmed=true 再次调用 memory_save 完成落盘。%s\n\n%s", conflictNote, draft)), nil
	}

	// 查重
	dup, err := s.dedup(ctx, m)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if dup != nil && !req.GetBool("force", false) {
		return mcp.NewToolResultText(fmt.Sprintf(
			"⚠ 检测到相似记忆(余弦 %.2f):\n- %s: %s\n\n如确认是重复,请放弃本次保存;如是修正/更新,请用 memory_update(supersede_by) 关联;确需并存请加 force=true 重试。",
			dup.Score, dup.ID, dup.Title)), nil
	}

	store := memory.NewStore(s.cfg.Core.RepoDir)
	if m.Sensitivity == "local-only" {
		if created, err := store.EnsureLocalGitignore(); err == nil && created {
			commitGit(s.cfg.Core.RepoDir, "chore: 建立 local-only 守卫(.gitignore /local/)")
		}
	}
	if err := store.Write(m); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.indexMemory(ctx, m); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// 取代语义:统一动作——确认时附带 supersedes 则同批把旧记忆标记为 superseded
	supersededNote := ""
	for _, oldID := range strSliceArg(req, "supersedes") {
		if err := s.markSuperseded(ctx, oldID, m.ID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		supersededNote += fmt.Sprintf("\n✔ 已标记取代: %s → %s(旧记忆默认不再被召回)", oldID, m.ID)
	}

	commitGit(s.cfg.Core.RepoDir, "memory: "+m.Title)
	pushAsync(s.cfg)
	s.refreshManagedAgentsMD()
	stats.Track(s.cfg.Core.IndexDir, "save", "mcp", m.Project, map[string]any{"id": m.ID, "type": m.Type})
	return mcp.NewToolResultText(fmt.Sprintf("✔ 记忆已保存: %s (%s)%s", m.ID, m.Title, supersededNote)), nil
}

// handleExtractDraft raw 文本 → LLM 提炼 → 草稿(不写盘)
func (s *Server) handleExtractDraft(ctx context.Context, req mcp.CallToolRequest, raw string) (*mcp.CallToolResult, error) {
	c := s.llmClient()
	if !c.Enabled() {
		return mcp.NewToolResultError("未配置 llm.api_key,无法使用 raw 自动提炼;请改用 title/content 显式传入"), nil
	}
	if hits := secrets.Scan(raw); len(hits) > 0 {
		return mcp.NewToolResultError(secrets.Err(hits).Error()), nil
	}
	draft, skipped, err := extract.Extract(ctx, c, s.cfg.LLM.ChatModel, raw)
	if err != nil {
		return mcp.NewToolResultError("提炼失败: " + err.Error()), nil
	}
	if skipped {
		return mcp.NewToolResultText("LLM 判断该内容无可沉淀的长期价值(纯寒暄/一次性操作),未生成草稿。"), nil
	}

	m := &memory.Memory{
		Title:       draft.Title,
		Body:        draft.Body,
		Type:        draft.Type,
		Project:     req.GetString("project", "global"),
		Tags:        draft.Tags,
		Entities:    draft.Entities,
		Status:      "active",
		Confidence:  "ai-inferred",
		Source:      harnessSource(ctx),
		Sensitivity: req.GetString("sensitivity", "normal"),
		Created:     time.Now(),
	}
	for _, a := range draft.Anchors {
		m.Anchors = append(m.Anchors, memory.Anchor{Kind: "code", Target: a})
	}
	for _, lk := range draft.Links {
		if lk.ID != "" {
			m.Links = append(m.Links, memory.Link{ID: lk.ID, Rel: lk.Rel})
		}
	}
	memory.ApplyReviewPolicy(m)
	m.ID = memory.NewID(m.Title, m.Created)

	// 提炼产出的结构化字段(tags/entities/anchors 由 LLM 重组,raw 扫描覆盖不到新词)再扫一遍
	{
		parts := []string{m.Title, m.Body, strings.Join(m.Tags, " "), strings.Join(m.Entities, " ")}
		for _, a := range m.Anchors {
			parts = append(parts, a.Target)
		}
		for _, lk := range m.Links {
			parts = append(parts, lk.ID)
		}
		if hits := secrets.ScanParts(parts...); len(hits) > 0 {
			return mcp.NewToolResultError(secrets.Err(hits).Error()), nil
		}
	}

	// 草稿阶段即查重,提前告诉用户可能重复
	dupNote := ""
	if dup, err := s.dedup(ctx, m); err == nil && dup != nil {
		dupNote = fmt.Sprintf("\n\n⚠ 查重提示:已存在相似记忆 %s(%s,相似度 %.2f),建议确认后再决定新增还是 update。", dup.ID, dup.Title, dup.Score)
	}
	dupNote += s.conflictNote(ctx, m)
	out, _ := memory.Marshal(m)
	return mcp.NewToolResultText(fmt.Sprintf(
		"【LLM 提炼草稿,尚未写入】请展示给用户确认;同意后把这些字段以 title/content/type/tags/entities/anchors + confirmed=true 调 memory_save 落盘。%s\n\n%s", dupNote, out)), nil
}

// dedup 计算向量并查重
func (s *Server) dedup(ctx context.Context, m *memory.Memory) (*retrieve.DupHit, error) {
	c := s.llmClient()
	if !c.Enabled() {
		return nil, nil
	}
	vec, err := c.Embed(ctx, s.cfg.LLM.EmbedModel, retrieve.MakeEmbedText(m.Title, m.Body, m.Tags, m.Entities))
	if err != nil {
		return nil, nil // embedding 失败不阻塞
	}
	idx, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer idx.Close()
	return retrieve.DedupCheck(ctx, idx, vec, s.cfg.Dedup.Threshold)
}

func (s *Server) handleUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := req.GetString("id", "")
	if id == "" {
		return mcp.NewToolResultError("id 不能为空"), nil
	}
	store := memory.NewStore(s.cfg.Core.RepoDir)
	m, err := store.GetByID(id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if st := req.GetString("status", ""); st != "" {
		m.Status = st
	}
	if by := req.GetString("supersede_by", ""); by != "" {
		m.Status = "superseded"
		m.Links = append(m.Links, memory.Link{ID: by, Rel: "superseded-by"})
	}
	if req.GetBool("vote", false) {
		m.Votes++
	}
	if ra := req.GetString("review_at", ""); ra != "" {
		t, err := time.Parse(time.RFC3339, ra)
		if err != nil {
			return mcp.NewToolResultError("review_at 需 RFC3339 格式: " + err.Error()), nil
		}
		m.ReviewAt = t
	}
	if err := store.Write(m); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.indexMemory(ctx, m); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	commitGit(s.cfg.Core.RepoDir, "memory-update: "+m.Title)
	pushAsync(s.cfg)
	s.refreshManagedAgentsMD()
	stats.Track(s.cfg.Core.IndexDir, "update", "mcp", m.Project, map[string]any{"id": m.ID})
	return mcp.NewToolResultText(fmt.Sprintf("✔ 已更新: %s (status=%s, votes=%d)", m.ID, m.Status, m.Votes)), nil
}

type memBrief struct {
	created int64
	id      string
	title   string
	typ     string
	votes   int
}

func (s *Server) handleContext(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project := s.resolveProject(req.GetString("project", ""))
	if project == "" {
		return mcp.NewToolResultError("无法确定 project(也无法从当前目录检测),请显式传 project 参数"), nil
	}
	store := memory.NewStore(s.cfg.Core.RepoDir)
	var all []memBrief
	_ = store.Walk(func(m *memory.Memory) error {
		if m.Project != project || m.Status != "active" {
			return nil
		}
		all = append(all, memBrief{m.Created.Unix(), m.ID, m.Title, m.Type, m.Votes})
		return nil
	})
	if len(all) == 0 {
		stats.Track(s.cfg.Core.IndexDir, "context", "mcp", project, nil)
		return mcp.NewToolResultText(fmt.Sprintf("项目 %s 暂无记忆。", project)), nil
	}

	byRecent := append([]memBrief(nil), all...)
	sort.Slice(byRecent, func(i, j int) bool { return byRecent[i].created > byRecent[j].created })
	byVotes := append([]memBrief(nil), all...)
	sort.Slice(byVotes, func(i, j int) bool { return byVotes[i].votes > byVotes[j].votes })

	var sb strings.Builder
	fmt.Fprintf(&sb, "📎 项目 %s 共有 %d 条活跃记忆\n", project, len(all))

	// 主题卷宗置顶（有的话最先看）
	if idx, err := s.openIndex(); err == nil {
		defer idx.Close()
		if topics, err := idx.Topics(ctx, project); err == nil && len(topics) > 0 {
			sb.WriteString("\n📑 主题卷宗:\n")
			for _, t := range topics {
				children, _ := idx.Children(ctx, t.ID)
				fmt.Fprintf(&sb, "- %s (%s) —— %d 条子记忆\n", t.Title, t.ID, len(children))
			}
		}
	}
	fmt.Fprintf(&sb, "\n最近 %d 条:\n", minInt(5, len(all)))
	for i := 0; i < 5 && i < len(byRecent); i++ {
		fmt.Fprintf(&sb, "- [%s] %s (%s)\n", byRecent[i].typ, byRecent[i].title, byRecent[i].id)
	}
	sb.WriteString("\n高票 top3:\n")
	for i := 0; i < 3 && i < len(byVotes); i++ {
		fmt.Fprintf(&sb, "- %s (%s, votes=%d)\n", byVotes[i].title, byVotes[i].id, byVotes[i].votes)
	}
	sb.WriteString("\n开始工作前建议优先用 memory_get 阅读 gotcha/bugfix 类记忆;具体问题用 memory_search 检索。")

	// 巡检捕获:锚点失活的静默提示行(只提示不动状态,确认由人 pensieve stale)
	if wd, err := os.Getwd(); err == nil {
		if suspects, err := ops.StaleSuspectsFromCwd(ctx, store, wd); err == nil && len(suspects) > 0 {
			fmt.Fprintf(&sb, "\n\n⏳ 待复核:%d 条记忆的代码锚点疑似失活(锚点文件被删或在其后有改动)。请运行 pensieve stale 查看详情并确认是否标记 stale。", len(suspects))
		}
	}
	// 决策复核超期提示:decision 是唯一自带保鲜期的类型
	if overdue := ops.DecisionReviewDue(store, time.Now()); len(overdue) > 0 {
		fmt.Fprintf(&sb, "\n⏳ 待复核:%d 条 decision 已过复核期(review_at 到期),用 update --review-at 顺延或 --status stale 标记。", len(overdue))
	}
	stats.Track(s.cfg.Core.IndexDir, "context", "mcp", project, map[string]any{"memories": len(all)})
	return mcp.NewToolResultText(sb.String()), nil
}

// ---------- helpers ----------

// resolveProject 缺省时回落到从工作目录检测到的项目
func (s *Server) resolveProject(v string) string {
	if v != "" {
		return v
	}
	return s.defaultProject
}

// detectProject 从 MCP 进程工作目录的 git remote 推导项目名 owner/repo
func detectProject() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	out, err := exec.Command("git", "-C", wd, "remote", "get-url", "origin").CombinedOutput()
	if err != nil {
		return ""
	}
	u := strings.TrimSpace(string(out))
	u = strings.TrimSuffix(strings.TrimSuffix(u, ".git"), "/")
	if i := strings.LastIndex(u, "/"); i >= 0 {
		repo := u[i+1:]
		u2 := u[:i]
		sep := "/"
		if strings.Contains(u2, ":") && !strings.Contains(u2, "//") {
			sep = ":"
		}
		if j := strings.LastIndex(u2, sep); j >= 0 {
			return u2[j+1:] + "/" + repo
		}
		return repo
	}
	return ""
}

func (s *Server) indexMemory(ctx context.Context, m *memory.Memory) error {
	var vec []float32
	if c := s.llmClient(); c.Enabled() {
		if v, err := c.Embed(ctx, s.cfg.LLM.EmbedModel, retrieve.MakeEmbedText(m.Title, m.Body, m.Tags, m.Entities)); err == nil {
			vec = v
		}
	}
	idx, err := s.openIndex()
	if err != nil {
		return err
	}
	defer idx.Close()
	return idx.Upsert(ctx, m, vec, s.cfg.LLM.EmbedModel)
}

// conflictNote 写入侧冲突捕获:对草稿做双阈值扫描,命中时给一段话提示 agent 在确认时携带 supersedes。
// 失败静默降级(不可阻塞写入)。
func (s *Server) conflictNote(ctx context.Context, m *memory.Memory) string {
	idx, err := s.openIndex()
	if err != nil {
		return ""
	}
	defer idx.Close()
	vs := ops.ConflictScan(ctx, idx, s.llmClient(), s.cfg.LLM.ChatModel, s.cfg.LLM.EmbedModel,
		s.cfg.Dedup.ConflictThreshold, s.cfg.Dedup.Threshold, m)
	if len(vs) == 0 {
		return ""
	}
	ids := make([]string, 0, len(vs))
	for _, v := range vs {
		ids = append(ids, fmt.Sprintf("%q", v.OldID))
	}
	hint := fmt.Sprintf("若本条是对旧记忆的取代性更新,确认落盘(confirmed=true)时请附带 supersedes=[%s]——将自动把旧记忆标记为 superseded,默认不再被检索召回。\n", strings.Join(ids, ","))
	return "\n\n" + ops.ConflictNote(vs, hint)
}

// markSuperseded 把旧记忆原子标记为「被 newID 取代」:改事实源 + 同步索引。
func (s *Server) markSuperseded(ctx context.Context, oldID, newID string) error {
	idx, err := s.openIndex()
	if err != nil {
		return err
	}
	defer idx.Close()
	return ops.MarkSuperseded(ctx, memory.NewStore(s.cfg.Core.RepoDir), idx, s.llmClient(), s.cfg.LLM.EmbedModel, oldID, newID)
}

func strSliceArg(req mcp.CallToolRequest, key string) []string {
	v, ok := req.GetArguments()[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// linkArgs 解析 links 参数: 数组元素是字符串(id) 或 {id,rel} 对象
func linkArgs(req mcp.CallToolRequest, key string) []memory.Link {
	v, ok := req.GetArguments()[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []memory.Link{}
	for _, item := range arr {
		switch t := item.(type) {
		case string:
			out = append(out, memory.Link{ID: t})
		case map[string]any:
			lk := memory.Link{}
			if id, ok := t["id"].(string); ok {
				lk.ID = id
			}
			if rel, ok := t["rel"].(string); ok {
				lk.Rel = rel
			}
			if lk.ID != "" {
				out = append(out, lk)
			}
		}
	}
	return out
}

// harnessSource 从 MCP 会话 clientInfo 推导调用方 agent 身份。
// 服务端注入(参考 ECC 硬化清单):Source 写在 Memory 上,llm 工具参数不可伪造。
func harnessSource(ctx context.Context) string {
	if sess := server.ClientSessionFromContext(ctx); sess != nil {
		if sci, ok := sess.(server.SessionWithClientInfo); ok {
			if info := sci.GetClientInfo(); info.Name != "" {
				return "mcp:" + info.Name
			}
		}
	}
	return "mcp-agent"
}

func commitGit(repoDir, msg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "git", "-C", repoDir, "status", "--porcelain").CombinedOutput(); err != nil || len(out) == 0 {
		return // 无变更不制造空 commit
	}
	_ = exec.CommandContext(ctx, "git", "-C", repoDir, "add", "-A").Run()
	_ = exec.CommandContext(ctx, "git", "-C", repoDir, "-c", "user.name=pensieve", "-c", "user.email=pensieve@local",
		"commit", "-q", "-m", msg).Run()
}

// refreshManagedAgentsMD 写操作后自动刷新当前工作区仓库的 AGENTS.md 托管区。
// 尽力而为:任何失败都只打 stderr(stdout 是 JSON-RPC 通道,不能打印)。
func (s *Server) refreshManagedAgentsMD() {
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	changed, err := export.RefreshManaged(wd, memory.NewStore(s.cfg.Core.RepoDir), export.DefaultOptions(), time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ AGENTS.md 托管区自动刷新失败: %v\n", err)
		return
	}
	if changed {
		fmt.Fprintln(os.Stderr, "✔ 已自动同步 AGENTS.md 托管区")
	}
}

// pushAsync 写入后异步推送远程（sync.mode=auto 时）
func pushAsync(cfg *config.Config) {
	if cfg.Sync.Mode != "auto" {
		return
	}
	pensievesync.New(cfg.Core.RepoDir).PushAsync()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
