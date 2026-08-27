"use client";


import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
  return useMemo<EntityDetail | null>(() => {
    if (consoleRoute.kind !== "entity") return null;
    const date = (value?: string | null) => value ? t("format.dateTime", { value: new Date(value) }) : "—";
    const fields = (values: Array<[string, unknown]>) => values.map(([label, value]) => ({ label, value: value === undefined || value === null || value === "" ? "—" : String(value) }));
    switch (consoleRoute.entity) {
      case "integration": {
        const value = integrations.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: t("entityDetails.api"), title: value.display_name, description: `${value.family_key} · ${value.version_key}`, fields: fields([[t("entityDetails.apiID"), value.id], [t("entityDetails.lifecycle"), value.lifecycle], [t("entityDetails.revision"), value.revision], [t("entityDetails.resources"), value.resources?.length ?? 0], [t("entityDetails.sdks"), value.sdks?.length ?? 0], [t("entityDetails.sunset"), date(value.sunset_at)]]) } : null;
      }
      case "resource-set": {
        const value = resourceSets.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: t("entityDetails.reusableResourceSet"), title: value.name, description: value.description || t("entityDetails.reusableAPIResourceConfiguration"), fields: fields([[t("entityDetails.uid"), value.id], [t("entityDetails.kind"), value.kind], [t("entityDetails.state"), value.state], [t("entityDetails.revision"), value.latest_revision?.revision ?? value.revision], [t("entityDetails.apis"), value.integration_ids?.length ?? 0]]) } : null;
      }
      case "source": {
        const value = sources.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: t("entityDetails.documentationSource"), title: value.name, description: value.location, fields: fields([[t("entityDetails.uid"), value.id], [t("entityDetails.kind"), value.kind], [t("entityDetails.visibility"), value.visibility], [t("entityDetails.crawlState"), value.crawlState], [t("entityDetails.pages"), value.pages], [t("entityDetails.revision"), value.revision]]) } : null;
      }
      case "tool": {
        const value = tools.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: t("entityDetails.tool"), title: `${value.namespace}.${value.name}`, description: value.description || t("entityDetails.agentFacingToolDefinition"), fields: fields([[t("entityDetails.uid"), value.id], [t("entityDetails.backend"), value.backend_kind ?? "http"], [t("entityDetails.method"), value.http_method], [t("entityDetails.state"), value.state], [t("entityDetails.revision"), value.revision]]) } : null;
      }
      case "connection": {
        const value = mcpConnections.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: t("entityDetails.mcpConnection"), title: value.name, description: value.endpoint, fields: fields([[t("entityDetails.uid"), value.id], [t("entityDetails.namespace"), value.namespace], [t("entityDetails.authentication"), t("entityDetails.accessToken")], [t("entityDetails.signedUserIdentity"), value.forward_user_identity], [t("entityDetails.state"), value.state], [t("entityDetails.lastInspected"), date(value.last_synced_at)]]) } : null;
      }
      case "report": {
        const value = reportSubmissions.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: t("entityDetails.supportOutbox"), title: value.summary, description: t("entityDetails.plaintextUserApprovedSupportSubmission"), fields: fields([[t("entityDetails.uid"), value.id], [t("entityDetails.kind"), value.kind], [t("entityDetails.state"), value.state], [t("entityDetails.api"), value.trusted_integration?.display_name], [t("entityDetails.created"), date(value.created_at)]]) } : null;
      }
      case "audit-event": {
        const value = auditEvents.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: t("entityDetails.auditEvent"), title: value.action, description: `${value.target_type} · ${value.target_id}`, fields: fields([[t("entityDetails.uid"), value.id], [t("entityDetails.actor"), value.actor_id], [t("entityDetails.request"), value.request_id], [t("entityDetails.created"), date(value.created_at)]]) } : null;
      }
      case "root-user": {
        const value = rootUsers.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: t("entityDetails.rootAdministrator"), title: value.display_name, description: value.email, fields: fields([[t("entityDetails.uid"), value.id], [t("entityDetails.role"), value.role], [t("entityDetails.status"), value.revoked_at ? t("entityDetails.revoked") : t("entityDetails.mfaActive")], [t("entityDetails.revoked"), date(value.revoked_at)]]) } : null;
      }
    }
  }, [consoleRoute, integrations, resourceSets, sources, tools, mcpConnections, reportSubmissions, auditEvents, rootUsers, t]);
}
