package analytics

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

// BigQuery runs queries with the cloud.google.com/go/bigquery client and
// Application Default Credentials (GOOGLE_APPLICATION_CREDENTIALS in the pod).
type BigQuery struct {
	client *bigquery.Client
}

// NewBigQuery creates a client billed to project. It finds credentials now
// but makes no API call until the first query.
func NewBigQuery(ctx context.Context, project string) (*BigQuery, error) {
	client, err := bigquery.NewClient(ctx, project)
	if err != nil {
		return nil, err
	}
	client.Location = Location
	return &BigQuery{client: client}, nil
}

// Query implements Querier.
func (b *BigQuery) Query(ctx context.Context, q Query, maxRows int) ([]Row, bool, error) {
	bq := b.client.Query(q.SQL)
	bq.Location = Location
	bq.MaxBytesBilled = MaxBytesBilled
	bq.Labels = map[string]string{"app": "sportsagentic-mcp"}
	for _, p := range q.Params {
		bq.Parameters = append(bq.Parameters, bigquery.QueryParameter{Name: p.Name, Value: p.Value})
	}

	it, err := bq.Read(ctx)
	if err != nil {
		return nil, false, err
	}
	var rows []Row
	for {
		var values map[string]bigquery.Value
		err := it.Next(&values)
		if errors.Is(err, iterator.Done) {
			return rows, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		if len(rows) == maxRows {
			return rows, true, nil
		}
		row := make(Row, len(values))
		for k, v := range values {
			row[k] = v
		}
		rows = append(rows, row)
	}
}

// Close releases the client's connections.
func (b *BigQuery) Close() error {
	if err := b.client.Close(); err != nil {
		return fmt.Errorf("closing BigQuery client: %w", err)
	}
	return nil
}
