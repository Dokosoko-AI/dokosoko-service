"use client";

import { useMemo } from "react";

import type { APIAuditEvent, APIIntegration, APIMCPConnection, APIResourceSet, APISupportSubmission, APITool, APIUser } from "../../lib/api";
import type { ConsoleRoute } from "../../lib/console-routes";
import type { EntityDetail, Source } from "./shared";

export function useEntityDetail({ consoleRoute, integrations, resourceSets, sources, tools, mcpConnections, reportSubmissions, auditEvents, rootUsers }: {
  consoleRoute: ConsoleRoute;
  integrations: APIIntegration[];
  resourceSets: APIResourceSet[];
  sources: Source[];
  tools: APITool[];
  mcpConnections: APIMCPConnection[];
  reportSubmissions: APISupportSubmission[];
  auditEvents: APIAuditEvent[];
  rootUsers: APIUser[];
}) {
  return useMemo<EntityDetail | null>(() => {
    if (consoleRoute.kind !== "entity") return null;
    const date = (value?: string | null) => value ? new Date(value).toLocaleString() : "—";
    const fields = (values: Array<[string, unknown]>) => values.map(([label, value]) => ({ label, value: value === undefined || value === null || value === "" ? "—" : String(value) }));
    switch (consoleRoute.entity) {
      case "integration": {
        const value = integrations.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "API", title: value.display_name, description: `${value.family_key} · ${value.version_key}`, fields: fields([["API ID", value.id], ["Lifecycle", value.lifecycle], ["Revision", value.revision], ["Resources", value.resources?.length ?? 0], ["SDKs", value.sdks?.length ?? 0], ["Sunset", date(value.sunset_at)]]) } : null;
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
        return value ? { eyebrow: "Tool", title: `${value.namespace}.${value.name}`, description: value.description || "Agent-facing tool definition.", fields: fields([["UID", value.id], ["Backend", value.backend_kind ?? "http"], ["Method", value.http_method], ["State", value.state], ["Revision", value.revision]]) } : null;
      }
      case "connection": {
        const value = mcpConnections.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "MCP connection", title: value.name, description: value.endpoint, fields: fields([["UID", value.id], ["Namespace", value.namespace], ["Authentication", "access token"], ["Signed user identity", value.forward_user_identity], ["State", value.state], ["Last inspected", date(value.last_synced_at)]]) } : null;
      }
      case "report": {
        const value = reportSubmissions.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Support outbox", title: value.summary, description: "Plaintext, user-approved support submission.", fields: fields([["UID", value.id], ["Kind", value.kind], ["State", value.state], ["API", value.trusted_integration?.display_name], ["Created", date(value.created_at)]]) } : null;
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
  }, [consoleRoute, integrations, resourceSets, sources, tools, mcpConnections, reportSubmissions, auditEvents, rootUsers]);
}
