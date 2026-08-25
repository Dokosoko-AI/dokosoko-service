package platform

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func toolTestSchemaObject(raw json.RawMessage) map[string]any {
	var schema map[string]any
	if json.Unmarshal(raw, &schema) != nil {
		return nil
	}
	return schema
}

func toolTestShapeForAI(shape model.JSONShape, schema map[string]any, depth int) model.JSONShape {
	result := model.JSONShape{Type: shape.Type, Length: shape.Length, Truncated: shape.Truncated}
	if !safeToolTestSchemaType(result.Type) {
		result.Type = "unknown"
	}
	if depth > 16 {
		result.Truncated = true
		return result
	}
	if shape.Type == "object" {
		result.Properties = make(map[string]model.JSONShape)
		declared, _ := schema["properties"].(map[string]any)
		for name, child := range shape.Properties {
			rawChildSchema, declaredProperty := declared[name]
			if !declaredProperty || !safeToolTestAnalysisName(name) {
				result.Truncated = true
				continue
			}
			childSchema, _ := rawChildSchema.(map[string]any)
			result.Properties[name] = toolTestShapeForAI(child, childSchema, depth+1)
		}
	}
	if shape.Type == "array" {
		itemSchema, _ := schema["items"].(map[string]any)
		result.Items = make([]model.JSONShape, 0, len(shape.Items))
		for _, child := range shape.Items {
			result.Items = append(result.Items, toolTestShapeForAI(child, itemSchema, depth+1))
		}
	}
	return result
}

func toolTestEvidenceForAI(run model.ToolTestRun, base ToolDraft) toolTestAIAnalysisEvidence {
	requestSchema := toolTestSchemaObject(base.InputSchema)
	responseSchema := toolTestSchemaObject(base.OutputSchema)
	findings := make([]toolTestAIFinding, 0, len(run.Findings))
	for _, finding := range run.Findings {
		phase := toolTestPhaseForAI(finding.Phase)
		code := toolTestFindingCodeForAI(finding.Code)
		if phase == "unknown" || code == "" {
			continue
		}
		findings = append(findings, toolTestAIFinding{Phase: phase, Code: code, Message: "A sanitized live-test finding was retained."})
		if len(findings) == 32 {
			break
		}
	}
	if findings == nil {
		findings = []toolTestAIFinding{}
	}
	var responseShape *model.JSONShape
	if run.ResponseShape != nil {
		projected := toolTestShapeForAI(*run.ResponseShape, responseSchema, 0)
		responseShape = &projected
	}
	return toolTestAIAnalysisEvidence{
		Method: toolTestMethodForAI(run.Method), AuthenticationType: toolTestAuthenticationTypeForAI(run.AuthenticationType), Outcome: toolTestOutcomeForAI(run.Outcome), Phase: toolTestPhaseForAI(run.Phase),
		NetworkCallPerformed: run.NetworkCallPerformed, UpstreamStatusCode: run.UpstreamStatusCode, ResponseBytes: run.ResponseBytes,
		DurationMS: run.DurationMS, RequestShape: toolTestShapeForAI(run.RequestShape, requestSchema, 0), ResponseShape: responseShape, Findings: findings,
	}
}

func toolTestContractForAI(base ToolDraft) toolTestAIContract {
	parameters := make([]string, 0)
	if parsed, err := url.Parse(base.Endpoint); err == nil {
		seen := map[string]bool{}
		for _, match := range toolBuilderPlaceholderPattern.FindAllStringSubmatch(parsed.Path, -1) {
			if len(match) > 1 && safeToolTestAnalysisName(match[1]) && !seen[match[1]] {
				seen[match[1]] = true
				parameters = append(parameters, match[1])
			}
		}
	}
	sort.Strings(parameters)
	requestMapping := ToolRequestMapping{ParameterLocations: map[string]string{}}
	for name, location := range base.RequestMapping.ParameterLocations {
		if safeToolTestAnalysisName(name) && (location == "path" || location == "query" || location == "header" || location == "body") {
			requestMapping.ParameterLocations[name] = location
		}
	}
	responseMapping := ToolResponseMapping{}
	if base.ResponseMapping.ResultPath != "" {
		parts := strings.Split(base.ResponseMapping.ResultPath, ".")
		safe := len(parts) > 0
		for _, part := range parts {
			safe = safe && safeToolTestAnalysisName(part)
		}
		if safe {
			responseMapping.ResultPath = base.ResponseMapping.ResultPath
		}
	}
	policy := base.AuthorizationPolicy
	policy.RequiredGrants = make([]string, 0, len(base.AuthorizationPolicy.RequiredGrants))
	for _, grant := range base.AuthorizationPolicy.RequiredGrants {
		if safeToolTestAnalysisName(grant) {
			policy.RequiredGrants = append(policy.RequiredGrants, grant)
		}
	}
	if !map[string]bool{"": true, "low": true, "medium": true, "high": true, "critical": true}[policy.Risk] {
		policy.Risk = ""
	}
	return toolTestAIContract{
		HTTPMethod: toolTestMethodForAI(base.HTTPMethod), TimeoutMS: base.TimeoutMS, PathParameterNames: parameters,
		InputSchema: toolTestSchemaForAI(base.InputSchema), OutputSchema: toolTestSchemaForAI(base.OutputSchema),
		AuthenticationType: toolTestAuthenticationTypeForAI(base.UpstreamAuth.Type), RequestMapping: requestMapping, ResponseMapping: responseMapping,
		AuthorizationPolicy: policy,
	}
}

