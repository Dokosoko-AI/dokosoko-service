package platform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func validToolBuilderDraft() platform.ToolDraft {
	return platform.ToolDraft{
		Namespace:           "catalog",
		Name:                "get_item",
		Description:         "Get one catalog item.",
		HTTPMethod:          "GET",
		Endpoint:            "https://api.vendor.example/v1/items/{item_id}",
		TimeoutMS:           5000,
		InputSchema:         json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"item_id":{"type":"string"},"note":{"type":"string"}},"required":["item_id"]}`),
		OutputSchema:        json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"found":{"type":"boolean"}},"required":["found"]}`),
		UpstreamAuth:        platform.ToolUpstreamAuth{Type: "none"},
		RequestMapping:      platform.ToolRequestMapping{ParameterLocations: map[string]string{"item_id": "path"}},
		ResponseMapping:     platform.ToolResponseMapping{},
		AuthorizationPolicy: platform.ToolPolicy{RequiredGrants: []string{}, Risk: "low"},
	}
}

func hasToolBuilderFinding(findings []platform.ToolDraftFinding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func TestValidateToolDraftIsLocalAndPreservesOrdinarySchemaFieldNames(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	result, err := service.ValidateToolDraft(context.Background(), "prod_acme", validToolBuilderDraft())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.NetworkCallPerformed {
		t.Fatalf("validation = %#v", result)
	}
	if !bytes.Contains(result.NormalizedDraft.InputSchema, []byte(`"note"`)) {
		t.Fatalf("schema field name was incorrectly treated as credential material: %s", result.NormalizedDraft.InputSchema)
	}
}

func TestValidateToolDraftRejectsCredentialShapedAgentFields(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	for _, test := range []struct {
		name   string
		field  string
		output bool
		code   string
	}{
		{name: "input API key", field: "X-API-Key", code: "sensitive_input_field"},
		{name: "input vendor token", field: "X-Vendor-Token", code: "sensitive_input_field"},
		{name: "output access token", field: "accessToken", output: true, code: "sensitive_output_field"},
	} {
		t.Run(test.name, func(t *testing.T) {
			draft := validToolBuilderDraft()
			schema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"` + test.field + `":{"type":"string"}}}`)
			if test.output {
				draft.OutputSchema = schema
			} else {
				draft.InputSchema = schema
				draft.Endpoint = "https://api.vendor.example/v1/items"
				draft.RequestMapping = platform.ToolRequestMapping{ParameterLocations: map[string]string{}}
			}
			result, err := service.ValidateToolDraft(context.Background(), "prod_acme", draft)
			if err != nil {
				t.Fatal(err)
			}
			if result.Valid || !hasToolBuilderFinding(result.Findings, test.code) {
				t.Fatalf("credential-shaped field was accepted: %#v", result)
			}
		})
	}
}

func TestValidateToolDraftRemovesCredentialMaterialFromEveryResponse(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	draft := validToolBuilderDraft()
	const secretValue = "super-secret-token-value-12345"
	draft.Endpoint = "https://user:" + secretValue + "@api.vendor.example/v1/items/{item_id}?api_key=" + secretValue + "#fragment"
	draft.RequestExample = map[string]any{"item_id": "one", "authorization": "Bearer " + secretValue}
	draft.ResponseExample = map[string]any{"found": true, "access_token": secretValue}
	result, err := service.ValidateToolDraft(context.Background(), "prod_acme", draft)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(secretValue)) || bytes.Contains(encoded, []byte("user:")) {
		t.Fatalf("validation leaked credential material: %s", encoded)
	}
	if result.NormalizedDraft.RequestExample != nil || result.NormalizedDraft.ResponseExample != nil || result.Valid {
		t.Fatalf("unsafe examples or endpoint were accepted: %#v", result)
	}
	if !hasToolBuilderFinding(result.Findings, "credential_material_removed") || !hasToolBuilderFinding(result.Findings, "endpoint_must_be_credential_free") {
		t.Fatalf("missing sanitized findings: %#v", result.Findings)
	}
}

func TestToolDraftCredentialPresenceIsDerivedFromContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := platform.New(store.NewMemory())
	draft := validToolBuilderDraft()
	draft.UpstreamAuth = platform.ToolUpstreamAuth{Type: "bearer"}
	draft.CredentialPresent = true // An inbound claim must not be trusted.

	detached, err := service.ValidateToolDraft(ctx, "prod_acme", draft)
	if err != nil {
		t.Fatal(err)
	}
	if detached.NormalizedDraft.CredentialPresent || detached.Valid || !hasToolBuilderFinding(detached.Findings, "credential_required") {
		t.Fatalf("detached validation trusted credential_present: %#v", detached)
	}

	withSaveIntent, err := service.ValidateToolDraftContext(ctx, "prod_acme", platform.ToolDraftContext{Draft: draft, CredentialWillBeSupplied: true})
	if err != nil {
		t.Fatal(err)
	}
	if !withSaveIntent.NormalizedDraft.CredentialPresent || !withSaveIntent.Valid {
		t.Fatalf("explicit final-save intent was not reflected: %#v", withSaveIntent)
	}
}

func TestToolDraftCredentialPresenceUsesMatchingStoredBaseCredential(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x45}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	draft := validToolBuilderDraft()
	draft.UpstreamAuth = platform.ToolUpstreamAuth{Type: "api_key_header", HeaderName: "X-API-Key"}
	auth, _ := json.Marshal(draft.UpstreamAuth)
	mapping, _ := json.Marshal(draft.RequestMapping)
	responseMapping, _ := json.Marshal(draft.ResponseMapping)
	policy, _ := json.Marshal(draft.AuthorizationPolicy)
	tool, err := service.CreateTool(ctx, platform.ToolInput{
		ProductID: "prod_acme", Namespace: draft.Namespace, Name: draft.Name, Description: draft.Description,
		InputSchema: draft.InputSchema, OutputSchema: draft.OutputSchema, Endpoint: draft.Endpoint, HTTPMethod: draft.HTTPMethod,
		UpstreamAuth: auth, Credential: "stored-api-key-secret", RequestMapping: mapping, ResponseMapping: responseMapping,
		AuthorizationPolicy: policy, TimeoutMS: draft.TimeoutMS,
	}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.ValidateToolDraftContext(ctx, tool.ProductID, platform.ToolDraftContext{Draft: draft, BaseToolID: tool.ID, BaseRevision: tool.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if !result.NormalizedDraft.CredentialPresent || !result.Valid {
		t.Fatalf("stored matching credential was not derived: %#v", result)
	}

	draft.UpstreamAuth = platform.ToolUpstreamAuth{Type: "api_key_header", HeaderName: "X-Different-Key"}
	changedConfig, err := service.ValidateToolDraftContext(ctx, tool.ProductID, platform.ToolDraftContext{Draft: draft, BaseToolID: tool.ID, BaseRevision: tool.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if changedConfig.NormalizedDraft.CredentialPresent || changedConfig.Valid {
		t.Fatalf("credential was reused after same-type auth configuration changed: %#v", changedConfig)
	}

	draft.UpstreamAuth = platform.ToolUpstreamAuth{Type: "api_key_header", HeaderName: "X-API-Key"}
	draft.Endpoint = "https://other.vendor.example/v1/items/{item_id}"
	changedOrigin, err := service.ValidateToolDraftContext(ctx, tool.ProductID, platform.ToolDraftContext{Draft: draft, BaseToolID: tool.ID, BaseRevision: tool.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if changedOrigin.NormalizedDraft.CredentialPresent || changedOrigin.Valid {
		t.Fatalf("credential was reused after endpoint origin changed: %#v", changedOrigin)
	}

	draft.Endpoint = tool.BaseURL
	draft.UpstreamAuth = platform.ToolUpstreamAuth{Type: "basic", Username: "service-user"}
	changedAuth, err := service.ValidateToolDraftContext(ctx, tool.ProductID, platform.ToolDraftContext{Draft: draft, BaseToolID: tool.ID, BaseRevision: tool.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if changedAuth.NormalizedDraft.CredentialPresent || changedAuth.Valid {
		t.Fatalf("credential from a different auth mode was reused: %#v", changedAuth)
	}
}

func TestImportCurlStripsCredentialValuesAndInfersContract(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	base := validToolBuilderDraft()
	base.Name, base.Description, base.Endpoint = "", "", ""
	base.UpstreamAuth = platform.ToolUpstreamAuth{Type: "delegated_oauth"}
	const secretValue = "super-secret-token-value-12345"
	input := platform.ToolDraftImportInput{
		ToolDraftContext: platform.ToolDraftContext{Draft: base},
		Source:           platform.ToolDraftImportSource{Kind: "curl", Value: `curl -X POST 'https://api.vendor.example/v1/items?user-id=2' -H 'Authorization: Bearer ` + secretValue + `' -H 'Content-Type: application/json' --data-raw '{"title":"example"}'`},
	}
	result, err := service.ImportToolDraft(context.Background(), "prod_acme", input, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	candidate := result.Candidates[0].Draft
	if candidate.Endpoint != "https://api.vendor.example/v1/items" || candidate.HTTPMethod != "POST" || candidate.UpstreamAuth.Type != "bearer" || candidate.CredentialPresent {
		t.Fatalf("unexpected imported candidate: %#v", candidate)
	}
	if candidate.RequestMapping.ParameterLocations["user-id"] != "query" || candidate.RequestMapping.ParameterLocations["user_id"] != "" || candidate.RequestMapping.ParameterLocations["title"] != "body" {
		t.Fatalf("request mapping = %#v", candidate.RequestMapping)
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte(secretValue)) || !hasToolBuilderFinding(result.Findings, "credential_material_not_imported") {
		t.Fatalf("import leaked a credential or omitted warning: %s", encoded)
	}
	input.CredentialWillBeSupplied = true
	withSaveIntent, err := service.ImportToolDraft(context.Background(), "prod_acme", input, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(withSaveIntent.Candidates) != 1 || !withSaveIntent.Candidates[0].Draft.CredentialPresent {
		t.Fatalf("import candidate did not use context-level save intent: %#v", withSaveIntent.Candidates)
	}
}

func TestImportCurlInfersFixedVendorAuthorizationScheme(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	base := validToolBuilderDraft()
	base.Name, base.Description, base.Endpoint = "", "", ""
	const secretValue = "vendor-secret-token-value-12345"
	result, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{
		ToolDraftContext: platform.ToolDraftContext{Draft: base},
		Source:           platform.ToolDraftImportSource{Kind: "curl", Value: `curl 'https://api.vendor.example/v1/me' -H 'Authorization: SSWS ` + secretValue + `'`},
	}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Draft.UpstreamAuth.Type != "authorization_scheme" || result.Candidates[0].Draft.UpstreamAuth.Scheme != "SSWS" {
		t.Fatalf("authorization scheme candidate = %#v", result.Candidates)
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte(secretValue)) || !hasToolBuilderFinding(result.Findings, "credential_material_not_imported") {
		t.Fatalf("import leaked credential or omitted warning: %s", encoded)
	}
}

func TestImportCurlSupportsLongOptionEqualsFormsWithoutLeakingCredentials(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	base := validToolBuilderDraft()
	base.Name, base.Description, base.Endpoint = "", "", ""

	t.Run("authorization header and body", func(t *testing.T) {
		const secretValue = "equals-form-bearer-secret-12345"
		result, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{
			ToolDraftContext: platform.ToolDraftContext{Draft: base},
			Source:           platform.ToolDraftImportSource{Kind: "curl", Value: `curl --silent --show-error --request=POST --url='https://api.vendor.example/v1/items?limit=2' --header='Authorization: Bearer ` + secretValue + `' --header='Content-Type: application/json' --data-raw='{"title":"example"}'`},
		}, platform.Actor{ID: "root"})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Candidates) != 1 {
			t.Fatalf("candidates = %#v", result.Candidates)
		}
		candidate := result.Candidates[0].Draft
		if candidate.Endpoint != "https://api.vendor.example/v1/items" || candidate.HTTPMethod != "POST" || candidate.UpstreamAuth.Type != "bearer" || candidate.RequestMapping.ParameterLocations["limit"] != "query" || candidate.RequestMapping.ParameterLocations["title"] != "body" {
			t.Fatalf("equals-form candidate = %#v", candidate)
		}
		encoded, _ := json.Marshal(result)
		if bytes.Contains(encoded, []byte(secretValue)) || !hasToolBuilderFinding(result.Findings, "credential_material_not_imported") {
			t.Fatalf("equals-form import leaked credential or omitted warning: %s", encoded)
		}
	})

	t.Run("basic user", func(t *testing.T) {
		const secretValue = "equals-form-basic-secret-12345"
		result, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{
			ToolDraftContext: platform.ToolDraftContext{Draft: base},
			Source:           platform.ToolDraftImportSource{Kind: "curl", Value: `curl --basic --user='service-user:` + secretValue + `' --url=https://api.vendor.example/v1/me`},
		}, platform.Actor{ID: "root"})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Candidates) != 1 || result.Candidates[0].Draft.UpstreamAuth.Type != "basic" || result.Candidates[0].Draft.UpstreamAuth.Username != "service-user" {
			t.Fatalf("basic equals-form candidate = %#v", result.Candidates)
		}
		encoded, _ := json.Marshal(result)
		if bytes.Contains(encoded, []byte(secretValue)) || !hasToolBuilderFinding(result.Findings, "credential_material_not_imported") {
			t.Fatalf("basic equals-form import leaked credential or omitted warning: %s", encoded)
		}
	})

	t.Run("oauth bearer", func(t *testing.T) {
		const secretValue = "equals-form-oauth-secret-12345"
		result, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{
			ToolDraftContext: platform.ToolDraftContext{Draft: base},
			Source:           platform.ToolDraftImportSource{Kind: "curl", Value: `curl --oauth2-bearer='` + secretValue + `' --url=https://api.vendor.example/v1/me`},
		}, platform.Actor{ID: "root"})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Candidates) != 1 || result.Candidates[0].Draft.UpstreamAuth.Type != "bearer" {
			t.Fatalf("OAuth equals-form candidate = %#v", result.Candidates)
		}
		encoded, _ := json.Marshal(result)
		if bytes.Contains(encoded, []byte(secretValue)) || !hasToolBuilderFinding(result.Findings, "credential_material_not_imported") {
			t.Fatalf("OAuth equals-form import leaked credential or omitted warning: %s", encoded)
		}
	})
}

