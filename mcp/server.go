package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/wellspring-cli/wellspring/internal/adapter"
	"github.com/wellspring-cli/wellspring/internal/registry"
)

// Server wraps the MCP server and adapter registry.
type Server struct {
	reg    *registry.Registry
	server *server.MCPServer
}

// NewServer creates a new MCP server that exposes all registered adapters as tools.
func NewServer(reg *registry.Registry, version string) *Server {
	s := server.NewMCPServer(
		"wellspring",
		version,
		server.WithToolCapabilities(true),
	)

	srv := &Server{
		reg:    reg,
		server: s,
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

			tool := mcp.NewTool(
				toolName,
				mcp.WithDescription(fmt.Sprintf("%s — %s: %s", adp.Category(), adp.Name(), ep)),
				mcp.WithString("limit", mcp.Description("Maximum number of results"), mcp.DefaultString("10")),
			)

			// Add source-specific parameters.
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
				tool = mcp.NewTool(
					toolName,
					mcp.WithDescription(fmt.Sprintf("Alpha Vantage — %s", ep)),
					mcp.WithString("symbol", mcp.Description("Stock ticker symbol"), mcp.Required()),
					mcp.WithString("limit", mcp.Description("Maximum number of results"), mcp.DefaultString("10")),
				)
			case "openmeteo":
				tool = mcp.NewTool(
					toolName,
					mcp.WithDescription(fmt.Sprintf("Open-Meteo — %s", ep)),
					mcp.WithString("latitude", mcp.Description("Latitude"), mcp.DefaultString("40.7128")),
					mcp.WithString("longitude", mcp.Description("Longitude"), mcp.DefaultString("-74.0060")),
				)
			case "worldbank":
				tool = mcp.NewTool(
					toolName,
					mcp.WithDescription(fmt.Sprintf("World Bank — %s", ep)),
					mcp.WithString("country", mcp.Description("Country code (ISO 3166-1 alpha-2)"), mcp.DefaultString("US")),
					mcp.WithString("limit", mcp.Description("Maximum number of results"), mcp.DefaultString("10")),
				)
			case "coingecko":
				tool = mcp.NewTool(
					toolName,
					mcp.WithDescription(fmt.Sprintf("CoinGecko — %s", ep)),
					mcp.WithString("limit", mcp.Description("Maximum number of results"), mcp.DefaultString("10")),
				)
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

// makeHandler creates an MCP tool handler for a given adapter and endpoint.
func (s *Server) makeHandler(a adapter.Adapter, endpoint string) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]string{
			"action": endpoint,
		}

		// Extract all string arguments from the request.
		if args := request.GetArguments(); args != nil {
			for key, val := range args {
				if strVal, ok := val.(string); ok {
					params[key] = strVal
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
			return mcp.NewToolResultError(fmt.Sprintf("marshaling results: %v", err)), nil
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
