"use client";


import { useTranslation } from "react-i18next";
import { Check, ExternalLink, LockKeyhole, ShieldCheck, TriangleAlert, Users } from "lucide-react";

import { Badge, Button, Dialog, Switch } from "../../core/control";
import type { useMCPWorkspaceState } from "../use-mcp-workspace";

export function MCPDialogs({ workspace, connectionReady }: {
  workspace: ReturnType<typeof useMCPWorkspaceState>;
  connectionReady: boolean;
}) {
  const { t } = useTranslation();
  const {
    mcpConnectionOpen, setMCPConnectionOpen,
    mcpImportOpen, setMCPImportOpen,
    mcpBusy,
    mcpCatalog,
    mcpSelectedTools, setMCPSelectedTools,
    mcpImportFailures,
    mcpName, setMCPName,
    mcpNamespace, setMCPNamespace,
    mcpEndpoint, setMCPEndpoint,
    mcpAccessToken, setMCPAccessToken,
    mcpForwardUserIdentity, setMCPForwardUserIdentity,
    mcpGrants, setMCPGrants,
    mcpConfirmationRequired, setMCPConfirmationRequired,
    createMCPConnection,
    importMCPTools,
  } = workspace;

  return <>
    <Dialog
      open={mcpConnectionOpen}
      onClose={setMCPConnectionOpen}
      title={t("mcpDialogs.connectThirdPartyMCP")}
      description={t("mcpDialogs.registerOneFixedUpstreamInspectItsCompleteCatalogThen")}
      actions={<><Button outline onClick={() => setMCPConnectionOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={mcpBusy || !connectionReady} onClick={createMCPConnection}>{mcpBusy ? t("mcpDialogs.inspecting") : t("mcpDialogs.connectInspect")}</Button></>}
    >
      <div className="auth-form compact-form">
        <a className="protocol-policy" href="https://blog.modelcontextprotocol.io/posts/2026-07-28/" target="_blank" rel="noreferrer"><ShieldCheck /><span><strong>{t("mcpDialogs.statelessMCPv2Only")}</strong><small>{t("mcpDialogs.protocolRevisionN2026N07N28NoLogicalLiveSessions")}</small></span><ExternalLink /></a>
        <div className="two-fields"><label className="auth-field"><span>{t("mcpDialogs.connectionName")}</span><input value={mcpName} onChange={(event) => setMCPName(event.target.value)} placeholder={t("mcpDialogs.supportOperations")} /></label><label className="auth-field"><span>{t("mcpDialogs.localNamespace")}</span><input value={mcpNamespace} onChange={(event) => setMCPNamespace(event.target.value)} placeholder="support" pattern="[a-z][a-z0-9_]*" /></label></div>
        <label className="auth-field"><span>{t("mcpDialogs.fixedHTTPSMCPEndpoint")}</span><input type="url" value={mcpEndpoint} onChange={(event) => setMCPEndpoint(event.target.value)} placeholder="https://mcp.vendor.com/v2" /><small>{t("mcpDialogs.defaultHTTPSPortOnlyRedirectsAndPrivateNetworkDestinations")}</small></label>
        <label className="auth-field"><span>{t("mcpDialogs.upstreamAccessToken")}</span><input type="password" autoComplete="off" value={mcpAccessToken} onChange={(event) => setMCPAccessToken(event.target.value)} /><small>{t("mcpDialogs.encryptedServerSideAndUsedOnlyForThisFixed")}</small></label>
        <Switch checked={mcpForwardUserIdentity} onChange={setMCPForwardUserIdentity} label={t("mcpDialogs.forwardSignedDokoSokoUserIdentity")} />
        <div className="private-default-note"><Users />{t("mcpDialogs.theInboundBearerTokenIsNeverForwardedWhenEnabled")}</div>
      </div>
    </Dialog>

    <Dialog
      open={mcpImportOpen}
      onClose={setMCPImportOpen}
      title={t("mcpDialogs.reviewToolsFrom", { value1: String(mcpCatalog?.connection.name ?? "upstream MCP") })}
      description={t("mcpDialogs.upstreamNamesDescriptionsSchemasAndAnnotationsAreUntrustedInput")}
      actions={<><Button outline onClick={() => setMCPImportOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={mcpBusy || mcpSelectedTools.length === 0} onClick={importMCPTools}>{mcpBusy ? t("mcpDialogs.pinningSchemas") : t("mcpDialogs.importDrafts", { count: mcpSelectedTools.length })}</Button></>}
    >
      <div className="mcp-import-review">
        <div className="import-summary"><Badge color="violet">{t("mcpDialogs.statelessMCPv2Only")}</Badge><code>{mcpCatalog?.connection.namespace}.*</code><span>{mcpCatalog?.tools.length ?? 0} {t("mcpDialogs.catalogTools")}</span></div>
        {Object.keys(mcpImportFailures).length > 0 && <div className="capability-unavailable mcp-import-failures" role="alert"><TriangleAlert /><span><strong>{t("mcpDialogs.someToolsWereRejected")}</strong><small>{t("mcpDialogs.closeThisDialogResolveTheLocalConflictThenInspect")}</small>{Object.entries(mcpImportFailures).map(([name, reason]) => <span key={name}><code>{name}</code><small>{reason}</small></span>)}</span></div>}
        <div className="catalog-list">{mcpCatalog?.tools.map((tool) => <label className="catalog-tool" key={tool.name}><input type="checkbox" checked={mcpSelectedTools.includes(tool.name)} onChange={(event) => setMCPSelectedTools((items) => event.target.checked ? [...items, tool.name] : items.filter((name) => name !== tool.name))} /><span className="check-box">{mcpSelectedTools.includes(tool.name) && <Check />}</span><span><strong>{tool.title || tool.name}</strong><code>{tool.name}</code><small>{tool.description || t("mcpDialogs.noUpstreamDescription")}</small></span><Badge color="zinc">{tool.schema_hash.slice(0, 12)}</Badge></label>)}</div>
        <label className="auth-field"><span>{t("mcpDialogs.requiredGrants")}</span><input value={mcpGrants} onChange={(event) => setMCPGrants(event.target.value)} placeholder={t("mcpDialogs.supportWriteDeveloperPro")} /><small>{t("mcpDialogs.evaluatedBeforeEveryUpstreamCallAccessEvaluationFailuresDeny")}</small></label>
        <Switch checked={mcpConfirmationRequired} onChange={setMCPConfirmationRequired} label={t("mcpDialogs.requireUserConfirmationBeforeExecution")} />
        <div className="private-default-note"><LockKeyhole />{t("mcpDialogs.importPinsEachSchemaHashPublishedToolsFailClosed")}</div>
      </div>
    </Dialog>
  </>;
}
