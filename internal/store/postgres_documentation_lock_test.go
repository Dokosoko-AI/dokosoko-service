package store

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

type recordedRowQuery struct {
	calls []string
	rows  [][]any
}

func (query *recordedRowQuery) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	query.calls = append(query.calls, sql)
	values := query.rows[0]
	query.rows = query.rows[1:]
	return recordedRow{values: values}
}

type recordedRow struct {
	values []any
}

func (row recordedRow) Scan(destinations ...any) error {
	for index, destination := range destinations {
		target := reflect.ValueOf(destination).Elem()
		if row.values[index] == nil {
			target.Set(reflect.Zero(target.Type()))
			continue
		}
		target.Set(reflect.ValueOf(row.values[index]))
	}
	return nil
}

func TestCreateCrawlJobLocksSourceBeforeInsert(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	value := model.CrawlJob{ID: "job", OrganisationID: "org", ProductID: "product", SourceID: "source"}
	query := &recordedRowQuery{rows: [][]any{
		{"source"},
		{"job", "org", "product", "source", "queued", 1, 0, 0, 0, "", "", now, nil, nil},
	}}

	created, err := createCrawlJobWithSourceLock(context.Background(), query, value)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != value.ID || len(query.calls) != 2 {
		t.Fatalf("created = %#v, calls = %d", created, len(query.calls))
	}
	if !strings.Contains(query.calls[0], "FROM sources") || !strings.Contains(query.calls[0], "FOR UPDATE") {
		t.Fatalf("first query did not lock the source row: %s", query.calls[0])
	}
	if !strings.Contains(query.calls[1], "INSERT INTO crawl_jobs") {
		t.Fatalf("second query did not create the crawl job: %s", query.calls[1])
	}
}
