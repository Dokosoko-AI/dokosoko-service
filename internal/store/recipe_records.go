package store

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

// RecipeMutation is the optimistic-concurrency boundary for a recipe
// aggregate. CatalogRevision is checked in the same critical section or SQL
// transaction as the recipe write so reviewed evidence cannot change between
// the service's grounding decision and persistence.
type RecipeMutation struct {
	ExpectedRevision        int64
	ExpectedCatalogRevision int64
	Audit                   *model.AuditEvent
}

func prepareRecipeRecord(value model.Recipe) (model.Recipe, error) {
	if value.ContractVersion == "" {
		value.ContractVersion = model.RecipeContractLegacyMCPV1
	}
	switch value.ContractVersion {
	case model.RecipeContractLegacyMCPV1:
		if value.IntegrationID != "" {
			return model.Recipe{}, errors.New("legacy recipes cannot have an Integration binding")
		}
	case model.RecipeContractProductIntegrationV2:
		if value.IntegrationID == "" {
			return model.Recipe{}, errors.New("product-integration recipes require an Integration binding")
		}
	default:
		return model.Recipe{}, errors.New("unsupported recipe contract version")
	}
	return value, nil
}

func prepareRecipeAudit(recipe model.Recipe, event *model.AuditEvent, allowedActions ...string) (prior, current []byte, outcome string, err error) {
	if event == nil {
		return nil, nil, "", nil
	}
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.ActorID) == "" || strings.TrimSpace(event.Action) == "" || strings.TrimSpace(event.RequestID) == "" || event.CreatedAt.IsZero() {
		return nil, nil, "", errors.New("audit event ID, actor, action, request ID, and creation time are required")
	}
	if event.OrganisationID != recipe.OrganisationID || event.ProductID != recipe.ProductID || event.TargetType != "recipe" || event.TargetID != recipe.ID {
		return nil, nil, "", errors.New("audit event does not match the recipe transition scope")
	}
	if len(allowedActions) > 0 && !slices.Contains(allowedActions, event.Action) {
		return nil, nil, "", errors.New("audit action does not match the recipe mutation")
	}
	prior, err = json.Marshal(event.Prior)
	if err != nil {
		return nil, nil, "", err
	}
	current, err = json.Marshal(event.Current)
	if err != nil {
		return nil, nil, "", err
	}
	outcome = event.Outcome
	if outcome == "" {
		outcome = "success"
	}
	return prior, current, outcome, nil
}

func validateRecipeMutation(mutation RecipeMutation) error {
	if mutation.ExpectedRevision < 0 {
		return errors.New("expected recipe revision cannot be negative")
	}
	if mutation.ExpectedCatalogRevision < 1 {
		return errors.New("expected catalog revision is required")
	}
	return nil
}

func validateRecipeImmutableBinding(stored, candidate model.Recipe) error {
	if candidate.ID != stored.ID ||
		candidate.OrganisationID != stored.OrganisationID ||
		candidate.ProductID != stored.ProductID ||
		candidate.IntegrationID != stored.IntegrationID ||
		candidate.ContractVersion != stored.ContractVersion ||
		candidate.Slug != stored.Slug ||
		candidate.Generated != stored.Generated ||
		candidate.StableURI != stored.StableURI {
		return ErrConflict
	}
	return nil
}

func validateRecipeTransition(stored, candidate model.Recipe) error {
	if err := validateRecipeImmutableBinding(stored, candidate); err != nil {
		return err
	}
	if candidate.AnalysisID != stored.AnalysisID ||
		candidate.Title != stored.Title ||
		candidate.Outcome != stored.Outcome ||
		candidate.Audience != stored.Audience ||
		candidate.Visibility != stored.Visibility ||
		!slices.Equal(candidate.Dependencies, stored.Dependencies) ||
		candidate.CurrentRevisionID != stored.CurrentRevisionID {
		return ErrConflict
	}

	switch candidate.State {
	case "approved":
		if stored.State != "review" || candidate.NeedsAttention || candidate.ApprovedBy == "" || candidate.ApprovedAt == nil || candidate.PublishedAt != nil {
			return ErrConflict
		}
	case "published":
		if stored.State != "approved" || stored.ApprovedBy == "" || stored.ApprovedAt == nil || candidate.NeedsAttention || candidate.PublishedAt == nil || candidate.ApprovedBy != stored.ApprovedBy || !recipeTimesEqual(candidate.ApprovedAt, stored.ApprovedAt) {
			return ErrConflict
		}
	case "outdated":
		if stored.State == "outdated" || !candidate.NeedsAttention || candidate.ApprovedBy != "" || candidate.ApprovedAt != nil || candidate.PublishedAt != nil {
			return ErrConflict
		}
	default:
		return ErrConflict
	}
	return nil
}

