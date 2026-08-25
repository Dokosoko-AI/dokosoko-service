package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
)

func Run(ctx context.Context, config Config) (Report, error) {
	if err := validateSecureEndpoint(config.Endpoint, config.AllowedLoopbackHTTP); err != nil {
		return Report{}, err
	}
	if config.Timeout <= 0 {
		config.Timeout = 20 * time.Second
	}
	httpClient := clientWithoutRedirects(config.HTTPClient, config.Timeout)
	started := time.Now().UTC()
	report := Report{Endpoint: strings.TrimRight(config.Endpoint, "/"), ProtocolVersion: ProtocolVersion, StartedAt: started}
	client := mcpClient{endpoint: config.Endpoint, origin: config.Origin, token: config.Token, httpClient: httpClient}

	discover := client.call(ctx, "server/discover", "", nil, false)
	discoverCheck := outcomeCheck("server/discover", discover, nil)
	if discoverCheck.Status == Pass && !supportsVersion(discover.Response.Result, ProtocolVersion) {
		discoverCheck.Status = Fail
		discoverCheck.Detail = "discovery did not advertise protocol version " + ProtocolVersion
	}
	report.Add(discoverCheck)

	resourcesOutcome := client.call(ctx, "resources/list", "", nil, false)
	resourcesCheck := outcomeCheck("resources/list", resourcesOutcome, nil)
	resourceURIs := []string{}
	if resourcesCheck.Status == Pass {
		var result struct {
			Resources []struct {
				URI string `json:"uri"`
			} `json:"resources"`
		}
		if json.Unmarshal(resourcesOutcome.Response.Result, &result) != nil || result.Resources == nil {
			resourcesCheck.Status = Fail
			resourcesCheck.Detail = "result.resources was not an array"
		} else {
			for _, item := range result.Resources {
				if item.URI != "" {
					resourceURIs = append(resourceURIs, item.URI)
				}
			}
		}
	}
	report.Add(resourcesCheck)
	for _, expected := range unique(config.ExpectedResources) {
		status := Pass
		detail := "resource was advertised"
		if !contains(resourceURIs, expected) {
			status, detail = Fail, "expected resource was not advertised"
		}
		report.Add(Check{Name: "resource expected: " + expected, Status: status, Required: true, Detail: detail})
	}
	readTargets := unique(config.ExpectedResources)
	if len(readTargets) == 0 && len(resourceURIs) > 0 {
		readTargets = []string{resourceURIs[0]}
	}
	if len(readTargets) == 0 {
		report.Add(Check{Name: "resources/read", Status: Skip, Detail: "no resource was advertised or configured"})
	} else {
		for _, uri := range readTargets {
			outcome := client.callWithParams(ctx, "resources/read", map[string]any{"uri": uri})
			report.Add(outcomeCheck("resources/read: "+uri, outcome, nil))
		}
	}

	toolsOutcome := client.call(ctx, "tools/list", "", nil, false)
	toolsCheck := outcomeCheck("tools/list", toolsOutcome, nil)
	toolNames := []string{}
	if toolsCheck.Status == Pass {
		var result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		if json.Unmarshal(toolsOutcome.Response.Result, &result) != nil || result.Tools == nil {
			toolsCheck.Status = Fail
			toolsCheck.Detail = "result.tools was not an array"
		} else {
			for _, item := range result.Tools {
				if item.Name != "" {
					toolNames = append(toolNames, item.Name)
				}
			}
		}
	}
	report.Add(toolsCheck)
	for _, expected := range unique(config.ExpectedTools) {
		status := Pass
		detail := "tool was advertised"
		if !contains(toolNames, expected) {
			status, detail = Fail, "expected tool was not advertised"
		}
		report.Add(Check{Name: "tool expected: " + expected, Status: status, Required: true, Detail: detail})
	}

	if config.CallTool == "" {
		detail := "no call tool was configured"
		if config.CallConfirmed {
			detail = "a confirmed call was requested without a call tool"
		}
		report.Add(Check{Name: "tools/call", Status: Skip, Required: config.CallConfirmed, Detail: detail})
	} else {
		var outcome callOutcome
		if config.CallConfirmed {
			outcome = client.callConfirmed(ctx, config.CallTool, config.CallArguments)
		} else {
			outcome = client.call(ctx, "tools/call", config.CallTool, config.CallArguments, false)
		}
		report.Add(outcomeCheck("tools/call: "+config.CallTool, outcome, nil))
	}

	runGrantChecks(ctx, &report, config, client, toolNames, httpClient)
	runConfirmationChecks(ctx, &report, config, client)
	if config.CheckUnauthenticated {
		if config.Token == "" {
			report.Add(Check{Name: "authorization.unauthenticated", Status: Skip, Required: true, Detail: "no authenticated token was configured"})
		} else {
			anonymous := mcpClient{endpoint: config.Endpoint, origin: config.Origin, httpClient: httpClient}
			outcome := anonymous.call(ctx, "server/discover", "", nil, false)
			check := Check{Name: "authorization.unauthenticated", Required: true, RequestID: outcome.RequestID, ResponseRequestID: outcome.ResponseRequestID, HTTPStatus: outcome.HTTPStatus, DurationMS: outcome.Duration.Milliseconds()}
			if outcome.HTTPStatus == http.StatusUnauthorized || outcome.HTTPStatus == http.StatusForbidden {
				check.Status, check.Detail = Pass, "unauthenticated request was denied"
			} else {
				check.Status, check.Detail = Fail, "unauthenticated request was not denied with HTTP 401 or 403"
			}
			report.Add(check)
		}
	}
	report.DurationMS = time.Since(started).Milliseconds()
	return report, nil
}

