package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (r *Runtime) executeAuthorized(ctx context.Context, productID, fullName string, tool model.Tool, arguments map[string]any, principal Principal) (any, error) {
	return r.executeAuthorizedTraced(ctx, productID, fullName, tool, arguments, principal, nil, true)
}

func (r *Runtime) executeHTTPAuthorized(ctx context.Context, tool model.Tool, arguments map[string]any, principal Principal) (any, error) {
	return r.executeHTTPAuthorizedTraced(ctx, tool.ProductID, tool.Namespace+"."+tool.Name, tool, arguments, principal, nil, true)
}

func tracePhase(category, existing string) string {
	if category == "upstream_authentication_failed" && existing == "token_exchange" {
		return existing
	}
	switch category {
	case "upstream_authentication_failed":
		return "auth"
	case "transport_failed":
		return "transport"
	case "upstream_status":
		return "upstream_status"
	case "response_read_failed", "response_invalid":
		return "json"
	case "response_mapping_failed":
		return "response_mapping"
	case "response_schema_mismatch":
		return "output_schema"
	case "success":
		return "success"
	default:
		return "preflight"
	}
}

func (r *Runtime) executeAuthorizedTraced(ctx context.Context, productID, fullName string, tool model.Tool, arguments map[string]any, principal Principal, trace *executionTrace, recordAudit bool) (returnValue any, returnErr error) {
	if tool.BackendKind == "http" {
		return r.executeHTTPAuthorizedTraced(ctx, productID, fullName, tool, arguments, principal, trace, recordAudit)
	}
	executor, ok := r.executor(tool)
	if !ok {
		return nil, ErrDenied
	}
	return r.executeRegisteredBackend(ctx, productID, fullName, tool, arguments, principal, executor, trace, recordAudit)
}

