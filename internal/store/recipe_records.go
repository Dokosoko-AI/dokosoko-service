package store

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

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

func prepareRecipeTransitionAudit(recipe model.Recipe, event *model.AuditEvent) (prior, current []byte, outcome string, err error) {
	if event == nil {
		return nil, nil, "", nil
	}
	if strings.TrimSpace(event.ID) == "" {
		return nil, nil, "", errors.New("audit event ID is required")
	}
	if event.OrganisationID != recipe.OrganisationID || event.ProductID != recipe.ProductID || event.TargetType != "recipe" || event.TargetID != recipe.ID {
		return nil, nil, "", errors.New("audit event does not match the recipe transition scope")
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
	if value.GeneratedBy == "ai" && (strings.TrimSpace(value.Model) == "" || strings.TrimSpace(value.PromptVersion) == "" || value.PromptHash == "") {
		return model.RecipeRevision{}, errors.New("AI recipe revisions require model and prompt provenance")
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
