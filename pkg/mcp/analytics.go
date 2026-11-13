package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmonteith/SportsAgentic/pkg/analytics"
)

// querierFactory builds the BigQuery client for a project. Tests swap in a fake.
type querierFactory func(ctx context.Context, project string) (analytics.Querier, error)

func newBigQueryQuerier(ctx context.Context, project string) (analytics.Querier, error) {
	return analytics.NewBigQuery(ctx, project)
}

const liveToolsHint = "For current or live scores, schedules, rankings and rosters, use the live API tools instead (get_cfb_scores, get_team_schedule, get_cfb_rankings)."

// registerAnalyticsTools adds the lakehouse tools. It runs only when
// GCP_PROJECT is set; if the BigQuery client can't be created, the server
// keeps serving the live tools and logs why.
func (s *Server) registerAnalyticsTools(project string, newQuerier querierFactory) {
	if !analytics.ValidProject(project) {
		log.Printf("Warning: GCP_PROJECT %q is not a valid project ID; analytics tools are disabled.", project)
		return
	}
	querier, err := newQuerier(context.Background(), project)
	if err != nil {
		log.Printf("Warning: BigQuery client unavailable (%v); analytics tools are disabled.", err)
		return
	}
	svc, err := analytics.NewService(project, querier)
	if err != nil {
		log.Printf("Warning: %v; analytics tools are disabled.", err)
		return
	}
	s.analytics = svc

	metricList := strings.Join(analytics.MetricNames(), ", ")

	teamAnalyticsTool := mcp.NewTool("get_team_analytics",
		mcp.WithDescription("Weekly efficiency analytics for a college football team from the SportsAgentic lakehouse "+
			"(BigQuery gold tables; completed games only, refreshed nightly). Use this for multi-week or multi-season "+
			"analytics, past games and efficiency stats: offensive EPA per play, success rate, explosiveness (EPA per "+
			"successful play) and points per drive, plus the def_* versions (what the defense allowed; lower is better). "+
			"Returns one row per week and a range total computed from summed plays and drives, not an average of weekly "+
			"rates. One season per call; for several seasons, call once per season. Pass compare_team to get a second "+
			"team over the same weeks. "+liveToolsHint),
		mcp.WithString("team", mcp.Required(), mcp.Description("Team name or common abbreviation (e.g. 'Colorado', 'CSU', 'Bama').")),
		mcp.WithNumber("season", mcp.Required(), mcp.Description("Season year, e.g. 2025.")),
		mcp.WithNumber("week_from", mcp.Description(fmt.Sprintf("First week to include (%d-%d). Omit to start at the first week.", analytics.MinWeek, analytics.MaxWeek))),
		mcp.WithNumber("week_to", mcp.Description(fmt.Sprintf("Last week to include (%d-%d). Omit to run to the end of the season. With season_type 'both', the week range applies to regular-season weeks and the postseason is included only when week_to is omitted.", analytics.MinWeek, analytics.MaxWeek))),
		withStringArray("metrics",
			"Metrics to return; default all. Values: "+metricList+". A comma-separated string is also accepted.",
			analytics.MetricNames()),
		mcp.WithString("compare_team", mcp.Description("Optional second team to compare over the same weeks.")),
		mcp.WithString("season_type", mcp.Enum(analytics.SeasonTypeRegular, analytics.SeasonTypePostseason, analytics.SeasonTypeBoth),
			mcp.Description("'regular', 'postseason' or 'both' (default). Postseason games are numbered separately, usually week 1.")),
	)
	s.mcpServer.AddTool(teamAnalyticsTool, s.handleGetTeamAnalytics)

	gameSummaryTool := mcp.NewTool("get_game_summary",
		mcp.WithDescription("Summary of one past college football game from the SportsAgentic lakehouse (BigQuery gold "+
			"tables, refreshed nightly and after game days): score, winner, plays, yards, EPA per play, success rate and "+
			"turnovers for each team. Pass game_id, or team + season + week. Use this for past games and efficiency "+
			"stats. "+liveToolsHint),
		mcp.WithNumber("game_id", mcp.Description("Game ID. When set, team, season and week are ignored.")),
		mcp.WithString("team", mcp.Description("Team name or abbreviation. Required with season and week when game_id is not set.")),
		mcp.WithNumber("season", mcp.Description("Season year, e.g. 2025. Required with team and week.")),
		mcp.WithNumber("week", mcp.Description(fmt.Sprintf("Week number (%d-%d). Required with team and season.", analytics.MinWeek, analytics.MaxWeek))),
		mcp.WithString("season_type", mcp.Enum(analytics.SeasonTypeRegular, analytics.SeasonTypePostseason, analytics.SeasonTypeBoth),
			mcp.Description("'regular', 'postseason' or 'both' (default). Postseason games are usually week 1.")),
	)
	s.mcpServer.AddTool(gameSummaryTool, s.handleGetGameSummary)

	log.Printf("Analytics tools enabled (BigQuery project %s, location %s).", project, analytics.Location)
}

