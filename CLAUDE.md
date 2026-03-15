# Wellspring (`wsp`)

Unified CLI that turns public-apis/public-apis from a catalog into a callable tool. One binary, one interface, any public data source.

## Tech Stack

- **Language**: Go
- **CLI**: Cobra + Viper
- **Output**: lipgloss (tables), stdlib JSON
- **MCP**: mcp-go (stdio transport)
- **Release**: goreleaser

## Architecture

Two-tier adapter system:
- **Declarative** (YAML in `sources/`): simple REST+JSON APIs — add a YAML file, no recompile
- **Coded** (Go in `internal/adapter/coded/`): complex APIs needing custom auth/pagination/rate limiting

Both implement the `Adapter` interface → produce normalized `DataPoint` output.

All adapters implement `ToolParams(endpoint string) []ToolParam` to self-describe their MCP tool parameters, enabling the MCP server to register tools uniformly without adapter-specific switch statements.

## Repository Structure

```
cmd/                    # Cobra commands (root, news, weather, crypto, finance, government, sources, update)
internal/
├── adapter/
│   ├── adapter.go      # Adapter interface + DataPoint struct
│   ├── coded/          # Handwritten Go adapters (reddit, alphavantage)
│   └── declarative/    # YAML-driven adapter engine
├── config/             # XDG config, env vars, precedence
├── output/             # Formatters: table, JSON, plain
├── ratelimit/          # Per-source sliding window rate limiter + file cache
└── registry/           # Source catalog, discovery, remote sync
sources/                # Built-in declarative YAML definitions
mcp/                    # MCP server mode (wsp serve)
```

## Supported Sources (6)

| Category | Source | Auth | Adapter Type |
|----------|--------|------|-------------|
| news | Hacker News | none | declarative |
| news | Reddit | none | coded |
| weather | Open-Meteo | none | declarative |
| crypto | CoinGecko | none | declarative |
| finance | Alpha Vantage | free key | coded |
| government | World Bank | none | declarative |

## Key Files

| File | Purpose |
|------|---------|
| `internal/adapter/adapter.go` | `Adapter` interface, `DataPoint` struct |
| `internal/adapter/declarative/engine.go` | YAML → HTTP client + response parser |
| `internal/config/config.go` | Config loading (XDG, env, flags) |
| `internal/registry/registry.go` | Source catalog + discovery |
| `cmd/root.go` | Root command, global flags |

## Commands

```
wsp news top [--source=reddit]       # News from HN or Reddit
wsp weather forecast --location=X    # Weather forecast
wsp crypto prices                    # Crypto prices
wsp finance quote --symbol=X         # Stock quotes (needs API key)
wsp government population --country=X # World Bank data
wsp sources [--supported] [--check]  # Discovery + health-check
wsp update                           # Force registry refresh
wsp serve                            # MCP server mode
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `WSP_ALPHA_VANTAGE_KEY` | Alpha Vantage API key |
| `WSP_CONFIG_DIR` | Override config dir |
| `WSP_CACHE_DIR` | Override cache dir |
| `NO_COLOR` | Disable color output |

## Exit Codes

0=success, 1=API error, 2=auth missing, 3=rate limited, 4=invalid input

## Development

```bash
go build -o wsp .        # Build
go test ./...            # Run tests
wsp news top --debug     # Debug mode (shows HTTP details on stderr)
```