func (c mcpClient) callWithParams(ctx context.Context, method string, values map[string]any) callOutcome {
	started := time.Now()
	requestID, err := randomID("mcpacc_")
	if err != nil {
		return callOutcome{TransportError: err, Duration: time.Since(started)}
	}
	meta := map[string]any{"io.modelcontextprotocol/protocolVersion": ProtocolVersion}
	params := map[string]any{"_meta": meta}
	for key, value := range values {
		params[key] = value
	}
	return c.callRaw(ctx, requestID, method, "", params, started)
}

func supportsVersion(raw json.RawMessage, version string) bool {
	var result struct {
		SupportedVersions []string `json:"supportedVersions"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return false
	}
	return contains(result.SupportedVersions, version)
}

func runGrantChecks(ctx context.Context, report *Report, config Config, client mcpClient, positiveTools []string, httpClient *http.Client) {
	if config.GrantTool == "" {
		report.Add(Check{Name: "authorization.grant.positive", Status: Skip, Detail: "no grant-gated tool was configured"})
		report.Add(Check{Name: "authorization.grant.negative", Status: Skip, Detail: "no grant-gated tool was configured"})
		if config.VerifyRestrictedCallDenied {
			report.Add(Check{Name: "authorization.grant.call-denied", Status: Skip, Required: true, Detail: "restricted invocation was requested without a grant-gated tool"})
		}
		return
	}
	if contains(positiveTools, config.GrantTool) {
		report.Add(Check{Name: "authorization.grant.positive", Status: Pass, Required: true, Detail: "grant-gated tool was visible with the primary token"})
	} else {
		report.Add(Check{Name: "authorization.grant.positive", Status: Fail, Required: true, Detail: "grant-gated tool was not visible with the primary token"})
	}
	if config.RestrictedToken == "" {
		report.Add(Check{Name: "authorization.grant.negative", Status: Skip, Required: true, Detail: "no restricted token was configured"})
		if config.VerifyRestrictedCallDenied {
			report.Add(Check{Name: "authorization.grant.call-denied", Status: Skip, Required: true, Detail: "no restricted token was configured"})
		}
		return
	}
	restricted := mcpClient{endpoint: config.Endpoint, origin: config.Origin, token: config.RestrictedToken, httpClient: httpClient}
	outcome := restricted.call(ctx, "tools/list", "", nil, false)
	check := outcomeCheck("authorization.grant.negative", outcome, nil)
	if check.Status == Pass {
		names, err := toolNames(outcome.Response.Result)
		if err != nil {
			check.Status, check.Detail = Fail, "restricted result.tools was not an array"
		} else if contains(names, config.GrantTool) {
			check.Status, check.Detail = Fail, "grant-gated tool was disclosed to the restricted token"
		} else {
			check.Detail = "grant-gated tool was hidden from the restricted token"
		}
	}
	report.Add(check)
	if config.VerifyRestrictedCallDenied {
		denied := restricted.call(ctx, "tools/call", config.GrantTool, config.GrantArguments, false)
		deniedCheck := Check{Name: "authorization.grant.call-denied", Required: true, RequestID: denied.RequestID, ResponseRequestID: denied.ResponseRequestID, HTTPStatus: denied.HTTPStatus, DurationMS: denied.Duration.Milliseconds()}
		if denied.TransportError == nil && denied.Response.Error != nil && (denied.Response.Error.Code == -32003 || denied.Response.Error.Code == -32601) {
			code := denied.Response.Error.Code
			deniedCheck.Status, deniedCheck.Detail, deniedCheck.RPCErrorCode = Pass, "restricted invocation was denied", &code
		} else if denied.HTTPStatus == http.StatusUnauthorized || denied.HTTPStatus == http.StatusForbidden {
			deniedCheck.Status, deniedCheck.Detail = Pass, "restricted invocation was denied"
		} else {
			deniedCheck.Status, deniedCheck.Detail = Fail, "restricted invocation was not denied"
		}
		report.Add(deniedCheck)
	}
}

func runConfirmationChecks(ctx context.Context, report *Report, config Config, client mcpClient) {
	if config.ConfirmationTool == "" {
		report.Add(Check{Name: "authorization.confirmation.negative", Status: Skip, Detail: "no confirmation-gated tool was configured"})
		report.Add(Check{Name: "authorization.confirmation.positive", Status: Skip, Required: config.VerifyConfirmedCall, Detail: "no confirmation-gated tool was configured"})
		return
	}
	idempotencyKey, err := randomID("mcpacc_idem_")
	if err != nil {
		report.Add(Check{Name: "authorization.confirmation.negative", Status: Fail, Required: true, Detail: "could not generate confirmation idempotency metadata"})
		report.Add(Check{Name: "authorization.confirmation.positive", Status: Skip, Required: config.VerifyConfirmedCall, Detail: "confirmation challenge could not be started"})
		return
	}
	code := -32003
	unconfirmed := client.callToolWithConfirmation(ctx, "tools/call", config.ConfirmationTool, config.ConfirmationArguments, false, "", idempotencyKey)
	negative := outcomeCheck("authorization.confirmation.negative", unconfirmed, &code)
	challenge, challengeErr := confirmationChallenge(unconfirmed)
	if negative.Status == Pass && challengeErr != nil {
		negative.Status = Fail
		negative.Detail = challengeErr.Error()
	}
	report.Add(negative)
	if !config.VerifyConfirmedCall {
		report.Add(Check{Name: "authorization.confirmation.positive", Status: Skip, Detail: "confirmed invocation was not enabled"})
		return
	}
	if challengeErr != nil {
		report.Add(Check{Name: "authorization.confirmation.positive", Status: Fail, Required: true, Detail: "server did not provide a usable confirmation challenge"})
		report.Add(Check{Name: "authorization.confirmation.replay", Status: Skip, Required: true, Detail: "no consumed challenge was available to replay"})
		return
	}
	confirmed := client.callToolWithConfirmation(ctx, "tools/call", config.ConfirmationTool, config.ConfirmationArguments, true, challenge, idempotencyKey)
	positive := outcomeCheck("authorization.confirmation.positive", confirmed, nil)
	report.Add(positive)
	if positive.Status != Pass {
		report.Add(Check{Name: "authorization.confirmation.replay", Status: Skip, Required: true, Detail: "the confirmed invocation did not consume its challenge"})
		return
	}
	replayed := client.callToolWithConfirmation(ctx, "tools/call", config.ConfirmationTool, config.ConfirmationArguments, true, challenge, idempotencyKey)
	replay := outcomeCheck("authorization.confirmation.replay", replayed, &code)
	if replay.Status == Pass {
		replay.Detail = "the consumed confirmation challenge was rejected on replay"
	}
	report.Add(replay)
}

func toolNames(raw json.RawMessage) ([]string, error) {
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Tools == nil {
		return nil, errors.New("result.tools was not an array")
	}
	values := make([]string, 0, len(result.Tools))
	for _, item := range result.Tools {
		values = append(values, item.Name)
	}
	return values, nil
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
