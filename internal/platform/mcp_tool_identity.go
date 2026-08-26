package platform

import "github.com/dokosoko/dokosoko-service/internal/model"

type canonicalMCPToolIdentity struct {
	name                       string
	authorizationPointID       string
	authorizationPointRevision int64
}

// ValidMCPToolSegment reports whether value can be used unchanged in a
// server-owned MCP tool name.
func ValidMCPToolSegment(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	for index, char := range value {
		valid := char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || index > 0 && (char == '-' || char == '_')
		if !valid {
			return false
		}
	}
	return true
}

// CanonicalMCPToolName resolves the exact name that tools/list can expose for
// tool in manifest. API-owned tools fail closed unless the current published,
// non-drifted tool has one exact binding in one unambiguous published API
// snapshot. Callers listing legacy or common tools must still reject duplicate
// names across the complete runtime catalog.
func CanonicalMCPToolName(manifest model.ProductManifest, tool model.Tool) (string, bool) {
	identity, ok := canonicalMCPToolIdentityFor(manifest, tool)
	return identity.name, ok
}

func canonicalMCPToolIdentityFor(manifest model.ProductManifest, tool model.Tool) (canonicalMCPToolIdentity, bool) {
	if tool.State != "published" || tool.UpstreamDrifted {
		return canonicalMCPToolIdentity{}, false
	}
	switch tool.Scope {
	case "":
		if !ValidMCPToolSegment(tool.Namespace) || !ValidMCPToolSegment(tool.Name) {
			return canonicalMCPToolIdentity{}, false
		}
		return canonicalMCPToolIdentity{name: tool.Namespace + "." + tool.Name}, true
	case model.ToolScopeCommon:
		if !ValidMCPToolSegment(tool.Name) {
			return canonicalMCPToolIdentity{}, false
		}
		return canonicalMCPToolIdentity{name: "common." + tool.Name}, true
	case model.ToolScopeAPI:
		if tool.OwnerIntegrationID == "" || !ValidMCPToolSegment(tool.Name) {
			return canonicalMCPToolIdentity{}, false
		}
		familyCounts := make(map[string]int, len(manifest.Integrations))
		for _, integration := range manifest.Integrations {
			familyCounts[integration.FamilyKey]++
		}
		ownerCount := 0
		bindingCount := 0
		nameCount := 0
		familyKey := ""
		identity := canonicalMCPToolIdentity{}
		for _, integration := range manifest.Integrations {
			if integration.ID == tool.OwnerIntegrationID {
				ownerCount++
				familyKey = integration.FamilyKey
			}
			for _, candidate := range integration.Tools {
				if integration.ID == tool.OwnerIntegrationID && candidate.Name == tool.Name {
					nameCount++
				}
				if candidate.ToolID != tool.ID {
					continue
				}
				bindingCount++
				if integration.ID != tool.OwnerIntegrationID || candidate.ToolRevision != tool.Revision || candidate.Namespace != tool.Namespace || candidate.Name != tool.Name || candidate.BackendKind != tool.BackendKind || candidate.AuthorizationPointID == "" || candidate.AuthorizationPointRevision < 1 {
					return canonicalMCPToolIdentity{}, false
				}
				authorizationCount := 0
				for _, point := range integration.AuthorizationPoints {
					if point.ID == candidate.AuthorizationPointID && point.Revision == candidate.AuthorizationPointRevision {
						authorizationCount++
					}
				}
				if authorizationCount != 1 {
					return canonicalMCPToolIdentity{}, false
				}
				identity.authorizationPointID = candidate.AuthorizationPointID
				identity.authorizationPointRevision = candidate.AuthorizationPointRevision
			}
		}
		if ownerCount == 1 && bindingCount == 1 && nameCount == 1 && familyCounts[familyKey] == 1 && ValidMCPToolSegment(familyKey) {
			identity.name = familyKey + ".custom." + tool.Name
			return identity, true
		}
	}
	return canonicalMCPToolIdentity{}, false
}

func currentMCPToolAuthorization(identity canonicalMCPToolIdentity, points []model.AuthorizationPoint, grants []model.GrantDefinition) bool {
	if identity.authorizationPointID == "" || identity.authorizationPointRevision < 1 {
		return false
	}
	activeGrants := make(map[string]bool, len(grants))
	for _, grant := range grants {
		activeGrants[grant.Key] = grant.State == "active"
	}
	matches := 0
	for _, point := range points {
		if point.ID != identity.authorizationPointID || point.Revision != identity.authorizationPointRevision {
			continue
		}
		matches++
		if point.State != "active" {
			return false
		}
		for _, required := range point.RequiredGrants {
			if !activeGrants[required] {
				return false
			}
		}
	}
	return matches == 1
}
