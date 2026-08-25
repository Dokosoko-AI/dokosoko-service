"use client";

import { BookOpen, KeyRound, Plus, Share2, ShieldCheck, TerminalSquare, TriangleAlert } from "lucide-react";
import { useEffect, useState } from "react";

import {
  APIError,
  api,
  type APIAccessConnection,
  type APIAuthorizationPoint,
  type APIIntegration,
  type APIIntegrationToolBinding,
  type APITool,
} from "../../../lib/api";
import { integrationPath, integrationToolBuilderPath, sectionPath } from "../../../lib/console-routes";
import { Badge, Button, Dialog } from "../../core/control";
import { PanelHeader } from "../../core/layout";
import {
  partitionIntegrationTools,
  toolCanAttachToIntegration,
  toolIsCommon,
  toolIsOwnedByIntegration,
} from "../../integrations/tool-scope";
import { ConsoleLink, EntityLink, unavailableConsoleCapability } from "../shared";

type IntegrationToolBindingSelection = { revision: number; authorizationPointID: string; authorizationPointRevision: number };

function integrationToolBindingSelectionSignature(selection: Record<string, IntegrationToolBindingSelection>) {
  return JSON.stringify(Object.entries(selection).sort(([left], [right]) => left.localeCompare(right)));
}

export function IntegrationToolsWorkspace({ integration, tools, providerManagementConnections, onMessage, onNavigate }: { integration: APIIntegration; tools: APITool[]; providerManagementConnections: APIAccessConnection[]; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [bindings, setBindings] = useState<APIIntegrationToolBinding[]>([]);
  const [authorizationPoints, setAuthorizationPoints] = useState<APIAuthorizationPoint[]>([]);
  const [bindingSelection, setBindingSelection] = useState<Record<string, IntegrationToolBindingSelection>>({});
  const [savedSignature, setSavedSignature] = useState<string | null>(null);
  const [bindingsLoading, setBindingsLoading] = useState(true);
  const [bindingsUnavailable, setBindingsUnavailable] = useState(false);
  const [bindingBusy, setBindingBusy] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [attachOpen, setAttachOpen] = useState(false);
  const [attachToolID, setAttachToolID] = useState("");
  const [attachPointID, setAttachPointID] = useState("");

  const activePoints = authorizationPoints.filter((point) => point.state === "active");
  const availableTools = tools.filter((tool) => tool.state === "published" && !tool.upstream_drifted && !bindingSelection[tool.id] && toolCanAttachToIntegration(tool, integration.id));
  const dirty = savedSignature !== null && integrationToolBindingSelectionSignature(bindingSelection) !== savedSignature;

  useEffect(() => {
    let cancelled = false;
    Promise.all([api.integrationToolBindings(integration.id), api.authorizationPoints(integration.id)]).then(([values, points]) => {
      if (cancelled) return;
      const next = Object.fromEntries(values.map((binding) => [binding.tool_id, { revision: binding.tool_revision, authorizationPointID: binding.authorization_point_id, authorizationPointRevision: binding.authorization_point_revision }]));
      setBindings(values);
      setAuthorizationPoints(points);
      setBindingSelection(next);
      setSavedSignature(integrationToolBindingSelectionSignature(next));
      setBindingsLoading(false);
      setBindingsUnavailable(false);
    }).catch(() => {
      if (cancelled) return;
      setBindings([]);
      setAuthorizationPoints([]);
      setBindingSelection({});
      setSavedSignature(null);
      setBindingsLoading(false);
      setBindingsUnavailable(true);
    });
    return () => { cancelled = true; };
  }, [integration.id, loadAttempt]);

  function retryBindings() {
    setBindingsLoading(true);
    setBindingsUnavailable(false);
    setLoadAttempt((value) => value + 1);
  }

  function openAttachDialog(toolID?: string) {
    const defaultTool = availableTools.find((tool) => tool.id === toolID) ?? availableTools[0];
    const defaultPoint = activePoints[0];
    setAttachToolID(defaultTool?.id ?? "");
    setAttachPointID(defaultPoint?.id ?? "");
    setAttachOpen(true);
  }

  function attachTool() {
    const tool = tools.find((candidate) => candidate.id === attachToolID && candidate.state === "published" && !candidate.upstream_drifted && toolCanAttachToIntegration(candidate, integration.id));
    const point = activePoints.find((candidate) => candidate.id === attachPointID);
    if (!tool || !point) return;
    setBindingSelection((current) => ({ ...current, [tool.id]: { revision: tool.revision, authorizationPointID: point.id, authorizationPointRevision: point.revision } }));
    setAttachOpen(false);
  }

  function removeBinding(toolID: string) {
    setBindingSelection((current) => {
      const next = { ...current };
      delete next[toolID];
      return next;
    });
	requestAnimationFrame(() => document.getElementById("save-api-bindings")?.focus());
  }

  function selectAuthorizationPoint(toolID: string, pointID: string) {
    const point = activePoints.find((candidate) => candidate.id === pointID);
    if (!point) return;
    setBindingSelection((current) => current[toolID] ? { ...current, [toolID]: { ...current[toolID], authorizationPointID: point.id, authorizationPointRevision: point.revision } } : current);
  }

  function selectCurrentToolRevision(toolID: string, revision: number) {
    setBindingSelection((current) => current[toolID] ? { ...current, [toolID]: { ...current[toolID], revision } } : current);
  }

  function resolveBinding(toolID: string, selection: IntegrationToolBindingSelection) {
    const tool = tools.find((candidate) => candidate.id === toolID) ?? bindings.find((binding) => binding.tool_id === toolID)?.tool;
    const point = authorizationPoints.find((candidate) => candidate.id === selection.authorizationPointID);
    const toolCurrent = Boolean(tool && tool.state === "published" && !tool.upstream_drifted && tool.revision === selection.revision && toolCanAttachToIntegration(tool, integration.id));
    const pointCurrent = Boolean(point && point.state === "active" && point.revision === selection.authorizationPointRevision);
    const issues = [
      !tool ? "tool missing" : !toolCanAttachToIntegration(tool, integration.id) ? "owned by another API" : tool.state !== "published" ? `tool ${tool.state}` : tool.upstream_drifted ? "schema drift" : tool.revision !== selection.revision ? `tool is now r${tool.revision}` : "",
      !point ? "authorization point missing" : point.state !== "active" ? `authorization ${point.state}` : point.revision !== selection.authorizationPointRevision ? `authorization is now r${point.revision}` : "",
    ].filter(Boolean);
    return { tool, point, current: toolCurrent && pointCurrent, issues };
  }

  async function saveBindings() {
    setBindingBusy(true);
    try {
      const value = await api.setIntegrationToolBindings(integration.id, Object.entries(bindingSelection).map(([tool_id, selection]) => ({ tool_id, revision: selection.revision, authorization_point_id: selection.authorizationPointID, authorization_point_revision: selection.authorizationPointRevision })));
      const next = Object.fromEntries(value.items.map((binding) => [binding.tool_id, { revision: binding.tool_revision, authorizationPointID: binding.authorization_point_id, authorizationPointRevision: binding.authorization_point_revision }]));
      setBindings(value.items);
      setBindingSelection(next);
      setSavedSignature(integrationToolBindingSelectionSignature(next));
      onMessage(value.items.length === 0 ? "All tool bindings cleared from this API draft." : `${value.items.length} exact tool revision${value.items.length === 1 ? "" : "s"} bound to this API draft.`);
    } catch (error) {
      onMessage(unavailableConsoleCapability(error) ? "Exact API tool bindings are not enabled in this deployment yet." : error instanceof APIError ? error.message : "Tool bindings could not be saved.");
    } finally { setBindingBusy(false); }
  }

  const selectedBindings = Object.entries(bindingSelection);
  const staleBindingCount = selectedBindings.filter(([toolID, selection]) => !resolveBinding(toolID, selection).current).length;
  const boundToolIDs = new Set(selectedBindings.map(([toolID]) => toolID));
  const toolGroups = partitionIntegrationTools(tools, boundToolIDs, integration.id);
  const apiOwnedToolIDs = new Set(toolGroups.apiOwned.map((tool) => tool.id));
  const commonToolIDs = new Set(toolGroups.attachedCommon.map((tool) => tool.id));
  const apiOwnedBindings = selectedBindings.filter(([toolID]) => apiOwnedToolIDs.has(toolID));
  const commonBindings = selectedBindings.filter(([toolID]) => commonToolIDs.has(toolID));
  const invalidBindings = selectedBindings.filter(([toolID]) => !apiOwnedToolIDs.has(toolID) && !commonToolIDs.has(toolID));
  const unboundAPITools = toolGroups.apiOwned.filter((tool) => !boundToolIDs.has(tool.id));
  const availableAPITools = availableTools.filter((tool) => toolIsOwnedByIntegration(tool, integration.id));
  const availableCommonTools = availableTools.filter(toolIsCommon);
  const reviewedDocumentation = integration.resources?.find((resource) => resource.kind === "documentation" && Boolean(resource.resolved_revision));
  const apiAdminConnection = providerManagementConnections.find((connection) => {
    if (connection.state !== "active") return false;
    const operationKeys = Object.keys(connection.definition?.operations ?? {}).map((key) => key.toLowerCase());
    return operationKeys.some((key) => /(credential|api[_-]?key)/.test(key) && /(create|issue|rotate|revoke)/.test(key));
  });
  const configuredAdminConnection = providerManagementConnections.find((connection) => connection.state === "active");
  const configuredEnvironmentVariable = typeof apiAdminConnection?.config.environment_variable === "string" ? apiAdminConnection.config.environment_variable : "";
  const familyEnvironmentVariable = `${integration.family_key.toUpperCase().replace(/[^A-Z0-9]+/g, "_").replace(/^_+|_+$/g, "").replace(/_API$/, "") || "SERVICE"}_API_KEY`;
  const adminEnvironmentVariable = configuredEnvironmentVariable === "SERVICE_API_KEY" || apiAdminConnection?.config.credential_scope === "shared" || apiAdminConnection?.config.shared === true ? "SERVICE_API_KEY" : familyEnvironmentVariable;

  const renderBindingRows = (entries: Array<[string, IntegrationToolBindingSelection]>) => entries.map(([toolID, selection]) => {
    const resolution = resolveBinding(toolID, selection);
    const tool = resolution.tool;
    const pointCurrent = resolution.point?.state === "active" && resolution.point.revision === selection.authorizationPointRevision;
    return <div className={`integration-tool-binding-row ${resolution.current ? "" : "stale"}`} key={toolID}>
      <span className="settings-icon">{tool?.backend_kind === "mcp" ? <Share2 /> : <TerminalSquare />}</span>
      <span className="integration-tool-binding-main">{tool ? <EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink> : <strong>{toolID}</strong>}<small>pinned tool revision {selection.revision}{tool ? ` · ${tool.backend_kind === "mcp" ? "MCP" : tool.http_method}` : ""}</small></span>
      <label className="tool-binding-action"><span className="sr-only">Authorization point for {tool ? `${tool.namespace}.${tool.name}` : toolID}</span><select disabled={bindingsLoading || bindingBusy} aria-label={`Authorization point for ${tool ? `${tool.namespace}.${tool.name}` : toolID}`} value={pointCurrent ? selection.authorizationPointID : ""} onChange={(event) => selectAuthorizationPoint(toolID, event.target.value)}>{!pointCurrent && <option value="" disabled>Choose a current authorization point</option>}{activePoints.map((point) => <option key={point.id} value={point.id}>{point.name} · r{point.revision}</option>)}</select><small>pinned authorization revision {selection.authorizationPointRevision}</small></label>
      <span className="tool-badges"><Badge color={resolution.current ? "green" : "red"}>{resolution.current ? "Current" : "Stale / unresolved"}</Badge>{tool?.upstream_drifted && <Badge color="red">schema drift</Badge>}<small className="binding-issue">{resolution.issues.join(" · ")}</small></span>
      <span className="binding-row-actions">{tool && tool.state === "published" && !tool.upstream_drifted && tool.revision !== selection.revision && <Button outline disabled={bindingsLoading || bindingBusy} onClick={() => selectCurrentToolRevision(toolID, tool.revision)}>Use r{tool.revision}</Button>}<Button outline disabled={bindingsLoading || bindingBusy} aria-label={`Remove ${tool ? `${tool.namespace}.${tool.name}` : toolID} from this API draft`} onClick={() => removeBinding(toolID)}>Remove</Button></span>
    </div>;
  });

  return <div className="integration-tab-content">
    {bindingsUnavailable && <div className="capability-unavailable"><TriangleAlert /><span><strong>Exact tool binding is unavailable.</strong><small>The current API bindings and authorization points could not be loaded. No changes can be saved.</small></span><Button outline onClick={retryBindings}>Retry</Button></div>}
    {activePoints.length === 0 && !bindingsLoading && !bindingsUnavailable && <div className="capability-unavailable"><ShieldCheck /><span><strong>Create an active action policy first.</strong><small>Every exposed tool must pin an exact authorization policy revision.</small></span><ConsoleLink path={integrationPath(integration.id, "access")} onNavigate={onNavigate} className="entity-back-link">Open Access</ConsoleLink></div>}
    <section className="panel integration-tool-bindings">
      <PanelHeader title="Built-in tools" description="DokoSoko exposes these API-scoped capabilities automatically when their reviewed source configuration is ready. They are not custom Tool records and do not need manual attachment." />
      <div className="provider-row"><span className="settings-icon"><BookOpen /></span><span><strong>Knowledge</strong><small><code>{integration.family_key}.knowledge.search</code> · {reviewedDocumentation ? `grounded in ${reviewedDocumentation.name}` : "requires attached reviewed documentation"}</small></span><Badge color={reviewedDocumentation ? "green" : "amber"}>{reviewedDocumentation ? "Automatic" : "Unavailable"}</Badge>{!reviewedDocumentation && <ConsoleLink path={integrationPath(integration.id, "documentation")} onNavigate={onNavigate} className="entity-back-link">Add documentation</ConsoleLink>}</div>
      <div className="provider-row"><span className="settings-icon"><KeyRound /></span><span><strong>API Admin</strong><small>{apiAdminConnection ? `${apiAdminConnection.name} · returns ${adminEnvironmentVariable}` : configuredAdminConnection ? `${configuredAdminConnection.name} does not declare credential issue, rotate, or revoke operations` : "requires an active Advanced provider-management connection"}</small></span><Badge color={apiAdminConnection ? "green" : "amber"}>{apiAdminConnection ? "Automatic" : "Unavailable"}</Badge>{!apiAdminConnection && <ConsoleLink path={integrationPath(integration.id, "access")} onNavigate={onNavigate} className="entity-back-link">Open Access Advanced</ConsoleLink>}</div>
    </section>
    <section className="panel integration-tool-summary">
      <PanelHeader title="Tools for this API" description="API-owned tools stay with this API. Common tools are reusable deployment capabilities attached here at an exact revision." action={<span className="heading-actions">{dirty && <Badge color="amber">Unsaved changes</Badge>}<ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link">Open common catalog</ConsoleLink><Button color="indigo" onClick={() => onNavigate(integrationToolBuilderPath(integration.id))}><Plus data-slot="icon" />Create API tool</Button><Button disabled={bindingsLoading || bindingBusy || bindingsUnavailable || activePoints.length === 0 || availableTools.length === 0} onClick={() => openAttachDialog()}><Plus data-slot="icon" />Attach tool</Button></span>} />
      <dl className="compact-metrics integration-tool-scope-summary"><div className="compact-metric"><dt>API owned</dt><dd><strong>{toolGroups.apiOwned.length}</strong><small>{apiOwnedBindings.length} attached</small></dd></div><div className="compact-metric"><dt>Common</dt><dd><strong>{commonBindings.length}</strong><small>attached here</small></dd></div><div className="compact-metric"><dt>Authorization</dt><dd><strong>{activePoints.length}</strong><small>active polic{activePoints.length === 1 ? "y" : "ies"}</small></dd></div></dl>
      <div className="panel-footer-actions"><small>{bindingsLoading ? "Loading current API configuration…" : `${selectedBindings.length} selected · ${bindings.length} currently saved${staleBindingCount > 0 ? ` · ${staleBindingCount} stale or unresolved` : ""}`}</small><Button id="save-api-bindings" disabled={bindingsLoading || bindingsUnavailable || bindingBusy || staleBindingCount > 0 || !dirty} onClick={saveBindings}>{bindingBusy ? "Saving…" : "Save API bindings"}</Button></div>
    </section>
    <section className="panel integration-tool-bindings">
      <PanelHeader title="API tools" description={`Definitions owned by ${integration.display_name}. They cannot be attached to another API.`} action={<span className="heading-actions"><Badge color="violet">{toolGroups.apiOwned.length} owned</Badge><Button color="indigo" onClick={() => onNavigate(integrationToolBuilderPath(integration.id))}><Plus data-slot="icon" />Create API tool</Button></span>} />
      {renderBindingRows(apiOwnedBindings)}
      {unboundAPITools.map((tool) => <div className="provider-row api-owned-tool-row" key={tool.id}><span className="settings-icon">{tool.backend_kind === "mcp" ? <Share2 /> : <TerminalSquare />}</span><span><EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink><small>Owned by this API · revision {tool.revision}</small></span><Badge color={tool.state === "published" && !tool.upstream_drifted ? "green" : tool.upstream_drifted ? "red" : "amber"}>{tool.upstream_drifted ? "Drifted" : tool.state}</Badge><Button outline disabled={bindingsLoading || bindingBusy || activePoints.length === 0 || !availableAPITools.some((candidate) => candidate.id === tool.id)} onClick={() => openAttachDialog(tool.id)}>Attach</Button></div>)}
      {!bindingsLoading && toolGroups.apiOwned.length === 0 && <div className="empty-row">No tool definition is owned by this API. Common tools can still be attached below.</div>}
      {bindingsLoading && <div className="empty-row">Loading API-owned tools…</div>}
    </section>
    <section className="panel integration-tool-bindings">
      <PanelHeader title="Attached common tools" description="Reusable deployment tools explicitly attached to this API draft." action={<Button outline disabled={bindingsLoading || bindingBusy || bindingsUnavailable || activePoints.length === 0 || availableCommonTools.length === 0} onClick={() => openAttachDialog()}><Plus data-slot="icon" />Attach common tool</Button>} />
      {renderBindingRows(commonBindings)}
      {!bindingsLoading && commonBindings.length === 0 && <div className="empty-row">No common tool is attached to this API.</div>}
      {bindingsLoading && <div className="empty-row">Loading common tool bindings…</div>}
    </section>
    {invalidBindings.length > 0 && <details className="panel advanced-details"><summary>Bindings that need review ({invalidBindings.length})</summary><div className="advanced-details-body integration-tool-bindings">{renderBindingRows(invalidBindings)}</div></details>}
    <Dialog open={attachOpen} onClose={setAttachOpen} title="Attach published tool" description="Choose one deployment tool and pin it to one active authorization-point revision for this API draft." actions={<><Button outline disabled={bindingBusy} onClick={() => setAttachOpen(false)}>Cancel</Button><Button color="indigo" disabled={bindingBusy || !attachToolID || !attachPointID} onClick={attachTool}>Attach tool</Button></>}>
      <div className="auth-form compact-form">
        <label className="auth-field"><span>Published tool</span><select value={attachToolID} onChange={(event) => setAttachToolID(event.target.value)}><option value="" disabled>No eligible tool selected</option>{availableAPITools.length > 0 && <optgroup label="Owned by this API">{availableAPITools.map((tool) => <option key={tool.id} value={tool.id}>{tool.namespace}.{tool.name} · r{tool.revision}</option>)}</optgroup>}{availableCommonTools.length > 0 && <optgroup label="Common tools">{availableCommonTools.map((tool) => <option key={tool.id} value={tool.id}>{tool.namespace}.{tool.name} · r{tool.revision}</option>)}</optgroup>}</select><small>Only common tools and tools owned by this API are eligible. Draft, retired, drifted, or foreign API tools fail closed.</small></label>
        <label className="auth-field"><span>Authorization point</span><select value={attachPointID} onChange={(event) => setAttachPointID(event.target.value)}><option value="" disabled>No active point selected</option>{activePoints.map((point) => <option key={point.id} value={point.id}>{point.name} · {point.action_type} · r{point.revision}</option>)}</select><small>The API pins this exact revision; later authorization changes make the binding stale until reviewed.</small></label>
      </div>
    </Dialog>
  </div>;
}
