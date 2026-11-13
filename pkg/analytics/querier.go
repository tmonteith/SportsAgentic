// Package analytics reads the lakehouse gold tables in BigQuery for the MCP
// analytics tools. Every query is a fixed, parameterized statement against an
// allowlisted gold table; callers never supply SQL, identifiers or ordering.
package analytics

import (
	"context"
	"fmt"
	"regexp"
)

// Limits that keep every query inside the BigQuery free tier.
const (
	// Location is the BigQuery location of the sa_catalog BigLake catalog.
	Location = "us-central1"
	// MaxBytesBilled caps each query at 100 MB billed; BigQuery fails the
	// query instead of scanning more.
	MaxBytesBilled int64 = 100 * 1000 * 1000
	// RowCap is the most rows any query returns.
	RowCap = 200
)

// Catalog and namespace of the gold tables (plan "Shared contracts").
const (
	catalog       = "sa_catalog"
	goldNamespace = "gold"
)

// Table is a gold table the analytics tools may read.
type Table string

const (
	TableTeamEfficiencyWeekly Table = "team_efficiency_weekly"
	TableGameSummaries        Table = "game_summaries"
)

// goldTables is the allowlist of tables a query may name. The MCP identity
// can read only gold, so silver and bronze never appear here.
var goldTables = map[Table]bool{
	TableTeamEfficiencyWeekly: true,
	TableGameSummaries:        true,
}

// projectPattern matches a standard GCP project ID. The project comes from the
// GCP_PROJECT env var and is part of a quoted identifier, so it is checked.
var projectPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)

// ValidProject reports whether project is a well-formed GCP project ID.
func ValidProject(project string) bool {
	return projectPattern.MatchString(project)
}

// tableRef returns the quoted four-part BigQuery name of an allowlisted gold
// table, `<project>.sa_catalog.gold.<table>`.
func tableRef(project string, t Table) (string, error) {
	if !goldTables[t] {
		return "", fmt.Errorf("table %q is not an allowlisted gold table", t)
	}
	if !ValidProject(project) {
		return "", fmt.Errorf("invalid GCP project %q", project)
	}
	return fmt.Sprintf("`%s.%s.%s.%s`", project, catalog, goldNamespace, t), nil
}

// Param is one named query parameter (@Name in the SQL).
type Param struct {
	Name  string
	Value any // int64, string, bool or []string
}

// Query is a parameterized statement built by this package.
type Query struct {
	SQL    string
	Params []Param
}

// Row is one result row keyed by column name. Values are int64, float64,
// string, bool, time.Time or nil, as the BigQuery client returns them.
type Row map[string]any

// Querier runs a query and returns at most maxRows rows. It reports whether
// more rows were available. Implementations must apply Location and
// MaxBytesBilled. Unit tests use a fake.
type Querier interface {
	Query(ctx context.Context, q Query, maxRows int) (rows []Row, truncated bool, err error)
}
