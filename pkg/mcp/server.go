package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	// Tool 1: Get CFB Scores & Schedule
	scoresTool := mcp.NewTool("get_cfb_scores",
		mcp.WithDescription("Fetch College Football scores, game schedules, and match results for a given year, week, date (YYYY-MM-DD), or team."),
		mcp.WithNumber("year", mcp.Description("Season year (defaults to current year)")),
		mcp.WithNumber("week", mcp.Description("Week number (1-15)")),
		mcp.WithString("date", mcp.Description("Specific date filter formatted as YYYY-MM-DD")),
		mcp.WithString("team", mcp.Description("Team name or common abbreviation (e.g., 'Ole Miss', 'Bama', 'Ohio State')")),
	)
	s.mcpServer.AddTool(scoresTool, s.handleGetScores)

	// Tool 2: Get Team Schedule
	scheduleTool := mcp.NewTool("get_team_schedule",
		mcp.WithDescription("Get past or upcoming schedule and game results for a specific College Football team."),
		mcp.WithString("team", mcp.Required(), mcp.Description("Team name or abbreviation (e.g., 'Georgia', 'Michigan', 'Texas')")),
		mcp.WithNumber("year", mcp.Description("Season year (defaults to current year)")),
	)
	s.mcpServer.AddTool(scheduleTool, s.handleGetSchedule)

	// Tool 3: Get Rankings
	rankingsTool := mcp.NewTool("get_cfb_rankings",
		mcp.WithDescription("Get national AP Top 25 or CFP rankings for a given season week."),
		mcp.WithNumber("year", mcp.Description("Season year (defaults to current year)")),
		mcp.WithNumber("week", mcp.Description("Week number")),
	)
	s.mcpServer.AddTool(rankingsTool, s.handleGetRankings)
}

func (s *Server) handleGetScores(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	year := request.Params.Arguments["year"]
	week := request.Params.Arguments["week"]
	team := request.Params.Arguments["team"]
	date := request.Params.Arguments["date"]

	var yearInt int
	if y, ok := year.(float64); ok && y > 0 {
		yearInt = int(y)
	} else {
		yearInt = time.Now().Year()
	}

	var weekInt int
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

	// Filter by date if specified
	if dateStr, ok := date.(string); ok && dateStr != "" {
		filtered := make([]cfbd.Game, 0)
		for _, g := range games {
			if len(g.StartDate) >= len(dateStr) && g.StartDate[:len(dateStr)] == dateStr {
				filtered = append(filtered, g)
			}
		}
		games = filtered
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
	if y, ok := request.Params.Arguments["year"].(float64); ok && y > 0 {
		yearInt = int(y)
	} else {
		yearInt = time.Now().Year()
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
	var yearInt int
	if y, ok := request.Params.Arguments["year"].(float64); ok && y > 0 {
		yearInt = int(y)
	} else {
		yearInt = time.Now().Year()
	}

	var weekInt int
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
	return sseServer.Start(addr)
}
