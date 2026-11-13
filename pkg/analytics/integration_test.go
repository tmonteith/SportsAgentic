//go:build integration

package analytics

// Runs the real queries against BigQuery. Needs GCP_PROJECT and Application
// Default Credentials that can read the gold tables:
//
//	GCP_PROJECT=<project> go test -tags integration ./pkg/analytics/
//
// Each query is capped at MaxBytesBilled like in production.

import (
	"context"
	"os"
	"testing"
)

func TestIntegrationGoldQueries(t *testing.T) {
	project := os.Getenv("GCP_PROJECT")
	if project == "" {
		t.Skip("GCP_PROJECT is not set")
	}
	ctx := context.Background()
	bq, err := NewBigQuery(ctx, project)
	if err != nil {
		t.Fatalf("creating BigQuery client: %v", err)
	}
	defer bq.Close()
	svc, err := NewService(project, bq)
	if err != nil {
		t.Fatal(err)
	}

	team, err := svc.ResolveTeam(ctx, "Buffs")
	if err != nil {
		t.Fatalf("ResolveTeam: %v", err)
	}
	t.Logf("Buffs -> %+v", team)

	from := 5
	res, err := svc.TeamAnalytics(ctx, TeamAnalyticsRequest{Team: "Colorado", CompareTeam: "Utah", Season: 2025, WeekFrom: &from})
	if err != nil {
		t.Fatalf("TeamAnalytics: %v", err)
	}
	if len(res.Teams) != 2 || res.Teams[0].RangeTotal == nil || len(res.Teams[0].Weeks) == 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
	t.Logf("Colorado 2025 since week 5: %d weeks, epa_per_play %v", len(res.Teams[0].Weeks), deref(res.Teams[0].RangeTotal.Metrics["epa_per_play"]))

	games, err := svc.GameSummary(ctx, GameSummaryRequest{Team: "Colorado", Season: 2025, Week: int(res.Teams[0].Weeks[0].Week), SeasonType: SeasonTypeRegular})
	if err != nil {
		t.Fatalf("GameSummary by team: %v", err)
	}
	byID, err := svc.GameSummary(ctx, GameSummaryRequest{GameID: games.Games[0].GameID})
	if err != nil || len(byID.Games) != 1 {
		t.Fatalf("GameSummary by id: %+v, %v", byID, err)
	}
	t.Logf("game %d: %s %v - %s %v", byID.Games[0].GameID, byID.Games[0].Home.Team, byID.Games[0].Home.Points, byID.Games[0].Away.Team, byID.Games[0].Away.Points)
}