// withStringArray adds an array-of-enum-strings property. mcp-go v0.10.3 has
// no array option, but its schema properties are a plain map.
func withStringArray(name, description string, enum []string) mcp.ToolOption {
	return func(t *mcp.Tool) {
		t.InputSchema.Properties[name] = map[string]interface{}{
			"type":        "array",
			"description": description,
			"items":       map[string]interface{}{"type": "string", "enum": enum},
			"uniqueItems": true,
		}
	}
}

func (s *Server) handleGetTeamAnalytics(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	req, err := parseTeamAnalyticsArgs(request.Params.Arguments)
	if err != nil {
		return toolError(err), nil
	}
	result, err := s.analytics.TeamAnalytics(ctx, req)
	if err != nil {
		return toolError(err), nil
	}
	return jsonResult(result), nil
}

func (s *Server) handleGetGameSummary(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	req, err := parseGameSummaryArgs(request.Params.Arguments)
	if err != nil {
		return toolError(err), nil
	}
	result, err := s.analytics.GameSummary(ctx, req)
	if err != nil {
		return toolError(err), nil
	}
	return jsonResult(result), nil
}

// argError is an argument problem, shown to the model as is.
type argError string

func (e argError) Error() string { return string(e) }

func toolError(err error) *mcp.CallToolResult {
	var userErr *analytics.UserError
	var argErr argError
	switch {
	case errors.As(err, &userErr):
		return mcp.NewToolResultError(userErr.Error())
	case errors.As(err, &argErr):
		return mcp.NewToolResultError(argErr.Error())
	default:
		log.Printf("analytics: %v", err)
		return mcp.NewToolResultError("The analytics tool failed. Try again later; the live tools still work.")
	}
}

// jsonResult returns compact JSON: a season of weekly rows for two teams is
// several KB, and the orchestrator's model context is small.
func jsonResult(v interface{}) *mcp.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return toolError(err)
	}
	return mcp.NewToolResultText(string(data))
}

func parseTeamAnalyticsArgs(args map[string]interface{}) (analytics.TeamAnalyticsRequest, error) {
	var req analytics.TeamAnalyticsRequest
	var err error
	if req.Team, err = stringArg(args, "team", true); err != nil {
		return req, err
	}
	if req.CompareTeam, err = stringArg(args, "compare_team", false); err != nil {
		return req, err
	}
	season, err := intArg(args, "season", true, analytics.MinSeason, analytics.MaxSeason)
	if err != nil {
		return req, err
	}
	req.Season = *season
	if req.WeekFrom, err = intArg(args, "week_from", false, analytics.MinWeek, analytics.MaxWeek); err != nil {
		return req, err
	}
	if req.WeekTo, err = intArg(args, "week_to", false, analytics.MinWeek, analytics.MaxWeek); err != nil {
		return req, err
	}
	if req.WeekFrom != nil && req.WeekTo != nil && *req.WeekFrom > *req.WeekTo {
		return req, argError(fmt.Sprintf("week_from (%d) is after week_to (%d).", *req.WeekFrom, *req.WeekTo))
	}
	if req.SeasonType, err = seasonTypeArg(args); err != nil {
		return req, err
	}
	metrics, err := metricsArg(args)
	if err != nil {
		return req, err
	}
	if req.Metrics, err = analytics.NormalizeMetrics(metrics); err != nil {
		return req, err
	}
	return req, nil
}

