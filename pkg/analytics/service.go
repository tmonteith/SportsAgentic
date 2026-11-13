package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tmonteith/SportsAgentic/pkg/resolver"
)

const (
	// CallTimeout bounds one tool call, team lookup included.
	CallTimeout = 15 * time.Second
	// ResultTTL is how long a query result is reused for the same arguments.
	ResultTTL = 10 * time.Minute
	// TeamTTL is how long a resolved team name → team_id stays cached.
	TeamTTL = 24 * time.Hour
	// unknownTeamTTL is how long a name that matched no team stays cached, so a
	// team added by the next nightly build is found without a restart.
	unknownTeamTTL = 10 * time.Minute

	// MinWeek and MaxWeek bound week arguments. Week 0 exists in the source.
	MinWeek = 0
	MaxWeek = 25
	// MinSeason and MaxSeason bound the season argument.
	MinSeason = 1900
	MaxSeason = 2100

	resultCacheEntries = 256
	teamCacheEntries   = 1024
)

// Season types stored in gold.
const (
	SeasonTypeRegular    = "regular"
	SeasonTypePostseason = "postseason"
	// SeasonTypeBoth is the tools' default: regular and postseason.
	SeasonTypeBoth = "both"
)

// UserError carries a short message that is safe to show the model.
type UserError struct{ msg string }

func (e *UserError) Error() string { return e.msg }

func userErrorf(format string, args ...any) error {
	return &UserError{msg: fmt.Sprintf(format, args...)}
}

// Team is a resolved team.
type Team struct {
	ID   string `json:"team_id"`
	Name string `json:"team"`
}

// Service answers the analytics tools from the gold tables.
type Service struct {
	project string
	querier Querier
	now     func() time.Time
	results *ttlCache[queryResult]
	teams   *ttlCache[teamLookup]
}

type queryResult struct {
	rows      []Row
	truncated bool
}

type teamLookup struct {
	team  Team
	found bool
}

// NewService returns a Service that reads gold tables in project through q.
func NewService(project string, q Querier) (*Service, error) {
	if !ValidProject(project) {
		return nil, fmt.Errorf("GCP_PROJECT %q is not a valid project ID", project)
	}
	return newService(project, q, time.Now), nil
}

func newService(project string, q Querier, now func() time.Time) *Service {
	return &Service{
		project: project,
		querier: q,
		now:     now,
		results: newTTLCache[queryResult](resultCacheEntries, now),
		teams:   newTTLCache[teamLookup](teamCacheEntries, now),
	}
}

// sqlText fills {table} with the quoted allowlisted table and {limit} with
// RowCap+1, so a capped result can be detected.
func (s *Service) sqlText(t Table, template string) (string, error) {
	ref, err := tableRef(s.project, t)
	if err != nil {
		return "", err
	}
	return strings.NewReplacer("{table}", ref, "{limit}", strconv.Itoa(RowCap+1)).Replace(template), nil
}

// run executes q, capped at RowCap rows. When cache is true, the result is
// reused for ResultTTL for the same statement and parameters.
func (s *Service) run(ctx context.Context, q Query, cache bool) (queryResult, error) {
	key := cacheKey(q)
	if cache {
		if r, ok := s.results.get(key); ok {
			return r, nil
		}
	}
	rows, truncated, err := s.querier.Query(ctx, q, RowCap)
	if err != nil {
		return queryResult{}, queryError(err)
	}
	if len(rows) > RowCap {
		rows, truncated = rows[:RowCap], true
	}
	r := queryResult{rows: rows, truncated: truncated}
	if cache {
		s.results.set(key, r, ResultTTL)
	}
	return r, nil
}

func cacheKey(q Query) string {
	params, _ := json.Marshal(q.Params)
	return q.SQL + "\x00" + string(params)
}