func unsafeToolTestAnalysisText(value string) bool {
	return containsToolBuilderSecretText(value) || toolTestAnalysisURL.MatchString(value) || toolTestAnalysisUUID.MatchString(value) || toolTestAnalysisNonce.MatchString(value) || toolTestAnalysisHashValue.MatchString(value)
}

func normalizeToolTestAnalysisConversation(question string, history []ToolTestAnalysisMessage) (string, []ToolTestAnalysisMessage, error) {
	question = strings.TrimSpace(question)
	if question == "" || len(question) > maxToolTestAnalysisQuestionBytes || !utf8.ValidString(question) || unsafeToolTestAnalysisText(question) {
		return "", nil, ErrToolTestAnalysisInvalidInput
	}
	if len(history) > maxToolTestAnalysisMessages {
		return "", nil, ErrToolTestAnalysisInvalidInput
	}
	result := make([]ToolTestAnalysisMessage, 0, len(history))
	total := 0
	for _, message := range history {
		message.Role = strings.ToLower(strings.TrimSpace(message.Role))
		message.Content = strings.TrimSpace(message.Content)
		if (message.Role != "user" && message.Role != "assistant") || message.Content == "" || len(message.Content) > maxToolTestAnalysisQuestionBytes || !utf8.ValidString(message.Content) || unsafeToolTestAnalysisText(message.Content) {
			return "", nil, ErrToolTestAnalysisInvalidInput
		}
		total += len(message.Role) + len(message.Content)
		if total > maxToolTestAnalysisHistoryBytes {
			return "", nil, ErrToolTestAnalysisInvalidInput
		}
		result = append(result, message)
	}
	return question, result, nil
}

func normalizeToolTestAIFindings(values []ToolDraftFinding) []ToolDraftFinding {
	result := make([]ToolDraftFinding, 0, len(values))
	for _, finding := range values {
		finding.Level = strings.ToLower(strings.TrimSpace(finding.Level))
		finding.Code = strings.ToLower(strings.TrimSpace(finding.Code))
		finding.Field = strings.TrimSpace(finding.Field)
		finding.Message = strings.TrimSpace(finding.Message)
		finding.Suggestion = strings.TrimSpace(finding.Suggestion)
		if (finding.Level != "warning" && finding.Level != "info") || finding.Code == "" || len(finding.Code) > 80 || finding.Message == "" || len(finding.Message) > 500 || len(finding.Field) > 120 || len(finding.Suggestion) > 500 || unsafeToolTestAnalysisText(finding.Code+" "+finding.Field+" "+finding.Message+" "+finding.Suggestion) {
			continue
		}
		result = append(result, finding)
		if len(result) == 32 {
			break
		}
	}
	sortToolBuilderFindings(result)
	return result
}

func containsToolTestSchemaExample(raw json.RawMessage) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return true
	}
	var visit func(any) bool
	visit = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if key == "example" || key == "examples" || key == "default" || key == "$comment" || visit(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		}
		return false
	}
	return visit(value)
}

func applyToolTestAIEditableDraft(base ToolDraft, raw string) (ToolDraft, error) {
	if len(raw) == 0 || len(raw) > 128<<10 {
		return ToolDraft{}, ErrToolTestAnalysisInvalidInput
	}
	var editable toolTestAIEditableDraft
	if strictJSON(json.RawMessage(raw), &editable) != nil || editable.Description == nil || editable.HTTPMethod == nil || editable.TimeoutMS == nil || len(editable.InputSchema) == 0 || len(editable.OutputSchema) == 0 || editable.RequestMapping == nil || editable.ResponseMapping == nil || editable.AuthorizationPolicy == nil {
		return ToolDraft{}, ErrToolTestAnalysisInvalidInput
	}
	encodedEditable, err := json.Marshal(editable)
	if err != nil || containsToolTestSchemaExample(editable.InputSchema) || containsToolTestSchemaExample(editable.OutputSchema) || containsToolBuilderSecretValue(json.RawMessage(encodedEditable)) {
		return ToolDraft{}, ErrToolBuilderUnsafeInput
	}
	candidate := cloneToolBuilderDraft(base)
	candidate.Description, candidate.HTTPMethod, candidate.TimeoutMS = *editable.Description, *editable.HTTPMethod, *editable.TimeoutMS
	candidate.InputSchema, candidate.OutputSchema = append(json.RawMessage(nil), editable.InputSchema...), append(json.RawMessage(nil), editable.OutputSchema...)
	candidate.RequestMapping, candidate.ResponseMapping, candidate.AuthorizationPolicy = *editable.RequestMapping, *editable.ResponseMapping, *editable.AuthorizationPolicy
	// Examples were deliberately omitted from the provider payload. Preserve
	// both exact base values regardless of the provider response.
	candidate.RequestExample = cloneToolBuilderDraft(base).RequestExample
	candidate.ResponseExample = cloneToolBuilderDraft(base).ResponseExample
	candidate.Endpoint, candidate.UpstreamAuth, candidate.Namespace, candidate.Name = base.Endpoint, base.UpstreamAuth, base.Namespace, base.Name
	candidate.CredentialPresent = false
	return candidate, nil
}
