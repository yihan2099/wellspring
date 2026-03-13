package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/wellspring-cli/wellspring/internal/adapter"
	"github.com/wellspring-cli/wellspring/internal/adapter/declarative"
	"github.com/wellspring-cli/wellspring/internal/ratelimit"
	"github.com/wellspring-cli/wellspring/internal/registry"
)

// Server wraps the MCP server and adapter registry.
type Server struct {
	reg     *registry.Registry
	server  *server.MCPServer
	limiter *ratelimit.Limiter
}

// NewServer creates a new MCP server that exposes all registered adapters as tools.
// The limiter enforces per-source rate limits for MCP tool calls.
func NewServer(reg *registry.Registry, version string, limiter *ratelimit.Limiter) *Server {
	s := server.NewMCPServer(
		"wellspring",
		version,
		server.WithToolCapabilities(true),
	)

	srv := &Server{
		reg:     reg,
		server:  s,
		limiter: limiter,
	}

	srv.registerTools()
	return srv
}

// registerTools registers all adapter endpoints as MCP tools.
func (s *Server) registerTools() {
	for _, a := range s.reg.All() {
		adp := a // capture for closure
		for _, endpoint := range a.Endpoints() {
			ep := endpoint // capture for closure
			toolName := fmt.Sprintf("%s_%s", adp.Name(), ep)

			var tool mcp.Tool

			// Coded adapters use hardcoded parameter definitions.
			switch adp.Name() {
			case "reddit":
				tool = mcp.NewTool(
					toolName,
					mcp.WithDescription(fmt.Sprintf("Reddit — %s posts from a subreddit", ep)),
					mcp.WithString("limit", mcp.Description("Maximum number of results"), mcp.DefaultString("10")),
					mcp.WithString("subreddit", mcp.Description("Subreddit name"), mcp.DefaultString("technology")),
					mcp.WithString("time", mcp.Description("Time filter (hour, day, week, month, year, all)"), mcp.DefaultString("day")),
				)
			case "alphavantage":
				if ep == "search" {
					tool = mcp.NewTool(
						toolName,
						mcp.WithDescription("Alpha Vantage — search for stock symbols"),
						mcp.WithString("query", mcp.Description("Search query (company name or partial symbol)"), mcp.Required()),
						mcp.WithString("symbol", mcp.Description("Alias for query (stock ticker symbol)")),
						mcp.WithString("limit", mcp.Description("Maximum number of results"), mcp.DefaultString("10")),
					)
				} else {
					tool = mcp.NewTool(
						toolName,
						mcp.WithDescription(fmt.Sprintf("Alpha Vantage — %s", ep)),
						mcp.WithString("symbol", mcp.Description("Stock ticker symbol"), mcp.Required()),
						mcp.WithString("limit", mcp.Description("Maximum number of results"), mcp.DefaultString("10")),
					)
				}
			default:
				// For declarative adapters, auto-generate tool parameters from the
				// YAML endpoint definition (query params + path template params).
				tool = s.buildDeclarativeTool(toolName, adp, ep)
			}

			handler := s.makeHandler(adp, ep)
			s.server.AddTool(tool, handler)
		}
	}

	// Add a sources discovery tool.
	sourcesTool := mcp.NewTool(
		"list_sources",
		mcp.WithDescription("List all available data sources and their status"),
		mcp.WithString("category", mcp.Description("Filter by category")),
		mcp.WithString("auth", mcp.Description("Filter by auth type")),
		mcp.WithBoolean("supported_only", mcp.Description("Show only sources with adapters")),
	)
	s.server.AddTool(sourcesTool, s.handleListSources)
}

// buildDeclarativeTool generates an MCP tool definition from a declarative adapter's
// YAML endpoint params and path template parameters.
func (s *Server) buildDeclarativeTool(toolName string, adp adapter.Adapter, ep string) mcp.Tool {
	opts := []mcp.ToolOption{
		mcp.WithDescription(fmt.Sprintf("%s — %s: %s", adp.Category(), adp.Name(), ep)),
		mcp.WithString("limit", mcp.Description("Maximum number of results"), mcp.DefaultString("10")),
	}

	if da, ok := adp.(*declarative.DeclarativeAdapter); ok {
		// Expose YAML-declared query params with their defaults.
		if params := da.EndpointParams(ep); params != nil {
			for k, v := range params {
				opts = append(opts, mcp.WithString(k, mcp.Description(k+" parameter"), mcp.DefaultString(v)))
			}
		}
		// Expose path template params (e.g., {country}, {id}).
		for _, p := range da.EndpointPathParams(ep) {
			// Skip "id" — internal resolution param, not user-facing.
			if p == "id" {
				continue
			}
			opts = append(opts, mcp.WithString(p, mcp.Description(p+" parameter")))
		}
	}

	return mcp.NewTool(toolName, opts...)
}

// makeHandler creates an MCP tool handler for a given adapter and endpoint.
func (s *Server) makeHandler(a adapter.Adapter, endpoint string) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Enforce per-source rate limits.
		if s.limiter != nil {
			if ok, wait := s.limiter.Allow(a.Name(), a.RateLimit()); !ok {
				return mcp.NewToolResultError(ratelimit.FormatRateLimitError(a.Name(), wait)), nil
			}
		}

		params := map[string]string{
			"action": endpoint,
		}

		// Extract all arguments from the request, coercing non-string types
		// (e.g., JSON numbers like {"limit": 10}) to their string representation.
		if args := request.GetArguments(); args != nil {
			for key, val := range args {
				if strVal, ok := val.(string); ok {
					params[key] = strVal
				} else if val != nil {
					params[key] = fmt.Sprintf("%v", val)
				}
			}
		}

		points, err := a.Fetch(ctx, params)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Marshal results as JSON.
		data, err := json.MarshalIndent(map[string]any{
			"source":  a.Name(),
			"count":   len(points),
			"results": points,
		}, "", "  ")
		if err != nil {
			// Return both MCP tool-result error (for display) and Go error (for
			// MCP-level error status), so clients can distinguish transport/marshal
			// errors from data-level errors.
			return mcp.NewToolResultError(fmt.Sprintf("marshaling results: %v", err)),
				fmt.Errorf("marshaling results: %w", err)
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

// handleListSources handles the list_sources MCP tool.
func (s *Server) handleListSources(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter := registry.SourceFilter{}

	args := request.GetArguments()
	if args != nil {
		if cat, ok := args["category"].(string); ok {
			filter.Category = cat
		}
		if auth, ok := args["auth"].(string); ok {
			filter.Auth = auth
		}
		if supported, ok := args["supported_only"].(bool); ok && supported {
			t := true
			filter.Supported = &t
		}
	}

	sources := s.reg.Sources(filter)

	data, err := json.MarshalIndent(map[string]any{
		"count":   len(sources),
		"sources": sources,
	}, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling sources: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

// ServeStdio starts the MCP server on stdio transport.
func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.server)
}