func TestImportCurlRejectsUnsupportedOptionsWithoutLeakingAttachedValues(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	base := validToolBuilderDraft()
	base.Name, base.Description, base.Endpoint = "", "", ""
	const secretValue = "unsupported-option-secret-12345"
	tests := []struct {
		name    string
		command string
	}{
		{name: "digest", command: `curl --digest --user='user:` + secretValue + `' https://api.vendor.example/v1/me`},
		{name: "ntlm", command: `curl --ntlm --user='user:` + secretValue + `' https://api.vendor.example/v1/me`},
		{name: "negotiate", command: `curl --negotiate --user='user:` + secretValue + `' https://api.vendor.example/v1/me`},
		{name: "anyauth", command: `curl --anyauth --user='user:` + secretValue + `' https://api.vendor.example/v1/me`},
		{name: "AWS SigV4", command: `curl --aws-sigv4='aws:amz:region:service:` + secretValue + `' https://api.vendor.example/v1/me`},
		{name: "proxy auth", command: `curl --proxy-user='user:` + secretValue + `' https://api.vendor.example/v1/me`},
		{name: "unknown long option", command: `curl --future-auth='` + secretValue + `' https://api.vendor.example/v1/me`},
		{name: "unknown short option", command: `curl -Z` + secretValue + ` https://api.vendor.example/v1/me`},
		{name: "redirect semantics", command: `curl --location https://api.vendor.example/v1/me`},
		{name: "option cannot be swallowed as header value", command: `curl --header --digest https://api.vendor.example/v1/me`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{
				ToolDraftContext: platform.ToolDraftContext{Draft: base},
				Source:           platform.ToolDraftImportSource{Kind: "curl", Value: test.command},
			}, platform.Actor{ID: "root"})
			if !errors.Is(err, platform.ErrToolBuilderInvalidInput) {
				t.Fatalf("error = %v", err)
			}
			if strings.Contains(err.Error(), secretValue) {
				t.Fatalf("error leaked attached option value: %v", err)
			}
		})
	}
}