// queryError logs the BigQuery error and returns a short message for the model.
// Raw BigQuery text never reaches the model.
func queryError(err error) error {
	log.Printf("analytics: BigQuery query failed: %v", err)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return userErrorf("The analytics database didn't answer within %d seconds. Try again later; the live tools still work.", int(CallTimeout.Seconds()))
	case errors.Is(err, context.Canceled):
		return userErrorf("The analytics query was cancelled.")
	case strings.Contains(err.Error(), "bytesBilledLimitExceeded"):
		return userErrorf("The analytics query would scan more than the 100 MB limit, so it was not run.")
	default:
		return userErrorf("The analytics database (BigQuery) is unavailable right now. Try again later; the live tools still work.")
	}
}

// --- Team resolution -------------------------------------------------------

const teamLookupSQL = `SELECT team_id, team_name, COUNT(*) AS games
FROM (
  SELECT home_team_id AS team_id, home_team_name AS team_name FROM {table}
  UNION ALL
  SELECT away_team_id AS team_id, away_team_name AS team_name FROM {table}
)
WHERE team_id IS NOT NULL AND LOWER(team_name) IN UNNEST(@names)
GROUP BY team_id, team_name
ORDER BY games DESC, team_id
LIMIT {limit}`

// ResolveTeam turns a user's team name into a gold team_id. It tries the
// resolver's canonical name first and then the input as typed, each matched
// case-insensitively and exactly against gold.game_summaries team names.
func (s *Service) ResolveTeam(ctx context.Context, input string) (Team, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Team{}, userErrorf("team is required.")
	}
	candidates := []string{strings.ToLower(resolver.ResolveTeamName(input))}
	if raw := strings.ToLower(input); raw != candidates[0] {
		candidates = append(candidates, raw)
	}

	var missing []string
	for _, name := range candidates {
		if _, ok := s.teams.get(name); !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		if err := s.lookupTeams(ctx, missing); err != nil {
			return Team{}, err
		}
	}
	for _, name := range candidates {
		if t, ok := s.teams.get(name); ok && t.found {
			return t.team, nil
		}
	}
	return Team{}, userErrorf("Unknown team '%s'. Use the school's name, for example 'Colorado' or 'Ohio State'.", input)
}

func (s *Service) lookupTeams(ctx context.Context, names []string) error {
	sql, err := s.sqlText(TableGameSummaries, teamLookupSQL)
	if err != nil {
		return err
	}
	r, err := s.run(ctx, Query{SQL: sql, Params: []Param{{Name: "names", Value: names}}}, false)
	if err != nil {
		return err
	}
	found := make(map[string]Team)
	for _, row := range r.rows {
		name := str(row["team_name"])
		key := strings.ToLower(name)
		if _, seen := found[key]; !seen {
			found[key] = Team{ID: str(row["team_id"]), Name: name}
		}
	}
	for _, name := range names {
		if t, ok := found[name]; ok {
			s.teams.set(name, teamLookup{team: t, found: true}, TeamTTL)
		} else {
			s.teams.set(name, teamLookup{}, unknownTeamTTL)
		}
	}
	return nil
}

// --- Team efficiency -------------------------------------------------------

type metricDef struct {
	name, num, den string
}

// metricDefs lists the metrics in canonical order. Each one is a ratio of
// stored sums, so a week range is Σnum / Σden, never an average of weekly rates.
var metricDefs = []metricDef{
	{"epa_per_play", "off_epa_sum", "off_epa_plays"},
	{"success_rate", "off_successes", "off_plays"},
	{"explosiveness", "off_success_epa_sum", "off_successes"},
	{"points_per_drive", "off_points", "off_drives"},
	{"def_epa_per_play", "def_epa_sum", "def_epa_plays"},
	{"def_success_rate", "def_successes", "def_plays"},
	{"def_explosiveness", "def_success_epa_sum", "def_successes"},
	{"def_points_per_drive", "def_points", "def_drives"},
}

// sumColumns are the stored sums the metrics need.
var sumColumns = []string{
	"off_plays", "off_epa_plays", "off_epa_sum", "off_successes", "off_success_epa_sum", "off_drives", "off_points",
	"def_plays", "def_epa_plays", "def_epa_sum", "def_successes", "def_success_epa_sum", "def_drives", "def_points",
}

