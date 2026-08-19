package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var (
	ErrProductVersionDrifted       = errors.New("product version has unavailable or drifted artifacts")
	ErrPromotionApprovalRequired   = errors.New("product version promotion requires approval")
	ErrPromotionSeparationOfDuties = errors.New("the publisher cannot approve this product version")
	ErrProductVersionImpact        = errors.New("acknowledge affected pins before deprecating this product version")
)

type ProductInstallationInput struct {
	ID            string
	CustomerID    string
	EnvironmentID string
	ExternalID    string
	Name          string
	State         string
	Revision      int64
}

type ProductVersionPinInput struct {
	Scope            string
	ScopeID          string
	CustomerID       string
	EnvironmentID    string
	InstallationID   string
	ProductVersionID string
	Reason           string
	Revision         int64
}

type ProductVersionPromotionInput struct {
	Action   string
	Note     string
	Revision int64
}

func productVersionManifestHash(productID, version, profileID string, definitionRevision int64, definition model.ProductDefinition) (string, error) {
	encoded, err := json.Marshal(struct {
		ProductID          string                  `json:"product_id"`
		Version            string                  `json:"version"`
		ProfileID          string                  `json:"profile_id"`
		DefinitionRevision int64                   `json:"definition_revision"`
		Definition         model.ProductDefinition `json:"definition"`
	}{productID, version, profileID, definitionRevision, definition})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func productVersionRolloutSelected(key, versionID string, percentage int) bool {
	if percentage >= 100 {
		return true
	}
	if percentage <= 0 || key == "" {
		return false
	}
	digest := sha256.Sum256([]byte(key + "\x00" + versionID))
	bucket := (int(digest[0])<<8 | int(digest[1])) % 100
	return bucket < percentage
}

type diffValue struct {
	Kind  string
	Path  string
	Value string
}

func versionDiffValues(version model.ProductVersion) map[string]diffValue {
	values := map[string]diffValue{}
	for _, binding := range version.Manifest.ProductBindings {
		if binding.Scope != "product" || !binding.Verified {
			continue
		}
		artifactPath := "product/artifact/" + binding.Kind + "/" + binding.Name
		values[artifactPath] = diffValue{Kind: "artifact", Path: artifactPath, Value: binding.Version}
	}
	var profile model.ProductProfile
	for _, candidate := range version.Manifest.Profiles {
		if candidate.ID == version.ProfileID {
			profile = candidate
			break
		}
	}
	for _, selection := range profile.Selections {
		for _, component := range version.Manifest.Components {
			if component.ID != selection.ComponentID {
				continue
			}
			for _, release := range component.Releases {
				if release.ID != selection.ReleaseID {
					continue
				}
				componentPath := "capability/" + component.Slug
				values[componentPath] = diffValue{Kind: "capability", Path: componentPath, Value: release.Version}
				for _, binding := range release.Bindings {
					if !binding.Verified {
						continue
					}
					artifactPath := componentPath + "/artifact/" + binding.Kind + "/" + binding.Name
					values[artifactPath] = diffValue{Kind: "artifact", Path: artifactPath, Value: binding.Version}
				}
			}
		}
	}
	return values
}

func generateProductVersionDiff(previous *model.ProductVersion, current model.ProductVersion, now time.Time) model.ProductVersionDiff {
	result := model.ProductVersionDiff{GeneratedAt: now, Added: []model.ProductVersionChange{}, Removed: []model.ProductVersionChange{}, Changed: []model.ProductVersionChange{}}
	if previous == nil {
		result.Summary = "Initial product release"
		for _, value := range versionDiffValues(current) {
			result.Added = append(result.Added, model.ProductVersionChange{Kind: value.Kind, Path: value.Path, After: value.Value})
		}
	} else {
		result.FromVersionID, result.FromVersion = previous.ID, previous.Version
		before, after := versionDiffValues(*previous), versionDiffValues(current)
		for path, value := range after {
			prior, ok := before[path]
			if !ok {
				result.Added = append(result.Added, model.ProductVersionChange{Kind: value.Kind, Path: path, After: value.Value})
			} else if prior.Value != value.Value {
				result.Changed = append(result.Changed, model.ProductVersionChange{Kind: value.Kind, Path: path, Before: prior.Value, After: value.Value})
			}
		}
		for path, value := range before {
			if _, ok := after[path]; !ok {
				result.Removed = append(result.Removed, model.ProductVersionChange{Kind: value.Kind, Path: path, Before: value.Value})
			}
		}
		result.Summary = fmt.Sprintf("%d added, %d removed, %d changed", len(result.Added), len(result.Removed), len(result.Changed))
	}
	less := func(values []model.ProductVersionChange) {
		sort.Slice(values, func(i, j int) bool { return values[i].Path < values[j].Path })
	}
	less(result.Added)
	less(result.Removed)
	less(result.Changed)
	return result
}

func selectedVersionBindings(version model.ProductVersion) []model.ProductBinding {
	bindings := make([]model.ProductBinding, 0)
	for _, binding := range version.Manifest.ProductBindings {
		if binding.Scope == "product" && binding.Verified {
			bindings = append(bindings, binding)
		}
	}
	var profile model.ProductProfile
	for _, candidate := range version.Manifest.Profiles {
		if candidate.ID == version.ProfileID {
			profile = candidate
			break
		}
	}
	for _, selection := range profile.Selections {
		for _, component := range version.Manifest.Components {
			if component.ID != selection.ComponentID {
				continue
			}
			for _, release := range component.Releases {
				if release.ID == selection.ReleaseID {
					for _, binding := range release.Bindings {
						if binding.Verified {
							bindings = append(bindings, binding)
						}
					}
				}
			}
		}
	}
	return bindings
}

func (s *Service) inspectProductVersionDrift(ctx context.Context, version model.ProductVersion) ([]model.ProductArtifactDrift, error) {
	result := make([]model.ProductArtifactDrift, 0)
	for _, binding := range selectedVersionBindings(version) {
		if binding.ReferenceID == "" {
			continue
		}
		drift := model.ProductArtifactDrift{Kind: binding.Kind, ReferenceID: binding.ReferenceID, Name: binding.Name, Expected: binding.Version, Status: "healthy"}
		switch binding.Kind {
		case "openapi", "docs", "git":
			value, err := s.store.Source(ctx, version.ProductID, binding.ReferenceID)
			if err != nil {
				drift.Status, drift.Message = "missing", "source no longer exists"
			} else if !value.Published || value.Quarantined {
				drift.Status, drift.Message = "unavailable", "source is unpublished or quarantined"
			}
		case "package":
			value, err := s.store.Package(ctx, version.ProductID, binding.ReferenceID)
			if err != nil {
				drift.Status, drift.Message = "missing", "package no longer exists"
			} else if !value.Published {
				drift.Status, drift.Message = "unavailable", "package is not published"
			} else {
				drift.Observed = value.Version
				if binding.Version != "" && binding.Version != value.Version {
					drift.Status, drift.Message = "changed", "package version no longer matches the snapshot"
				}
			}
		case "tool":
			value, err := s.store.Tool(ctx, version.ProductID, binding.ReferenceID)
			if err != nil {
				drift.Status, drift.Message = "missing", "tool no longer exists"
			} else if value.State != "published" || value.UpstreamDrifted {
				drift.Status, drift.Message = "unavailable", "tool is unpublished or its upstream schema drifted"
			}
		case "mcp":
			value, err := s.store.MCPConnection(ctx, version.ProductID, binding.ReferenceID)
			if err != nil {
				drift.Status, drift.Message = "missing", "MCP connection no longer exists"
			} else if value.State != "active" || value.ProtocolVersion != model.StatelessMCPv2Protocol {
				drift.Status, drift.Message = "unavailable", "MCP connection is inactive or not Stateless MCPv2"
			}
		}
		if drift.Status != "healthy" {
			result = append(result, drift)
		}
	}
	return result, nil
}

func (s *Service) ReconcileProductVersion(ctx context.Context, productID, versionID string, expected int64, actor Actor) (model.ProductVersion, error) {
	current, err := s.store.ProductVersion(ctx, productID, versionID)
	if err != nil {
		return model.ProductVersion{}, err
	}
	details, err := s.inspectProductVersionDrift(ctx, current)
	if err != nil {
		return model.ProductVersion{}, err
	}
	checked := s.now()
	current.DriftStatus, current.DriftDetails, current.DriftCheckedAt = "healthy", details, &checked
	if len(details) != 0 {
		current.DriftStatus = "drifted"
	}
	updated, err := s.store.UpdateProductVersion(ctx, current, expected)
	if err != nil {
		return model.ProductVersion{}, err
	}
	_, _ = s.store.BumpProductCatalogRevision(ctx, productID)
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "product.version.drift.checked", TargetType: "product_version", TargetID: updated.ID, Current: map[string]any{"product_version": updated.Version, "manifest_hash": updated.ManifestHash, "drift_status": updated.DriftStatus, "finding_count": len(updated.DriftDetails)}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func (s *Service) PromoteProductVersion(ctx context.Context, productID, versionID string, input ProductVersionPromotionInput, actor Actor) (model.ProductVersion, error) {
	input.Action, input.Note = strings.ToLower(strings.TrimSpace(input.Action)), strings.TrimSpace(input.Note)
	if len(input.Note) > 500 || (input.Action != "request" && input.Action != "approve" && input.Action != "reject") {
		return model.ProductVersion{}, errors.New("promotion action or note is invalid")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.ProductVersion{}, err
	}
	current, err := s.store.ProductVersion(ctx, productID, versionID)
	if err != nil {
		return model.ProductVersion{}, err
	}
	prior := current.PromotionState
	switch input.Action {
	case "request":
		if current.DeprecatedAt != nil || current.DriftStatus == "drifted" || current.PromotionState == "pending" {
			return model.ProductVersion{}, ErrProductVersionDrifted
		}
		if product.RequirePromotionApproval && actor.ID == "" {
			return model.ProductVersion{}, ErrPromotionApprovalRequired
		}
		current.PromotionState, current.PromotionNote, current.PromotionRequestedBy = "pending", input.Note, actor.ID
	case "approve":
		if current.PromotionState != "pending" {
			return model.ProductVersion{}, ErrPromotionApprovalRequired
		}
		requester := current.PromotionRequestedBy
		if requester == "" {
			requester = current.PublisherActorID
		}
		if product.RequirePromotionApproval && (actor.ID == "" || actor.ID == requester) {
			return model.ProductVersion{}, ErrPromotionSeparationOfDuties
		}
		details, inspectErr := s.inspectProductVersionDrift(ctx, current)
		if inspectErr != nil {
			return model.ProductVersion{}, inspectErr
		}
		if len(details) != 0 {
			return model.ProductVersion{}, ErrProductVersionDrifted
		}
		now := s.now()
		current.PromotionState, current.PromotionNote, current.ReleaseStage = "approved", input.Note, "active"
		current.ApprovedBy, current.ApprovedAt = actor.ID, &now
		current.DriftStatus, current.DriftDetails, current.DriftCheckedAt = "healthy", details, &now
		current.IsLatest, current.IsLTS = current.RequestedLatest, current.RequestedLTS
	case "reject":
		if current.PromotionState != "pending" || input.Note == "" {
			return model.ProductVersion{}, errors.New("a pending promotion and rejection note are required")
		}
		current.PromotionState, current.PromotionNote = "rejected", input.Note
	}
	updated, err := s.store.UpdateProductVersion(ctx, current, input.Revision)
	if err != nil {
		return model.ProductVersion{}, err
	}
	_, _ = s.store.BumpProductCatalogRevision(ctx, productID)
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "product.version.promotion." + input.Action, TargetType: "product_version", TargetID: updated.ID, Prior: map[string]any{"promotion_state": prior}, Current: map[string]any{"product_version": updated.Version, "promotion_state": updated.PromotionState, "release_stage": updated.ReleaseStage, "is_latest": updated.IsLatest, "is_lts": updated.IsLTS, "note": input.Note}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func validScopedIdentifier(value string) bool {
	return value != "" && len(value) <= 200 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func (s *Service) SaveProductInstallation(ctx context.Context, productID string, input ProductInstallationInput, actor Actor) (model.ProductInstallation, error) {
	input.ID, input.CustomerID, input.EnvironmentID, input.ExternalID, input.Name, input.State = strings.TrimSpace(input.ID), strings.TrimSpace(input.CustomerID), strings.TrimSpace(input.EnvironmentID), strings.TrimSpace(input.ExternalID), strings.TrimSpace(input.Name), strings.ToLower(strings.TrimSpace(input.State))
	if !validScopedIdentifier(input.CustomerID) || !validScopedIdentifier(input.ExternalID) || input.EnvironmentID == "" || input.Name == "" || len(input.Name) > 120 || (input.State != "active" && input.State != "paused") {
		return model.ProductInstallation{}, errors.New("installation fields are invalid")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.ProductInstallation{}, err
	}
	environments, err := s.store.Environments(ctx, productID)
	if err != nil {
		return model.ProductInstallation{}, err
	}
	foundEnvironment := false
	for _, environment := range environments {
		foundEnvironment = foundEnvironment || environment.ID == input.EnvironmentID
	}
	if !foundEnvironment {
		return model.ProductInstallation{}, errors.New("installation environment does not belong to this product")
	}
	if input.ID == "" {
		input.ID, err = randomUUID()
		if err != nil {
			return model.ProductInstallation{}, err
		}
	}
	value, err := s.store.SaveProductInstallation(ctx, model.ProductInstallation{ID: input.ID, OrganisationID: product.OrganisationID, ProductID: productID, CustomerID: input.CustomerID, EnvironmentID: input.EnvironmentID, ExternalID: input.ExternalID, Name: input.Name, State: input.State}, input.Revision)
	if err != nil {
		return model.ProductInstallation{}, err
	}
	_, _ = s.store.BumpProductCatalogRevision(ctx, productID)
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: value.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "product.installation.saved", TargetType: "product_installation", TargetID: value.ID, Current: map[string]any{"customer_id": value.CustomerID, "environment_id": value.EnvironmentID, "external_id": value.ExternalID, "state": value.State}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, nil
}

func (s *Service) ProductVersionImpact(ctx context.Context, productID, versionID string) (model.ProductVersionImpact, error) {
	version, err := s.store.ProductVersion(ctx, productID, versionID)
	if err != nil {
		return model.ProductVersionImpact{}, err
	}
	result := model.ProductVersionImpact{ProductVersionID: version.ID, ProductVersion: version.Version, AffectedCustomers: []string{}, AffectedEnvironments: []string{}, AffectedInstallations: []string{}}
	pins, err := s.store.ProductVersionPins(ctx, productID)
	if err != nil {
		return result, err
	}
	for _, pin := range pins {
		if pin.ProductVersionID != versionID {
			continue
		}
		switch pin.Scope {
		case "customer":
			result.CustomerPins++
			result.AffectedCustomers = append(result.AffectedCustomers, pin.ScopeID)
		case "environment":
			result.EnvironmentPins++
			result.AffectedEnvironments = append(result.AffectedEnvironments, pin.ScopeID)
		case "installation":
			result.InstallationPins++
			result.AffectedInstallations = append(result.AffectedInstallations, pin.ScopeID)
		}
	}
	activity, err := s.store.ProductVersionActivity(ctx, productID, versionID, s.now().Add(-30*24*time.Hour))
	if err != nil {
		return result, err
	}
	result.Requests30Days, result.ToolCalls30Days = activity.Requests, activity.ToolCalls
	return result, nil
}

func (s *Service) SaveScopedProductVersionPin(ctx context.Context, productID string, input ProductVersionPinInput, actor Actor) (model.ProductVersionPin, error) {
	input.Scope, input.ScopeID, input.CustomerID, input.EnvironmentID, input.InstallationID, input.Reason = strings.ToLower(strings.TrimSpace(input.Scope)), strings.TrimSpace(input.ScopeID), strings.TrimSpace(input.CustomerID), strings.TrimSpace(input.EnvironmentID), strings.TrimSpace(input.InstallationID), strings.TrimSpace(input.Reason)
	if !validScopedIdentifier(input.ScopeID) || len(input.Reason) > 500 || (input.Scope != "customer" && input.Scope != "environment" && input.Scope != "installation") {
		return model.ProductVersionPin{}, errors.New("pin scope, identifier, or reason is invalid")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.ProductVersionPin{}, err
	}
	version, err := s.store.ProductVersion(ctx, productID, input.ProductVersionID)
	if err != nil {
		return model.ProductVersionPin{}, err
	}
	if version.DeprecatedAt != nil {
		return model.ProductVersionPin{}, ErrProductVersionDeprecated
	}
	if input.Scope == "customer" {
		input.CustomerID, input.ScopeID = input.ScopeID, input.ScopeID
	}
	if input.Scope == "environment" {
		environments, environmentErr := s.store.Environments(ctx, productID)
		if environmentErr != nil {
			return model.ProductVersionPin{}, environmentErr
		}
		found := false
		for _, environment := range environments {
			found = found || environment.ID == input.ScopeID
		}
		if !found {
			return model.ProductVersionPin{}, errors.New("pin environment does not belong to this product")
		}
		input.EnvironmentID = input.ScopeID
	}
	if input.Scope == "installation" {
		installation, installationErr := s.store.ProductInstallation(ctx, productID, input.ScopeID)
		if installationErr != nil {
			return model.ProductVersionPin{}, installationErr
		}
		input.InstallationID, input.EnvironmentID, input.CustomerID = installation.ID, installation.EnvironmentID, installation.CustomerID
	}
	var prior model.ProductVersionPin
	prior, err = s.store.ProductVersionPin(ctx, productID, input.Scope, input.ScopeID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return model.ProductVersionPin{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.ProductVersionPin{}, err
	}
	value, err := s.store.SaveProductVersionPin(ctx, model.ProductVersionPin{ID: id, OrganisationID: product.OrganisationID, ProductID: productID, Scope: input.Scope, ScopeID: input.ScopeID, CustomerID: input.CustomerID, EnvironmentID: input.EnvironmentID, InstallationID: input.InstallationID, ProductVersionID: version.ID, ProductVersion: version.Version, Reason: input.Reason}, input.Revision)
	if err != nil {
		return model.ProductVersionPin{}, err
	}
	action := "created"
	if prior.ID != "" {
		action = "updated"
	}
	historyID, _ := randomUUID()
	_ = s.store.AppendProductVersionPinHistory(ctx, model.ProductVersionPinHistory{ID: historyID, OrganisationID: value.OrganisationID, ProductID: productID, PinID: value.ID, Scope: value.Scope, ScopeID: value.ScopeID, PriorVersion: prior.ProductVersion, ProductVersion: value.ProductVersion, Action: action, Reason: value.Reason, ActorID: actor.ID, CreatedAt: s.now()})
	_, _ = s.store.BumpProductCatalogRevision(ctx, productID)
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: value.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "product.version.pinned", TargetType: "product_version_pin", TargetID: value.ID, Prior: map[string]any{"product_version": prior.ProductVersion}, Current: map[string]any{"scope": value.Scope, "scope_id": value.ScopeID, "customer_id": value.CustomerID, "product_version": value.ProductVersion, "reason": value.Reason}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, nil
}
