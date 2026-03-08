# Wellspring (`wsp`)

One binary. One interface. Any public data source.

Wellspring turns [public-apis/public-apis](https://github.com/public-apis/public-apis) from a catalog into a tool. They maintain the list. We make it callable.

```
wsp news top                           # Hacker News top stories
wsp news top --source=reddit           # Reddit hot posts
wsp weather forecast --location=Tokyo  # 7-day weather forecast
wsp crypto prices                      # Top cryptocurrencies
wsp finance quote --symbol=AAPL        # Stock quote
wsp government population --country=US # World Bank data
wsp sources                            # Discover 1400+ APIs
wsp serve                              # MCP server for AI agents
```

## Install

### From source

```bash
go install github.com/wellspring-cli/wellspring@latest
```

### From releases

Download the latest binary from [Releases](https://github.com/wellspring-cli/wellspring/releases).

## Quick Start

```bash
# No API key needed for these:
wsp news top                                    # Hacker News
wsp news top --source=reddit --subreddit=golang  # Reddit
wsp weather current --location="New York"        # Current weather
wsp crypto prices --limit=5                      # Top 5 crypto
wsp government gdp --country=JP                  # Japan GDP

# Needs a free API key:
export WSP_ALPHA_VANTAGE_KEY=your_key
wsp finance quote --symbol=AAPL                  # Stock quote
```

## Supported Sources

| Category | Source | Auth | What it provides |
|---|---|---|---|
| `news` | Hacker News | None | Top/new/best stories |
| `news` | Reddit | None | Subreddit posts (hot/top/new) |
| `weather` | Open-Meteo | None | Forecasts, current conditions |
| `crypto` | CoinGecko | None | Prices, market cap, trending |
| `finance` | Alpha Vantage | Free key | Stock quotes, time series |
| `government` | World Bank | None | Population, GDP, indicators |

```bash
wsp sources --supported    # See all supported sources
wsp sources --category=news  # Filter by category
```

## Output Formats

```bash
wsp news top                  # Pretty table (default in terminal)
wsp news top --json           # Structured JSON (default when piped)
wsp news top --plain          # Tab-separated plain text
wsp news top | jq '.results[0].title'  # Pipe to jq
```

### JSON Schema

Every command returns the same consistent schema:

```json
{
  "ok": true,
  "source": "hackernews",
  "count": 10,
  "results": [
    {
      "source": "hackernews",
      "category": "news",
      "time": "2025-01-15T10:30:00Z",
      "title": "Show HN: Something Cool",
      "value": 142,
      "url": "https://example.com",
      "meta": {
        "author": "username",
        "comments": 47
      }
    }
  ]
}
```

## Commands

### News

```bash
wsp news top                              # HN top stories
wsp news new                              # HN newest stories
wsp news best                             # HN best stories
wsp news top --source=reddit              # Reddit hot posts
wsp news top --source=reddit --subreddit=programming --time=week
```

### Weather

```bash
wsp weather forecast                      # 7-day forecast (New York default)
wsp weather forecast --location="London"  # By city name
wsp weather forecast --lat=35.7 --lon=139.7  # By coordinates
wsp weather current --location="Paris"    # Current conditions
```

### Crypto

```bash
wsp crypto prices                         # Top 10 by market cap
wsp crypto prices --limit=25              # Top 25
wsp crypto trending                       # Trending coins
```

### Finance

```bash
export WSP_ALPHA_VANTAGE_KEY=your_key     # Free at alphavantage.co
wsp finance quote --symbol=AAPL           # Stock quote
wsp finance daily --symbol=MSFT           # Daily prices
wsp finance search --query=Tesla          # Symbol search
```

### Government

```bash
wsp government population --country=US    # US population
wsp government gdp --country=CN           # China GDP
wsp government countries                  # List countries
wsp government indicators --country=BR --indicator=SP.POP.TOTL
```

### Discovery

```bash
wsp sources                               # All known APIs
wsp sources --supported                   # Sources with adapters
wsp sources --category=news               # Filter by category
wsp sources --auth=none                   # No-auth sources only
wsp sources --check                       # Health-check endpoints
```

### MCP Server

```bash
wsp serve                                 # Start MCP server (stdio)
```

Exposes all adapters as MCP tools for AI agents.

## Configuration

Config file: `~/.config/wellspring/config.toml`

```toml
[general]
default_format = "table"
default_limit = 10
cache_ttl = "5m"

[keys]
alpha_vantage = "your_key_here"

[sources.reddit]
default_subreddit = "technology"
```

### Environment Variables

| Variable | Description |
|---|---|
| `WSP_ALPHA_VANTAGE_KEY` | Alpha Vantage API key |
| `WSP_CONFIG_DIR` | Override config directory |
| `WSP_CACHE_DIR` | Override cache directory |
| `NO_COLOR` | Disable color output |

### Custom Sources

Drop YAML files in `~/.config/wellspring/sources/` to add custom adapters:

```yaml
name: myapi
category: custom
auth: none
base_url: https://api.example.com

endpoints:
  list:
    path: /items.json
    method: GET

mapping:
  title: .name
  url: .link
  value: .score
```

## Global Flags

```
--json          Structured JSON output
--plain         Tab-separated plain text
--quiet / -q    Suppress non-data output
--no-color      Disable colors
--limit / -n    Max results (default 10)
--cache         Cache duration (e.g., "5m")
--offline       Use cached/built-in sources only
--debug         Show request details on stderr
--version       Print version
```

## Exit Codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | API error |
| 2 | Auth missing |
| 3 | Rate limited |
| 4 | Invalid input |

## Architecture

Wellspring uses a two-tier adapter system:

- **Declarative adapters** (YAML): For simple REST+JSON APIs. Add a YAML file to support a new source.
- **Coded adapters** (Go): For APIs with complex auth, pagination, or rate limit handling.

Both implement the same `Adapter` interface and produce normalized `DataPoint` output.

## Contributing

PRs welcome! The easiest way to contribute is to add a new declarative adapter:

1. Create a YAML file in `sources/`
2. Test it with `wsp <category> <action>`
3. Submit a PR

## License

MIT
