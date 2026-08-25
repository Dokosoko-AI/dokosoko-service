"use client";

import { useMemo } from "react";

import type {
  APIAccessConnection,
  APIAccessDefinition,
  APIAuditEvent,
  APIIntegration,
  APIIntegrationRun,
  APIMCPConnection,
  APIProductInstallation,
  APIProductVersion,
  APIResourceSet,
  APISupportRoute,
  APISupportSubmission,
  APITool,
  APIUser,
  APIWidget,
} from "../../lib/api";
import type { ConsoleRoute } from "../../lib/console-routes";
import type { EntityDetail, Source } from "./shared";

export function useEntityDetail({ consoleRoute, integrations, widgets, resourceSets, sources, tools, mcpConnections, accessDefinitions, accessConnections, productInstallations, productVersions, integrationRuns, supportRoutes, reportSubmissions, auditEvents, rootUsers }: {
  consoleRoute: ConsoleRoute;
  integrations: APIIntegration[];
  widgets: APIWidget[];
  resourceSets: APIResourceSet[];
  sources: Source[];
  tools: APITool[];
  mcpConnections: APIMCPConnection[];
  accessDefinitions: APIAccessDefinition[];
  accessConnections: APIAccessConnection[];
  productInstallations: APIProductInstallation[];
  productVersions: APIProductVersion[];
  integrationRuns: APIIntegrationRun[];
  supportRoutes: APISupportRoute[];
  reportSubmissions: APISupportSubmission[];
  auditEvents: APIAuditEvent[];
  rootUsers: APIUser[];
}) {
return useMemo<EntityDetail | null>(() => {
  if (consoleRoute.kind !== "entity") return null;
  const date = (value?: string) => value ? new Date(value).toLocaleString() : "—";
  const fields = (values: Array<[string, unknown]>) => values.map(([label, value]) => ({ label, value: value === undefined || value === null || value === "" ? "—" : String(value) }));
  switch (consoleRoute.entity) {
    case "integration": {
      const value = integrations.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "API", title: value.display_name, description: `${value.family_key} · ${value.version_key}`, fields: fields([["API ID", value.id], ["Lifecycle", value.lifecycle], ["Revision", value.revision], ["Resources", value.resources?.length ?? 0], ["Access connections", value.access_connection_ids?.length ?? 0], ["Sunset", date(value.sunset_at)]]) } : null;
    }
    case "widget": {
      const value = widgets.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Authenticated widget", title: value.name, description: `${value.integration_ids.length} APIs · ${value.allowed_origins.length} origins`, fields: fields([["UID", value.id], ["State", value.state], ["Revision", value.revision]]) } : null;
    }
    case "resource-set": {
      const value = resourceSets.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Reusable resource set", title: value.name, description: value.description || "Reusable API resource configuration.", fields: fields([["UID", value.id], ["Kind", value.kind], ["State", value.state], ["Revision", value.latest_revision?.revision ?? value.revision], ["APIs", value.integration_ids?.length ?? 0]]) } : null;
    }
    case "source": {
      const value = sources.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Documentation source", title: value.name, description: value.location, fields: fields([["UID", value.id], ["Kind", value.kind], ["Visibility", value.visibility], ["Crawl state", value.crawlState], ["Pages", value.pages], ["Revision", value.revision]]) } : null;
    }
    case "tool": {
      const value = tools.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Tool", title: `${value.namespace}.${value.name}`, description: value.description || "Agent-facing tool definition.", fields: fields([["UID", value.id], ["Backend", value.backend_kind ?? "http"], ["Method", value.http_method], ["State", value.state], ["Revision", value.revision], ["Timeout", `${value.timeout_ms} ms`]]) } : null;
    }
    case "connection": {
      const value = mcpConnections.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "MCP connection", title: value.name, description: value.endpoint, fields: fields([["UID", value.id], ["Namespace", value.namespace], ["Protocol", value.protocol_version], ["Authentication", value.auth_mode], ["State", value.state], ["Last inspected", date(value.last_synced_at)]]) } : null;
    }
    case "access-definition": {
      const value = accessDefinitions.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Access definition", title: value.name, description: `${value.instance_label_plural} managed by the provider.`, fields: fields([["UID", value.id], ["Service key", value.service_key], ["Cardinality", value.instance_cardinality], ["Credential scope", value.credential_scope], ["Authentication", value.management_auth_type], ["State", value.state]]) } : null;
    }
    case "access-connection": {
      const value = accessConnections.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Service connection", title: value.name, description: value.definition?.name || "Provider-owned service connection.", fields: fields([["UID", value.id], ["State", value.state], ["Region", value.region], ["Environment", value.environment_id], ["APIs", value.integration_ids?.length ?? 0], ["Revision", value.revision]]) } : null;
    }
    case "installation": {
      const value = productInstallations.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Installation", title: value.name, description: value.external_id, fields: fields([["UID", value.id], ["Customer account", value.customer_account_id], ["Environment", value.environment_id], ["State", value.state], ["Revision", value.revision], ["Updated", date(value.updated_at)]]) } : null;
    }
    case "release": {
      const value = productVersions.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Compatibility snapshot", title: value.version, description: value.diff.summary, fields: fields([["UID", value.id], ["Profile", value.profile_name], ["Stage", value.release_stage], ["Promotion", value.promotion_state], ["Rollout", `${value.rollout_percentage}%`], ["Manifest", value.manifest_hash]]) } : null;
    }
    case "run": {
      const value = integrationRuns.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Connector run", title: value.requested_outcome, description: `Run ${value.id}`, fields: fields([["UID", value.id], ["State", value.state], ["Environment", value.environment_id], ["Reported success", value.reported_success], ["Validated success", value.validated_success], ["Started", date(value.started_at)], ["Finished", date(value.finished_at)]]) } : null;
    }
    case "support-route": {
      const value = supportRoutes.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Reporting policy", title: value.name, description: value.is_default ? "Default policy for unassigned APIs." : "API-specific support delivery.", fields: fields([["UID", value.id], ["State", value.state], ["Bug reports", value.bug_reports_enabled ? "Enabled" : "Disabled"], ["Feedback", value.feedback_enabled ? "Enabled" : "Disabled"], ["Retention", `${value.retention_days} days`], ["APIs", value.integration_ids?.length ?? 0]]) } : null;
    }
    case "report": {
      const value = reportSubmissions.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Report submission", title: value.summary, description: "Sanitized submission metadata. Decrypted report content remains administrator-gated.", fields: fields([["UID", value.id], ["Kind", value.kind], ["State", value.state], ["API", value.trusted_integration?.display_name], ["Delivery attempts", value.attempts], ["Created", date(value.created_at)]]) } : null;
    }
    case "audit-event": {
      const value = auditEvents.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Audit event", title: value.action, description: `${value.target_type} · ${value.target_id}`, fields: fields([["UID", value.id], ["Actor", value.actor_id], ["Request", value.request_id], ["Created", date(value.created_at)]]) } : null;
    }
    case "root-user": {
      const value = rootUsers.find((item) => item.id === consoleRoute.uid);
      return value ? { eyebrow: "Root administrator", title: value.display_name, description: value.email, fields: fields([["UID", value.id], ["Role", value.role], ["Status", value.revoked_at ? "Revoked" : "MFA active"], ["Revoked", date(value.revoked_at)]]) } : null;
    }
  }
}, [consoleRoute, integrations, widgets, resourceSets, sources, tools, mcpConnections, accessDefinitions, accessConnections, productInstallations, productVersions, integrationRuns, supportRoutes, reportSubmissions, auditEvents, rootUsers]);

}

