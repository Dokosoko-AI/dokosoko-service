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
		if value.IntegrationID != "" || len(value.APIAttachments) != 0 {
			return model.Recipe{}, errors.New("legacy recipes cannot have an Integration binding")
		}
	case model.RecipeContractProductIntegrationV2:
		if value.IntegrationID == "" {
			return model.Recipe{}, errors.New("product-integration recipes require an Integration binding")
		}
		if len(value.APIAttachments) == 0 {
			value.APIAttachments = []model.RecipeAPIAttachment{{IntegrationID: value.IntegrationID}}
		}
	case model.RecipeContractDeploymentV3:
		if value.IntegrationID != "" {
			return model.Recipe{}, errors.New("deployment recipes cannot be owned by one Integration")
		}
	default:
		return model.Recipe{}, errors.New("unsupported recipe contract version")
	}
	attachments, err := normalizeRecipeAPIAttachments(value.APIAttachments)
	if err != nil {
		return model.Recipe{}, err
	}
	value.APIAttachments = attachments
	if value.ContractVersion == model.RecipeContractProductIntegrationV2 && (len(attachments) != 1 || attachments[0].IntegrationID != value.IntegrationID) {
		return model.Recipe{}, errors.New("product-integration recipe attachment must match its Integration binding")
	}
	return value, nil
}

func normalizeRecipeAPIAttachments(values []model.RecipeAPIAttachment) ([]model.RecipeAPIAttachment, error) {
	if len(values) > 8 {
		return nil, errors.New("recipes may attach at most 8 APIs")
	}
	result := append([]model.RecipeAPIAttachment(nil), values...)
	for index := range result {
		result[index].IntegrationID = strings.TrimSpace(result[index].IntegrationID)
		if result[index].IntegrationID == "" {
			return nil, errors.New("recipe API attachment IDs are required")
		}
	}
	slices.SortFunc(result, func(left, right model.RecipeAPIAttachment) int {
		return strings.Compare(left.IntegrationID, right.IntegrationID)
	})
	for index := 1; index < len(result); index++ {
		if result[index-1].IntegrationID == result[index].IntegrationID {
			return nil, errors.New("recipe API attachments must be unique")
		}
	}
	return result, nil
}

func recipeAttachmentIDs(values []model.RecipeAPIAttachment) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.IntegrationID
	}
	return result
}

func normalizeRecipeAPIBindings(values []model.RecipeAPIBinding) ([]model.RecipeAPIBinding, error) {
	if len(values) > 8 {
		return nil, errors.New("recipe revisions may bind at most 8 APIs")
	}
	result := append([]model.RecipeAPIBinding(nil), values...)
	for index := range result {
		result[index].IntegrationID = strings.TrimSpace(result[index].IntegrationID)
		result[index].IntegrationRevisionID = strings.TrimSpace(result[index].IntegrationRevisionID)
		result[index].IntegrationManifestHash = strings.TrimSpace(result[index].IntegrationManifestHash)
		if result[index].IntegrationID == "" || result[index].IntegrationRevisionID == "" || !validRecipeSHA256(result[index].IntegrationManifestHash) {
			return nil, errors.New("recipe API bindings require an Integration, exact revision, and SHA-256 manifest hash")
		}
	}
	slices.SortFunc(result, func(left, right model.RecipeAPIBinding) int {
		return strings.Compare(left.IntegrationID, right.IntegrationID)
	})
	for index := 1; index < len(result); index++ {
		if result[index-1].IntegrationID == result[index].IntegrationID {
			return nil, errors.New("recipe API bindings must be unique")
		}
	}
	return result, nil
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
		!slices.Equal(candidate.APIAttachments, stored.APIAttachments) ||
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

func recipeDeletionAllowed(recipe model.Recipe) bool {
	return recipe.ContractVersion == model.RecipeContractLegacyMCPV1 || recipe.State == "outdated"
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

	if recipe.ContractVersion == model.RecipeContractLegacyMCPV1 {
		if len(value.APIBindings) != 0 {
			return model.RecipeRevision{}, errors.New("legacy recipe revisions cannot have API bindings")
		}
		return value, nil
	}
	if recipe.ContractVersion == model.RecipeContractProductIntegrationV2 {
		if recipe.IntegrationID == "" || value.SpecVersion != model.RecipeSpecVersion2 || value.IntegrationRevisionID == "" {
			return model.RecipeRevision{}, errors.New("product-integration recipe revisions require an exact Integration and v2 spec binding")
		}
		if len(value.APIBindings) == 0 {
			value.APIBindings = []model.RecipeAPIBinding{{IntegrationID: recipe.IntegrationID, IntegrationRevisionID: value.IntegrationRevisionID, IntegrationManifestHash: value.IntegrationManifestHash}}
		}
	} else if recipe.ContractVersion == model.RecipeContractDeploymentV3 {
		if value.SpecVersion != model.RecipeSpecVersion3 || value.IntegrationRevisionID != "" || value.IntegrationManifestHash != "" {
			return model.RecipeRevision{}, errors.New("deployment recipe revisions require a v3 spec and revision-level API bindings")
		}
	}
	bindings, err := normalizeRecipeAPIBindings(value.APIBindings)
	if err != nil {
		return model.RecipeRevision{}, err
	}
	value.APIBindings = bindings
	if !slices.Equal(recipeAttachmentIDs(recipe.APIAttachments), recipeBindingIntegrationIDs(bindings)) {
		return model.RecipeRevision{}, errors.New("recipe revision API bindings must exactly match current API attachments")
	}
	var typed model.RecipeSpec
	decoder := json.NewDecoder(bytes.NewReader(value.Spec))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&typed); err != nil {
		return model.RecipeRevision{}, errors.New("recipe spec is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return model.RecipeRevision{}, errors.New("recipe spec is invalid")
	}
	if typed.Title != recipe.Title || typed.Outcome != recipe.Outcome {
		return model.RecipeRevision{}, errors.New("recipe spec does not match its recipe identity")
	}
	if recipe.ContractVersion == model.RecipeContractProductIntegrationV2 && (typed.SchemaVersion != model.RecipeSpecVersion2 || typed.IntegrationID != recipe.IntegrationID || len(typed.APIAttachments) != 0) {
		return model.RecipeRevision{}, errors.New("product-integration recipe spec does not match its Integration binding")
	}
	if recipe.ContractVersion == model.RecipeContractDeploymentV3 {
		attachments, attachmentErr := normalizeRecipeAPIAttachments(typed.APIAttachments)
		if attachmentErr != nil || typed.SchemaVersion != model.RecipeSpecVersion3 || typed.IntegrationID != "" || !slices.Equal(attachments, recipe.APIAttachments) {
			return model.RecipeRevision{}, errors.New("deployment recipe spec does not match its API attachments")
		}
	}
	return value, nil
}

func recipeBindingIntegrationIDs(values []model.RecipeAPIBinding) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.IntegrationID
	}
	return result
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
