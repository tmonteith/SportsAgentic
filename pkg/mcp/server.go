package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tmonteith/SportsAgentic/pkg/analytics"
	"github.com/tmonteith/SportsAgentic/pkg/cfbd"
	"github.com/tmonteith/SportsAgentic/pkg/resolver"
)

type Server struct {
	mcpServer  *server.MCPServer
	cfbdClient *cfbd.Client
	analytics  *analytics.Service // nil unless GCP_PROJECT is set
}

// NewServer registers the live CFBD tools, plus the lakehouse analytics tools
// when the GCP_PROJECT env var is set.
func NewServer(apiKey string) *Server {
	return newServer(apiKey, strings.TrimSpace(os.Getenv("GCP_PROJECT")), newBigQueryQuerier)
}

func newServer(apiKey, gcpProject string, newQuerier querierFactory) *Server {
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
	if gcpProject != "" {
		srv.registerAnalyticsTools(gcpProject, newQuerier)
	}
	return srv
}

func (s *Server) registerTools() {
	// Tool 1: Get CFB Scores & Schedule
	scoresTool := mcp.NewTool("get_cfb_scores",
		mcp.WithDescription("Fetch College Football scores, game schedules, and match results. IMPORTANT: Every call needs 'team' or 'week'; a call with neither is rejected because a whole nationwide season is too large. Always provide 'team' if a specific school or matchup is requested."),
		mcp.WithNumber("year", mcp.Description("Season year (defaults to current year)")),
		mcp.WithNumber("week", mcp.Description("Week number (1-15). Required unless 'team' is provided.")),
		mcp.WithString("date", mcp.Description("Narrows a 'team' or 'week' result to one date, formatted as YYYY-MM-DD")),
		mcp.WithString("team", mcp.Description("Team name or common abbreviation (e.g., 'CSU', 'Ole Miss', 'Bama', 'Ohio State'). ALWAYS provide this when asking about a specific team.")),
	)
	s.mcpServer.AddTool(scoresTool, s.handleGetScores)

	// Tool 2: Get Team Schedule
	scheduleTool := mcp.NewTool("get_team_schedule",
		mcp.WithDescription("Get past or upcoming schedule and game results for a specific College Football team."),
		mcp.WithString("team", mcp.Required(), mcp.Description("Team name or abbreviation (e.g., 'Georgia', 'Michigan', 'Texas', 'CSU')")),
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
	if t, ok := team.(string); ok && strings.TrimSpace(t) != "" {
		teamName = resolver.ResolveTeamName(t)
	}

	// Without a team or week, CFBD returns the whole nationwide season, which takes
	// longer than MCP clients wait and is too large to use. CFBD has no date filter,
	// so the date check below runs after the fetch and doesn't narrow the query.
	if teamName == "" && weekInt <= 0 {
		return mcp.NewToolResultError("get_cfb_scores needs a 'team' or a 'week': a whole nationwide season is too large to return. Call it again with one of them; 'year' and 'date' alone are not enough."), nil
	}

	games, err := s.cfbdClient.GetGames(yearInt, weekInt, teamName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch games: %v", err)), nil
	}

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
	baseURL := "http://localhost" + addr
	if !strings.HasPrefix(addr, ":") {
		baseURL = "http://" + addr
	}
	sseServer := server.NewSSEServer(s.mcpServer, baseURL)
	return sseServer.Start(addr)
}
