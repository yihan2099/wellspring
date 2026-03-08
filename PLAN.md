# Wellspring — Project Plan

A unified CLI that turns [public-apis/public-apis](https://github.com/public-apis/public-apis) from a catalog into a tool. They maintain the list. We make it callable.

## Problem

- **405k stars** on public-apis — massive demand, but it's just a README with links
- The gap: knowing an API exists ≠ being able to use it (you still need endpoints, auth, parsing, rate limits)
- Developers and agents waste time wrangling each API's quirks individually
- No general-purpose CLI exists that unifies public data sources (OpenBB does this for finance only)

## Vision

One binary. One interface. Any public data source.

**Built on public-apis, not overlapping with it:**

| Layer | Who owns it | What it does |
|---|---|---|
| **Catalog** | [public-apis/public-apis](https://github.com/public-apis/public-apis) | What APIs exist, auth type, category, alive/dead |
| **Adapter** | Wellspring | How to call them, parse responses, normalize output |
| **CLI** | Wellspring | Unified interface for humans and agents |

```
wsp weather alerts --location="New York"
wsp finance stocks --symbol=AAPL --period=1m
wsp news top --source=hackernews --limit=10
wsp government census --dataset=population --year=2024
wsp crypto prices --coin=btc --format=json
```

## Design Principles (from clig.dev)

1. **Human-first, agent-ready** — pretty tables by default, `--json` for agents, `--plain` for scripts
2. **Subcommands as categories** — `wsp <category> <action>` (noun-verb)
3. **Flags over positional args** — explicit, scriptable, future-proof
4. **Stderr for status, stdout for data** — always pipeable
5. **Sensible defaults** — works with zero config for no-auth APIs
6. **Single binary** — no runtime dependencies
7. **No secrets in flags** — env vars or `--api-key-file` only
8. **Progress to stderr** — spinners for network calls, suppressed in non-TTY
9. **Consistent exit codes** — 0=success, 1=API error, 2=auth missing, 3=rate limited, 4=invalid input

## Architecture

```
wellspring/
├── cmd/                    # CLI entry points (Cobra commands)
│   ├── root.go             # Root command, global flags
│   ├── weather.go          # wsp weather ...
│   ├── finance.go          # wsp finance ...
│   ├── news.go             # wsp news ...
│   ├── crypto.go           # wsp crypto ...
│   ├── government.go       # wsp government ...
│   ├── sources.go          # wsp sources (discovery)
│   └── update.go           # wsp update (manual registry refresh)
├── internal/
│   ├── adapter/            # API adapters
│   │   ├── adapter.go      # Common adapter interface
│   │   ├── coded/          # Complex APIs — handwritten Go
│   │   │   ├── reddit.go
│   │   │   └── alphavantage.go
│   │   └── declarative/    # Simple APIs — YAML-driven
│   │       └── engine.go   # Reads YAML → builds HTTP client + parser
│   ├── output/             # Output formatting
│   │   ├── table.go        # Human-readable tables
│   │   ├── json.go         # Structured JSON
│   │   └── plain.go        # Machine-readable plain text
│   ├── config/             # Configuration management
│   │   └── config.go       # XDG config, env vars, precedence
│   ├── ratelimit/          # Rate limiting & caching
│   │   └── ratelimit.go    # Per-source rate limits, local cache
│   └── registry/           # Source registry & discovery + sync
│       ├── registry.go     # List available sources, categories
│       └── sync.go         # Background registry refresh
├── sources/                # Built-in declarative source definitions
│   ├── hackernews.yaml
│   ├── openmeteo.yaml
│   ├── coingecko.yaml
│   └── worldbank.yaml
├── mcp/                    # MCP server mode
│   └── server.go           # Expose all adapters as MCP tools
├── go.mod
├── go.sum
├── PLAN.md
├── LICENSE
└── README.md
```

### Two-Tier Adapter System

**Coded adapters** — handwritten Go for APIs with complex auth, pagination, or multi-step fetches (e.g. Reddit OAuth, Alpha Vantage rate limit quirks).

**Declarative adapters** — YAML definitions for simple REST+JSON APIs. Adding a new source = adding a YAML file, no recompile needed.

```yaml
# sources/hackernews.yaml
name: hackernews
category: news
auth: none
base_url: https://hacker-news.firebaseio.com/v0

endpoints:
  top:
    path: /topstories.json
    method: GET
    pagination: none
  item:
    path: /item/{id}.json
    method: GET

mapping:
  title: .title
  url:   .url
  time:  .time | unix
  value: .score
  meta:
    author: .by
    comments: .descendants

rate_limit:
  requests: 30
  per: 1m
```

### Relationship with public-apis

Wellspring builds **on top of** public-apis, not alongside it. Clear separation of concerns:

```
┌──────────────────────────────────────────────────────┐
│       public-apis/public-apis (UPSTREAM)              │
│       They own: catalog, categories, link validation  │
│       ~1400 APIs across ~50 categories                │
└──────────────────┬───────────────────────────────────┘
                   │
                   │  Nightly CI sync: parse their README,
                   │  build catalog.json (name, category,
                   │  auth, docs URL, alive/dead)
                   │
                   ▼
┌──────────────────────────────────────────────────────┐
│       wellspring adapter registry (OURS)              │
│       We own: how to call + parse each API            │
│       YAML/Go adapters for supported sources          │
│                                                       │
│  Catalog:  1400 known APIs  (from public-apis)       │
│  Adapters:    6 supported   (from us)                │
│  Status:    "1394 more available — PRs welcome"      │
└──────────────────┬───────────────────────────────────┘
                   │
                   │  Background sync to user's machine
                   │  (ETag caching, never blocks commands)
                   │
                   ▼
┌──────────────────────────────────────────────────────┐
│       wsp CLI (user's machine)                        │
│                                                       │
│  wsp sources        → shows ALL 1400 known APIs      │
│  wsp sources --supported → shows the 6 with adapters │
│  wsp news top       → calls supported adapters       │
└──────────────────────────────────────────────────────┘
```

**What we delegate to public-apis:**
- Discovering new APIs → they curate, we inherit
- Detecting dead APIs → their link validation CI catches it, we mark as dead
- Categorization → we use their ~50 categories as-is
- Auth requirements → they track none/apiKey/OAuth per API

**What we own:**
- Adapter definitions (YAML/Go) — how to actually call and parse each API
- Unified output schema — every source returns the same DataPoint format
- Rate limiting, caching, error handling per source
- The CLI and MCP server interface

**What this means for maintenance:**
- API removed from public-apis → automatically flagged as deprecated in `wsp sources`
- New API added to public-apis → appears in `wsp sources` as "no adapter yet"
- API breaks → if public-apis catches it (link validation), we inherit the signal
- We only maintain adapters for the sources we support, not the catalog

### Catalog Sync Pipeline

```yaml
# .github/workflows/sync-catalog.yml — runs nightly
name: Sync public-apis catalog
on:
  schedule:
    - cron: '0 4 * * *'
steps:
  - name: Fetch public-apis README
    run: curl -s https://raw.githubusercontent.com/public-apis/public-apis/master/README.md -o upstream.md
  - name: Parse to catalog.json
    run: go run ./cmd/sync-catalog upstream.md > catalog.json
    # Extracts: name, description, auth, https, cors, docs_url, category
  - name: Diff against current catalog
    run: diff catalog.json registry/catalog.json || echo "Updates found"
  - name: Commit if changed
    run: |
      # Auto-commit updated catalog
      # Open issue if a supported source was removed upstream
```

### Adapter Registry & Auto-Sync

The CLI ships with built-in source definitions, but stays current via a remote registry hosted on GitHub. Updates happen transparently — no user action required.

**Remote registry:** `raw.githubusercontent.com/wellspring-cli/registry/main/`
- `catalog.json` — full API catalog (synced from public-apis nightly)
- `adapters/` — YAML adapter definitions for supported sources

**Sync strategy — cache with background refresh:**

```
wsp <any command>
    │
    ├─ Registry cached and <24h old?
    │   ├─ YES → use cache, kick off background refresh if >12h
    │   └─ NO  → use built-in defaults, fetch in background
    │
    ├─ Background fetch uses If-None-Match (ETag):
    │   ├─ 304 Not Modified → no download (~50ms, no bandwidth)
    │   └─ 200 OK → write to cache, available on next command
    │
    └─ NEVER blocks the user's command on a registry fetch
```

**Cache location:**

```
~/.cache/wellspring/
├── catalog.json           # Full API catalog (from public-apis)
├── adapters/              # Cached adapter definitions
├── registry.etag          # GitHub ETag for conditional requests
└── registry.updated_at    # Last successful fetch timestamp
```

**Precedence (high to low):**
1. User-defined local sources (`~/.config/wellspring/sources/`)
2. Cached remote registry (`~/.cache/wellspring/`)
3. Built-in sources (compiled into binary)

**Key properties:**
- Works offline — always falls back to built-in sources
- Zero latency — never blocks commands on network I/O
- Always fresh — background refresh means sources stay current within 24h
- Manual override — `wsp update sources` forces an immediate refresh
- Extensible — users drop custom YAML files in `~/.config/wellspring/sources/`

### Core Interface

Every adapter (coded or declarative) implements:

```go
type Adapter interface {
    Name() string
    Category() string
    RequiresAuth() bool
    Fetch(ctx context.Context, params map[string]string) ([]DataPoint, error)
    RateLimit() RateLimitConfig
}

type DataPoint struct {
    Source    string         `json:"source"`
    Category string         `json:"category"`
    Time     time.Time      `json:"time"`
    Title    string         `json:"title,omitempty"`
    Value    any            `json:"value,omitempty"`
    Meta     map[string]any `json:"meta,omitempty"`
    URL      string         `json:"url,omitempty"`
}
```

Uniform output schema regardless of the underlying API or adapter type.

## MVP Scope (v0.1)

### Categories & Sources (no-auth or free-key APIs only)

| Category | Source | Auth | What it provides |
|---|---|---|---|
| `weather` | Open-Meteo | None | Forecasts, current conditions, alerts |
| `news` | Hacker News API | None | Top/new/best stories, comments |
| `news` | Reddit JSON | None | Subreddit top/hot posts |
| `finance` | Alpha Vantage | Free key | Stock quotes, time series |
| `crypto` | CoinGecko | None | Prices, market cap, trending |
| `government` | World Bank | None | Economic indicators, population |

### Global Flags

```
--json          Structured JSON output (default for non-TTY)
--plain         Tab-separated plain text
--quiet / -q    Suppress all non-data output
--no-color      Disable color output
--limit / -n    Max results (default: 10)
--cache         Cache duration override (e.g. "5m", "1h")
--offline       Skip registry sync, use built-in/cached sources only
--config        Path to config file
--debug         Show request/response details on stderr
--version       Print version
--help / -h     Help text
```

### MCP Server Mode

```
wsp serve        # Start as MCP server (stdio transport)
```

Exposes each category/action as an MCP tool so any LLM can call it directly.

### Discovery & Updates

```
wsp sources                      # List ALL known APIs (from public-apis catalog)
wsp sources --supported          # List only sources with adapters (callable)
wsp sources --category=news      # Filter by category
wsp sources --auth=none          # Filter by auth type
wsp sources --check              # Health-check supported source endpoints
wsp update                       # Force immediate catalog + adapter refresh
```

Example output:

```
$ wsp sources --category=news
  Source            Auth     Status
  ────────────────────────────────────
✓ Hacker News      none     supported
✓ Reddit           none     supported
  Dev.to           none     no adapter
  News API         apiKey   no adapter
  The Guardian     apiKey   no adapter
  GNews            apiKey   no adapter
  Currents         apiKey   no adapter

  2 supported · 5 available · PRs welcome → github.com/wellspring-cli/registry
```

## Milestones

### M0 — Skeleton (Week 1)
- [ ] Go module, Cobra CLI scaffolding, global flags
- [ ] Adapter interface definition (coded + declarative)
- [ ] Declarative adapter engine (YAML → HTTP client + parser)
- [ ] Output formatters (table, JSON, plain)
- [ ] Config loading (XDG, env vars)
- [ ] public-apis catalog parser (README → catalog.json)
- [ ] `wsp sources` discovery command (shows full catalog + adapter status)
- [ ] One working adapter (Hacker News — declarative YAML, no auth)

### M1 — Core Adapters (Week 2-3)
- [ ] Open-Meteo weather adapter (declarative)
- [ ] Reddit adapter (coded — OAuth complexity)
- [ ] CoinGecko crypto adapter (declarative)
- [ ] Alpha Vantage finance adapter (coded — rate limit logic)
- [ ] World Bank government data adapter (declarative)
- [ ] Rate limiting & response caching

### M2 — Registry & Agent Mode (Week 4)
- [ ] Nightly CI pipeline: sync public-apis README → catalog.json
- [ ] Remote registry sync (background refresh, ETag caching)
- [ ] `wsp update` manual refresh command
- [ ] Auto-detect dead/removed APIs from upstream catalog changes
- [ ] User-defined local source overrides (`~/.config/wellspring/sources/`)
- [ ] MCP server mode (`wsp serve`)
- [ ] Auto-detect TTY for output format
- [ ] `--json` structured output with consistent schema
- [ ] Exit code standardization

### M3 — Polish (Week 5)
- [ ] Error messages with actionable suggestions (clig.dev style)
- [ ] Help text with examples for every command
- [ ] `wsp` with no args shows quick-start guide
- [ ] `wsp sources --check` health-check command
- [ ] Man page generation
- [ ] Homebrew formula, goreleaser for cross-platform binaries
- [ ] README with demos

### Future (post-MVP)
- `wellspring-cli/registry` — separate repo for community-contributed adapter definitions
- `wsp dev new-source` / `wsp dev test-source` — scaffolding tools for contributors
- `wsp trending` — cross-source trending aggregation
- Webhook/watch mode (`wsp watch crypto prices --coin=btc --interval=30s`)
- Data export (CSV, SQLite)
- Auto-suggest adapters for popular no-auth APIs that lack one
- More sources: NASA, SEC EDGAR, FDA, USGS earthquakes, arXiv, PubMed

## Tech Stack

| Component | Choice | Why |
|---|---|---|
| Language | Go | Single binary, fast, Cobra ecosystem |
| CLI framework | [Cobra](https://github.com/spf13/cobra) | Industry standard, subcommand support |
| Config | [Viper](https://github.com/spf13/viper) | Pairs with Cobra, env/file/flag precedence |
| HTTP | `net/http` + retries | Stdlib, no deps |
| Output tables | [lipgloss](https://github.com/charmbracelet/lipgloss) / [tablewriter](https://github.com/olekukonez/tablewriter) | Pretty terminal output |
| MCP | [mcp-go](https://github.com/mark3labs/mcp-go) | Go MCP SDK |
| Testing | stdlib `testing` + [testify](https://github.com/stretchr/testify) | Simple assertions |
| Release | [goreleaser](https://goreleaser.com/) | Cross-platform binary builds |

## Config

`~/.config/wellspring/config.toml`:

```toml
[general]
default_format = "table"    # table | json | plain
default_limit = 10
cache_dir = "~/.cache/wellspring"
cache_ttl = "5m"

[keys]
alpha_vantage = ""          # or set WSP_ALPHA_VANTAGE_KEY env var

[sources.reddit]
default_subreddit = "technology"
```

## Binary Name

`wsp` — short, easy to type, natural finger flow, not taken.

Full name `wellspring` for the repo/project, `wsp` for the CLI command.

## Competitive Positioning

| | Wellspring | OpenBB | wttr.in | public-apis |
|---|---|---|---|---|
| Multi-category | Yes | Finance only | Weather only | List only |
| CLI | Yes | Yes | curl-based | No |
| Agent/MCP support | Yes | Yes | No | No |
| Single binary | Yes | Python | Service | N/A |
| No-auth quick start | Yes | Partial | Yes | N/A |
| Builds on public-apis | Yes | No | No | Is it |

**Positioning:** public-apis is the phone book. Wellspring is the phone.

## License

MIT
