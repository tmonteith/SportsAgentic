package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmonteith/SportsAgentic/pkg/analytics"
)

const testProject = "sports-agent-511915"

var liveTools = []string{"get_cfb_rankings", "get_cfb_scores", "get_team_schedule"}

// stubQuerier answers team lookups for Colorado and returns rows for every
// other query.
type stubQuerier struct {
	calls atomic.Int32
	rows  []analytics.Row
}

func (q *stubQuerier) Query(ctx context.Context, query analytics.Query, maxRows int) ([]analytics.Row, bool, error) {
	q.calls.Add(1)
	if strings.Contains(query.SQL, "UNION ALL") {
		for _, p := range query.Params {
			for _, name := range p.Value.([]string) {
				if name == "colorado" {
					return []analytics.Row{{"team_id": "38", "team_name": "Colorado"}}, false, nil
				}
			}
		}
		return nil, false, nil
	}
	return q.rows, false, nil
}

func stubFactory(q analytics.Querier) querierFactory {
	return func(ctx context.Context, project string) (analytics.Querier, error) { return q, nil }
}

func listToolSchemas(t *testing.T, srv *Server) map[string]map[string]interface{} {
	t.Helper()
	resp := srv.mcpServer.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("encoding tools/list response: %v", err)
	}
	var decoded struct {
		Result struct {
			Tools []struct {
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				InputSchema map[string]interface{} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decoding tools/list: %v", err)
	}
	out := make(map[string]map[string]interface{})
	for _, tool := range decoded.Result.Tools {
		tool.InputSchema["_description"] = tool.Description
		out[tool.Name] = tool.InputSchema
	}
	return out
}

func names(tools map[string]map[string]interface{}) []string {
	var out []string
	for name := range tools {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func TestAnalyticsToolsNotRegisteredWithoutGCPProject(t *testing.T) {
	t.Setenv("GCP_PROJECT", "")
	srv := NewServer("")
	if got := names(listToolSchemas(t, srv)); strings.Join(got, ",") != strings.Join(liveTools, ",") {
		t.Errorf("tools = %v, want only %v", got, liveTools)
	}
	if srv.analytics != nil {
		t.Error("analytics service created without GCP_PROJECT")
	}
}

func TestAnalyticsToolsRegisteredWithGCPProject(t *testing.T) {
	srv := newServer("", testProject, stubFactory(&stubQuerier{}))
	tools := listToolSchemas(t, srv)
	want := append([]string{"get_game_summary", "get_team_analytics"}, liveTools...)
	sort.Strings(want)
	if got := names(tools); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tools = %v, want %v", got, want)
	}

	// The live tools keep their schemas.
	scores := tools["get_cfb_scores"]["properties"].(map[string]interface{})
	for _, p := range []string{"year", "week", "date", "team"} {
		if _, ok := scores[p]; !ok {
			t.Errorf("get_cfb_scores lost parameter %s", p)
		}
	}

	ta := tools["get_team_analytics"]
	metrics := ta["properties"].(map[string]interface{})["metrics"].(map[string]interface{})
	items := metrics["items"].(map[string]interface{})
	if metrics["type"] != "array" || len(items["enum"].([]interface{})) != 8 {
		t.Errorf("metrics schema = %v, want an array of 8 enum strings", metrics)
	}
	required := ta["required"].([]interface{})
	if len(required) != 2 || required[0] != "team" || required[1] != "season" {
		t.Errorf("required = %v, want [team season]", required)
	}
	for _, name := range []string{"get_team_analytics", "get_game_summary"} {
		desc := tools[name]["_description"].(string)
		if !strings.Contains(desc, "multi-week") && !strings.Contains(desc, "past games") {
			t.Errorf("%s description doesn't say when to use it: %s", name, desc)
		}
		if !strings.Contains(desc, "get_cfb_scores") {
			t.Errorf("%s description doesn't point to the live tools: %s", name, desc)
		}
	}
}

func TestAnalyticsToolsSkippedWhenClientFails(t *testing.T) {
	tests := map[string]struct {
		project string
		factory querierFactory
	}{
		"client error": {testProject, func(context.Context, string) (analytics.Querier, error) {
			return nil, errors.New("no credentials")
		}},
		"malformed project": {"Bad Project!", stubFactory(&stubQuerier{})},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newServer("", tt.project, tt.factory)
			if got := names(listToolSchemas(t, srv)); strings.Join(got, ",") != strings.Join(liveTools, ",") {
				t.Errorf("tools = %v, want only %v", got, liveTools)
			}
		})
	}
}

func callTool(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]interface{}) *mcp.CallToolResult {
	t.Helper()
	var request mcp.CallToolRequest
	request.Params.Arguments = args
	result, err := handler(context.Background(), request)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	return result
}

