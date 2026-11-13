package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

const testProject = "sports-agent-511915"

// fakeDB answers the package's queries from in-memory rows, applying the same
// filters the SQL does, and records every query.
type fakeDB struct {
	mu      sync.Mutex
	teams   map[string]string // display name -> team_id
	weekly  []Row
	games   []Row
	err     error
	queries []Query
	maxRows []int
}

func (f *fakeDB) Query(ctx context.Context, q Query, maxRows int) ([]Row, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, q)
	f.maxRows = append(f.maxRows, maxRows)
	if f.err != nil {
		return nil, false, f.err
	}
	p := paramMap(q)
	var out []Row
	switch {
	case strings.Contains(q.SQL, "UNION ALL"):
		for _, name := range p["names"].([]string) {
			for display, id := range f.teams {
				if strings.ToLower(display) == name {
					out = append(out, Row{"team_id": id, "team_name": display, "games": int64(12)})
				}
			}
		}
	case strings.Contains(q.SQL, string(TableTeamEfficiencyWeekly)):
		ids := p["team_ids"].([]string)
		for _, r := range f.weekly {
			week := r["week"].(int64)
			inWindow := (r["season_type"] == "regular" && week >= p["regular_week_from"].(int64) && week <= p["regular_week_to"].(int64)) ||
				(r["season_type"] == "postseason" && week >= p["postseason_week_from"].(int64) && week <= p["postseason_week_to"].(int64))
			if r["season"] == p["season"] && contains(ids, r["team_id"].(string)) && inWindow {
				out = append(out, r)
			}
		}
	case strings.Contains(q.SQL, "game_id = @game_id"):
		for _, r := range f.games {
			if r["game_id"] == p["game_id"] {
				out = append(out, r)
			}
		}
	default:
		for _, r := range f.games {
			st := p["season_type"].(string)
			if r["season"] == p["season"] && r["week"] == p["week"] &&
				(r["home_team_id"] == p["team_id"] || r["away_team_id"] == p["team_id"]) &&
				(st == "" || r["season_type"] == st) {
				out = append(out, r)
			}
		}
	}
	return out, false, nil
}

func (f *fakeDB) count(substr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, q := range f.queries {
		if strings.Contains(q.SQL, substr) {
			n++
		}
	}
	return n
}

func (f *fakeDB) last() Query {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries[len(f.queries)-1]
}

func paramMap(q Query) map[string]any {
	m := make(map[string]any)
	for _, p := range q.Params {
		m[p.Name] = p.Value
	}
	return m
}

// weekRow builds a gold.team_efficiency_weekly row. Defensive sums mirror the
// offensive ones at half size.
func weekRow(team string, week int64, seasonType string, plays, epaPlays int64, epaSum float64, successes int64, successEPA float64, drives, points int64) Row {
	return Row{
		"team_id": team, "season": int64(2025), "week": week, "season_type": seasonType, "games": int64(1),
		"off_plays": plays, "off_epa_plays": epaPlays, "off_epa_sum": epaSum, "off_successes": successes,
		"off_success_epa_sum": successEPA, "off_drives": drives, "off_points": points,
		"def_plays": plays / 2, "def_epa_plays": epaPlays / 2, "def_epa_sum": epaSum / 2, "def_successes": successes / 2,
		"def_success_epa_sum": successEPA / 2, "def_drives": drives / 2, "def_points": points / 2,
	}
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newTestService(db *fakeDB) (*Service, *clock) {
	c := &clock{t: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)}
	return newService(testProject, db, c.now), c
}

func seasonDB() *fakeDB {
	db := &fakeDB{teams: map[string]string{"Colorado": "38", "Utah": "254", "Ole Miss": "145"}}
	for w := int64(1); w <= 12; w++ {
		db.weekly = append(db.weekly, weekRow("38", w, "regular", 60, 60, 6, 30, 30, 10, 28))
		db.weekly = append(db.weekly, weekRow("254", w, "regular", 70, 70, 14, 35, 35, 12, 35))
	}
	db.weekly = append(db.weekly, weekRow("38", 1, "postseason", 65, 65, -6.5, 25, 20, 11, 17))
	return db
}