// MetricNames returns every metric name in canonical order.
func MetricNames() []string {
	names := make([]string, len(metricDefs))
	for i, m := range metricDefs {
		names[i] = m.name
	}
	return names
}

// NormalizeMetrics validates names and returns them deduplicated in canonical
// order. An empty list means every metric.
func NormalizeMetrics(names []string) ([]string, error) {
	if len(names) == 0 {
		return MetricNames(), nil
	}
	want := make(map[string]bool)
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" {
			continue
		}
		if !isMetric(n) {
			return nil, userErrorf("Unknown metric '%s'. Valid metrics: %s.", n, strings.Join(MetricNames(), ", "))
		}
		want[n] = true
	}
	if len(want) == 0 {
		return MetricNames(), nil
	}
	var out []string
	for _, m := range metricDefs {
		if want[m.name] {
			out = append(out, m.name)
		}
	}
	return out, nil
}

func isMetric(name string) bool {
	for _, m := range metricDefs {
		if m.name == name {
			return true
		}
	}
	return false
}

const teamEfficiencySQL = `SELECT team_id, season, week, season_type, games,
  off_plays, off_epa_plays, off_epa_sum, off_successes, off_success_epa_sum, off_drives, off_points,
  def_plays, def_epa_plays, def_epa_sum, def_successes, def_success_epa_sum, def_drives, def_points
FROM {table}
WHERE season = @season
  AND team_id IN UNNEST(@team_ids)
  AND ((season_type = 'regular' AND week BETWEEN @regular_week_from AND @regular_week_to)
    OR (season_type = 'postseason' AND week BETWEEN @postseason_week_from AND @postseason_week_to))
ORDER BY team_id, IF(season_type = 'regular', 0, 1), week
LIMIT {limit}`

// TeamAnalyticsRequest is a validated get_team_analytics call.
type TeamAnalyticsRequest struct {
	Team        string
	CompareTeam string
	Season      int
	WeekFrom    *int
	WeekTo      *int
	SeasonType  string // regular, postseason or both
	Metrics     []string
}

// TeamAnalyticsResult is the tool's JSON answer.
type TeamAnalyticsResult struct {
	Source     string       `json:"source"`
	Season     int          `json:"season"`
	SeasonType string       `json:"season_type"`
	WeekFrom   *int         `json:"week_from"`
	WeekTo     *int         `json:"week_to"`
	WeekRule   string       `json:"week_rule"`
	Metrics    []string     `json:"metrics"`
	Teams      []TeamSeries `json:"teams"`
	Truncated  bool         `json:"truncated,omitempty"`
}

// TeamSeries is one team's weekly rows and range total.
type TeamSeries struct {
	Team
	Weeks      []WeekMetrics `json:"weeks"`
	RangeTotal *RangeTotal   `json:"range_total"`
	Note       string        `json:"note,omitempty"`
}

// WeekMetrics is one team-week. A metric is null when its denominator is zero.
type WeekMetrics struct {
	Week       int64               `json:"week"`
	SeasonType string              `json:"season_type"`
	Games      int64               `json:"games"`
	Metrics    map[string]*float64 `json:"metrics"`
}

// RangeTotal is the whole selected range, computed from summed numerators and
// denominators.
type RangeTotal struct {
	Weeks   int                 `json:"weeks"`
	Games   int64               `json:"games"`
	Metrics map[string]*float64 `json:"metrics"`
}

// weekWindow is the week range applied to each season type. An empty window
// is (1, 0), which BETWEEN never matches.
type weekWindow struct {
	regularFrom, regularTo, postFrom, postTo int
}

