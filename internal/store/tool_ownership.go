package store

import (
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func normalizeToolOwnership(scope, ownerIntegrationID string) (string, string, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	ownerIntegrationID = strings.TrimSpace(ownerIntegrationID)
	if scope == "" {
		scope = model.ToolScopeCommon
	}
	switch scope {
	case model.ToolScopeCommon:
		if ownerIntegrationID != "" {
			return "", "", ErrConflict
		}
		return scope, "", nil
	case model.ToolScopeAPI:
		if ownerIntegrationID == "" {
			return "", "", ErrConflict
		}
		return scope, ownerIntegrationID, nil
	default:
		return "", "", ErrConflict
	}
}

func toolMayBindIntegration(tool model.Tool, integration model.Integration) bool {
	if tool.ProductID != integration.DeploymentID || tool.OrganisationID != integration.OrganisationID {
		return false
	}
	switch tool.Scope {
	case model.ToolScopeCommon:
		return tool.OwnerIntegrationID == ""
	case model.ToolScopeAPI:
		return tool.OwnerIntegrationID == integration.ID
	default:
		return false
	}
}