func TestImportCurlRejectsAmbiguousOrUnrepresentableRequestShapes(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	base := validToolBuilderDraft()
	base.Name, base.Description, base.Endpoint = "", "", ""
	tests := []struct {
		name    string
		command string
	}{
		{name: "multiple URLs", command: `curl https://api.vendor.example/one https://api.vendor.example/two`},
		{name: "multiple bodies", command: `curl --data=one --data=two https://api.vendor.example/items`},
		{name: "file-backed header", command: `curl --header=@headers.txt https://api.vendor.example/items`},
		{name: "header without value separator", command: `curl --header=Authorization https://api.vendor.example/items`},
		{name: "header with line break", command: "curl --header='X-Safe: one\nX-Injected: two' https://api.vendor.example/items"},
		{name: "file-backed data", command: `curl --data=@body.json https://api.vendor.example/items`},
		{name: "file-backed urlencoded data", command: `curl --data-urlencode=payload@body.json https://api.vendor.example/items`},
		{name: "inline value on valueless flag", command: `curl --silent=true https://api.vendor.example/items`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{
				ToolDraftContext: platform.ToolDraftContext{Draft: base},
				Source:           platform.ToolDraftImportSource{Kind: "curl", Value: test.command},
			}, platform.Actor{ID: "root"})
			if !errors.Is(err, platform.ErrToolBuilderInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestImportPostmanAndOpenAPIYAMLProduceCandidates(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	base := validToolBuilderDraft()
	base.Name, base.Description, base.Endpoint = "", "", ""
	base.UpstreamAuth = platform.ToolUpstreamAuth{Type: "delegated_oauth"}
	postman := `{"info":{"name":"Example","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json","extra":"ignored"},"item":[{"name":"List items","request":{"method":"GET","header":[],"url":{"raw":"https://api.vendor.example/v1/items"},"auth":{"type":"noauth"}}}]}`
	postmanResult, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{ToolDraftContext: platform.ToolDraftContext{Draft: base}, Source: platform.ToolDraftImportSource{Kind: "postman", Value: postman}}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(postmanResult.Candidates) != 1 || postmanResult.Candidates[0].Draft.Name != "list_items" {
		t.Fatalf("postman candidates = %#v", postmanResult.Candidates)
	}

	openapi := `openapi: 3.0.3
servers:
  - url: https://api.vendor.example
paths:
  /v1/items/{item_id}:
    get:
      operationId: getItem
      summary: Get an item
      parameters:
        - name: item_id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  found:
                    type: boolean
                required: [found]
`
	openAPIResult, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{ToolDraftContext: platform.ToolDraftContext{Draft: base}, Source: platform.ToolDraftImportSource{Kind: "openapi_document", Value: openapi}}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(openAPIResult.Candidates) != 1 {
		t.Fatalf("openapi candidates = %#v", openAPIResult.Candidates)
	}
	candidate := openAPIResult.Candidates[0].Draft
	if candidate.Name != "getitem" || candidate.RequestMapping.ParameterLocations["item_id"] != "path" || candidate.Endpoint != "https://api.vendor.example/v1/items/{item_id}" || candidate.UpstreamAuth.Type != "none" {
		t.Fatalf("openapi candidate = %#v", candidate)
	}
}

func TestImportPreservesPostmanAndOpenAPIAuthenticationSemantics(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	base := validToolBuilderDraft()
	base.Name, base.Description, base.Endpoint = "", "", ""

	postman := `{"info":{"name":"Example","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},"item":[{"name":"List items","request":{"method":"GET","header":[],"url":{"raw":"https://api.vendor.example/v1/items"},"auth":{"type":"apikey","apikey":[{"key":"key","value":"vendor-key"},{"key":"in","value":"query"}]}}}]}`
	postmanResult, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{ToolDraftContext: platform.ToolDraftContext{Draft: base}, Source: platform.ToolDraftImportSource{Kind: "postman", Value: postman}}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(postmanResult.Candidates) != 1 || postmanResult.Candidates[0].Draft.UpstreamAuth.Type != "api_key_query" || postmanResult.Candidates[0].Draft.UpstreamAuth.QueryName != "vendor-key" {
		t.Fatalf("postman API-key semantics were not preserved: %#v", postmanResult.Candidates)
	}

	openapi := `openapi: 3.0.3
servers:
  - url: https://api.vendor.example
components:
  securitySchemes:
    machine:
      type: oauth2
      flows:
        clientCredentials:
          tokenUrl: https://identity.vendor.example/oauth/token
          scopes:
            items.read: Read items
            items.write: Write items
paths:
  /v1/items:
    get:
      security:
        - machine: [items.read]
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties: {}
`
	openAPIResult, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{ToolDraftContext: platform.ToolDraftContext{Draft: base}, Source: platform.ToolDraftImportSource{Kind: "openapi_document", Value: openapi}}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(openAPIResult.Candidates) != 1 {
		t.Fatalf("OpenAPI candidates = %#v", openAPIResult.Candidates)
	}
	auth := openAPIResult.Candidates[0].Draft.UpstreamAuth
	if auth.Type != "oauth_client_credentials" || auth.TokenURL != "https://identity.vendor.example/oauth/token" || !reflect.DeepEqual(auth.Scopes, []string{"items.read"}) || !hasToolBuilderFinding(openAPIResult.Candidates[0].Findings, "invalid_upstream_auth") {
		t.Fatalf("OpenAPI client-credentials metadata was collapsed: auth=%#v findings=%#v", auth, openAPIResult.Candidates[0].Findings)
	}

	compound := strings.Replace(openapi, "- machine: [items.read]", "- machine: [items.read]\n          another: []", 1)
	compound = strings.Replace(compound, "machine:\n      type: oauth2", "another:\n      type: apiKey\n      in: header\n      name: X-Other-Key\n    machine:\n      type: oauth2", 1)
	compoundResult, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{ToolDraftContext: platform.ToolDraftContext{Draft: base}, Source: platform.ToolDraftImportSource{Kind: "openapi_document", Value: compound}}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(compoundResult.Candidates) != 1 || compoundResult.Candidates[0].Valid || !hasToolBuilderFinding(compoundResult.Candidates[0].Findings, "invalid_upstream_auth") {
		t.Fatalf("compound OpenAPI security was silently weakened: %#v", compoundResult.Candidates)
	}
}

func TestImportHonorsPostmanAuthenticationInheritanceAndOverrides(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	base := validToolBuilderDraft()
	base.Name, base.Description, base.Endpoint = "", "", ""
	postman := `{
  "info":{"name":"Auth inheritance","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
  "auth":{"type":"apikey","apikey":[{"key":"key","value":"catalog_key"},{"key":"in","value":"query"}]},
  "item":[
    {"name":"Collection inherited","request":{"method":"GET","url":{"raw":"https://api.vendor.example/collection"}}},
    {"name":"Folder","auth":{"type":"basic","basic":[{"key":"username","value":"folder-user"}]},"item":[
      {"name":"Folder inherited","request":{"method":"GET","url":{"raw":"https://api.vendor.example/folder"}}},
      {"name":"Explicit no auth","request":{"method":"GET","url":{"raw":"https://api.vendor.example/public"},"auth":{"type":"noauth"}}},
      {"name":"Request override","request":{"method":"GET","url":{"raw":"https://api.vendor.example/override"},"auth":{"type":"apikey","apikey":[{"key":"key","value":"X-Request-Key"},{"key":"in","value":"header"}]}}},
      {"name":"Unsupported digest","request":{"method":"GET","url":{"raw":"https://api.vendor.example/digest"},"auth":{"type":"digest"}}}
    ]}
  ]
}`
	result, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{ToolDraftContext: platform.ToolDraftContext{Draft: base}, Source: platform.ToolDraftImportSource{Kind: "postman", Value: postman}}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 5 {
		t.Fatalf("Postman candidates=%#v", result.Candidates)
	}
	candidates := make(map[string]platform.ToolDraftImportCandidate, len(result.Candidates))
	for _, candidate := range result.Candidates {
		candidates[candidate.Draft.Name] = candidate
	}
	if auth := candidates["collection_inherited"].Draft.UpstreamAuth; auth.Type != "api_key_query" || auth.QueryName != "catalog_key" {
		t.Fatalf("collection auth=%#v", auth)
	}
	if auth := candidates["folder_inherited"].Draft.UpstreamAuth; auth.Type != "basic" || auth.Username != "folder-user" {
		t.Fatalf("folder auth=%#v", auth)
	}
	if auth := candidates["explicit_no_auth"].Draft.UpstreamAuth; auth.Type != "none" {
		t.Fatalf("noauth override=%#v", auth)
	}
	if auth := candidates["request_override"].Draft.UpstreamAuth; auth.Type != "api_key_header" || auth.HeaderName != "X-Request-Key" {
		t.Fatalf("request override=%#v", auth)
	}
	unsupported := candidates["unsupported_digest"]
	if unsupported.Valid || unsupported.Draft.UpstreamAuth.Type == "delegated_oauth" || !hasToolBuilderFinding(unsupported.Findings, "invalid_upstream_auth") {
		t.Fatalf("unsupported auth silently weakened=%#v", unsupported)
	}
}

func TestImportRejectsUnsupportedOrAmbiguousOpenAPIAuthentication(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	base := validToolBuilderDraft()
	base.Name, base.Description, base.Endpoint = "", "", ""
	tests := []struct {
		name   string
		scheme string
	}{
		{name: "cookie api key", scheme: "type: apiKey\n      in: cookie\n      name: session"},
		{name: "missing HTTP scheme", scheme: "type: http"},
		{name: "challenge HTTP scheme", scheme: "type: http\n      scheme: digest"},
		{name: "multiple OAuth flows", scheme: "type: oauth2\n      flows:\n        clientCredentials:\n          tokenUrl: https://identity.vendor.example/token\n          scopes: {items.read: Read}\n        authorizationCode:\n          authorizationUrl: https://identity.vendor.example/authorize\n          tokenUrl: https://identity.vendor.example/token\n          scopes: {items.read: Read}"},
		{name: "password OAuth flow", scheme: "type: oauth2\n      flows:\n        password:\n          tokenUrl: https://identity.vendor.example/token\n          scopes: {items.read: Read}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := "openapi: 3.0.3\nservers:\n  - url: https://api.vendor.example\ncomponents:\n  securitySchemes:\n    selected:\n      " + test.scheme + "\npaths:\n  /items:\n    get:\n      security:\n        - selected: [items.read]\n      responses:\n        '200': {description: ok}\n"
			result, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{ToolDraftContext: platform.ToolDraftContext{Draft: base}, Source: platform.ToolDraftImportSource{Kind: "openapi_document", Value: document}}, platform.Actor{ID: "root"})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Candidates) != 1 || result.Candidates[0].Valid || result.Candidates[0].Draft.UpstreamAuth.Type == "delegated_oauth" || !hasToolBuilderFinding(result.Candidates[0].Findings, "invalid_upstream_auth") {
				t.Fatalf("unsupported OpenAPI auth was weakened: %#v", result.Candidates)
			}
		})
	}
}

