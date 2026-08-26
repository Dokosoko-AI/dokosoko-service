package httpapi

import (
	"encoding/json"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

const reportingAgentInstructions = " When a likely connector-specific defect is found, offer to prepare a bug report but do not submit automatically. Before using a support reporting tool, show the user a concise preview of what will be shared and obtain explicit approval. Explain that DokoSoko adds the authenticated subject, account or installation, applicable API publication, and request metadata; contact name and email are added only when allow_contact is approved. Submit only relevant, sanitized context; never include credentials, tokens, unrelated conversation, complete files, or unapproved personal data. For feedback, preserve the user's meaning and never invent ratings, sentiment, or claims."

func reportOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"submission_id": map[string]any{"type": "string"},
			"status":        map[string]any{"type": "string", "const": "queued"},
		},
		"required": []string{"submission_id", "status"},
	}
}

func mergeMetadata(current any, additions map[string]any) map[string]any {
	result := make(map[string]any)
	if existing, ok := current.(map[string]any); ok {
		for key, value := range existing {
			result[key] = value
		}
	}
	for key, value := range additions {
		result[key] = value
	}
	return result
}

func bugReportToolDefinition() map[string]any {
	return map[string]any{
		"name":        "support.report_bug",
		"description": "Queue a connector bug report only after explicit user confirmation. First show the user a concise preview of the exact report content and disclose that trusted authenticated-account, installation, API-publication, and request metadata will be added. Contact details are added only when allow_contact is approved. Include relevant reproduction details and sanitized diagnostics only; never include secrets, credentials, unrelated conversation, complete files, or unapproved personal data.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"integration_id":     map[string]any{"type": "string", "description": "The affected Integration ID from com.dokosoko/supportCapabilities metadata. Omit only for a legacy connector with no Integration catalog."},
				"summary":            map[string]any{"type": "string", "minLength": 1, "maxLength": 160, "description": "A concise user-approved title for the defect."},
				"description":        map[string]any{"type": "string", "minLength": 1, "maxLength": 10000, "description": "What happened and why it appears related to this connector."},
				"reproduction_steps": map[string]any{"type": "array", "maxItems": 20, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 1000}},
				"expected_behavior":  map[string]any{"type": "string", "maxLength": 4000},
				"actual_behavior":    map[string]any{"type": "string", "maxLength": 4000},
				"error_code":         map[string]any{"type": "string", "maxLength": 120},
				"error_message":      map[string]any{"type": "string", "maxLength": 8000},
				"stack_trace":        map[string]any{"type": "string", "maxLength": 16000, "description": "Sanitized relevant stack frames only."},
				"diagnostic_context": map[string]any{"type": "string", "maxLength": 20000, "description": "A bounded, sanitized context summary; do not send full files or conversations."},
				"related_tool":       map[string]any{"type": "string", "pattern": `^[A-Za-z0-9_.-]{1,160}$`},
				"severity":           map[string]any{"type": "string", "enum": []string{"unknown", "low", "medium", "high", "critical"}, "default": "unknown"},
				"allow_contact":      map[string]any{"type": "boolean", "default": false, "description": "Share the authenticated user's contact details for follow-up only when explicitly approved."},
				"idempotency_key":    map[string]any{"type": "string", "minLength": 16, "maxLength": 200, "description": "A stable unique key for this exact approved report."},
			},
			"required": []string{"summary", "description", "idempotency_key"},
		},
		"outputSchema": reportOutputSchema(),
		"annotations":  map[string]any{"readOnlyHint": false, "idempotentHint": true, "destructiveHint": false, "openWorldHint": true},
		"_meta":        map[string]any{"com.dokosoko/confirmationRequired": true, "com.dokosoko/dataHandling": "plaintext-queued-sanitized-user-approved"},
	}
}

func feedbackToolDefinition() map[string]any {
	return map[string]any{
		"name":        "support.submit_feedback",
		"description": "Queue connector feedback expressed by the user only after explicit confirmation. First show the user a concise preview and disclose that trusted authenticated-account, installation, API-publication, and request metadata will be added. Contact details are added only when allow_contact is approved. Preserve the user's meaning and distinguish it from agent-generated context; never invent ratings, sentiment, claims, or personal details.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"integration_id":  map[string]any{"type": "string", "description": "The Integration ID the experience relates to, from com.dokosoko/supportCapabilities metadata."},
				"message":         map[string]any{"type": "string", "minLength": 1, "maxLength": 10000, "description": "The user's feedback, faithfully summarized or quoted with approval."},
				"category":        map[string]any{"type": "string", "enum": []string{"general", "usability", "documentation", "performance", "feature_request", "other"}, "default": "general"},
				"rating":          map[string]any{"type": "integer", "minimum": 1, "maximum": 5, "description": "Include only when the user explicitly supplied or approved the rating."},
				"related_tool":    map[string]any{"type": "string", "pattern": `^[A-Za-z0-9_.-]{1,160}$`},
				"allow_contact":   map[string]any{"type": "boolean", "default": false, "description": "Share the authenticated user's contact details for follow-up only when explicitly approved."},
				"idempotency_key": map[string]any{"type": "string", "minLength": 16, "maxLength": 200, "description": "A stable unique key for this exact approved feedback."},
			},
			"required": []string{"message", "idempotency_key"},
		},
		"outputSchema": reportOutputSchema(),
		"annotations":  map[string]any{"readOnlyHint": false, "idempotentHint": true, "destructiveHint": false, "openWorldHint": true},
		"_meta":        map[string]any{"com.dokosoko/confirmationRequired": true, "com.dokosoko/dataHandling": "plaintext-queued-sanitized-user-approved"},
	}
}