func validateRecipeRevisionChange(stored, candidate model.Recipe) error {
	if err := validateRecipeImmutableBinding(stored, candidate); err != nil {
		return err
	}
	if candidate.CurrentRevisionID != stored.CurrentRevisionID || candidate.State != "review" || !candidate.NeedsAttention || candidate.ApprovedBy != "" || candidate.ApprovedAt != nil || candidate.PublishedAt != nil {
		return ErrConflict
	}
	return nil
}

func recipeTransitionAuditActions(stored, candidate model.Recipe) []string {
	switch candidate.State {
	case "approved":
		return []string{"recipe.approved"}
	case "published":
		return []string{"recipe.published"}
	case "outdated":
		return []string{"recipe.outdated"}
	default:
		return nil
	}
}

func recipeTransitionBumpsCatalog(stored, candidate model.Recipe) bool {
	return stored.State == "published" || candidate.State == "published"
}

func recipeTimesEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func prepareRecipeRevisionRecord(recipe model.Recipe, value model.RecipeRevision) (model.RecipeRevision, error) {
	if value.SpecVersion == 0 {
		value.SpecVersion = 1
	}
	spec, err := recipeSpecJSON(value.Spec)
	if err != nil {
		return model.RecipeRevision{}, err
	}
	value.Spec = spec

	if (value.IntegrationRevisionID == "") != (value.IntegrationManifestHash == "") {
		return model.RecipeRevision{}, errors.New("recipe Integration revision ID and manifest hash must be recorded together")
	}
	if value.IntegrationManifestHash != "" && !validRecipeSHA256(value.IntegrationManifestHash) {
		return model.RecipeRevision{}, errors.New("recipe Integration manifest hash must be a SHA-256 digest")
	}
	if value.PromptHash != "" && !validRecipeSHA256(value.PromptHash) {
		return model.RecipeRevision{}, errors.New("recipe prompt hash must be a SHA-256 digest")
	}
	if value.GeneratedBy == "ai" && (strings.TrimSpace(value.Model) == "" || strings.TrimSpace(value.PromptVersion) == "" || value.PromptHash == "") {
		return model.RecipeRevision{}, errors.New("AI recipe revisions require model and prompt provenance")
	}

	if recipe.ContractVersion != model.RecipeContractProductIntegrationV2 {
		return value, nil
	}
	if recipe.IntegrationID == "" || value.SpecVersion != model.RecipeSpecVersion2 || value.IntegrationRevisionID == "" {
		return model.RecipeRevision{}, errors.New("product-integration recipe revisions require an exact Integration and v2 spec binding")
	}
	var typed model.RecipeSpec
	decoder := json.NewDecoder(bytes.NewReader(value.Spec))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&typed); err != nil {
		return model.RecipeRevision{}, errors.New("product-integration recipe spec is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return model.RecipeRevision{}, errors.New("product-integration recipe spec is invalid")
	}
	if typed.SchemaVersion != model.RecipeSpecVersion2 || typed.IntegrationID != recipe.IntegrationID || typed.Title != recipe.Title || typed.Outcome != recipe.Outcome {
		return model.RecipeRevision{}, errors.New("product-integration recipe spec does not match its recipe binding")
	}
	return value, nil
}

func recipeSpecJSON(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return []byte(`{}`), nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, errors.New("recipe spec must be a JSON object")
	}
	return append([]byte(nil), raw...), nil
}

func validRecipeSHA256(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
