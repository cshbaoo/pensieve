# Pensieve 🪄

**Long-term memory for AI coding agents and dev teams.**
Markdown as the single source of truth · Git-versioned · MCP-native · agent-agnostic.

Named after the "Pensieve" in Harry Potter — pull memories out, store them, replay anytime.
（中文简介：把团队的每一次踩坑、决策、发现沉淀为可被任何 AI 编程助手直接复用的资产。)

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8.svg)](go.mod)
[![MCP](https://img.shields.io/badge/MCP-server-8A2BE2.svg)](https://modelcontextprotocol.io)

---

## Why

Your AI coding assistant forgets everything between sessions. Your team's hard-won lessons live
in chat logs nobody can find. Wikis rot. **Pensieve turns every pitfall, decision and discovery
into a reusable asset** — searchable by any agent, reviewable like code, owned by you.

- 📝 **Markdown is the source of truth** — memories are plain `.md` files, Git-versioned, never locked in a vendor's black box
- 🔍 **Hybrid retrieval** — FTS5 full-text + vector semantics + entity links, fused and ranked
- 🔌 **Any agent, any LLM** — speaks MCP: OpenCode / Cursor / Claude Code / Codex / Windsurf …
- 🛡️ **Write-side governance** — secret scanning and semantic dedup *before* anything is persisted; every save is a human-approved draft
- 🔄 **Git sync** — multi-device and team sharing through nothing but a git remote

## Install

**Prebuilt binaries (recommended)** — every tag ships 6 platforms + SHA256 checksums:

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/cshbaoo/pensieve/main/install.sh | sh

# Windows (PowerShell)
irm https://raw.githubusercontent.com/cshbaoo/pensieve/main/install.ps1 | iex
```

Or from source — requires **Go 1.25+** and Git:

```bash
go install github.com/cshbaoo/pensieve@latest
# or
git clone https://github.com/cshbaoo/pensieve.git
cd pensieve && go build -o pensieve .   # Windows: pensieve.exe
```

## Quick start

```bash
pensieve init          # create the memory repo & index (~/.pensieve)
pensieve doctor        # health check

# optional but recommended: point it at any OpenAI-compatible LLM gateway
# (enables semantic search, LLM-assisted extraction and dedup)
# edit ~/.pensieve/config.toml:
#   [llm]
#   base_url    = "https://your-gateway/v1"
#   api_key     = "${YOUR_API_KEY_ENV}"
#   chat_model  = "<your-chat-model>"
#   embed_model = "<your-embedding-model>"
```

## Use it from your AI tool (MCP)

**OpenCode** (`~/.config/opencode/opencode.json`):

```json
{ "mcp": { "pensieve": { "type": "local", "command": ["pensieve", "serve"], "enabled": true } } }
```

**Cursor / Claude Code**:

```json
{ "mcpServers": { "pensieve": { "command": "pensieve", "args": ["serve"] } } }
```

Once wired, just talk naturally:

- "Check pensieve for anything about list pagination before I touch this API"
- "Save this debugging conclusion as a memory"

Memories are saved as **drafts for human confirmation** — nothing sneaks into the record.

## Export to AGENTS.md

Make every agent in the repo aware of the memory system — even ones without MCP config:

```bash
pensieve export           # generate/update a managed section in ./AGENTS.md
pensieve export --check   # CI: fail if AGENTS.md is out of sync with the memory store
pensieve export --out CLAUDE.md   # other harness filenames work too
```

The managed section contains only *discipline instructions and one-line gotcha pointers* —
details always stay in the memory store. Writes via MCP auto-refresh the managed section.

## CLI

```bash
pensieve add "..."             # write a memory (LLM-assisted draft → confirm)
pensieve search "cpu shows 0"  # hybrid search
pensieve get <id>              # read full memory
pensieve list                  # browse recent
pensieve update <id> --vote    # upvote / mark superseded
pensieve topic ...             # cluster memories into topic dossiers
pensieve stats                 # usage & reuse metrics
pensieve undo                   # undo the last write
pensieve export [--check]       # AGENTS.md managed section
pensieve stale [--mark]         # patrol drifted code anchors (file deleted / changed since)
pensieve sync                  # pull + push the git remote
pensieve reindex               # rebuild the index
pensieve doctor                # health check (incl. drifted code anchors)
pensieve serve                 # run the MCP server (stdio)
```

## Drift defense

Memories get stale as the world changes; pensieve catches drift at three moments — always as *suggestions*, never silent auto-edits:

- **On write** — drafts are scanned against the store: a *conflict band* (cosine 0.60–0.85) catches "similar but different" claims, and an LLM judges whether the new memory contradicts an old one. Confirming with `supersedes` / `--supersedes` atomically marks the old memory **superseded** in the same commit.
- **On patrol** — `pensieve stale` (also surfaced in `memory_context` briefings and `pensieve doctor`) flags memories whose code anchors were deleted or changed after the memory was created. Nothing changes state until you confirm.
- **On read** — superseded memories are **excluded from search by default** (`--include-superseded` / `include_superseded` to opt in for archaeology), and `get` prepends a banner pointing to the successor, so a stale conclusion can't be mistaken for current truth.

> 📖 详细中文使用教程见 [docs/usage.md](docs/usage.md)

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
  URLs are rejected *before* they can touch the record (that's the whole point)
- 🧹 **Semantic dedup** blocks duplicates before persistence
- 🏠 **Local-first** — MCP server listens on stdio only; no network except your LLM gateway and git remote
- 🕳️ `sensitivity: local-only` memories never leave the machine

## Development

```bash
go build ./...
go test ./...
```

Contributions welcome — issues and PRs.

## License

[Apache-2.0](LICENSE)