func TestImportClassifiesSingleInteractiveOpenAPIOAuthFlowAsDelegated(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	base := validToolBuilderDraft()
	base.Name, base.Description, base.Endpoint = "", "", ""
	flows := []struct {
		name string
		body string
	}{
		{name: "authorizationCode", body: "authorizationCode:\n          authorizationUrl: https://identity.vendor.example/authorize\n          tokenUrl: https://identity.vendor.example/token\n          scopes: {items.read: Read}"},
		{name: "implicit", body: "implicit:\n          authorizationUrl: https://identity.vendor.example/authorize\n          scopes: {items.read: Read}"},
	}
	for _, flow := range flows {
		t.Run(flow.name, func(t *testing.T) {
			document := "openapi: 3.0.3\nservers:\n  - url: https://api.vendor.example\ncomponents:\n  securitySchemes:\n    selected:\n      type: oauth2\n      flows:\n        " + flow.body + "\npaths:\n  /items:\n    get:\n      security:\n        - selected: [items.read]\n      responses:\n        '200': {description: ok}\n"
			result, err := service.ImportToolDraft(context.Background(), "prod_acme", platform.ToolDraftImportInput{ToolDraftContext: platform.ToolDraftContext{Draft: base}, Source: platform.ToolDraftImportSource{Kind: "openapi_document", Value: document}}, platform.Actor{ID: "root"})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Candidates) != 1 || result.Candidates[0].Draft.UpstreamAuth.Type != "delegated_oauth" {
				t.Fatalf("interactive OAuth flow=%#v", result.Candidates)
			}
		})
	}
}

