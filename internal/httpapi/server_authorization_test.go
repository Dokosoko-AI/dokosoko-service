package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestAuthorizationAndToolEditorHTTPFlow(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)

	createdIntegration := request(t, handler, http.MethodPost, "/api/v1/integrations", "doko_admin_demo", `{"family_key":"billing-api","version_key":"v1","display_name":"Billing API","description":"Vendor-neutral billing contract.","visibility":"private","lifecycle":"active"}`)
	if createdIntegration.Code != http.StatusCreated {
		t.Fatalf("create integration = %d: %s", createdIntegration.Code, createdIntegration.Body.String())
	}
	var integration model.Integration
	if err := json.Unmarshal(createdIntegration.Body.Bytes(), &integration); err != nil {
		t.Fatal(err)
	}
	prepareHTTPPrivateIntegrationFoundations(t, handler, integration.ID)

	unregisteredPoint := request(t, handler, http.MethodPost, "/api/v1/integrations/"+integration.ID+"/authorization-points", "doko_admin_demo", `{"key":"billing.invoice.delete","name":"Delete invoice","description":"Delete one invoice.","action_type":"destructive","required_grants":["billing.invoices.delete"],"confirmation_required":true,"decision_ttl_seconds":60,"state":"active"}`)
	if unregisteredPoint.Code != http.StatusBadRequest || !strings.Contains(unregisteredPoint.Body.String(), "register required grants") {
		t.Fatalf("unregistered authorization point = %d: %s", unregisteredPoint.Code, unregisteredPoint.Body.String())
	}

	createdGrant := request(t, handler, http.MethodPost, "/api/v1/grant-definitions", "doko_admin_demo", `{"key":"billing.invoices.delete","display_name":"Delete invoices","description":"Allows deletion of one invoice after confirmation.","risk":"critical","state":"active"}`)
	if createdGrant.Code != http.StatusCreated {
		t.Fatalf("create grant = %d: %s", createdGrant.Code, createdGrant.Body.String())
	}
	var grant model.GrantDefinition
	if err := json.Unmarshal(createdGrant.Body.Bytes(), &grant); err != nil {
		t.Fatal(err)
	}
	if grant.Key != "billing.invoices.delete" || grant.Revision != 1 {
		t.Fatalf("unexpected grant: %#v", grant)
	}

	createdPoint := request(t, handler, http.MethodPost, "/api/v1/integrations/"+integration.ID+"/authorization-points", "doko_admin_demo", `{"key":"billing.invoice.delete","name":"Delete invoice","description":"Delete one invoice.","action_type":"destructive","required_grants":["billing.invoices.delete"],"confirmation_required":true,"decision_ttl_seconds":60,"state":"active"}`)
	if createdPoint.Code != http.StatusCreated {
		t.Fatalf("create authorization point = %d: %s", createdPoint.Code, createdPoint.Body.String())
	}
	var point model.AuthorizationPoint
	if err := json.Unmarshal(createdPoint.Body.Bytes(), &point); err != nil {
		t.Fatal(err)
	}

	createdTool := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools", "doko_admin_demo", `{"organisation_id":"org_acme","namespace":"billing","name":"delete_invoice","description":"Delete one invoice after explicit confirmation.","input_schema":{"type":"object","additionalProperties":false,"properties":{"invoice_id":{"type":"string"}},"required":["invoice_id"]},"output_schema":{"type":"object","additionalProperties":false,"properties":{"deleted":{"type":"boolean"}},"required":["deleted"]},"endpoint":"https://api.vendor.example/v1/invoices/delete","http_method":"POST","authorization_policy":{"required_grants":["billing.invoices.delete"],"confirmation_required":true,"risk":"critical","idempotency_required":true},"timeout_ms":5000}`)
	if createdTool.Code != http.StatusCreated {
		t.Fatalf("create tool = %d: %s", createdTool.Code, createdTool.Body.String())
	}
	var tool model.Tool
	if err := json.Unmarshal(createdTool.Body.Bytes(), &tool); err != nil {
		t.Fatal(err)
	}

	invalidDryRun := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/dry-run", "doko_admin_demo", `{"arguments":{}}`)
	if invalidDryRun.Code != http.StatusBadRequest {
		t.Fatalf("invalid dry run = %d: %s", invalidDryRun.Code, invalidDryRun.Body.String())
	}
	validDryRun := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/dry-run", "doko_admin_demo", `{"arguments":{"invoice_id":"inv_42"}}`)
	if validDryRun.Code != http.StatusOK || !strings.Contains(validDryRun.Body.String(), `"network_call_performed":false`) || !strings.Contains(validDryRun.Body.String(), `"destination_origin":"https://api.vendor.example"`) {
		t.Fatalf("valid dry run = %d: %s", validDryRun.Code, validDryRun.Body.String())
	}

	updatedTool := request(t, handler, http.MethodPut, "/api/v1/products/prod_acme/tools/"+tool.ID, "doko_admin_demo", `{"description":"Delete exactly one invoice after explicit confirmation.","input_schema":{"type":"object","additionalProperties":false,"properties":{"invoice_id":{"type":"string"}},"required":["invoice_id"]},"output_schema":{"type":"object","additionalProperties":false,"properties":{"deleted":{"type":"boolean"}},"required":["deleted"]},"endpoint":"https://api.vendor.example/v2/invoices/delete","http_method":"POST","authorization_policy":{"required_grants":["billing.invoices.delete"],"confirmation_required":true,"risk":"critical","idempotency_required":true},"timeout_ms":7000,"revision":1}`)
	if updatedTool.Code != http.StatusOK {
		t.Fatalf("update tool = %d: %s", updatedTool.Code, updatedTool.Body.String())
	}
	if err := json.Unmarshal(updatedTool.Body.Bytes(), &tool); err != nil {
		t.Fatal(err)
	}
	if tool.Revision != 2 || tool.HTTPMethod != http.MethodPost || !strings.Contains(updatedTool.Body.String(), `"endpoint":"https://api.vendor.example/v2/invoices/delete"`) {
		t.Fatalf("tool update was incomplete: %#v", tool)
	}

	publishedTool := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/publish", "doko_admin_demo", `{"revision":2}`)
	if publishedTool.Code != http.StatusOK {
		t.Fatalf("publish tool = %d: %s", publishedTool.Code, publishedTool.Body.String())
	}
	if err := json.Unmarshal(publishedTool.Body.Bytes(), &tool); err != nil {
		t.Fatal(err)
	}
	if tool.State != "published" || tool.Revision != 3 {
		t.Fatalf("unexpected published tool: %#v", tool)
	}
	immutableUpdate := request(t, handler, http.MethodPut, "/api/v1/products/prod_acme/tools/"+tool.ID, "doko_admin_demo", `{"description":"Changed after publication.","input_schema":{"type":"object"},"output_schema":{"type":"object"},"endpoint":"https://api.vendor.example/v3/invoices/delete","http_method":"POST","authorization_policy":{"required_grants":["billing.invoices.delete"],"confirmation_required":true,"risk":"critical"},"timeout_ms":7000,"revision":3}`)
	if immutableUpdate.Code != http.StatusBadRequest || !strings.Contains(immutableUpdate.Body.String(), "immutable") {
		t.Fatalf("published tool update = %d: %s", immutableUpdate.Code, immutableUpdate.Body.String())
	}

	boundTools := request(t, handler, http.MethodPut, "/api/v1/integrations/"+integration.ID+"/tools", "doko_admin_demo", `{"tools":[{"tool_id":"`+tool.ID+`","revision":3,"authorization_point_id":"`+point.ID+`","authorization_point_revision":1}]}`)
	if boundTools.Code != http.StatusOK || !strings.Contains(boundTools.Body.String(), `"tool_revision":3`) || !strings.Contains(boundTools.Body.String(), `"authorization_point_revision":1`) {
		t.Fatalf("bind tool revision = %d: %s", boundTools.Code, boundTools.Body.String())
	}
	missingTools := request(t, handler, http.MethodPut, "/api/v1/integrations/"+integration.ID+"/tools", "doko_admin_demo", `{}`)
	if missingTools.Code != http.StatusBadRequest || !strings.Contains(missingTools.Body.String(), "tools is required") {
		t.Fatalf("missing tools = %d: %s", missingTools.Code, missingTools.Body.String())
	}
	invalidToolSelections := []struct {
		name    string
		body    string
		message string
	}{
		{name: "blank tool id", body: `{"tools":[{"tool_id":"   ","revision":3,"authorization_point_id":"` + point.ID + `","authorization_point_revision":1}]}`, message: "tool_id is required"},
		{name: "missing point", body: `{"tools":[{"tool_id":"` + tool.ID + `","revision":3}]}`, message: "authorization_point_id"},
		{name: "duplicate tool id", body: `{"tools":[{"tool_id":"` + tool.ID + `","revision":3,"authorization_point_id":"` + point.ID + `","authorization_point_revision":1},{"tool_id":"` + tool.ID + `","revision":4,"authorization_point_id":"` + point.ID + `","authorization_point_revision":1}]}`, message: "selected more than once"},
	}
	for _, test := range invalidToolSelections {
		t.Run(test.name, func(t *testing.T) {
			response := request(t, handler, http.MethodPut, "/api/v1/integrations/"+integration.ID+"/tools", "doko_admin_demo", test.body)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), test.message) {
				t.Fatalf("invalid tool binding replacement = %d: %s", response.Code, response.Body.String())
			}
		})
	}
	stillBound := request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID+"/tools", "doko_admin_demo", "")
	if stillBound.Code != http.StatusOK || !strings.Contains(stillBound.Body.String(), tool.ID) {
		t.Fatalf("missing tools request changed bindings = %d: %s", stillBound.Code, stillBound.Body.String())
	}
	var bindings struct {
		Items []model.IntegrationToolBinding `json:"items"`
	}
	if err := json.Unmarshal(stillBound.Body.Bytes(), &bindings); err != nil {
		t.Fatal(err)
	}
	if len(bindings.Items) != 1 || bindings.Items[0].ToolID != tool.ID || bindings.Items[0].ToolRevision != 3 || bindings.Items[0].AuthorizationPointID != point.ID || bindings.Items[0].AuthorizationPointRevision != point.Revision {
		t.Fatalf("invalid tool selections changed bindings: %#v", bindings.Items)
	}
	publishStatus := request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID, "doko_admin_demo", "")
	if publishStatus.Code != http.StatusOK || !strings.Contains(publishStatus.Body.String(), `"authorization_points":[{`) || !strings.Contains(publishStatus.Body.String(), `"tools":[{`) || !strings.Contains(publishStatus.Body.String(), `"ready":true`) {
		t.Fatalf("integration status omitted exact policy inputs = %d: %s", publishStatus.Code, publishStatus.Body.String())
	}
	publishedIntegration := preflightAndPublishIntegration(t, handler, integration.ID)
	if publishedIntegration.Code != http.StatusCreated || !strings.Contains(publishedIntegration.Body.String(), `"tool_revision":3`) || !strings.Contains(publishedIntegration.Body.String(), `"billing.invoice.delete"`) {
		t.Fatalf("publish integration = %d: %s", publishedIntegration.Code, publishedIntegration.Body.String())
	}

	missingCloneRevision := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/clone", "doko_admin_demo", `{"namespace":"billing","name":"delete_invoice_v2"}`)
	if missingCloneRevision.Code != http.StatusBadRequest || !strings.Contains(missingCloneRevision.Body.String(), "revision is required") {
		t.Fatalf("missing clone revision = %d: %s", missingCloneRevision.Code, missingCloneRevision.Body.String())
	}
	staleClone := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/clone", "doko_admin_demo", `{"namespace":"billing","name":"delete_invoice_v2","revision":2}`)
	if staleClone.Code != http.StatusConflict || !strings.Contains(staleClone.Body.String(), `"code":"revision_conflict"`) {
		t.Fatalf("stale clone revision = %d: %s", staleClone.Code, staleClone.Body.String())
	}
	clonedTool := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/clone", "doko_admin_demo", `{"namespace":"billing","name":"delete_invoice_v2","revision":3}`)
	if clonedTool.Code != http.StatusCreated || !strings.Contains(clonedTool.Body.String(), `"state":"draft"`) || !strings.Contains(clonedTool.Body.String(), `"name":"delete_invoice_v2"`) {
		t.Fatalf("clone tool = %d: %s", clonedTool.Code, clonedTool.Body.String())
	}
	retiredTool := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/retire", "doko_admin_demo", `{"revision":3}`)
	if retiredTool.Code != http.StatusOK || !strings.Contains(retiredTool.Body.String(), `"state":"retired"`) {
		t.Fatalf("retire tool = %d: %s", retiredTool.Code, retiredTool.Body.String())
	}
	invalidatedStatus := request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID, "doko_admin_demo", "")
	if invalidatedStatus.Code != http.StatusOK || !strings.Contains(invalidatedStatus.Body.String(), `"code":"tool_revision_unresolved"`) || !strings.Contains(invalidatedStatus.Body.String(), `"ready":false`) {
		t.Fatalf("retired binding did not fail readiness = %d: %s", invalidatedStatus.Code, invalidatedStatus.Body.String())
	}
}