func ptr(i int) *int { return &i }

func TestTableRefUsesAllowlistAndFourPartName(t *testing.T) {
	ref, err := tableRef(testProject, TableTeamEfficiencyWeekly)
	if err != nil {
		t.Fatal(err)
	}
	if want := "`sports-agent-511915.sa_catalog.gold.team_efficiency_weekly`"; ref != want {
		t.Errorf("tableRef = %s, want %s", ref, want)
	}
	if _, err := tableRef(testProject, Table("silver.teams")); err == nil {
		t.Error("silver table was allowed")
	}
	if _, err := tableRef("bad`project", TableGameSummaries); err == nil {
		t.Error("malformed project was allowed")
	}
	if _, err := NewService("x`; DROP", &fakeDB{}); err == nil {
		t.Error("NewService accepted a malformed project")
	}
}

func TestRangeMetricsComeFromSumsNotAverages(t *testing.T) {
	db := &fakeDB{teams: map[string]string{"Colorado": "38"}}
	// Week 1: 10 EPA plays, EPA 5.0 (0.5/play). Week 2: 90 EPA plays, EPA 9.0 (0.1/play).
	// Average of rates = 0.3; the right answer is 14/100 = 0.14.
	db.weekly = []Row{
		weekRow("38", 1, "regular", 10, 10, 5, 5, 4, 2, 14),
		weekRow("38", 2, "regular", 90, 90, 9, 30, 21, 10, 20),
	}
	svc, _ := newTestService(db)

	res, err := svc.TeamAnalytics(context.Background(), TeamAnalyticsRequest{Team: "Colorado", Season: 2025})
	if err != nil {
		t.Fatal(err)
	}
	total := res.Teams[0].RangeTotal
	checks := map[string]float64{
		"epa_per_play":         0.14,             // 14 / 100
		"success_rate":         0.35,             // 35 / 100
		"explosiveness":        0.7143,           // 25 / 35
		"points_per_drive":     2.8333,           // 34 / 12
		"def_epa_per_play":     0.14,             // 7 / 50
		"def_success_rate":     0.34,             // 17 / 50 (integer halves: 2 + 15)
		"def_explosiveness":    0.7353,           // 12.5 / 17
		"def_points_per_drive": round4(17.0 / 6), // (7 + 10) / (1 + 5)
	}
	for name, want := range checks {
		got := total.Metrics[name]
		if got == nil || *got != want {
			t.Errorf("range %s = %v, want %v", name, deref(got), want)
		}
	}
	if total.Weeks != 2 || total.Games != 2 {
		t.Errorf("range weeks/games = %d/%d, want 2/2", total.Weeks, total.Games)
	}
	if got := res.Teams[0].Weeks[0].Metrics["epa_per_play"]; got == nil || *got != 0.5 {
		t.Errorf("week 1 epa_per_play = %v, want 0.5", deref(got))
	}
}

func round4(f float64) float64 { return float64(int64(f*10000+0.5)) / 10000 }

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func TestZeroDenominatorsReturnNull(t *testing.T) {
	db := &fakeDB{teams: map[string]string{"Colorado": "38"}}
	row := weekRow("38", 3, "regular", 0, 0, 0, 0, 0, 0, 0)
	row["def_epa_sum"] = nil // NULL sums count as zero
	db.weekly = []Row{row}
	svc, _ := newTestService(db)

	res, err := svc.TeamAnalytics(context.Background(), TeamAnalyticsRequest{Team: "Colorado", Season: 2025})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range [][]map[string]*float64{{res.Teams[0].Weeks[0].Metrics, res.Teams[0].RangeTotal.Metrics}} {
		for _, metrics := range m {
			for _, name := range MetricNames() {
				if v, ok := metrics[name]; !ok || v != nil {
					t.Errorf("%s = %v (present %v), want null", name, deref(v), ok)
				}
			}
		}
	}
	data, _ := json.Marshal(res.Teams[0].RangeTotal)
	if !strings.Contains(string(data), `"epa_per_play":null`) {
		t.Errorf("JSON doesn't show null: %s", data)
	}
}

