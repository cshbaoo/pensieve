package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
)

func toolSearch() mcp.Tool {
	return mcp.NewTool("memory_search",
		mcp.WithDescription("混合检索历史记忆(全文+语义+实体)。开始任务/排查问题前先查,避免重复踩坑。"),
		mcp.WithString("query", mcp.Required(), mcp.Description("检索问题,如 'ListJobs 接口慢'")),
		mcp.WithString("project", mcp.Description("限定项目,如 myteam/my-api 或 myteam/my-service")),
		mcp.WithString("type", mcp.Description("限定类型: bugfix|gotcha|pattern|api|decision|snippet")),
		mcp.WithNumber("limit", mcp.Description("返回条数,默认 5")),
		mcp.WithBoolean("include_superseded", mcp.Description("包含已被取代的记忆(默认不召回;仅在需要溯源历史结论时传 true)")),
	)
}

func toolGet() mcp.Tool {
	return mcp.NewTool("memory_get",
		mcp.WithDescription("按 id 读取一条记忆的完整内容(含元数据与正文)。"),
		mcp.WithString("id", mcp.Required(), mcp.Description("记忆 id,如 mem_20260813_a3f2")),
	)
}

func toolSave() mcp.Tool {
	return mcp.NewTool("memory_save",
		mcp.WithDescription("保存一条新记忆。两种模式: ①传 raw 原文让 LLM 自动提炼出结构化草稿; ②显式传 title/content/tags 等。两种模式都先返回草稿(confirmed 缺省 false),用户确认后再 confirmed=true 落盘。写入前自动做密钥扫描+相似度查重+冲突检测;若草稿提示与旧记忆结论冲突,确认时请附带 supersedes=[\"旧id\"],落盘时会同时把旧记忆标记为 superseded(取代),默认不再被召回。"),
		mcp.WithString("raw", mcp.Description("原始文本(会话记录/排查过程),由 LLM 提炼为结构化记忆")),
		mcp.WithString("title", mcp.Description("一句话标题(显式模式必填)")),
		mcp.WithString("content", mcp.Description("正文 Markdown(显式模式必填)")),
		mcp.WithString("type", mcp.Description("bugfix|gotcha|pattern|api|decision|snippet|requirement|topic,默认 pattern")),
		mcp.WithString("project", mcp.Description("如 myteam/my-service;默认 global")),
		mcp.WithArray("tags", mcp.Description("标签数组"), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithArray("entities", mcp.Description("涉及的接口/函数/文件名等实体"), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithArray("anchors", mcp.Description("关联代码位置,如 internal/foo/bar.go:123"), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithArray("links", mcp.Description("关联其他记忆:[\"mem_xx\"] 或 [{\"id\":\"mem_xx\",\"rel\":\"contains|related|superseded-by|caused-by|fixes\"}]"), mcp.Items(map[string]any{"type": []string{"string", "object"}})),
		mcp.WithBoolean("confirmed", mcp.Description("true=确认落盘;false/缺省=仅返回草稿")),
		mcp.WithBoolean("force", mcp.Description("查重命中后仍强制并存(默认拦截)")),
		mcp.WithArray("supersedes", mcp.Description("本条记忆取代的旧记忆 id 列表;落盘时自动将旧记忆标记为 superseded(confirmed=true 时生效)"), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithString("sensitivity", mcp.Description("normal|local-only(敏感记忆:仅本机不推远程)")),
		mcp.WithString("review_at", mcp.Description("复核期 RFC3339(可选);decision 类型缺省自动设为创建后 60 天")),
	)
}

func toolUpdate() mcp.Tool {
	return mcp.NewTool("memory_update",
		mcp.WithDescription("更新已有记忆:改状态、标记被取代、点赞、顺延复核期。"),
		mcp.WithString("id", mcp.Required(), mcp.Description("记忆 id")),
		mcp.WithString("status", mcp.Description("active|stale|superseded|archived")),
		mcp.WithString("supersede_by", mcp.Description("取代它的新记忆 id(同时置状态 superseded)")),
		mcp.WithBoolean("vote", mcp.Description("true=给这条记忆 +1 票")),
		mcp.WithString("review_at", mcp.Description("重设复核期(RFC3339);复核确认无误后顺延")),
	)
}

func toolContext() mcp.Tool {
	return mcp.NewTool("memory_context",
		mcp.WithDescription("进入某个项目时拉取记忆简报:活跃记忆数、最近条目、高票条目。project 可省略——会自动使用当前工作区所在项目。"),
		mcp.WithString("project", mcp.Description("如 myteam/my-service;缺省自动用当前工作区项目")),
	)
}