func TestAuthorizationPatchRequiresCompleteReplacementWithoutMutation(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)

	createdIntegration := request(t, handler, http.MethodPost, "/api/v1/integrations", "doko_admin_demo", `{"family_key":"accounts-api","version_key":"v1","display_name":"Accounts API","description":"Account administration contract.","visibility":"private","lifecycle":"active"}`)
	if createdIntegration.Code != http.StatusCreated {
		t.Fatalf("create integration = %d: %s", createdIntegration.Code, createdIntegration.Body.String())
	}
	var integration model.Integration
	if err := json.Unmarshal(createdIntegration.Body.Bytes(), &integration); err != nil {
		t.Fatal(err)
	}

	createdGrant := request(t, handler, http.MethodPost, "/api/v1/grant-definitions", "doko_admin_demo", `{"key":"accounts.records.delete","display_name":"Delete account records","description":"Allows confirmed account deletion.","risk":"critical","state":"active"}`)
	if createdGrant.Code != http.StatusCreated {
		t.Fatalf("create grant = %d: %s", createdGrant.Code, createdGrant.Body.String())
	}
	var grant model.GrantDefinition
	if err := json.Unmarshal(createdGrant.Body.Bytes(), &grant); err != nil {
		t.Fatal(err)
	}

	createdPoint := request(t, handler, http.MethodPost, "/api/v1/integrations/"+integration.ID+"/authorization-points", "doko_admin_demo", `{"key":"accounts.record.delete","name":"Delete account","description":"Delete one account record.","action_type":"destructive","required_grants":["accounts.records.delete"],"confirmation_required":true,"decision_ttl_seconds":60,"state":"active"}`)
	if createdPoint.Code != http.StatusCreated {
		t.Fatalf("create authorization point = %d: %s", createdPoint.Code, createdPoint.Body.String())
	}
	var point model.AuthorizationPoint
	if err := json.Unmarshal(createdPoint.Body.Bytes(), &point); err != nil {
		t.Fatal(err)
	}

	withoutField := func(t *testing.T, source map[string]any, field string) string {
		t.Helper()
		value := make(map[string]any, len(source)-1)
		for key, item := range source {
			if key != field {
				value[key] = item
			}
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}

	grantReplacement := map[string]any{
		"key": grant.Key, "display_name": "Changed grant", "description": "Changed description.",
		"risk": "low", "state": "deprecated", "revision": grant.Revision,
	}
	for _, field := range []string{"key", "display_name", "description", "risk", "state", "revision"} {
		t.Run("grant missing "+field, func(t *testing.T) {
			response := request(t, handler, http.MethodPatch, "/api/v1/grant-definitions/"+grant.ID, "doko_admin_demo", withoutField(t, grantReplacement, field))
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_request") {
				t.Fatalf("incomplete grant replacement = %d: %s", response.Code, response.Body.String())
			}
		})
	}

	pointReplacement := map[string]any{
		"key": point.Key, "name": "Changed point", "description": "Changed description.", "action_type": "read",
		"required_grants": []string{}, "confirmation_required": false, "decision_ttl_seconds": 300,
		"state": "deprecated", "revision": point.Revision,
	}
	for _, field := range []string{"key", "name", "description", "action_type", "required_grants", "confirmation_required", "decision_ttl_seconds", "state", "revision"} {
		t.Run("authorization point missing "+field, func(t *testing.T) {
			response := request(t, handler, http.MethodPatch, "/api/v1/integrations/"+integration.ID+"/authorization-points/"+point.ID, "doko_admin_demo", withoutField(t, pointReplacement, field))
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_request") {
				t.Fatalf("incomplete authorization point replacement = %d: %s", response.Code, response.Body.String())
			}
		})
	}

	grantList := request(t, handler, http.MethodGet, "/api/v1/grant-definitions", "doko_admin_demo", "")
	var grants struct {
		Items []model.GrantDefinition `json:"items"`
	}
	if grantList.Code != http.StatusOK {
		t.Fatalf("list grants = %d: %s", grantList.Code, grantList.Body.String())
	}
	if err := json.Unmarshal(grantList.Body.Bytes(), &grants); err != nil {
		t.Fatal(err)
	}
	if len(grants.Items) != 1 || grants.Items[0].ID != grant.ID || grants.Items[0].DisplayName != grant.DisplayName || grants.Items[0].Description != grant.Description || grants.Items[0].Risk != grant.Risk || grants.Items[0].State != grant.State || grants.Items[0].Revision != grant.Revision {
		t.Fatalf("incomplete grant replacement mutated state: %#v", grants.Items)
	}

	pointList := request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID+"/authorization-points", "doko_admin_demo", "")
	var points struct {
		Items []model.AuthorizationPoint `json:"items"`
	}
	if pointList.Code != http.StatusOK {
		t.Fatalf("list authorization points = %d: %s", pointList.Code, pointList.Body.String())
	}
	if err := json.Unmarshal(pointList.Body.Bytes(), &points); err != nil {
		t.Fatal(err)
	}
	if len(points.Items) != 1 || points.Items[0].ID != point.ID || points.Items[0].Name != point.Name || points.Items[0].Description != point.Description || points.Items[0].ActionType != point.ActionType || len(points.Items[0].RequiredGrants) != 1 || points.Items[0].RequiredGrants[0] != grant.Key || points.Items[0].ConfirmationRequired != point.ConfirmationRequired || points.Items[0].DecisionTTLSeconds != point.DecisionTTLSeconds || points.Items[0].State != point.State || points.Items[0].Revision != point.Revision {
		t.Fatalf("incomplete authorization point replacement mutated state: %#v", points.Items)
	}
}

func TestToolCreationRejectsUnknownGrantAsClientError(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)
	created := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools", "doko_admin_demo", `{"organisation_id":"org_acme","namespace":"billing","name":"list_invoices","description":"List invoices.","input_schema":{"type":"object","additionalProperties":false,"properties":{}},"output_schema":{"type":"object","additionalProperties":false,"properties":{}},"endpoint":"https://api.vendor.example/v1/invoices","http_method":"GET","authorization_policy":{"required_grants":["billing.invoices.read"],"confirmation_required":false},"timeout_ms":5000}`)
	if created.Code != http.StatusBadRequest || !strings.Contains(created.Body.String(), "unregistered_grant") {
		t.Fatalf("unknown grant creation = %d: %s", created.Code, created.Body.String())
	}
	var failure struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Error.Code != "invalid_resource" {
		t.Fatalf("unexpected error contract: %#v", failure)
	}
}