// planWeeks applies week_from/week_to. Postseason weeks are numbered
// separately (usually week 1), so with both season types the range applies to
// regular-season weeks, and the postseason is included only when the range is
// open-ended (no week_to).
func planWeeks(seasonType string, from, to *int) (weekWindow, string) {
	f, t := MinWeek, MaxWeek
	if from != nil {
		f = *from
	}
	if to != nil {
		t = *to
	}
	switch seasonType {
	case SeasonTypeRegular:
		return weekWindow{f, t, 1, 0}, "Regular season only; week_from/week_to filter regular-season weeks."
	case SeasonTypePostseason:
		return weekWindow{1, 0, f, t}, "Postseason only; week_from/week_to filter postseason weeks (postseason games are usually week 1)."
	default:
		if to == nil {
			return weekWindow{f, t, MinWeek, MaxWeek}, "week_from/week_to filter regular-season weeks; the postseason is included because week_to is not set."
		}
		return weekWindow{f, t, 1, 0}, "week_from/week_to filter regular-season weeks; the postseason is excluded because week_to is set (use season_type='postseason' for it)."
	}
}

// TeamAnalytics answers get_team_analytics.
func (s *Service) TeamAnalytics(ctx context.Context, req TeamAnalyticsRequest) (*TeamAnalyticsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()

	metrics, err := NormalizeMetrics(req.Metrics)
	if err != nil {
		return nil, err
	}
	seasonType := req.SeasonType
	if seasonType == "" {
		seasonType = SeasonTypeBoth
	}

	team, err := s.ResolveTeam(ctx, req.Team)
	if err != nil {
		return nil, err
	}
	teams := []Team{team}
	if strings.TrimSpace(req.CompareTeam) != "" {
		other, err := s.ResolveTeam(ctx, req.CompareTeam)
		if err != nil {
			return nil, err
		}
		if other.ID == team.ID {
			return nil, userErrorf("compare_team '%s' is the same team as team '%s'.", req.CompareTeam, req.Team)
		}
		teams = append(teams, other)
	}

	ids := make([]string, len(teams))
	for i, t := range teams {
		ids[i] = t.ID
	}
	sort.Strings(ids) // normalized for the cache key

	window, rule := planWeeks(seasonType, req.WeekFrom, req.WeekTo)
	sql, err := s.sqlText(TableTeamEfficiencyWeekly, teamEfficiencySQL)
	if err != nil {
		return nil, err
	}
	r, err := s.run(ctx, Query{SQL: sql, Params: []Param{
		{Name: "season", Value: int64(req.Season)},
		{Name: "team_ids", Value: ids},
		{Name: "regular_week_from", Value: int64(window.regularFrom)},
		{Name: "regular_week_to", Value: int64(window.regularTo)},
		{Name: "postseason_week_from", Value: int64(window.postFrom)},
		{Name: "postseason_week_to", Value: int64(window.postTo)},
	}}, true)
	if err != nil {
		return nil, err
	}
	if len(r.rows) == 0 {
		return nil, userErrorf("No efficiency data for %s in %d %s. Only completed games are included; use get_cfb_scores for games in progress.",
			teamNames(teams), req.Season, describeWeeks(seasonType, req.WeekFrom, req.WeekTo))
	}

	byTeam := make(map[string][]Row)
	for _, row := range r.rows {
		id := str(row["team_id"])
		byTeam[id] = append(byTeam[id], row)
	}

	result := &TeamAnalyticsResult{
		Source:     "gold." + string(TableTeamEfficiencyWeekly),
		Season:     req.Season,
		SeasonType: seasonType,
		WeekFrom:   req.WeekFrom,
		WeekTo:     req.WeekTo,
		WeekRule:   rule,
		Metrics:    metrics,
		Truncated:  r.truncated,
	}
	for _, t := range teams {
		result.Teams = append(result.Teams, buildSeries(t, byTeam[t.ID], metrics))
	}
	return result, nil
}

func buildSeries(t Team, rows []Row, metrics []string) TeamSeries {
	series := TeamSeries{Team: t, Weeks: []WeekMetrics{}}
	if len(rows) == 0 {
		series.Note = "No data for this team in the selected range."
		return series
	}
	total := make(map[string]float64)
	var games int64
	for _, row := range rows {
		sums := make(map[string]float64)
		for _, col := range sumColumns {
			v := num(row[col])
			sums[col] = v
			total[col] += v
		}
		g, _ := toInt64(row["games"])
		games += g
		week, _ := toInt64(row["week"])
		series.Weeks = append(series.Weeks, WeekMetrics{
			Week:       week,
			SeasonType: str(row["season_type"]),
			Games:      g,
			Metrics:    computeMetrics(sums, metrics),
		})
	}
	series.RangeTotal = &RangeTotal{Weeks: len(rows), Games: games, Metrics: computeMetrics(total, metrics)}
	return series
}