func (r *Runtime) executeHTTPAuthorizedTraced(ctx context.Context, productID, fullName string, tool model.Tool, arguments map[string]any, principal Principal, trace *executionTrace, recordAudit bool) (returnValue any, returnErr error) {
	var err error
	tool, err = prepareRuntimeTool(tool, principal.EnvironmentID)
	if err != nil {
		return nil, err
	}
	if SchemaContainsSensitiveFields(tool.InputSchema) || SchemaContainsSensitiveFields(tool.OutputSchema) || ValueContainsSensitiveFields(arguments) {
		return nil, ErrDenied
	}
	method := strings.ToUpper(tool.HTTPMethod)
	var policy struct {
		IdempotencyRequired bool `json:"idempotency_required"`
	}
	if json.Unmarshal(tool.AuthorizationPolicy, &policy) != nil {
		return nil, ErrDenied
	}
	if principal.IdempotencyKey != "" && !validIdempotencyKey(principal.IdempotencyKey) {
		return nil, ErrInvalidIdempotencyKey
	}
	if method != http.MethodGet && policy.IdempotencyRequired && !validIdempotencyKey(principal.IdempotencyKey) {
		return nil, ErrInvalidIdempotencyKey
	}
	auditCategory, auditOutcome, auditStatusCode := "preflight_failed", "failure", 0
	defer func() {
		if trace != nil {
			trace.Category = auditCategory
			trace.Phase = tracePhase(auditCategory, trace.Phase)
			trace.StatusCode = auditStatusCode
		}
		if !recordAudit {
			return
		}
		current := map[string]any{"tool": fullName, "category": auditCategory}
		if auditStatusCode != 0 {
			current["status_code"] = auditStatusCode
		}
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if auditErr := r.store.AppendAudit(auditCtx, model.AuditEvent{ID: auditID(), OrganisationID: tool.OrganisationID, ProductID: productID, ActorID: principal.Subject, Action: "tool.executed", TargetType: "tool", TargetID: tool.ID, Current: current, RequestID: principal.RequestID, Outcome: auditOutcome, CreatedAt: time.Now().UTC()}); auditErr != nil {
			returnValue = nil
			returnErr = errors.Join(returnErr, fmt.Errorf("append tool execution audit: %w", auditErr))
		}
	}()
	auditCategory = "rate_limited"
	if !r.acquireUpstreamSlot(productID, tool) {
		return nil, ErrRateLimited
	}
	defer r.releaseUpstreamSlot(productID, tool)
	if !r.allowUpstreamConnection(productID, tool) {
		return nil, ErrRateLimited
	}
	auditCategory = "unsafe_destination"
	parsed, address, err := r.safeDestination(ctx, tool.BaseURL)
	if err != nil {
		return nil, err
	}
	auditCategory = "configuration_invalid"
	auth, err := toolUpstreamAuth(tool)
	if err != nil {
		return nil, err
	}
	mapping, err := toolRequestMapping(tool)
	if err != nil {
		return nil, err
	}
	auditCategory = "request_mapping_failed"
	query := parsed.Query()
	headers := make(http.Header)
	bodyArguments := make(map[string]any)
	for key, value := range arguments {
		location := mapping.ParameterLocations[key]
		if location == "" {
			if method == http.MethodGet {
				location = "query"
			} else {
				location = "body"
			}
		}
		switch location {
		case "path":
			parsed.Path, err = applyPathArgument(parsed.Path, key, value)
			if err != nil {
				return nil, err
			}
		case "query":
			text, scalarErr := requestScalarText(value)
			if scalarErr != nil {
				return nil, scalarErr
			}
			query.Set(key, text)
		case "header":
			headerName := strings.ReplaceAll(key, "_", "-")
			headerValue, scalarErr := requestScalarText(value)
			if scalarErr != nil {
				return nil, scalarErr
			}
			if strings.ContainsAny(headerValue, "\r\n\x00") {
				return nil, errors.New("mapped request header value is invalid")
			}
			headers.Set(headerName, headerValue)
		case "body":
			if method == http.MethodGet {
				return nil, errors.New("GET tools cannot send a request body")
			}
			bodyArguments[key] = value
		default:
			return nil, ErrDenied
		}
	}
	if strings.ContainsAny(parsed.Path, "{}") {
		return nil, errors.New("required path argument is missing")
	}
	parsed.RawQuery = query.Encode()
	var body io.Reader
	if method != http.MethodGet && len(bodyArguments) > 0 {
		encoded, _ := json.Marshal(bodyArguments)
		body = bytes.NewReader(encoded)
	}
	if auth.Type == "api_key_query" {
		credential, credentialErr := r.toolCredential(ctx, tool)
		if credentialErr != nil {
			return nil, credentialErr
		}
		query := parsed.Query()
		query.Set(auth.QueryName, string(credential))
		parsed.RawQuery = query.Encode()
		wipe(credential)
	}
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), body)
	if err != nil {
		return nil, errors.New("tool API request configuration is invalid")
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, values := range headers {
		request.Header[name] = append([]string(nil), values...)
	}
	auditCategory = "upstream_authentication_failed"
	switch auth.Type {
	case "delegated_oauth":
		if principal.DelegatedAPIOrigin == "" || principal.DelegatedAccessToken == "" {
			return nil, ErrDenied
		}
		if !sameOrigin(parsed.String(), principal.DelegatedAPIOrigin) {
			return nil, ErrUnsafeDestination
		}
		request.Header.Set("Authorization", "Bearer "+principal.DelegatedAccessToken)
	case "none":
	case "bearer", "authorization_scheme", "api_key_header", "basic", "custom_header":
		credential, credentialErr := r.toolCredential(ctx, tool)
		if credentialErr != nil {
			return nil, credentialErr
		}
		switch auth.Type {
		case "bearer":
			request.Header.Set("Authorization", "Bearer "+string(credential))
		case "authorization_scheme":
			request.Header.Set("Authorization", auth.Scheme+" "+string(credential))
		case "api_key_header":
			request.Header.Set(auth.HeaderName, prefixedCredential(auth.Prefix, credential))
		case "basic":
			request.SetBasicAuth(auth.Username, string(credential))
		case "custom_header":
			request.Header.Set(auth.HeaderName, prefixedCredential(auth.Prefix, credential))
		}
		wipe(credential)
	case "api_key_query":
	case "oauth_client_credentials":
		if trace != nil {
			trace.Phase = "token_exchange"
		}
		tokenType, token, tokenErr := r.oauthClientTokenTraced(ctx, tool, auth, trace)
		if tokenErr != nil {
			return nil, tokenErr
		}
		request.Header.Set("Authorization", tokenType+" "+token)
	default:
		return nil, ErrDenied
	}
	if method != http.MethodGet && policy.IdempotencyRequired {
		request.Header.Set("Idempotency-Key", upstreamIdempotencyKey(productID, tool, principal))
	}
	auditCategory = "transport_failed"
	if trace != nil {
		trace.NetworkCallPerformed = true
	}
	response, err := r.client(parsed, address, time.Duration(tool.TimeoutMS)*time.Millisecond).Do(request)
	if err != nil {
		return nil, errors.New("tool API request failed")
	}
	defer response.Body.Close()
	auditStatusCode = response.StatusCode
	auditCategory = "upstream_status"
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("tool API returned %s", response.Status)
	}
	auditCategory = "response_read_failed"
	encoded, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
	if err != nil || len(encoded) > 1<<20 {
		return nil, errors.New("tool API response exceeds the 1 MiB limit")
	}
	if trace != nil {
		trace.ResponseBytes = int64(len(encoded))
	}
	auditCategory = "response_invalid"
	var output any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&output); err != nil {
		return nil, fmt.Errorf("tool API returned invalid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("tool API returned multiple JSON values")
	}
	if trace != nil {
		shape := jsonShape(output)
		trace.ResponseShape = &shape
	}
	auditCategory = "response_mapping_failed"
	responseMap, err := toolResponseMapping(tool)
	if err != nil {
		return nil, err
	}
	output, err = extractMappedResult(output, responseMap.ResultPath)
	if err != nil {
		return nil, err
	}
	if trace != nil {
		shape := jsonShapeForSchema(output, tool.OutputSchema)
		trace.ResponseShape = &shape
	}
	auditCategory = "response_schema_mismatch"
	object, ok := output.(map[string]any)
	if !ok {
		return nil, errors.New("tool output schema mismatch: response must resolve to an object")
	}
	if err := ValidateArguments(tool.OutputSchema, object); err != nil {
		return nil, fmt.Errorf("tool output schema mismatch: %w", err)
	}
	output = object
	auditCategory, auditOutcome = "success", "success"
	return output, nil
}