func TestWeekRangeFiltering(t *testing.T) {
	tests := map[string]struct {
		seasonType string
		from, to   *int
		want       []string // season_type:week
	}{
		"all weeks, both":         {SeasonTypeBoth, nil, nil, weeks("regular", 1, 12, "postseason:1")},
		"since week 5, both":      {SeasonTypeBoth, ptr(5), nil, weeks("regular", 5, 12, "postseason:1")},
		"weeks 5-8, both":         {SeasonTypeBoth, ptr(5), ptr(8), weeks("regular", 5, 8)},
		"through week 3, regular": {SeasonTypeRegular, nil, ptr(3), weeks("regular", 1, 3)},
		"regular only":            {SeasonTypeRegular, nil, nil, weeks("regular", 1, 12)},
		"postseason only":         {SeasonTypePostseason, nil, nil, []string{"postseason:1"}},
		"postseason week 2+":      {SeasonTypePostseason, ptr(2), nil, nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			svc, _ := newTestService(seasonDB())
			res, err := svc.TeamAnalytics(context.Background(), TeamAnalyticsRequest{
				Team: "Colorado", Season: 2025, SeasonType: tt.seasonType, WeekFrom: tt.from, WeekTo: tt.to,
			})
			if tt.want == nil {
				var ue *UserError
				if !errors.As(err, &ue) || !strings.Contains(err.Error(), "No efficiency data for Colorado") {
					t.Fatalf("err = %v, want a no-data UserError", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, w := range res.Teams[0].Weeks {
				got = append(got, fmt.Sprintf("%s:%d", w.SeasonType, w.Week))
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("weeks = %v, want %v", got, tt.want)
			}
			if res.Teams[0].RangeTotal.Weeks != len(tt.want) {
				t.Errorf("range_total.weeks = %d, want %d", res.Teams[0].RangeTotal.Weeks, len(tt.want))
			}
		})
	}
}

func weeks(seasonType string, from, to int, extra ...string) []string {
	var out []string
	for w := from; w <= to; w++ {
		out = append(out, fmt.Sprintf("%s:%d", seasonType, w))
	}
	return append(out, extra...)
}

func TestWeekRangeIsParameterized(t *testing.T) {
	db := seasonDB()
	svc, _ := newTestService(db)
	if _, err := svc.TeamAnalytics(context.Background(), TeamAnalyticsRequest{Team: "Colorado", Season: 2025, WeekFrom: ptr(5)}); err != nil {
		t.Fatal(err)
	}
	q := db.last()
	p := paramMap(q)
	want := map[string]any{
		"season": int64(2025), "regular_week_from": int64(5), "regular_week_to": int64(MaxWeek),
		"postseason_week_from": int64(MinWeek), "postseason_week_to": int64(MaxWeek),
	}
	for k, v := range want {
		if p[k] != v {
			t.Errorf("param %s = %v, want %v", k, p[k], v)
		}
	}
	if strings.Contains(q.SQL, "2025") || strings.Contains(q.SQL, "'38'") {
		t.Errorf("SQL contains a literal argument:\n%s", q.SQL)
	}
	if !strings.Contains(q.SQL, "`sports-agent-511915.sa_catalog.gold.team_efficiency_weekly`") {
		t.Errorf("SQL doesn't read the gold table:\n%s", q.SQL)
	}
}

func TestCompareTeam(t *testing.T) {
	db := seasonDB()
	svc, _ := newTestService(db)
	res, err := svc.TeamAnalytics(context.Background(), TeamAnalyticsRequest{
		Team: "utes", CompareTeam: "Buffs", Season: 2025, WeekFrom: ptr(1), WeekTo: ptr(2), Metrics: []string{"epa_per_play"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Teams) != 2 || res.Teams[0].Name != "Utah" || res.Teams[1].Name != "Colorado" {
		t.Fatalf("teams = %+v, want Utah then Colorado", res.Teams)
	}
	if got := *res.Teams[0].RangeTotal.Metrics["epa_per_play"]; got != 0.2 {
		t.Errorf("Utah epa_per_play = %v, want 0.2", got)
	}
	if got := *res.Teams[1].RangeTotal.Metrics["epa_per_play"]; got != 0.1 {
		t.Errorf("Colorado epa_per_play = %v, want 0.1", got)
	}
	if len(res.Teams[0].RangeTotal.Metrics) != 1 {
		t.Errorf("metrics = %v, want only epa_per_play", res.Teams[0].RangeTotal.Metrics)
	}
	ids := paramMap(db.last())["team_ids"].([]string)
	if strings.Join(ids, ",") != "254,38" {
		t.Errorf("team_ids = %v, want one query for both, sorted", ids)
	}
	if n := db.count(string(TableTeamEfficiencyWeekly)); n != 1 {
		t.Errorf("ran %d efficiency queries, want 1", n)
	}

	if _, err := svc.TeamAnalytics(context.Background(), TeamAnalyticsRequest{Team: "Colorado", CompareTeam: "buffs", Season: 2025}); err == nil ||
		!strings.Contains(err.Error(), "same team") {
		t.Errorf("same team err = %v", err)
	}
	if _, err := svc.TeamAnalytics(context.Background(), TeamAnalyticsRequest{Team: "Colorado", CompareTeam: "Nowhere Tech", Season: 2025}); err == nil ||
		err.Error() != "Unknown team 'Nowhere Tech'. Use the school's name, for example 'Colorado' or 'Ohio State'." {
		t.Errorf("unknown compare err = %v", err)
	}
}

func TestCompareTeamWithoutDataGetsNote(t *testing.T) {
	db := seasonDB()
	svc, _ := newTestService(db)
	res, err := svc.TeamAnalytics(context.Background(), TeamAnalyticsRequest{Team: "Colorado", CompareTeam: "Ole Miss", Season: 2025})
	if err != nil {
		t.Fatal(err)
	}
	if res.Teams[1].Note == "" || res.Teams[1].RangeTotal != nil || len(res.Teams[1].Weeks) != 0 {
		t.Errorf("team without rows = %+v, want empty weeks and a note", res.Teams[1])
	}
}

func TestResolveTeamAndCache(t *testing.T) {
	db := seasonDB()
	svc, clk := newTestService(db)
	ctx := context.Background()

	team, err := svc.ResolveTeam(ctx, "Buffs")
	if err != nil || team != (Team{ID: "38", Name: "Colorado"}) {
		t.Fatalf("ResolveTeam(Buffs) = %+v, %v", team, err)
	}
	if got := paramMap(db.last())["names"].([]string); strings.Join(got, ",") != "colorado,buffs" {
		t.Errorf("lookup names = %v, want resolver name then raw input", got)
	}

	// "colorado" was cached by the first lookup.
	if _, err := svc.ResolveTeam(ctx, "COLORADO"); err != nil {
		t.Fatal(err)
	}
	if n := db.count("UNION ALL"); n != 1 {
		t.Errorf("ran %d team lookups, want 1 (cached)", n)
	}

	clk.advance(TeamTTL + time.Second)
	if _, err := svc.ResolveTeam(ctx, "colorado"); err != nil {
		t.Fatal(err)
	}
	if n := db.count("UNION ALL"); n != 2 {
		t.Errorf("ran %d team lookups after 24 h, want 2", n)
	}

	// The resolver turns "ole miss" into "Mississippi"; the raw input still matches gold's "Ole Miss".
	if team, err := svc.ResolveTeam(ctx, "Ole Miss"); err != nil || team.ID != "145" {
		t.Errorf("ResolveTeam(Ole Miss) = %+v, %v", team, err)
	}

	_, err = svc.ResolveTeam(ctx, "Nowhere Tech")
	var ue *UserError
	if !errors.As(err, &ue) || !strings.HasPrefix(err.Error(), "Unknown team 'Nowhere Tech'") {
		t.Errorf("unknown team err = %v", err)
	}
	before := db.count("UNION ALL")
	svc.ResolveTeam(ctx, "nowhere tech")
	if db.count("UNION ALL") != before {
		t.Error("unknown team was not cached")
	}
	clk.advance(unknownTeamTTL + time.Second)
	svc.ResolveTeam(ctx, "nowhere tech")
	if db.count("UNION ALL") != before+1 {
		t.Error("unknown team stayed cached past its TTL")
	}

	if _, err := svc.ResolveTeam(ctx, "  "); err == nil {
		t.Error("blank team accepted")
	}
}

func TestResultCache(t *testing.T) {
	db := seasonDB()
	svc, clk := newTestService(db)
	ctx := context.Background()
	req := TeamAnalyticsRequest{Team: "Colorado", CompareTeam: "Utah", Season: 2025, WeekFrom: ptr(5)}

	if _, err := svc.TeamAnalytics(ctx, req); err != nil {
		t.Fatal(err)
	}
	// Same arguments written differently: alias names, swapped teams, other metrics.
	same := TeamAnalyticsRequest{Team: "utes", CompareTeam: "buffs", Season: 2025, WeekFrom: ptr(5), Metrics: []string{"success_rate"}}
	if _, err := svc.TeamAnalytics(ctx, same); err != nil {
		t.Fatal(err)
	}
	if n := db.count(string(TableTeamEfficiencyWeekly)); n != 1 {
		t.Errorf("ran %d efficiency queries, want 1 (cached)", n)
	}

	other := req
	other.WeekFrom = ptr(6)
	svc.TeamAnalytics(ctx, other)
	if n := db.count(string(TableTeamEfficiencyWeekly)); n != 2 {
		t.Errorf("ran %d efficiency queries, want 2 (different weeks)", n)
	}

	clk.advance(ResultTTL + time.Second)
	svc.TeamAnalytics(ctx, req)
	if n := db.count(string(TableTeamEfficiencyWeekly)); n != 3 {
		t.Errorf("ran %d efficiency queries, want 3 (expired)", n)
	}
}

func TestErrorsAreNotCached(t *testing.T) {
	db := seasonDB()
	svc, _ := newTestService(db)
	ctx := context.Background()
	svc.ResolveTeam(ctx, "Colorado") // cache the team

	db.err = errors.New("boom")
	if _, err := svc.TeamAnalytics(ctx, TeamAnalyticsRequest{Team: "Colorado", Season: 2025}); err == nil {
		t.Fatal("want an error")
	}
	db.err = nil
	if _, err := svc.TeamAnalytics(ctx, TeamAnalyticsRequest{Team: "Colorado", Season: 2025}); err != nil {
		t.Fatalf("error was cached: %v", err)
	}
}

func TestRowCap(t *testing.T) {
	db := &fakeDB{teams: map[string]string{"Colorado": "38"}}
	for i := 0; i < 250; i++ {
		db.weekly = append(db.weekly, weekRow("38", 1, "regular", 1, 1, 1, 1, 1, 1, 1))
	}
	svc, _ := newTestService(db)
	res, err := svc.TeamAnalytics(context.Background(), TeamAnalyticsRequest{Team: "Colorado", Season: 2025})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Teams[0].Weeks) != RowCap || !res.Truncated {
		t.Errorf("got %d rows, truncated=%v; want %d and true", len(res.Teams[0].Weeks), res.Truncated, RowCap)
	}
	for i, max := range db.maxRows {
		if max != RowCap {
			t.Errorf("query %d maxRows = %d, want %d", i, max, RowCap)
		}
		if !strings.Contains(db.queries[i].SQL, "LIMIT 201") {
			t.Errorf("query %d has no LIMIT 201:\n%s", i, db.queries[i].SQL)
		}
	}
}

func TestQueryErrorsAreShortAndSafe(t *testing.T) {
	tests := map[string]struct {
		err  error
		want string
	}{
		"permission": {errors.New("googleapi: Error 403: Access Denied: Table sports-agent-511915:sa_catalog.silver.teams"), "BigQuery) is unavailable"},
		"timeout":    {fmt.Errorf("read: %w", context.DeadlineExceeded), "didn't answer within 15 seconds"},
		"bytes":      {errors.New("googleapi: Error 400: Query exceeded limit for bytes billed: 100000000., bytesBilledLimitExceeded"), "100 MB limit"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			db := seasonDB()
			db.err = tt.err
			svc, _ := newTestService(db)
			_, err := svc.TeamAnalytics(context.Background(), TeamAnalyticsRequest{Team: "Colorado", Season: 2025})
			var ue *UserError
			if !errors.As(err, &ue) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want a UserError containing %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "googleapi") || strings.Contains(err.Error(), "silver") {
				t.Errorf("raw BigQuery text leaked: %q", err)
			}
		})
	}
}

func gameRow(id, season, week int64, seasonType string, home, homeName, away, awayName string, hp, ap int64) Row {
	winner := any(nil)
	if hp > ap {
		winner = home
	} else if ap > hp {
		winner = away
	}
	return Row{
		"game_id": id, "season": season, "week": week, "season_type": seasonType,
		"start_time_utc": time.Date(2025, 9, 6, 19, 0, 0, 0, time.UTC), "status": "final",
		"home_team_id": home, "home_team_name": homeName, "away_team_id": away, "away_team_name": awayName,
		"home_points": hp, "away_points": ap, "winner_team_id": winner,
		"home_plays": int64(70), "away_plays": int64(65), "home_yards": int64(420), "away_yards": int64(350),
		"home_epa_per_play": 0.123456, "away_epa_per_play": nil, "home_success_rate": 0.45, "away_success_rate": 0.4,
		"home_turnovers": int64(1), "away_turnovers": int64(2),
	}
}

func TestGameSummary(t *testing.T) {
	db := seasonDB()
	db.games = []Row{
		gameRow(401, 2025, 2, "regular", "38", "Colorado", "254", "Utah", 31, 24),
		gameRow(402, 2025, 1, "postseason", "145", "Ole Miss", "38", "Colorado", 20, 27),
	}
	svc, _ := newTestService(db)
	ctx := context.Background()

	res, err := svc.GameSummary(ctx, GameSummaryRequest{GameID: 401})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Games[0]
	if g.Winner != "Colorado" || *g.Home.Points != 31 || *g.Home.EPAPerPlay != 0.1235 || g.Away.EPAPerPlay != nil {
		t.Errorf("game = %+v", g)
	}

	res, err = svc.GameSummary(ctx, GameSummaryRequest{Team: "buffs", Season: 2025, Week: 1, SeasonType: SeasonTypeBoth})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Games) != 1 || res.Games[0].GameID != 402 || res.Games[0].Winner != "Colorado" {
		t.Errorf("week 1 games = %+v", res.Games)
	}
	if p := paramMap(db.last()); p["season_type"] != "" || p["team_id"] != "38" {
		t.Errorf("params = %v", p)
	}

	_, err = svc.GameSummary(ctx, GameSummaryRequest{Team: "Colorado", Season: 2025, Week: 1, SeasonType: SeasonTypeRegular})
	if err == nil || !strings.HasPrefix(err.Error(), "No game for Colorado in 2025 week 1") {
		t.Errorf("no-game err = %v", err)
	}
	if _, err := svc.GameSummary(ctx, GameSummaryRequest{GameID: 999}); err == nil || !strings.Contains(err.Error(), "No game with game_id 999") {
		t.Errorf("unknown game err = %v", err)
	}
}

func TestNormalizeMetrics(t *testing.T) {
	got, err := NormalizeMetrics([]string{" def_epa_per_play", "EPA_PER_PLAY", "epa_per_play", ""})
	if err != nil || strings.Join(got, ",") != "epa_per_play,def_epa_per_play" {
		t.Errorf("NormalizeMetrics = %v, %v", got, err)
	}
	if got, _ := NormalizeMetrics(nil); len(got) != 8 {
		t.Errorf("default metrics = %v, want all 8", got)
	}
	if _, err := NormalizeMetrics([]string{"yards"}); err == nil || !strings.Contains(err.Error(), "Unknown metric 'yards'") {
		t.Errorf("bad metric err = %v", err)
	}
}

func TestTTLCacheStaysBounded(t *testing.T) {
	c := &clock{t: time.Now()}
	cache := newTTLCache[int](3, c.now)
	for i := 0; i < 10; i++ {
		cache.set(fmt.Sprint(i), i, time.Minute)
	}
	if cache.len() != 3 {
		t.Errorf("len = %d, want 3", cache.len())
	}
	if v, ok := cache.get("9"); !ok || v != 9 {
		t.Errorf("newest entry missing")
	}
}