// computeMetrics divides the summed numerator by the summed denominator for
// each metric, returning nil when the denominator is zero.
func computeMetrics(sums map[string]float64, metrics []string) map[string]*float64 {
	out := make(map[string]*float64, len(metrics))
	for _, m := range metricDefs {
		if !contains(metrics, m.name) {
			continue
		}
		out[m.name] = ratio(sums[m.num], sums[m.den])
	}
	return out
}

func ratio(num, den float64) *float64 {
	if den == 0 {
		return nil
	}
	v := math.Round(num/den*10000) / 10000
	return &v
}

func describeWeeks(seasonType string, from, to *int) string {
	label := map[string]string{
		SeasonTypeRegular:    "regular season",
		SeasonTypePostseason: "postseason",
		SeasonTypeBoth:       "regular season and postseason",
	}[seasonType]
	switch {
	case from != nil && to != nil:
		return fmt.Sprintf("%s weeks %d-%d", label, *from, *to)
	case from != nil:
		return fmt.Sprintf("%s from week %d", label, *from)
	case to != nil:
		return fmt.Sprintf("%s through week %d", label, *to)
	default:
		return label
	}
}

func teamNames(teams []Team) string {
	names := make([]string, len(teams))
	for i, t := range teams {
		names[i] = t.Name
	}
	return strings.Join(names, " or ")
}

// --- Game summaries --------------------------------------------------------

const gameColumns = `game_id, season, week, season_type, start_time_utc, status,
  home_team_id, home_team_name, away_team_id, away_team_name, home_points, away_points, winner_team_id,
  home_plays, away_plays, home_yards, away_yards, home_epa_per_play, away_epa_per_play,
  home_success_rate, away_success_rate, home_turnovers, away_turnovers`

const gameByIDSQL = `SELECT ` + gameColumns + `
FROM {table}
WHERE game_id = @game_id
LIMIT {limit}`

const gameByTeamWeekSQL = `SELECT ` + gameColumns + `
FROM {table}
WHERE season = @season
  AND week = @week
  AND (home_team_id = @team_id OR away_team_id = @team_id)
  AND (@season_type = '' OR season_type = @season_type)
ORDER BY start_time_utc, game_id
LIMIT {limit}`

// GameSummaryRequest is a validated get_game_summary call: GameID, or
// Team + Season + Week.
type GameSummaryRequest struct {
	GameID     int64
	Team       string
	Season     int
	Week       int
	SeasonType string // regular, postseason or both
}

// GameSummaryResult is the tool's JSON answer.
type GameSummaryResult struct {
	Source    string        `json:"source"`
	Games     []GameSummary `json:"games"`
	Truncated bool          `json:"truncated,omitempty"`
}

// GameSummary is one game from gold.game_summaries.
type GameSummary struct {
	GameID       int64      `json:"game_id"`
	Season       int64      `json:"season"`
	Week         *int64     `json:"week"`
	SeasonType   string     `json:"season_type,omitempty"`
	StartTimeUTC *time.Time `json:"start_time_utc"`
	Status       string     `json:"status,omitempty"`
	Home         GameSide   `json:"home"`
	Away         GameSide   `json:"away"`
	Winner       string     `json:"winner,omitempty"`
}

// GameSide is one team's line in a game.
type GameSide struct {
	TeamID      string   `json:"team_id"`
	Team        string   `json:"team"`
	Points      *int64   `json:"points"`
	Plays       *int64   `json:"plays"`
	Yards       *int64   `json:"yards"`
	EPAPerPlay  *float64 `json:"epa_per_play"`
	SuccessRate *float64 `json:"success_rate"`
	Turnovers   *int64   `json:"turnovers"`
}

