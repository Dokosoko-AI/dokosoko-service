package httpapi

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestExactRecipeMatchNeverGuesses(t *testing.T) {
	values := []model.Recipe{
		{StableURI: "dokosoko://products/demo/recipes/create-customer", Slug: "create-customer", Title: "Create a customer", Outcome: "The application creates one customer."},
		{StableURI: "dokosoko://products/demo/recipes/create-payment", Slug: "create-payment", Title: "Create a payment", Outcome: "The application creates one payment."},
	}

	selected, matches := exactRecipeMatch(values, " Create-payment ")
	if selected == nil || selected.Slug != "create-payment" || len(matches) != 1 {
		t.Fatalf("exact normalized match = %#v, matches = %#v", selected, matches)
	}

	selected, matches = exactRecipeMatch(values, "create")
	if selected != nil || len(matches) != 0 {
		t.Fatalf("partial request must not select a recipe: selected=%#v matches=%#v", selected, matches)
	}

	selected, matches = exactRecipeMatch(values, "something else")
	if selected != nil || len(matches) != 0 {
		t.Fatalf("unmatched request must not select a recipe: selected=%#v matches=%#v", selected, matches)
	}
}

func TestExactRecipeMatchReportsAmbiguity(t *testing.T) {
	values := []model.Recipe{
		{StableURI: "dokosoko://products/demo/recipes/customer-a", Slug: "customer-a", Title: "Create a customer", Outcome: "Create a customer."},
		{StableURI: "dokosoko://products/demo/recipes/customer-b", Slug: "customer-b", Title: "Create a customer", Outcome: "Create a customer."},
	}

	selected, matches := exactRecipeMatch(values, "create a customer")
	if selected != nil || len(matches) != 2 {
		t.Fatalf("ambiguous request selected=%#v matches=%#v", selected, matches)
	}
}

func TestSortedRecipeSummariesExposeOnlyDeliveryMetadata(t *testing.T) {
	publishedAt := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	values := []model.Recipe{
		{ID: "internal-b", OrganisationID: "org", ProductID: "product", StableURI: "dokosoko://products/demo/recipes/zeta", Slug: "zeta", Title: "Zeta", Outcome: "Implement Zeta.", IntegrationID: "integration-zeta", ContractVersion: "product-integration-v2", CurrentRevisionID: "revision-zeta", PublishedAt: &publishedAt},
		{ID: "internal-a", OrganisationID: "org", ProductID: "product", StableURI: "dokosoko://products/demo/recipes/alpha", Slug: "alpha", Title: "Alpha", Outcome: "Implement Alpha.", IntegrationID: "integration-alpha", ContractVersion: "product-integration-v2", CurrentRevisionID: "revision-alpha", PublishedAt: &publishedAt},
	}

	summaries := sortedRecipeSummaries(values)
	if len(summaries) != 2 || summaries[0].Slug != "alpha" || summaries[1].Slug != "zeta" {
		t.Fatalf("summaries are not deterministic: %#v", summaries)
	}
	if summaries[0].IntegrationID != "integration-alpha" || summaries[0].ContractVersion != "product-integration-v2" || summaries[0].RevisionID != "revision-alpha" || summaries[0].PublishedAt != &publishedAt {
		t.Fatalf("delivery metadata = %#v", summaries[0])
	}
}

func TestBoundedRecipeSummariesLimitsSelectionErrors(t *testing.T) {
	values := make([]model.Recipe, maxRecipeSelectionCandidates+3)
	for index := range values {
		key := fmt.Sprintf("item-%02d", index)
		values[index] = model.Recipe{StableURI: "dokosoko://products/demo/recipes/" + key, Slug: key, Title: key}
	}

	summaries, truncated := boundedRecipeSummaries(values)
	if !truncated || len(summaries) != maxRecipeSelectionCandidates {
		t.Fatalf("bounded summaries length=%d truncated=%t", len(summaries), truncated)
	}
}

func TestRecipeAvailableForMCPRequiresCompleteV2Publication(t *testing.T) {
	publishedAt := time.Now().UTC()
	valid := model.Recipe{
		State:             "published",
		ContractVersion:   model.RecipeContractProductIntegrationV2,
		IntegrationID:     "integration",
		StableURI:         "dokosoko://products/demo/recipes/example",
		CurrentRevisionID: "revision",
		PublishedAt:       &publishedAt,
		Visibility:        model.VisibilityPrivate,
	}
	if !recipeAvailableForMCP(valid, false) {
		t.Fatal("complete private v2 publication was rejected")
	}
	if recipeAvailableForMCP(valid, true) {
		t.Fatal("private publication was exposed publicly")
	}

	public := valid
	public.Visibility = model.VisibilityPublic
	if !recipeAvailableForMCP(public, true) {
		t.Fatal("complete public v2 publication was rejected")
	}

	tests := []struct {
		name   string
		mutate func(*model.Recipe)
	}{
		{name: "legacy contract", mutate: func(value *model.Recipe) { value.ContractVersion = model.RecipeContractLegacyMCPV1 }},
		{name: "missing integration", mutate: func(value *model.Recipe) { value.IntegrationID = "" }},
		{name: "missing URI", mutate: func(value *model.Recipe) { value.StableURI = "" }},
		{name: "missing revision", mutate: func(value *model.Recipe) { value.CurrentRevisionID = "" }},
		{name: "missing publication time", mutate: func(value *model.Recipe) { value.PublishedAt = nil }},
		{name: "attention required", mutate: func(value *model.Recipe) { value.NeedsAttention = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if recipeAvailableForMCP(candidate, false) {
				t.Fatalf("invalid recipe was available: %#v", candidate)
			}
		})
	}
}

func TestRecipeToolArgumentsMatchAdvertisedSchemas(t *testing.T) {
	for _, test := range []struct {
		name      string
		arguments map[string]any
		key       string
		want      string
		valid     bool
	}{
		{name: "exact plan input", arguments: map[string]any{"outcome": " Create a payment "}, key: "outcome", want: "Create a payment", valid: true},
		{name: "exact check input", arguments: map[string]any{"recipe_uri": "dokosoko://products/demo/recipes/payment"}, key: "recipe_uri", want: "dokosoko://products/demo/recipes/payment", valid: true},
		{name: "extra property", arguments: map[string]any{"outcome": "payment", "unexpected": true}, key: "outcome"},
		{name: "wrong type", arguments: map[string]any{"outcome": 42}, key: "outcome"},
		{name: "blank", arguments: map[string]any{"outcome": "  "}, key: "outcome"},
		{name: "too long", arguments: map[string]any{"outcome": strings.Repeat("x", 501)}, key: "outcome"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, valid := exactRecipeToolStringArgument(test.arguments, test.key)
			if valid != test.valid || got != test.want {
				t.Fatalf("argument = %q, valid=%t; want %q, %t", got, valid, test.want, test.valid)
			}
		})
	}
}
