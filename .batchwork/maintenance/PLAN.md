# Wellspring Maintenance Audit — Action Plan

Audit date: 2026-03-12

---

## P0 — Critical

- [x] #001 [Security] Fix API key leakage: build request URLs with `net/url.Values` instead of `fmt.Sprintf`; pass key via header or build URL without key in string; ensure all error paths mask credentials (`alphavantage.go`)
- [x] #016 [Error Handling] Coerce non-string MCP arguments: add `fmt.Sprintf("%v", val)` fallback for non-string types in `makeHandler` arg extraction loop (`mcp/server.go:118`)

## P1 — High

- [x] #002 [Race Conditions] Move `initGlobals()` inside `PersistentPreRunE` on rootCmd so flags are parsed before global state reads them (`cmd/root.go`)
- [x] #003 [Type Safety] Add nil-params guard or document contract that `Fetch()` always receives non-nil map (`alphavantage.go:54`)
- [x] #004 [Reliability] Add retry with exponential backoff (max 3 attempts) for transient HTTP errors (429, 5xx) in both `reddit.go` and `alphavantage.go`
- [x] #005 [Error Handling] Extract AV error-checking (`"Note"`, `"Information"` keys) into `doRequest()` so all fetch functions benefit, including `fetchSearch` which currently lacks it (`alphavantage.go`)
- [x] #017 [API Contracts] Generate MCP tool parameters from declarative YAML endpoint params instead of hardcoding per-source switch; keep switch only for coded adapters (`mcp/server.go:54-90`)
- [x] #018 [Security] Consolidate API key resolution: remove duplicate env-var lookup from `NewAlphaVantageAdapter()`, route through `config.GetAPIKey()` with uppercase normalization and debug logging (`config.go`, `alphavantage.go`)
- [x] #019 [Reliability] Wire rate limiter into MCP server; enforce per-source rate limits for MCP tool calls (`mcp/server.go`)

## P2 — Medium

- [x] #006 [Code Quality] Replace string-based `exitCode()` with sentinel error types (`ErrRateLimit`, `ErrAuthRequired`, `ErrInvalidInput`) and `errors.Is()` (`cmd/root.go`)
- [ ] #007 [Data Integrity] Log warnings when expected API fields are missing from Alpha Vantage responses; consider returning partial-data indicator in `DataPoint.Meta` (`alphavantage.go`)
- [x] #008 [API Contracts] Validate `action` against `Endpoints()` in Reddit `Fetch()`; validate `limit` range (clamp to 1-100); source default subreddit from config (`reddit.go`)
- [ ] #009 [API Contracts] Unify search parameter naming: accept both `--query` and `--symbol` for search, document the canonical name; expose `query` in MCP tool def for search endpoint (`alphavantage.go`, `mcp/server.go`)
- [ ] #010 [Security] Use `req.URL` builder (`net/url`) to add API key as query param after URL construction, or pass key via `X-API-Key` header if Alpha Vantage supports it (`alphavantage.go`)
- [x] #011 [Reliability] Check and log errors from `LoadUserSources()` and `LoadCatalog()` in `initGlobals()`; surface warnings unless `--quiet` (`cmd/root.go:152-155`)
- [ ] #012 [Performance] Use `url.PathEscape()` for subreddit name in URL construction (`reddit.go:90`)
- [ ] #013 [Reliability] Validate parsed limit: clamp to `[1, maxLimit]` range; log warning on invalid input instead of silently defaulting (`alphavantage.go:146-157`)
- [ ] #020 [API Contracts] Make CoinGecko `per_page` and `page` overridable via user params in declarative engine, or document the limit (`coingecko.yaml`, declarative engine)
- [ ] #021 [API Contracts] Replace hardcoded `date: "2015:2024"` with dynamic range (e.g., `{current_year-10}:{current_year}`) or make user-overridable (`worldbank.yaml`)
- [ ] #022 [Reliability] Add `max_resolve` or `limit` parameter to HackerNews list endpoints to cap ID resolution count (`hackernews.yaml`, declarative engine)
- [ ] #023 [Code Quality] Add defensive parsing for `ParsePublicAPIsREADME`: handle escaped pipes, log malformed lines, add unit tests with edge cases (`registry.go:285`)
- [ ] #024 [Error Handling] Return MCP-level error (second return value) in addition to tool-result error for marshaling failures, so clients can distinguish transport vs. data errors (`mcp/server.go:130`)
- [ ] #025 [Dependencies] Pin charmbracelet transitive deps to tagged releases; check if lipgloss has a newer version that uses stable deps; or vendor (`go.mod`)
- [ ] #026 [Environment] Document that catalog metadata takes precedence over adapter metadata in `Sources()` merge logic; add comment explaining design choice (`registry.go:134`)

## P3 — Low

- [ ] #014 [Code Quality] Remove unnecessary `RunE` from rootCmd or add a comment explaining why it's kept (`cmd/root.go:70`)
- [ ] #015 [Code Quality] Update `Description()` to match actual `Endpoints()` — remove "technical indicators" or add indicator endpoint (`alphavantage.go:43-45`)
- [ ] #027 [Code Quality] Add validation to `RateLimitConfig`: require `Requests > 0` and `Per > 0`; validate on adapter registration (`adapter.go`)
- [ ] #028 [API Contracts] Add YAML comment documenting NYC default coordinates; consider requiring explicit lat/lon with helpful error message (`openmeteo.yaml`)
- [ ] #029 [Code Quality] Move generic tool construction inside the `default` case of the switch, so it is only built for unknown sources (`mcp/server.go:47`)
- [ ] #030 [Error Handling] Handle `os.UserHomeDir()` error: return error from `ConfigDir()`/`DefaultCacheDir()` or fall back to `/tmp/wellspring` with a warning (`config.go:54,67`)