func TestTeamAnalyticsArgumentValidation(t *testing.T) {
	tests := map[string]struct {
		args map[string]interface{}
		want string
	}{
		"no team":            {map[string]interface{}{"season": float64(2025)}, "'team' is required"},
		"blank team":         {map[string]interface{}{"team": " ", "season": float64(2025)}, "'team' is required"},
		"no season":          {map[string]interface{}{"team": "Colorado"}, "'season' is required"},
		"season string":      {map[string]interface{}{"team": "Colorado", "season": "2025"}, "'season' must be a whole number"},
		"season fraction":    {map[string]interface{}{"team": "Colorado", "season": 2025.5}, "'season' must be a whole number"},
		"week out of range":  {map[string]interface{}{"team": "Colorado", "season": float64(2025), "week_from": float64(40)}, "'week_from' must be between 0 and 25"},
		"weeks reversed":     {map[string]interface{}{"team": "Colorado", "season": float64(2025), "week_from": float64(9), "week_to": float64(5)}, "week_from (9) is after week_to (5)"},
		"bad metric":         {map[string]interface{}{"team": "Colorado", "season": float64(2025), "metrics": []interface{}{"yards"}}, "Unknown metric 'yards'"},
		"bad metric string":  {map[string]interface{}{"team": "Colorado", "season": float64(2025), "metrics": "epa_per_play, rushing"}, "Unknown metric 'rushing'"},
		"metric not string":  {map[string]interface{}{"team": "Colorado", "season": float64(2025), "metrics": []interface{}{1.0}}, "'metrics' must be a list"},
		"bad season type":    {map[string]interface{}{"team": "Colorado", "season": float64(2025), "season_type": "bowl"}, "'season_type' must be"},
		"compare not string": {map[string]interface{}{"team": "Colorado", "season": float64(2025), "compare_team": 5.0}, "'compare_team' must be a string"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			q := &stubQuerier{}
			srv := newServer("", testProject, stubFactory(q))
			result := callTool(t, srv.handleGetTeamAnalytics, tt.args)
			if !result.IsError || !strings.Contains(resultText(t, result), tt.want) {
				t.Errorf("result = %q (error %v), want error containing %q", resultText(t, result), result.IsError, tt.want)
			}
			if n := q.calls.Load(); n != 0 {
				t.Errorf("ran %d queries for invalid arguments, want 0", n)
			}
		})
	}
}

func TestGameSummaryArgumentValidation(t *testing.T) {
	tests := map[string]map[string]interface{}{
		"nothing":        {},
		"team only":      {"team": "Colorado"},
		"no week":        {"team": "Colorado", "season": float64(2025)},
		"no team":        {"season": float64(2025), "week": float64(5)},
		"bad game_id":    {"game_id": -4.0},
		"string game_id": {"game_id": "401"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			q := &stubQuerier{}
			srv := newServer("", testProject, stubFactory(q))
			result := callTool(t, srv.handleGetGameSummary, args)
			if !result.IsError {
				t.Errorf("got success %q, want an error", resultText(t, result))
			}
			if n := q.calls.Load(); n != 0 {
				t.Errorf("ran %d queries for invalid arguments, want 0", n)
			}
		})
	}
}

func TestTeamAnalyticsOverMCP(t *testing.T) {
	q := &stubQuerier{rows: []analytics.Row{
		{"team_id": "38", "season": int64(2025), "week": int64(5), "season_type": "regular", "games": int64(1),
			"off_epa_plays": int64(50), "off_epa_sum": 10.0, "off_plays": int64(50), "off_successes": int64(20)},
		{"team_id": "38", "season": int64(2025), "week": int64(6), "season_type": "regular", "games": int64(1),
			"off_epa_plays": int64(150), "off_epa_sum": 10.0, "off_plays": int64(150), "off_successes": int64(60)},
	}}
	srv := newServer("", testProject, stubFactory(q))

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_team_analytics",` +
		`"arguments":{"team":"Buffs","season":2025,"week_from":5,"metrics":["epa_per_play","points_per_drive"]}}}`
	resp := srv.mcpServer.HandleMessage(context.Background(), json.RawMessage(body))
	data, _ := json.Marshal(resp)
	var rpc struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &rpc); err != nil || rpc.Result.IsError || len(rpc.Result.Content) != 1 {
		t.Fatalf("response = %s", data)
	}
	var out analytics.TeamAnalyticsResult
	if err := json.Unmarshal([]byte(rpc.Result.Content[0].Text), &out); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	total := out.Teams[0].RangeTotal
	if out.Teams[0].Name != "Colorado" || *total.Metrics["epa_per_play"] != 0.1 || total.Metrics["points_per_drive"] != nil {
		t.Errorf("result = %s", rpc.Result.Content[0].Text)
	}
	if !strings.Contains(rpc.Result.Content[0].Text, `"points_per_drive":null`) {
		t.Errorf("zero-drive metric isn't null: %s", rpc.Result.Content[0].Text)
	}
}

func TestUnknownTeamMessage(t *testing.T) {
	srv := newServer("", testProject, stubFactory(&stubQuerier{}))
	result := callTool(t, srv.handleGetTeamAnalytics, map[string]interface{}{"team": "Nowhere Tech", "season": float64(2025)})
	if !result.IsError || !strings.HasPrefix(resultText(t, result), "Unknown team 'Nowhere Tech'") {
		t.Errorf("result = %q", resultText(t, result))
	}
}