func parseGameSummaryArgs(args map[string]interface{}) (analytics.GameSummaryRequest, error) {
	var req analytics.GameSummaryRequest
	gameID, err := int64Arg(args, "game_id")
	if err != nil {
		return req, err
	}
	if gameID != nil {
		req.GameID = *gameID
		return req, nil
	}
	if req.Team, err = stringArg(args, "team", false); err != nil {
		return req, err
	}
	season, err := intArg(args, "season", false, analytics.MinSeason, analytics.MaxSeason)
	if err != nil {
		return req, err
	}
	week, err := intArg(args, "week", false, analytics.MinWeek, analytics.MaxWeek)
	if err != nil {
		return req, err
	}
	if req.Team == "" || season == nil || week == nil {
		return req, argError("get_game_summary needs 'game_id', or 'team' + 'season' + 'week'.")
	}
	req.Season, req.Week = *season, *week
	if req.SeasonType, err = seasonTypeArg(args); err != nil {
		return req, err
	}
	return req, nil
}

func stringArg(args map[string]interface{}, name string, required bool) (string, error) {
	v, ok := args[name]
	if !ok || v == nil {
		if required {
			return "", argError(fmt.Sprintf("'%s' is required.", name))
		}
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", argError(fmt.Sprintf("'%s' must be a string.", name))
	}
	s = strings.TrimSpace(s)
	if s == "" && required {
		return "", argError(fmt.Sprintf("'%s' is required.", name))
	}
	return s, nil
}

// intArg reads a whole number within [min, max]. JSON numbers arrive as float64.
func intArg(args map[string]interface{}, name string, required bool, min, max int) (*int, error) {
	v, ok := args[name]
	if !ok || v == nil {
		if required {
			return nil, argError(fmt.Sprintf("'%s' is required.", name))
		}
		return nil, nil
	}
	f, ok := v.(float64)
	if !ok || f != math.Trunc(f) {
		return nil, argError(fmt.Sprintf("'%s' must be a whole number.", name))
	}
	if f < float64(min) || f > float64(max) {
		return nil, argError(fmt.Sprintf("'%s' must be between %d and %d.", name, min, max))
	}
	i := int(f)
	return &i, nil
}

func int64Arg(args map[string]interface{}, name string) (*int64, error) {
	v, ok := args[name]
	if !ok || v == nil {
		return nil, nil
	}
	f, ok := v.(float64)
	if !ok || f != math.Trunc(f) || f < 1 || f > 1e15 {
		return nil, argError(fmt.Sprintf("'%s' must be a positive whole number.", name))
	}
	i := int64(f)
	return &i, nil
}

func seasonTypeArg(args map[string]interface{}) (string, error) {
	s, err := stringArg(args, "season_type", false)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(s) {
	case "", analytics.SeasonTypeBoth:
		return analytics.SeasonTypeBoth, nil
	case analytics.SeasonTypeRegular:
		return analytics.SeasonTypeRegular, nil
	case analytics.SeasonTypePostseason:
		return analytics.SeasonTypePostseason, nil
	default:
		return "", argError("'season_type' must be 'regular', 'postseason' or 'both'.")
	}
}

// metricsArg accepts an array of strings, or one comma-separated string.
func metricsArg(args map[string]interface{}) ([]string, error) {
	v, ok := args["metrics"]
	if !ok || v == nil {
		return nil, nil
	}
	switch x := v.(type) {
	case string:
		return strings.Split(x, ","), nil
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				return nil, argError("'metrics' must be a list of metric names.")
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, argError("'metrics' must be a list of metric names.")
	}
}