type toolBuilderAIDoer struct {
	response string
	request  []byte
}

func (d *toolBuilderAIDoer) Do(request *http.Request) (*http.Response, error) {
	d.request, _ = io.ReadAll(request.Body)
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(d.response)), Request: request}, nil
}

func TestProposeToolDraftUsesSanitizedStructuredAnalysisOutput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x65}, 32))
	if err != nil {
		t.Fatal(err)
	}
	candidate := validToolBuilderDraft()
	candidate.Description = "AI proposed catalog lookup."
	draftJSON, _ := json.Marshal(candidate)
	structured, _ := json.Marshal(map[string]any{"summary": "Updated the description.", "reply": "Review the candidate before saving.", "draft_json": string(draftJSON)})
	providerBody, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(structured)}}}, "usage": map[string]any{"total_tokens": 20}})
	doer := &toolBuilderAIDoer{response: string(providerBody)}
	service := platform.NewWithVaultAndProductBuilderDoer(memory, vault, doer)
	actor := platform.Actor{ID: "root", RequestID: "req-builder"}
	connection, err := service.SaveAIProviderConnection(ctx, platform.AIProviderConnectionInput{OrganisationID: "org_acme", DeploymentID: "prod_acme", Provider: "openai-compatible", Endpoint: "https://llm.example.com", Credential: "provider-secret", Enabled: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveAIWorkloadProfile(ctx, platform.AIWorkloadProfileInput{OrganisationID: "org_acme", ProductID: "prod_acme", Workload: "analysis", ProviderConnectionID: connection.ID, Model: "analysis-model", MaxInputTokens: 8192, MaxOutputTokens: 4096, DailyTokenBudget: 20000, Enabled: true}, actor); err != nil {
		t.Fatal(err)
	}
	history := []platform.ToolBuilderChatMessage{
		{Role: "user", Content: "Should this remain a read-only lookup?"},
		{Role: "assistant", Content: "Yes. The current draft uses GET and has low risk."},
	}
	proposal, err := service.ProposeToolDraft(ctx, "prod_acme", platform.ToolDraftProposalInput{ToolDraftContext: platform.ToolDraftContext{Draft: validToolBuilderDraft()}, Instruction: "Improve the description.", History: history}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Draft.Description != candidate.Description || proposal.ProposalID == "" || proposal.BaseFingerprint == "" || proposal.Reply != "Review the candidate before saving." {
		t.Fatalf("proposal = %#v", proposal)
	}
	if bytes.Contains(doer.request, []byte("provider-secret")) {
		t.Fatalf("provider credential leaked into prompt body: %s", doer.request)
	}
	for _, expected := range []string{"Should this remain a read-only lookup?", "The current draft uses GET and has low risk."} {
		if !bytes.Contains(doer.request, []byte(expected)) {
			t.Fatalf("bounded conversation history was not sent to the analysis workload: %s", doer.request)
		}
	}

	spoofed := validToolBuilderDraft()
	spoofed.UpstreamAuth = platform.ToolUpstreamAuth{Type: "bearer"}
	spoofed.CredentialPresent = true
	candidate.UpstreamAuth = platform.ToolUpstreamAuth{Type: "bearer"}
	candidate.CredentialPresent = true
	draftJSON, _ = json.Marshal(candidate)
	structured, _ = json.Marshal(map[string]any{"summary": "Kept bearer authentication.", "reply": "A credential is still required.", "draft_json": string(draftJSON)})
	providerBody, _ = json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(structured)}}}, "usage": map[string]any{"total_tokens": 20}})
	doer.response = string(providerBody)
	spoofedProposal, err := service.ProposeToolDraft(ctx, "prod_acme", platform.ToolDraftProposalInput{ToolDraftContext: platform.ToolDraftContext{Draft: spoofed}, Instruction: "Keep bearer authentication."}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if spoofedProposal.Draft.CredentialPresent || spoofedProposal.Valid || !hasToolBuilderFinding(spoofedProposal.Findings, "credential_required") {
		t.Fatalf("AI or inbound draft changed derived credential presence: %#v", spoofedProposal)
	}
	if bytes.Contains(doer.request, []byte(`credential_present\":true`)) {
		t.Fatalf("spoofed credential presence reached the AI prompt: %s", doer.request)
	}
	if _, err := service.ProposeToolDraft(ctx, "prod_acme", platform.ToolDraftProposalInput{ToolDraftContext: platform.ToolDraftContext{Draft: validToolBuilderDraft()}, Instruction: "Use Authorization: Bearer super-secret-token-value"}, actor); !errors.Is(err, platform.ErrToolBuilderUnsafeInput) {
		t.Fatalf("secret instruction error = %v", err)
	}
}
