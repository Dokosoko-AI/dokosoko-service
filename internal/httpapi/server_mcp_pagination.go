package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"

	"github.com/dokosoko/dokosoko-service/internal/identity"
)

const mcpResourcePageItems = 32
const mcpResourcePageBytes = 64 << 10
const mcpCursorMaxBytes = 512

var errMCPCursor = errors.New("Invalid or expired cursor; restart this list without a cursor")
var errMCPResourceSize = errors.New("A resource descriptor exceeds the discovery page budget")

type mcpListCursor struct {
	Version  int    `json:"v"`
	Snapshot string `json:"s"`
	Offset   int    `json:"o"`
}

// Only the digest leaves the server. Bind a continuation to the request scope,
// but exclude rotating evaluation IDs/timestamps and upstream credentials.
// Every page still resolves current publication and authorization state first;
// the cursor is a consistency token, never an authorization credential.
func mcpListScope(r *http.Request, deploymentID string, public bool, revision int64) any {
	return mcpCatalogScope(r.Context(), deploymentID, public, revision, "resources/list")
}

func mcpCatalogScope(ctx context.Context, deploymentID string, public bool, revision int64, method string) any {
	principal, _ := ctx.Value(principalKey).(identity.Principal)
	return struct {
		Deployment                                                                        string
		Public                                                                            bool
		Revision                                                                          int64
		Method                                                                            string
		Issuer, Subject, Client, Account, ExternalAccount, Installation, Policy, Resource string
		Grants                                                                            map[string]bool
		Scopes                                                                            []string
	}{deploymentID, public, revision, method, principal.Issuer, principal.Subject, principal.ClientID, principal.CustomerAccountID, principal.ExternalCustomerID, principal.InstallationID, principal.PolicyVersion, principal.Resource, principal.Grants, sortedMCPScopes(principal.Scopes)}
}

func sortedMCPScopes(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func mcpResourcePage(params json.RawMessage, resources []map[string]any, scope any) ([]map[string]any, string, error) {
	return mcpCatalogPage(params, resources, scope, "uri", mcpResourcePageItems, mcpResourcePageBytes)
}

func mcpCatalogPage(params json.RawMessage, resources []map[string]any, scope any, key string, itemLimit, byteLimit int) ([]map[string]any, string, error) {
	return mcpOrderedCatalogPage(params, resources, scope, itemLimit, byteLimit, func(i, j int) bool { return resources[i][key].(string) < resources[j][key].(string) })
}

func mcpOrderedCatalogPage(params json.RawMessage, resources []map[string]any, scope any, itemLimit, byteLimit int, less func(int, int) bool) ([]map[string]any, string, error) {
	// The order and complete authorized catalog form the snapshot. A changed
	// revision, publication, grant set or descriptor invalidates a continuation.
	sort.Slice(resources, less)
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	if err := encoder.Encode(scope); err != nil {
		return nil, "", err
	}
	if err := encoder.Encode(resources); err != nil {
		return nil, "", err
	}
	snapshot := hex.EncodeToString(hash.Sum(nil))
	var values map[string]json.RawMessage
	if json.Unmarshal(params, &values) != nil {
		return nil, "", errMCPCursor
	}
	offset := 0
	if raw, exists := values["cursor"]; exists {
		var encoded string
		if json.Unmarshal(raw, &encoded) != nil || len(encoded) == 0 || len(encoded) > mcpCursorMaxBytes {
			return nil, "", errMCPCursor
		}
		data, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, "", errMCPCursor
		}
		var cursor mcpListCursor
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&cursor) != nil || cursor.Version != 1 || cursor.Snapshot != snapshot || cursor.Offset < 1 || cursor.Offset >= len(resources) {
			return nil, "", errMCPCursor
		}
		canonical, _ := json.Marshal(cursor)
		if !bytes.Equal(data, canonical) {
			return nil, "", errMCPCursor
		}
		offset = cursor.Offset
	}
	end, size := offset, 2 // JSON array brackets.
	for end < len(resources) && end-offset < itemLimit {
		encoded, err := json.Marshal(resources[end])
		if err != nil {
			return nil, "", err
		}
		if len(encoded)+2 > byteLimit {
			return nil, "", errMCPResourceSize
		}
		extra := len(encoded)
		if end > offset {
			extra++
		}
		if size+extra > byteLimit {
			break
		}
		size += extra
		end++
	}
	next := ""
	if end < len(resources) {
		data, _ := json.Marshal(mcpListCursor{Version: 1, Snapshot: snapshot, Offset: end})
		next = base64.RawURLEncoding.EncodeToString(data)
	}
	return resources[offset:end], next, nil
}