// GameSummary answers get_game_summary.
func (s *Service) GameSummary(ctx context.Context, req GameSummaryRequest) (*GameSummaryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()

	var q Query
	var notFound string
	if req.GameID > 0 {
		sql, err := s.sqlText(TableGameSummaries, gameByIDSQL)
		if err != nil {
			return nil, err
		}
		q = Query{SQL: sql, Params: []Param{{Name: "game_id", Value: req.GameID}}}
		notFound = fmt.Sprintf("No game with game_id %d in the lakehouse. It may not be loaded yet; use get_cfb_scores for current games.", req.GameID)
	} else {
		team, err := s.ResolveTeam(ctx, req.Team)
		if err != nil {
			return nil, err
		}
		seasonType := req.SeasonType
		if seasonType == SeasonTypeBoth {
			seasonType = ""
		}
		sql, err := s.sqlText(TableGameSummaries, gameByTeamWeekSQL)
		if err != nil {
			return nil, err
		}
		q = Query{SQL: sql, Params: []Param{
			{Name: "season", Value: int64(req.Season)},
			{Name: "week", Value: int64(req.Week)},
			{Name: "team_id", Value: team.ID},
			{Name: "season_type", Value: seasonType},
		}}
		notFound = fmt.Sprintf("No game for %s in %d week %d. Check the week (postseason games are usually week 1 with season_type 'postseason'), or use get_team_schedule.", team.Name, req.Season, req.Week)
	}

	r, err := s.run(ctx, q, true)
	if err != nil {
		return nil, err
	}
	if len(r.rows) == 0 {
		return nil, userErrorf("%s", notFound)
	}
	result := &GameSummaryResult{Source: "gold." + string(TableGameSummaries), Games: []GameSummary{}, Truncated: r.truncated}
	for _, row := range r.rows {
		result.Games = append(result.Games, toGameSummary(row))
	}
	return result, nil
}

func toGameSummary(row Row) GameSummary {
	g := GameSummary{
		Week:         intPtr(row["week"]),
		SeasonType:   str(row["season_type"]),
		StartTimeUTC: timePtr(row["start_time_utc"]),
		Status:       str(row["status"]),
		Home:         gameSide(row, "home"),
		Away:         gameSide(row, "away"),
	}
	g.GameID, _ = toInt64(row["game_id"])
	g.Season, _ = toInt64(row["season"])
	switch winner := str(row["winner_team_id"]); {
	case winner == "":
	case winner == g.Home.TeamID:
		g.Winner = g.Home.Team
	case winner == g.Away.TeamID:
		g.Winner = g.Away.Team
	}
	return g
}

func gameSide(row Row, side string) GameSide {
	return GameSide{
		TeamID:      str(row[side+"_team_id"]),
		Team:        str(row[side+"_team_name"]),
		Points:      intPtr(row[side+"_points"]),
		Plays:       intPtr(row[side+"_plays"]),
		Yards:       intPtr(row[side+"_yards"]),
		EPAPerPlay:  roundedPtr(row[side+"_epa_per_play"]),
		SuccessRate: roundedPtr(row[side+"_success_rate"]),
		Turnovers:   intPtr(row[side+"_turnovers"]),
	}
}

// --- Value helpers ---------------------------------------------------------

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

func toInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case float64:
		return int64(x), true
	default:
		return 0, false
	}
}

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	case int:
		return float64(x), true
	default:
		return 0, false
	}
}

// num treats a NULL sum as zero, as SQL SUM does.
func num(v any) float64 {
	f, _ := toFloat64(v)
	return f
}

func intPtr(v any) *int64 {
	if i, ok := toInt64(v); ok {
		return &i
	}
	return nil
}

func roundedPtr(v any) *float64 {
	if f, ok := toFloat64(v); ok {
		f = math.Round(f*10000) / 10000
		return &f
	}
	return nil
}

func timePtr(v any) *time.Time {
	if t, ok := v.(time.Time); ok {
		t = t.UTC()
		return &t
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
