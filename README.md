<div align="center">

# 🪄 Pensieve

**Long-term memory for AI coding agents and dev teams.**

Markdown as the single source of truth · Git-versioned · MCP-native · Agent-agnostic

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8.svg)](go.mod)
[![MCP](https://img.shields.io/badge/MCP-Server-8A2BE2.svg)](https://modelcontextprotocol.io)
[![Platforms](https://img.shields.io/badge/Platforms-macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-lightgrey.svg)](#install)

[Quick Start](#quick-start) · [Integrations](#use-it-from-your-ai-tool) · [中文教程](docs/usage.md) · [Issues](https://github.com/cshbaoo/pensieve/issues)

</div>

---

Your AI coding assistant forgets everything between sessions.
Your team's hard-won lessons live in chat logs nobody can find. Wikis rot.

**Pensieve turns every pitfall, decision and discovery into a reusable asset** —
searchable by any agent, reviewable like code, owned by you.
Named after the Pensieve in Harry Potter: pull memories out, store them, replay anytime.

（中文简介：把团队的每一次踩坑、决策、发现沉淀为可被任何 AI 编程助手直接复用的资产。）

## How it works

```
you / your agent                         Pensieve                        your repo

 "remember this"  ───── draft ────▶  ┌───────────────┐                  ┌──────────────┐
 "any pitfalls?"  ◀── ranked hit ──  │ hybrid index  │ ═══ git sync ══▶ │ plain .md    │
                                     │ FTS5+vectors  │ ◀══════════════  │ git-versioned│
                                     │ governance    │                  │ owned by you │
                                     └───────────────┘                  └──────────────┘
```

Every write is a **human-approved draft** — secret-scanned, deduplicated, conflict-checked
before it ever touches the record. Nothing sneaks in; nothing rots silently.

## Highlights

| | |
|---|---|
| 📝 **Markdown is the source of truth** | Plain `.md` files, Git-versioned, never locked in a vendor's black box |
| 🔍 **Hybrid retrieval** | FTS5 full-text + vector semantics + entity links, fused and ranked |
| 🔌 **Any agent, any LLM** | Speaks MCP: OpenCode · Cursor · Claude Code · Codex · Windsurf … |
| 🛡️ **Write-side governance** | Secret scanning & semantic dedup run *before* anything is persisted |
| 🧭 **Drift defense** | Catches stale memories on write, on patrol, on read — always as suggestions, never silent edits |
| 🔄 **Git sync** | Multi-device & team sharing through nothing but a git remote |

## Install

**Prebuilt binaries (recommended)** — every tag ships 6 platforms (macOS / Linux / Windows, amd64 + arm64) with SHA256 checksums:

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/cshbaoo/pensieve/main/install.sh | sh

# Windows (PowerShell)
irm https://raw.githubusercontent.com/cshbaoo/pensieve/main/install.ps1 | iex
```

From source — requires **Go 1.25+** and Git:

```bash
go install github.com/cshbaoo/pensieve@latest
# or
git clone https://github.com/cshbaoo/pensieve.git && cd pensieve && go build -o pensieve .
```

## Quick start

```bash
pensieve init          # create the memory repo & index (~/.pensieve)
pensieve doctor        # health check
```

Optional but recommended — point Pensieve at any OpenAI-compatible LLM gateway to enable
semantic search, LLM-assisted extraction and dedup. Edit `~/.pensieve/config.toml`:

```toml
[llm]
base_url    = "https://your-gateway/v1"
api_key     = "${YOUR_API_KEY_ENV}"
chat_model  = "<your-chat-model>"
embed_model = "<your-embedding-model>"
```

## Use it from your AI tool

**OpenCode** — `~/.config/opencode/opencode.json`:

```json
{ "mcp": { "pensieve": { "type": "local", "command": ["pensieve", "serve"], "enabled": true } } }
```

**Cursor / Claude Code**:

```json
{ "mcpServers": { "pensieve": { "command": "pensieve", "args": ["serve"] } } }
```

Once wired, just talk naturally:

> *"Check pensieve for anything about list pagination before I touch this API."*
> *"Save this debugging conclusion as a memory."*

Memories come back as **drafts for human confirmation** — nothing sneaks into the record.

## Command reference

| Command | Description |
|---|---|
| `pensieve add "..."` | Write a memory (LLM-assisted draft → confirm) |
| `pensieve search "cpu shows 0"` | Hybrid search |
| `pensieve get <id>` | Read a full memory |
| `pensieve list` | Browse recent memories |
| `pensieve update <id> --vote` | Upvote / mark stale or superseded |
| `pensieve topic ...` | Cluster memories into topic dossiers |
| `pensieve stats` | Local usage & reuse metrics |
| `pensieve stats --export r.json` | Export telemetry as hand-auditable JSON (counts only) |
| `pensieve stats -f a.json -f b.json` | View teammates' exports, auto-merged into a team view |
| `pensieve undo` | Undo the last write |
| `pensieve export [--check] [--out FILE]` | Generate / verify the managed section in AGENTS.md |
| `pensieve stale [--mark]` | Patrol drifted code anchors (file deleted / changed since) |
| `pensieve sync` | Pull + push the git remote |
| `pensieve reindex` | Rebuild the index |
| `pensieve doctor` | Health check (incl. drifted code anchors) |
| `pensieve serve` | Run the MCP server (stdio) |

## Export to AGENTS.md

Make every agent in the repo aware of the memory system — even ones without MCP config:

```bash
pensieve export                # generate / update a managed section in ./AGENTS.md
pensieve export --check        # CI: fail if AGENTS.md drifts from the memory store
pensieve export --out CLAUDE.md   # other harness filenames work too
```

The managed section contains only *discipline instructions and one-line gotcha pointers* —
details always stay in the memory store. Writes via MCP auto-refresh the managed section.

## Drift defense

Memories go stale as the world changes. Pensieve catches drift at three moments —
always as **suggestions**, never silent auto-edits:

- **On write** — drafts are scanned against the store: a *conflict band* (cosine 0.60–0.85)
  catches "similar but different" claims, and an LLM judges whether the new memory contradicts
  an old one. Confirming with `supersedes` marks the old memory **superseded** in the same commit.
- **On patrol** — `pensieve stale` (also surfaced in `memory_context` briefings and `pensieve doctor`)
  flags memories whose code anchors were deleted or changed after the memory was created.
- **On read** — superseded memories are excluded from search by default
  (`--include-superseded` to opt in), and `get` prepends a banner pointing to the successor.

## Multi-device & team sharing

Your memory repo is just git. To share or restore:

```bash
# new device
pensieve init --from <your-git-remote>

# existing local repo: attach a remote
git -C ~/.pensieve/repo remote add origin <your-git-remote>
pensieve sync
```

## Privacy & security

- 🔒 **Secret scanning on every write** — keys, tokens, JWTs, connection strings and webhook
  URLs are rejected *before* they can touch the record
- 🧹 **Semantic dedup** blocks duplicates before persistence
- 🏠 **Local-first** — the MCP server speaks stdio only; no network traffic except your
  LLM gateway and your git remote
- 🕳️ **`local-only` sensitivity** — marked memories never leave the machine
- 📤 **Telemetry stays yours** — usage stats are local by default; `pensieve stats --export`
  writes a hand-auditable JSON (counts only — never search queries or memory content)
  that *you* choose to share

## Documentation

| Document | Contents |
|---|---|
| [docs/usage.md](docs/usage.md) | 详细中文使用教程 |
| [docs/crd-work-handoff.md](docs/crd-work-handoff.md) | Concept requirements: cross-device work handoff (draft) |

## Development

```bash
go build ./...
go test ./...
```

Contributions welcome — issues and PRs.

## License

[Apache-2.0](LICENSE)
