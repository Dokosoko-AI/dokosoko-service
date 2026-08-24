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

type recipeUpdateQuery struct {
	sql    string
	args   []any
	values []any
}

func (query *recipeUpdateQuery) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	query.sql = sql
	query.args = args
	return recipeUpdateRow{values: query.values}
}

type recipeUpdateRow struct {
	values []any
}

func (row recipeUpdateRow) Scan(destinations ...any) error {
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

func TestUpdateRecipeRowPersistsCurrentAnalysisBinding(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	dependencies := []byte(`[{"kind":"resource_set","resource_id":"documentation-set","version":"documentation-set-r2"}]`)
	value := model.Recipe{
		ID:                "recipe-id",
		ProductID:         "product-id",
		AnalysisID:        "analysis-current",
		Slug:              "connect-docs",
		Title:             "Connect docs",
		Outcome:           "Use current docs.",
		Audience:          "developer",
		State:             "review",
		Generated:         true,
		NeedsAttention:    true,
		Visibility:        model.VisibilityPrivate,
		CurrentRevisionID: "recipe-revision-2",
		StableURI:         "dokosoko://products/product/recipes/connect-docs",
	}
	query := &recipeUpdateQuery{values: []any{
		value.ID, "organisation-id", value.ProductID, value.AnalysisID, value.Slug, value.Title, value.Outcome, value.Audience,
		value.State, value.Generated, value.NeedsAttention, value.Visibility, dependencies, value.CurrentRevisionID, value.StableURI,
		"", nil, nil, int64(8), now, now,
	}}

	saved, err := updateRecipeRow(context.Background(), query, value, dependencies, 7)
	if err != nil {
		t.Fatal(err)
	}
	if saved.AnalysisID != value.AnalysisID {
		t.Fatalf("saved analysis binding = %q, want %q", saved.AnalysisID, value.AnalysisID)
	}
	if !strings.Contains(query.sql, "analysis_id=nullif($3,'')::uuid") {
		t.Fatalf("recipe update does not persist analysis_id: %s", query.sql)
	}
	if len(query.args) != 15 || query.args[2] != value.AnalysisID || query.args[14] != int64(7) {
		t.Fatalf("recipe update arguments = %#v", query.args)
	}
}
