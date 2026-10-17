package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tmonteith/SportsAgentic/pkg/cfbd"
	"github.com/tmonteith/SportsAgentic/pkg/resolver"
)

type Server struct {
	mcpServer  *server.MCPServer
	cfbdClient *cfbd.Client
}

func NewServer(apiKey string) *Server {
	s := server.NewMCPServer(
		"College Football Agent",
		"1.0.0",
		server.WithLogging(),
	)

	cfbdClient := cfbd.NewClient(apiKey)

	srv := &Server{
		mcpServer:  s,
		cfbdClient: cfbdClient,
	}

	srv.registerTools()
	return srv
}

func (s *Server) registerTools() {
	// Tool 1: Get CFB Scores
	scoresTool := mcp.NewTool("get_cfb_scores",
		mcp.WithDescription("Fetch College Football scores and match results for a given year, week, or team."),
		mcp.WithNumber("year", mcp.Description("Season year (e.g. 2024)")),
		mcp.WithNumber("week", mcp.Description("Week number (1-15)")),
		mcp.WithString("team", mcp.Description("Team name or common abbreviation (e.g., 'Ole Miss', 'Bama', 'Ohio State')")),
	)
	s.mcpServer.AddTool(scoresTool, s.handleGetScores)

	// Tool 2: Get Team Schedule
	scheduleTool := mcp.NewTool("get_team_schedule",
		mcp.WithDescription("Get past or upcoming schedule and game results for a specific College Football team."),
		mcp.WithString("team", mcp.Required(), mcp.Description("Team name or abbreviation (e.g., 'Georgia', 'Michigan', 'Texas')")),
		mcp.WithNumber("year", mcp.Description("Season year (e.g. 2024)")),
	)
	s.mcpServer.AddTool(scheduleTool, s.handleGetSchedule)

	// Tool 3: Get Rankings
	rankingsTool := mcp.NewTool("get_cfb_rankings",
		mcp.WithDescription("Get national AP Top 25 or CFP rankings for a given season week."),
		mcp.WithNumber("year", mcp.Description("Season year (e.g. 2024)")),
		mcp.WithNumber("week", mcp.Description("Week number")),
	)
	s.mcpServer.AddTool(rankingsTool, s.handleGetRankings)
}

func (s *Server) handleGetScores(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	year := request.Params.Arguments["year"]
	week := request.Params.Arguments["week"]
	team := request.Params.Arguments["team"]

	var yearInt, weekInt int
	if y, ok := year.(float64); ok {
		yearInt = int(y)
	} else {
		yearInt = 2024
	}

	if w, ok := week.(float64); ok {
		weekInt = int(w)
	}

	var teamName string
	if t, ok := team.(string); ok && t != "" {
		teamName = resolver.ResolveTeamName(t)
	}

	games, err := s.cfbdClient.GetGames(yearInt, weekInt, teamName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch games: %v", err)), nil
	}

	data, err := json.MarshalIndent(games, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to serialize games data: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetSchedule(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	team, ok := request.Params.Arguments["team"].(string)
	if !ok || team == "" {
		return mcp.NewToolResultError("team parameter is required"), nil
	}

	resolvedTeam := resolver.ResolveTeamName(team)

	var yearInt int
	if y, ok := request.Params.Arguments["year"].(float64); ok {
		yearInt = int(y)
	} else {
		yearInt = 2024
	}

	games, err := s.cfbdClient.GetGames(yearInt, 0, resolvedTeam)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch schedule for %s: %v", resolvedTeam, err)), nil
	}

	data, err := json.MarshalIndent(games, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to serialize schedule data: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetRankings(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var yearInt, weekInt int
	if y, ok := request.Params.Arguments["year"].(float64); ok {
		yearInt = int(y)
	} else {
		yearInt = 2024
	}

	if w, ok := request.Params.Arguments["week"].(float64); ok {
		weekInt = int(w)
	}

	rankings, err := s.cfbdClient.GetRankings(yearInt, weekInt)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch rankings: %v", err)), nil
	}

	data, err := json.MarshalIndent(rankings, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to serialize rankings: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.mcpServer)
}

func (s *Server) ServeSSE(addr string) error {
	sseServer := server.NewSSEServer(s.mcpServer, "http://"+addr)

	mux := http.NewServeMux()
	mux.Handle("/sse", sseServer.SSEHandler())
	mux.Handle("/message", sseServer.MessageHandler())

	// Direct REST API route for easy web / curl / bot access
	mux.HandleFunc("/api/scores", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		team := r.URL.Query().Get("team")
		resolved := resolver.ResolveTeamName(team)
		games, err := s.cfbdClient.GetGames(2024, 0, resolved)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(games)
	})

	return http.ListenAndServe(addr, mux)
}
