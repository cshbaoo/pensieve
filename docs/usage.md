# Pensieve 使用教程

> 面向一线用户的完整教程：从安装到日常闭环。预计阅读 10 分钟，动手一遍 30 分钟。

Pensieve 是**编程记忆系统**：把你和 AI 每次踩坑、决策、发现沉淀为 Markdown 记忆，让任何 AI 工具下次自动想起来。

核心理念三句话：
1. **不用主动记**——AI 会问你要不要存
2. **不用主动查**——干活前先搜,相关记忆自动出现
3. **就在 `~/.pensieve/`**——就是一个 git 仓库,大不了手翻

---

## 目录

- [1. 安装](#1-安装)
- [2. 初始化与配置](#2-初始化与配置)
- [3. 接入 AI 工具（MCP）](#3-接入-ai-工具mcp)
- [4. 日常使用：三种姿势](#4-日常使用三种姿势)
- [5. 新接手一个项目怎么做](#5-新接手一个项目怎么做)
- [6. 多设备与团队共享](#6-多设备与团队共享)
- [7. 维护场景手册](#7-维护场景手册)
- [8. 常见问题 FAQ](#8-常见问题-faq)
- [9. 命令速查表](#9-命令速查表)

---

## 1. 安装

**前置要求**：本机装有 Git（记忆仓库基于它）。

### 方式 A：go install

```bash
go install github.com/cshbaoo/pensieve@latest
```

### 方式 B：源码编译

```bash
git clone https://github.com/cshbaoo/pensieve.git
cd pensieve
go build -o pensieve .    # Windows: go build -o pensieve.exe .
```

### 方式 C：别人给你的编译好的二进制

直接放进 PATH 即可（单文件,无依赖）。

验证：

```bash
pensieve --help
```

---

## 2. 初始化与配置

### 2.1 一键初始化

```bash
pensieve init
```

会创建出：

```
~/.pensieve/
├── config.toml        # 全局配置
├── repo/              # 记忆仓库 = git 仓库（唯一事实源）
│   └── global/
└── index/
    └── index.db       # SQLite 检索索引(可随时删除重建)
```

### 2.2 配置 LLM（语义能力的开关）

编辑 `~/.pensieve/config.toml`,把下面这段填上**任意 OpenAI 兼容端点**（可为 OpenAI 官方、自建 vLLM/One-API 网关、Ollama 等)：

```toml
[llm]
base_url    = "https://你的网关/v1"
api_key     = "${你的环境变量名}"   # 或直接写字符串(此文件不进 git,可接受)
chat_model  = "你的对话模型"        # 提炼记忆用,如 kimi-k3 / gpt-4o-mini
embed_model = "你的向量模型"        # 语义检索用,如 qwen3-embedding / text-embedding-3-small
```

> 不配 LLM 也能用:纯全文检索 + 手动 `add`。配上就解锁语义检索/自动提炼/查重。

### 2.3 自检

```bash
pensieve doctor
```

六绿即健康:

```
✔ git 可用
✔ 仓库完整
✔ 索引健康 (N 条记忆)
✔ LLM chat 端点
✔ embedding 端点
✔ 远程仓库
```

---

## 3. 接入 AI 工具（MCP）

一劳永逸,每个工具只配一次:

### OpenCode

`~/.config/opencode/opencode.json` 顶层加:

```json
{
  "mcp": {
    "pensieve": { "type": "local", "command": ["pensieve", "serve"], "enabled": true }
  }
}
```

> Windows 上若 `pensieve` 不在 PATH,写绝对路径如 `C:\\\\path\\\\to\\\\pensieve.exe`。

重启后输入 `/mcp` 应看到 `pensieve ✓ connected — 5 tools`。

### pi

```bash
pi mcp add pensieve -- pensieve serve
```

### Cursor / Claude Code / VSCode

各自 MCP 配置文件中:

```json
{ "mcpServers": { "pensieve": { "command": "pensieve", "args": ["serve"] } } }
```

**多个工具可同时接同一个 pensieve,记忆完全一致共享**——因为它们读写的是同一份 `~/.pensieve`。

---

## 4. 日常使用：三种姿势

### 姿势 1：AI 对话里说人话（频率最高,零记忆负担）

| 你想干嘛 | 直接说 |
|---------|--------|
| 查 | "查一下 pensieve 里 cpu 单位的问题" |
| 存 | "把刚才的结论存成 pensieve 记忆" |
| 看上下文 | "进这个项目前,先让 pensieve 给我个简报" |
| 改 | "给 pensieve 里那条 sqlite 记忆点个赞" |
| 作废 | "把 mem_xxx 标记为过时" |

**存的过程长这样**（草稿确认机制,防乱记）:

```
你: 把这个排查结论存一下
AI: (调 memory_save,confirmed 缺省 false)
Pensieve: 返回草稿 → AI 展示给你
你: "存" / "改一下标乡" / "不存"
AI: confirmed=true 再调一次 → 落盘
```

### 姿势 2：终端命令

```bash
pensieve add --auto "$(cat 排查记录.md)"    # LLM 提炼,最快
pensieve add -t gotcha -p my/项目 "标题" "正文"
pensieve search "缓存列表慢" -p myteam/my-service -n 5
pensieve list --project myteam/my-service
pensieve get mem_20260813_a3f2
pensieve update mem_xxx --status stale
pensieve sync                # 手动同步远程(通常不需要,自动会做)
pensieve reindex             # 索引感觉不对时重建
pensieve doctor
```

### 姿势 3：手写文件（兜底终极形态）

`~/.pensieve/repo/<project>/2026/08/` 下随便写个 `mem_20260813_xxxx.md`:

```markdown
---
id: mem_20260813_xxxx
type: gotcha
title: xxx
project: global
status: active
confidence: human
source: manual
votes: 0
sensitivity: normal
created: 2026-08-13T10:00:00+08:00
---

正文 markdown。
```

然后 `pensieve init`（会重新建索引）。**这就是永不锁定的含义**。

---

## 5. 新接手一个项目怎么做

记住一点:**项目 ≠ 新仓库**。一个机器只有一个 `~/.pensieve`,"项目"只是里面的一个分区目录。

### 新项目上手流程

```bash
# ① 看 remote 定项目名(约定)
cd C:\project\myteam\new-service
git remote get-url origin        # https://xxx/myteam/new-service.git → 项目名 "myteam/new-service"

# ② 起 AI 会话,让它种苗
#    "通读这个项目,把值得记的东西存成 pensieve 记忆,project 用 myteam/new-service"
#    第一批建议: 项目构建方式 / 核心约定 / 已知坑 → 2-5 条
```

### 之后再进这个项目

> "Pensieve 给我这个项目的简报" → AI 调 `memory_context`
> → 返回活跃记忆数、最近条目、高票条目——**10 秒恢复上下文**。

---

## 6. 多设备与团队共享

### 6.1 多设备（你自己换电脑）

第一台机器:

```bash
# 到自有 GitLab 建一个空仓库,如 team/pensieve-memories
git -C ~/.pensieve/repo remote add origin https://git.example.com/team/pensieve-memories.git
pensieve sync        # 推上去
```

新机器:

```bash
pensieve init --from https://git.example.com/team/pensieve-memories.git
# ✔ 克隆 + 自动建索引 → 直接用,记忆无差别
```

### 6.2 团队成员

同上,每人 `init --from 同一个远程地址`。**自动同步已默认开启**:
- 写入即推（后台异步）
- 启动即拉（每次 `serve`/`init` 时）
- 定时拉（每 30 分钟,可在 config.toml 改）

冲突几乎不会发生：记忆是单文件追加式,人A和人B几乎不会写到同一文件。万一冲突,`pensieve doctor` 会提示,手动 `git pull --rebase` 一下就行。

### 6.3 不想同步的敏感记忆

frontmatter 里写 `sensitivity: local-only`,这种记忆**永远不会被 push 到远程**（只在本地）。

---

## 7. 维护场景手册

### 我感觉搜不准了?

```bash
pensieve reindex     # 全量重建索引(10秒级,零数据损失)
```

### 我写的不小心写错了?

直接编辑那个 .md 文件（在 `~/.pensieve/repo/` 下),改完 `pensieve reindex`。或在 AI 里说"改 mem_xxx"。

### 我想删一条?

```bash
pensieve update mem_xxx --status archived   # 软删除,检索屏蔽但历史保留
# 或真删文件: rm ~/.pensieve/repo/.../mem_xxx.md && pensieve reindex
```

⚠️ 如果内容含密钥且已经 commit——`pensieve update` 删不掉 git 历史,需要重写历史或认为已泄露并轮换密钥。这就是为什么写入前的密钥扫描这么重要。

### 换 AI 工具（比如从 Cursor 到 OpenCode）?

什么都不用动。关掉旧工具的 MCP 配置行,在新工具加一行。`~/.pensieve` 里的记忆原封不动。

### 彻底卸载?

删掉 `~/.pensieve` 目录 + 各 AI 工具里的 MCP 配置。无残留。

---

## 8. 常见问题 FAQ

**Q: 我一定要配 LLM 吗？**
不配也能用（全文检索 + 手写/显式 add），配了才解锁语义搜索/自动提炼/查重/实体。**强烈推荐配。**

**Q: 我的记忆会被发到网上吗？**
三个地方数据会出去：① LLM API（你配的网关,提炼/向量化时文本发给它）② 你配置的远程 git 仓库 ③ 没了。没有任何遥测。

**Q: AI 会不会乱存东西？**
默认有双重闸门：LLM 提炼后必须经你确认（confirmed 流程）才落盘,且入库前还会过密钥扫描和查重。

**Q: 记忆很多以后会不会变慢？**
10 万条以内检索仍在 200ms 内（设计目标的量）。真到百万级可再做量化/ANN 升级,普通团队永远到不了。

**Q: 跟 Notion/飞书文档有什么区别？**
Notion 是人去查文档；Pensieve 是 AI 主动在对的时刻把记忆端到你面前。一个被动,一个主动。

**Q: 和 MemoryCP 什么关系?**
Pensieve 是记忆系统；MemoryCP / AGENTS.md 是 AI 的"用户手册",两边可并用。

---

## 9. 命令速查表

```
pensieve init [--from URL]    初始化(或克隆远程)
pensieve add <标题> [正文]     手动写入(--auto 让 LLM 提炼)
pensieve search <查询>         混合检索(-p 项目 -t 类型 -n 条数)
pensieve list                  最近记忆(-p -n)
pensieve get <id>              读全文
pensieve update <id>           状态/取代/投票(--status/--by/--vote)
pensieve sync                  立即 pull+push 远程
pensieve reindex               完全重建索引
pensieve doctor                六维自检
pensieve serve                 启动 MCP server(给 AI 工具用)
pensieve import <dir>          批量导入(v2 待做)
```

**在 AI 对话框里可用的工具:** `memory_search` `memory_get` `memory_save` `memory_update` `memory_context`

---

祝沉淀愉快 🪄 —— 用一个月后你会惊讶自己记得多少东西。
