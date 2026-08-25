package tools

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func validationPaths(err error, rawSchema json.RawMessage) (string, string) {
	if err == nil {
		return "", ""
	}
	var currentSchema map[string]any
	if ValidateSchema(rawSchema) == nil {
		_ = json.Unmarshal(rawSchema, &currentSchema)
	}
	message := err.Error()
	if start := strings.Index(message, "arguments"); start >= 0 {
		message = message[start:]
	}
	end := strings.IndexByte(message, ' ')
	if end < 0 {
		end = len(message)
	}
	path := message[:end]
	if !strings.HasPrefix(path, "arguments") {
		return "", ""
	}
	path = strings.TrimPrefix(path, "arguments")
	instancePath, schemaPath := "", ""
	for len(path) > 0 {
		switch path[0] {
		case '.':
			path = path[1:]
			end = len(path)
			if index := strings.IndexAny(path, ".[ "); index >= 0 {
				end = index
			}
			name := path[:end]
			if name == "" {
				return instancePath, schemaPath
			}
			properties, _ := currentSchema["properties"].(map[string]any)
			childSchema, declared := properties[name].(map[string]any)
			if !declared {
				instancePath += "/[unexpected-property]"
				schemaPath += "/additionalProperties"
				return instancePath, schemaPath
			}
			if safeDeclaredShapeKey(name) {
				encoded := escapeJSONPointer(name)
				instancePath += "/" + encoded
				schemaPath += "/properties/" + encoded
			} else {
				instancePath += "/[schema-property]"
				schemaPath += "/properties/[schema-property]"
			}
			currentSchema = childSchema
			path = path[end:]
		case '[':
			closing := strings.IndexByte(path, ']')
			if closing < 2 {
				return instancePath, schemaPath
			}
			index := path[1:closing]
			if _, parseErr := strconv.Atoi(index); parseErr != nil {
				return instancePath, schemaPath
			}
			instancePath += "/" + index
			schemaPath += "/items"
			currentSchema, _ = currentSchema["items"].(map[string]any)
			path = path[closing+1:]
		default:
			return instancePath, schemaPath
		}
	}
	for _, keyword := range []string{"additionalProperties", "required", "minLength", "maxLength", "minimum", "maximum", "minItems", "maxItems", "uniqueItems", "enum", "type"} {
		if strings.Contains(message, keyword) || keyword == "type" && strings.Contains(message, " must be ") {
			schemaPath += "/" + keyword
			break
		}
	}
	return instancePath, schemaPath
}

func testFailureFinding(trace executionTrace, err error, outputSchema json.RawMessage) model.ToolTestFinding {
	finding := model.ToolTestFinding{Phase: tracePhase(trace.Category, trace.Phase)}
	switch trace.Category {
	case "rate_limited":
		finding.Code, finding.Message = "rate_limited", "The upstream connection test rate limit was exceeded."
	case "unsafe_destination":
		finding.Code, finding.Message = "unsafe_destination", "The destination failed the network safety policy."
	case "configuration_invalid":
		finding.Code, finding.Message = "configuration_invalid", "The stored HTTP tool configuration is invalid."
	case "request_mapping_failed":
		finding.Code, finding.Message = "request_mapping_failed", "The request could not be constructed from the declared mapping."
	case "upstream_authentication_failed":
		if finding.Phase == "token_exchange" {
			finding.Code, finding.Message = "token_exchange_failed", "The OAuth client-credentials token exchange failed safely."
		} else {
			finding.Code, finding.Message = "upstream_authentication_failed", "The configured upstream authentication could not be applied."
		}
	case "transport_failed":
		finding.Code, finding.Message = "transport_failed", "The one-shot upstream request failed at the transport boundary."
	case "upstream_status":
		finding.Code, finding.Message = "upstream_status_rejected", "The upstream API returned a non-success status."
	case "response_read_failed":
		finding.Code, finding.Message = "response_size_or_read_failed", "The response could not be read within the 1 MiB safety limit."
	case "response_invalid":
		finding.Code, finding.Message = "invalid_json_response", "The upstream response was not exactly one valid JSON value."
	case "response_mapping_failed":
		finding.Code, finding.Message = "response_mapping_failed", "The declared response mapping did not resolve."
	case "response_schema_mismatch":
		finding.Code, finding.Message = "output_schema_mismatch", "The mapped response did not match the declared output schema."
		finding.InstancePath, finding.SchemaPath = validationPaths(err, outputSchema)
	default:
		finding.Phase = "preflight"
		finding.Code, finding.Message = "preflight_failed", "The tool test failed deterministic preflight validation."
	}
	return finding
}

// ExecuteHTTPDraftTest executes the supplied exact stored draft through the
// same hardened HTTP path as a published call. It deliberately skips public
// discovery and authorization lookup, never retries, and returns only
// sanitized evidence suitable for short-lived administrator diagnostics.
func (r *Runtime) ExecuteHTTPDraftTest(ctx context.Context, productID string, tool model.Tool, arguments map[string]any, principal Principal) DraftTestReport {
	started := time.Now()
	report := DraftTestReport{Outcome: "failure", Phase: "preflight", RequestShape: jsonShapeForSchema(arguments, tool.InputSchema), Findings: []model.ToolTestFinding{}}
	finish := func() DraftTestReport {
		report.DurationMS = time.Since(started).Milliseconds()
		if report.DurationMS < 0 {
			report.DurationMS = 0
		}
		return report
	}
	if tool.BackendKind != "http" {
		report.Findings = append(report.Findings, model.ToolTestFinding{Phase: "preflight", Code: "http_tool_required", Message: "Live test runs are available only for stored HTTP tools."})
		return finish()
	}
	if arguments == nil {
		arguments = map[string]any{}
		report.RequestShape = jsonShapeForSchema(arguments, tool.InputSchema)
	}
	auth, err := toolUpstreamAuth(tool)
	if err != nil {
		report.Findings = append(report.Findings, model.ToolTestFinding{Phase: "preflight", Code: "authentication_configuration_invalid", Message: "The stored upstream authentication configuration is invalid."})
		return finish()
	}
	report.AuthenticationType = auth.Type
	if err := ValidateArguments(tool.InputSchema, arguments); err != nil {
		instancePath, schemaPath := validationPaths(err, tool.InputSchema)
		report.Findings = append(report.Findings, model.ToolTestFinding{Phase: "preflight", Code: "input_schema_mismatch", Message: "The supplied arguments did not match the declared input schema.", InstancePath: instancePath, SchemaPath: schemaPath})
		return finish()
	}
	if auth.Type == "delegated_oauth" {
		report.Phase = "auth"
		report.Findings = append(report.Findings, model.ToolTestFinding{Phase: "auth", Code: "test_authorization_unavailable", Message: "Delegated OAuth test authorization is required and cannot be supplied as raw token material."})
		return finish()
	}
	trace := executionTrace{Category: "preflight_failed", Phase: "preflight"}
	_, err = r.executeAuthorizedTraced(ctx, productID, tool.Namespace+"."+tool.Name, tool, arguments, principal, &trace, false)
	report.Phase = trace.Phase
	report.NetworkCallPerformed = trace.NetworkCallPerformed
	report.UpstreamStatusCode = trace.StatusCode
	report.ResponseBytes = trace.ResponseBytes
	report.ResponseShape = trace.ResponseShape
	if err != nil {
		report.Findings = append(report.Findings, testFailureFinding(trace, err, tool.OutputSchema))
		return finish()
	}
	report.Outcome, report.Phase = "success", "success"
	return finish()
}

// ExecuteBound executes one tool only when the selected Integration action is
// still the exact active point revision that was published with the tool.
