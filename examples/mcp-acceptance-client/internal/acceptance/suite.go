package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

func Run(ctx context.Context, config Config) (Report, error) {
	if err := validateSecureEndpoint(config.Endpoint, config.AllowedLoopbackHTTP); err != nil {
		return Report{}, err
	}
	if config.TaskPlan != nil {
		if err := config.TaskPlan.validate(config.Endpoint); err != nil {
			return Report{}, err
		}
	}
	if config.Timeout <= 0 {
		config.Timeout = 20 * time.Second
	}
	httpClient := clientWithoutRedirects(config.HTTPClient, config.Timeout)
	started := time.Now().UTC()
	report := Report{ClientName: "DokoSoko MCP acceptance client", ClientVersion: ClientVersion, EvidenceOrigin: "acceptance_client_observation", Endpoint: strings.TrimRight(config.Endpoint, "/"), ProtocolVersion: ProtocolVersion, StartedAt: started}
	expectations := map[string]*ResourceExpectation{}
	discoveryTargets := append([]string(nil), config.ExpectedResources...)
	readTargets := append([]string(nil), config.ExpectedResources...)
	if config.TaskPlan != nil {
		report.Task = &TaskReport{Selection: config.TaskPlan.Task, PlanSHA256: config.TaskPlan.fingerprint(), RetrievalStatus: Fail, ImplementationStatus: "not_run"}
		for index := range config.TaskPlan.Resources {
			value := &config.TaskPlan.Resources[index]
			expectations[value.URI] = value
			readTargets = append(readTargets, value.URI)
			if value.Discover {
				discoveryTargets = append(discoveryTargets, value.URI)
			}
		}
	}
	client := mcpClient{endpoint: config.Endpoint, origin: config.Origin, token: config.Token, httpClient: httpClient}

	discover := client.call(ctx, "server/discover", "", nil, false)
	discoverCheck := outcomeCheck("server/discover", discover, nil)
	if discoverCheck.Status == Pass && !supportsVersion(discover.Response.Result, ProtocolVersion) {
		discoverCheck.Status = Fail
		discoverCheck.Detail = "discovery did not advertise protocol version " + ProtocolVersion
	}
	report.Add(discoverCheck)

	resourceCatalog := client.listAll(ctx, "resources/list", "resources", "uri")
	resourcesCheck := resourceCatalog.Check
	resourceURIs := []string{}
	descriptions := []resourceDescription{}
	if resourcesCheck.Status == Pass {
		for _, raw := range resourceCatalog.Items {
			var item resourceDescription
			if json.Unmarshal(raw, &item) != nil {
				resourcesCheck.Status, resourcesCheck.Detail = Fail, "resource descriptor was invalid"
				break
			}
			descriptions = append(descriptions, item)
			resourceURIs = append(resourceURIs, item.URI)
		}
	}
	report.Add(resourcesCheck)
	for _, expected := range unique(discoveryTargets) {
		status := Pass
		detail := "resource was advertised"
		matches := []resourceDescription{}
		for _, description := range descriptions {
			if description.URI == expected {
				matches = append(matches, description)
			}
		}
		if len(matches) != 1 {
			status, detail = Fail, "expected resource was absent or advertised more than once"
		} else if value := expectations[expected]; value != nil && (!metadataMatches(matches[0].Metadata, value.Metadata) || value.MIMEType != "" && matches[0].MIMEType != value.MIMEType) {
			status, detail = Fail, "advertised revision, publication or media type does not match the reviewed expectation"
		}
		resourcesOutcome := resourceCatalog.Requests[expected]
		report.Add(Check{Name: "resource expected: " + expected, Status: status, Required: true, Detail: detail, ResourceURI: expected, RequestID: resourcesOutcome.RequestID, ResponseRequestID: resourcesOutcome.ResponseRequestID})
	}
	readTargets = unique(readTargets)
	if len(readTargets) == 0 && len(resourceURIs) > 0 {
		readTargets = []string{resourceURIs[0]}
	}
	if len(readTargets) == 0 {
		report.Add(Check{Name: "resources/read", Status: Skip, Detail: "no resource was advertised or configured"})
	} else {
		for _, uri := range readTargets {
			outcome := client.callWithParams(ctx, "resources/read", map[string]any{"uri": uri})
			report.Add(resourceReadCheck(uri, outcome, expectations[uri]))
		}
	}
	if report.Task != nil && report.Summary.Failed == 0 && report.Summary.RequiredSkipped == 0 {
		report.Task.RetrievalStatus = Pass
	}

	toolCatalog := client.listAll(ctx, "tools/list", "tools", "name")
	toolsCheck := toolCatalog.Check
	toolNames := []string{}
	if toolsCheck.Status == Pass {
		for name := range toolCatalog.Requests {
			toolNames = append(toolNames, name)
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
	catalog := restricted.listAll(ctx, "tools/list", "tools", "name")
	check := catalog.Check
	check.Name = "authorization.grant.negative"
	if check.Status == Pass {
		if _, exists := catalog.Requests[config.GrantTool]; exists {
			check.Status, check.Detail = Fail, "grant-gated tool was disclosed to the restricted token"
		} else {
			check.Detail = "grant-gated tool was hidden from every discovery page for the restricted token"
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
