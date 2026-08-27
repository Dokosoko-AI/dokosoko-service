"use client";


import { useTranslation } from "react-i18next";
import { BookOpen, Plus, Share2, ShieldCheck, TerminalSquare, TriangleAlert } from "lucide-react";
import { useEffect, useState } from "react";

import {
  APIError,
  api,
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

export function IntegrationToolsWorkspace({ integration, tools, onMessage, onNavigate }: { integration: APIIntegration; tools: APITool[]; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
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
    const toolState = tool?.state === "draft" ? t("enumLabels.draft") : tool?.state === "published" ? t("enumLabels.published") : tool?.state === "retired" ? t("enumLabels.retired") : tool?.state ?? "";
    const pointState = point?.state === "active" ? t("enumLabels.active") : point?.state === "deprecated" ? t("enumLabels.deprecated") : point?.state ?? "";
    const issues = [
      !tool ? t("integrationTools.toolMissing") : !toolCanAttachToIntegration(tool, integration.id) ? t("integrationTools.ownedByAnotherAPI") : tool.state !== "published" ? t("integrationTools.toolState", { state: String(toolState) }) : tool.upstream_drifted ? t("integrationTools.schemaDrift") : tool.revision !== selection.revision ? t("integrationTools.toolIsNowRevision", { revision: tool.revision }) : "",
      !point ? t("integrationTools.authorizationPointMissing") : point.state !== "active" ? t("integrationTools.authorizationState", { state: String(pointState) }) : point.revision !== selection.authorizationPointRevision ? t("integrationTools.authorizationIsNowRevision", { revision: point.revision }) : "",
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
      onMessage(value.items.length === 0 ? t("integrationTools.allToolBindingsClearedFromThisAPIDraft") : t("integrationTools.exactToolRevisionsBound", { count: value.items.length }));
    } catch (error) {
      onMessage(unavailableConsoleCapability(error) ? t("integrationTools.exactAPIToolBindingsAreNotEnabledInThis") : error instanceof APIError ? error.message : t("integrationTools.toolBindingsCouldNotBeSaved"));
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
  const renderBindingRows = (entries: Array<[string, IntegrationToolBindingSelection]>) => entries.map(([toolID, selection]) => {
    const resolution = resolveBinding(toolID, selection);
    const tool = resolution.tool;
    const pointCurrent = resolution.point?.state === "active" && resolution.point.revision === selection.authorizationPointRevision;
    return <div className={`integration-tool-binding-row ${resolution.current ? "" : "stale"}`} key={toolID}>
      <span className="settings-icon">{tool?.backend_kind === "mcp" ? <Share2 /> : <TerminalSquare />}</span>
      <span className="integration-tool-binding-main">{tool ? <EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink> : <strong>{toolID}</strong>}<small>{t("integrationTools.pinnedToolRevision")} {selection.revision}{tool ? t("integrationTools.copy", { http_method: String(tool.backend_kind === "mcp" ? "MCP" : tool.http_method) }) : ""}</small></span>
      <label className="tool-binding-action"><span className="sr-only">{t("integrationTools.authorizationPointFor")} {tool ? t("integrationTools.copy2", { namespace: String(tool.namespace), name: String(tool.name) }) : toolID}</span><select disabled={bindingsLoading || bindingBusy} aria-label={t("integrationTools.authorizationPointFor2", { toolID: String(tool ? `${tool.namespace}.${tool.name}` : toolID) })} value={pointCurrent ? selection.authorizationPointID : ""} onChange={(event) => selectAuthorizationPoint(toolID, event.target.value)}>{!pointCurrent && <option value="" disabled>{t("integrationTools.chooseACurrentAuthorizationPoint")}</option>}{activePoints.map((point) => <option key={point.id} value={point.id}>{point.name} {t("integrationTools.r")}{point.revision}</option>)}</select><small>{t("integrationTools.pinnedAuthorizationRevision")} {selection.authorizationPointRevision}</small></label>
      <span className="tool-badges"><Badge color={resolution.current ? "green" : "red"}>{resolution.current ? t("integrationTools.current") : t("integrationTools.staleUnresolved")}</Badge>{tool?.upstream_drifted && <Badge color="red">{t("integrationTools.schemaDrift")}</Badge>}<small className="binding-issue">{resolution.issues.join(" · ")}</small></span>
      <span className="binding-row-actions">{tool && tool.state === "published" && !tool.upstream_drifted && tool.revision !== selection.revision && <Button outline disabled={bindingsLoading || bindingBusy} onClick={() => selectCurrentToolRevision(toolID, tool.revision)}>{t("integrationTools.useR")}{tool.revision}</Button>}<Button outline disabled={bindingsLoading || bindingBusy} aria-label={t("integrationTools.removeFromThisAPIDraft", { toolID: String(tool ? `${tool.namespace}.${tool.name}` : toolID) })} onClick={() => removeBinding(toolID)}>{t("integrationTools.remove")}</Button></span>
    </div>;
  });

  return <div className="integration-tab-content">
    {bindingsUnavailable && <div className="capability-unavailable"><TriangleAlert /><span><strong>{t("integrationTools.exactToolBindingIsUnavailable")}</strong><small>{t("integrationTools.theCurrentAPIBindingsAndAuthorizationPointsCouldNot")}</small></span><Button outline onClick={retryBindings}>{t("common.retry")}</Button></div>}
    {activePoints.length === 0 && !bindingsLoading && !bindingsUnavailable && <div className="capability-unavailable"><ShieldCheck /><span><strong>{t("integrationTools.createAnActiveActionPolicyFirst")}</strong><small>{t("integrationTools.everyExposedToolMustPinAnExactAuthorizationPolicy")}</small></span><ConsoleLink path={integrationPath(integration.id, "access")} onNavigate={onNavigate} className="entity-back-link">{t("integrationTools.openAccess")}</ConsoleLink></div>}
    <section className="panel integration-tool-bindings">
      <PanelHeader title={t("integrationTools.builtInTools")} description={t("integrationTools.dokosokoExposesTheseAPIScopedCapabilitiesAutomaticallyWhenTheir")} />
      <div className="provider-row"><span className="settings-icon"><BookOpen /></span><span><strong>{t("integrationTools.knowledge")}</strong><small><code>{integration.family_key}.knowledge.search</code> · {reviewedDocumentation ? t("integrationTools.groundedIn", { name: String(reviewedDocumentation.name) }) : t("integrationTools.requiresAttachedReviewedDocumentation")}</small></span><Badge color={reviewedDocumentation ? "green" : "amber"}>{reviewedDocumentation ? t("integrationTools.automatic") : t("integrationTools.unavailable")}</Badge>{!reviewedDocumentation && <ConsoleLink path={integrationPath(integration.id, "documentation")} onNavigate={onNavigate} className="entity-back-link">{t("integrationTools.addDocumentation")}</ConsoleLink>}</div>
    </section>
    <section className="panel integration-tool-summary">
      <PanelHeader title={t("integrationTools.toolsForThisAPI")} description={t("integrationTools.apiOwnedToolsStayWithThisAPICommonTools")} action={<span className="heading-actions">{dirty && <Badge color="amber">{t("integrationTools.unsavedChanges")}</Badge>}<ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link">{t("integrationTools.openCommonCatalog")}</ConsoleLink><Button color="indigo" onClick={() => onNavigate(integrationToolBuilderPath(integration.id))}><Plus data-slot="icon" />{t("integrationTools.createAPITool")}</Button><Button disabled={bindingsLoading || bindingBusy || bindingsUnavailable || activePoints.length === 0 || availableTools.length === 0} onClick={() => openAttachDialog()}><Plus data-slot="icon" />{t("integrationTools.attachTool")}</Button></span>} />
      <dl className="compact-metrics integration-tool-scope-summary"><div className="compact-metric"><dt>{t("integrationTools.apiOwned")}</dt><dd><strong>{toolGroups.apiOwned.length}</strong><small>{t("integrationTools.attachments", { count: apiOwnedBindings.length })}</small></dd></div><div className="compact-metric"><dt>{t("integrationTools.common")}</dt><dd><strong>{commonBindings.length}</strong><small>{t("integrationTools.attachedHere")}</small></dd></div><div className="compact-metric"><dt>{t("integrationTools.authorization")}</dt><dd><strong>{activePoints.length}</strong><small>{t("integrationTools.activePolicies", { count: activePoints.length })}</small></dd></div></dl>
      <div className="panel-footer-actions"><small>{bindingsLoading ? t("integrationTools.loadingCurrentAPIConfiguration") : staleBindingCount > 0 ? t("integrationTools.selectedSavedAndStale", { selected: selectedBindings.length, saved: bindings.length, stale: staleBindingCount }) : t("integrationTools.selectedAndSaved", { selected: selectedBindings.length, saved: bindings.length })}</small><Button id="save-api-bindings" disabled={bindingsLoading || bindingsUnavailable || bindingBusy || staleBindingCount > 0 || !dirty} onClick={saveBindings}>{bindingBusy ? t("common.saving") : t("integrationTools.saveAPIBindings")}</Button></div>
    </section>
    <section className="panel integration-tool-bindings">
      <PanelHeader title={t("integrationTools.apiTools")} description={t("integrationTools.definitionsOwnedByTheyCannotBeAttachedToAnother", { display_name: String(integration.display_name) })} action={<span className="heading-actions"><Badge color="violet">{t("integrationTools.ownedTools", { count: toolGroups.apiOwned.length })}</Badge><Button color="indigo" onClick={() => onNavigate(integrationToolBuilderPath(integration.id))}><Plus data-slot="icon" />{t("integrationTools.createAPITool")}</Button></span>} />
      {renderBindingRows(apiOwnedBindings)}
      {unboundAPITools.map((tool) => <div className="provider-row api-owned-tool-row" key={tool.id}><span className="settings-icon">{tool.backend_kind === "mcp" ? <Share2 /> : <TerminalSquare />}</span><span><EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink><small>{t("integrationTools.ownedByThisAPIRevision")} {tool.revision}</small></span><Badge color={tool.state === "published" && !tool.upstream_drifted ? "green" : tool.upstream_drifted ? "red" : "amber"}>{tool.upstream_drifted ? t("integrationTools.drifted") : tool.state}</Badge><Button outline disabled={bindingsLoading || bindingBusy || activePoints.length === 0 || !availableAPITools.some((candidate) => candidate.id === tool.id)} onClick={() => openAttachDialog(tool.id)}>{t("integrationTools.attach")}</Button></div>)}
      {!bindingsLoading && toolGroups.apiOwned.length === 0 && <div className="empty-row">{t("integrationTools.noToolDefinitionIsOwnedByThisAPICommon")}</div>}
      {bindingsLoading && <div className="empty-row">{t("integrationTools.loadingAPIOwnedTools")}</div>}
    </section>
    <section className="panel integration-tool-bindings">
      <PanelHeader title={t("integrationTools.attachedCommonTools")} description={t("integrationTools.reusableDeploymentToolsExplicitlyAttachedToThisAPIDraft")} action={<Button outline disabled={bindingsLoading || bindingBusy || bindingsUnavailable || activePoints.length === 0 || availableCommonTools.length === 0} onClick={() => openAttachDialog()}><Plus data-slot="icon" />{t("integrationTools.attachCommonTool")}</Button>} />
      {renderBindingRows(commonBindings)}
      {!bindingsLoading && commonBindings.length === 0 && <div className="empty-row">{t("integrationTools.noCommonToolIsAttachedToThisAPI")}</div>}
      {bindingsLoading && <div className="empty-row">{t("integrationTools.loadingCommonToolBindings")}</div>}
    </section>
    {invalidBindings.length > 0 && <details className="panel advanced-details"><summary>{t("integrationTools.bindingsThatNeedReview")}{invalidBindings.length})</summary><div className="advanced-details-body integration-tool-bindings">{renderBindingRows(invalidBindings)}</div></details>}
    <Dialog open={attachOpen} onClose={setAttachOpen} title={t("integrationTools.attachPublishedTool")} description={t("integrationTools.chooseOneDeploymentToolAndPinItToOne")} actions={<><Button outline disabled={bindingBusy} onClick={() => setAttachOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={bindingBusy || !attachToolID || !attachPointID} onClick={attachTool}>{t("integrationTools.attachTool")}</Button></>}>
      <div className="auth-form compact-form">
        <label className="auth-field"><span>{t("integrationTools.publishedTool")}</span><select value={attachToolID} onChange={(event) => setAttachToolID(event.target.value)}><option value="" disabled>{t("integrationTools.noEligibleToolSelected")}</option>{availableAPITools.length > 0 && <optgroup label={t("integrationTools.ownedByThisAPI")}>{availableAPITools.map((tool) => <option key={tool.id} value={tool.id}>{tool.namespace}.{tool.name} {t("integrationTools.r")}{tool.revision}</option>)}</optgroup>}{availableCommonTools.length > 0 && <optgroup label={t("integrationTools.commonTools")}>{availableCommonTools.map((tool) => <option key={tool.id} value={tool.id}>{tool.namespace}.{tool.name} {t("integrationTools.r")}{tool.revision}</option>)}</optgroup>}</select><small>{t("integrationTools.onlyCommonToolsAndToolsOwnedByThisAPI")}</small></label>
        <label className="auth-field"><span>{t("integrationTools.authorizationPoint")}</span><select value={attachPointID} onChange={(event) => setAttachPointID(event.target.value)}><option value="" disabled>{t("integrationTools.noActivePointSelected")}</option>{activePoints.map((point) => <option key={point.id} value={point.id}>{point.name} · {point.action_type} {t("integrationTools.r")}{point.revision}</option>)}</select><small>{t("integrationTools.theAPIPinsThisExactRevisionLaterAuthorizationChanges")}</small></label>
      </div>
    </Dialog>
  </div>;
}
