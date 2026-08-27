"use client";


import { useTranslation } from "react-i18next";
import { KeyRound, Plus, ShieldCheck, TriangleAlert } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import {
  APIError,
  api,
  type APIAuthorizationPoint,
  type APIGrantDefinition,
  type APIIntegration,
} from "../../../lib/api";
import { Badge, Button, Dialog } from "../../core/control";
import { PanelHeader, SectionHeader } from "../../core/layout";
import { unavailableConsoleCapability } from "../shared";

export function AuthorizationPolicyWorkspace({ integration, onMessage }: { integration: APIIntegration; onMessage: (message: string) => void }) {
  const { t } = useTranslation();
  const [definitions, setDefinitions] = useState<APIGrantDefinition[]>([]);
  const [points, setPoints] = useState<APIAuthorizationPoint[]>([]);
  const [catalogUnavailable, setCatalogUnavailable] = useState(false);
  const [busy, setBusy] = useState(false);
  const [grantOpen, setGrantOpen] = useState(false);
  const [editingGrant, setEditingGrant] = useState<APIGrantDefinition | null>(null);
  const [grantKey, setGrantKey] = useState("");
  const [grantName, setGrantName] = useState("");
  const [grantDescription, setGrantDescription] = useState("");
  const [grantRisk, setGrantRisk] = useState<APIGrantDefinition["risk"]>("low");
  const [grantState, setGrantState] = useState<APIGrantDefinition["state"]>("active");
  const [pointOpen, setPointOpen] = useState(false);
  const [editingPoint, setEditingPoint] = useState<APIAuthorizationPoint | null>(null);
  const [pointKey, setPointKey] = useState("");
  const [pointName, setPointName] = useState("");
  const [pointDescription, setPointDescription] = useState("");
  const [pointAction, setPointAction] = useState<APIAuthorizationPoint["action_type"]>("read");
  const [pointGrants, setPointGrants] = useState<string[]>([]);
  const [pointConfirmation, setPointConfirmation] = useState(false);
  const [pointTTL, setPointTTL] = useState("300");
  const [pointState, setPointState] = useState<APIAuthorizationPoint["state"]>("draft");
  const integrationID = integration.id;

  const loadAuthorization = useCallback(async () => {
    setPoints([]);
    const pointRequest = integrationID ? api.authorizationPoints(integrationID) : Promise.resolve([] as APIAuthorizationPoint[]);
    const [definitionResult, pointResult] = await Promise.allSettled([api.grantDefinitions(), pointRequest]);
    if (definitionResult.status === "fulfilled") setDefinitions(definitionResult.value);
    if (pointResult.status === "fulfilled") setPoints(pointResult.value);
    const unavailable = (definitionResult.status === "rejected" && unavailableConsoleCapability(definitionResult.reason)) || (pointResult.status === "rejected" && unavailableConsoleCapability(pointResult.reason));
    setCatalogUnavailable(unavailable);
  }, [integrationID]);

  useEffect(() => {
    const task = window.setTimeout(() => { void loadAuthorization(); }, 0);
    return () => window.clearTimeout(task);
  }, [loadAuthorization]);

  const registeredKeys = new Set(definitions.filter((definition) => definition.state === "active").map((definition) => definition.key));

  function openGrant(value?: APIGrantDefinition) {
    setEditingGrant(value ?? null); setGrantKey(value?.key ?? ""); setGrantName(value?.display_name ?? ""); setGrantDescription(value?.description ?? ""); setGrantRisk(value?.risk ?? "low"); setGrantState(value?.state ?? "active"); setGrantOpen(true);
  }

  async function saveGrant() {
    setBusy(true);
    try {
      const input = { key: grantKey.trim(), display_name: grantName.trim(), description: grantDescription.trim(), risk: grantRisk, state: grantState };
      if (editingGrant) await api.updateGrantDefinition(editingGrant.id, { ...input, revision: editingGrant.revision }); else await api.createGrantDefinition(input);
      await loadAuthorization(); setGrantOpen(false); onMessage(editingGrant ? t("authorization.grantDefinitionUpdated") : t("authorization.grantRegisteredForPolicyUse"));
    } catch (error) { onMessage(error instanceof APIError ? error.message : t("authorization.grantDefinitionCouldNotBeSaved")); } finally { setBusy(false); }
  }

  function openPoint(value?: APIAuthorizationPoint) {
    setEditingPoint(value ?? null); setPointKey(value?.key ?? ""); setPointName(value?.name ?? ""); setPointDescription(value?.description ?? ""); setPointAction(value?.action_type ?? "read"); setPointGrants(value?.required_grants ?? []); setPointConfirmation(value?.confirmation_required ?? false); setPointTTL(String(value?.decision_ttl_seconds ?? 300)); setPointState(value?.state ?? "draft"); setPointOpen(true);
  }

  async function savePoint() {
    setBusy(true);
    try {
      const input = { key: pointKey.trim(), name: pointName.trim(), description: pointDescription.trim(), action_type: pointAction, required_grants: pointGrants, confirmation_required: pointAction === "destructive" ? true : pointConfirmation, decision_ttl_seconds: Number(pointTTL), state: pointState };
      if (editingPoint) await api.updateAuthorizationPoint(integration.id, editingPoint.id, { ...input, revision: editingPoint.revision }); else await api.createAuthorizationPoint(integration.id, input);
      await loadAuthorization(); setPointOpen(false); onMessage(editingPoint ? t("authorization.actionPolicyUpdated") : t("authorization.actionPolicyCreated"));
    } catch (error) { onMessage(error instanceof APIError ? error.message : t("authorization.actionPolicyCouldNotBeSaved")); } finally { setBusy(false); }
  }

  return <>
    <SectionHeader title={t("authorization.actionPolicies")} description={t("authorization.defineTheExactActionsAndGrantsToolsRequire", { display_name: String(integration.display_name) })} />
    <div className="notice authorization-policy-notice"><ShieldCheck /><span><strong>{t("authorization.policiesDoNotAuthenticateCustomers")}</strong> {t("authorization.theConfiguredIdentityProviderResolvesTheCustomerFirstThese")}</span></div>
    {catalogUnavailable && <div className="capability-unavailable"><TriangleAlert /><span><strong>{t("authorization.authorizationCatalogueUnavailable")}</strong><small>{t("authorization.existingToolPoliciesRemainVisibleButGrantAndAction")}</small></span></div>}
    <section className="panel"><PanelHeader title={t("authorization.apiActionPolicies")} description={t("authorization.eachToolBindingPinsOneExactActivePolicyRevision")} action={<Button disabled={catalogUnavailable || definitions.every((definition) => definition.state !== "active")} onClick={() => openPoint()}><Plus data-slot="icon" />{t("authorization.addPolicy")}</Button>} />{points.map((point) => <div className="provider-row authorization-point-row" key={point.id}><span className="settings-icon"><ShieldCheck /></span><span><strong>{point.name}</strong><small><code>{point.key}</code> · {point.required_grants.join(", ") || t("authorization.noGrants")} {t("authorization.ttlSeconds", { seconds: point.decision_ttl_seconds })}</small></span><span className="tool-badges"><Badge color={point.action_type === "destructive" ? "red" : point.action_type === "write" ? "amber" : "blue"}>{point.action_type}</Badge>{point.confirmation_required && <Badge color="violet">{t("authorization.confirmation")}</Badge>}<Badge color={point.state === "active" ? "green" : "zinc"}>{point.state}</Badge></span><Button outline onClick={() => openPoint(point)}>{t("authorization.edit")}</Button></div>)}{points.length === 0 && <div className="empty-row">{t("authorization.noActionPolicyHasBeenConfiguredFor")} {integration.display_name}{t("authorization.registerAGrantInAdvancedThenAddTheFirst")}</div>}</section>
    <details className="panel advanced-details"><summary>{t("authorization.deploymentGrantRegistryAdvanced")}</summary><div className="advanced-details-body"><PanelHeader title={t("authorization.grantRegistry")} description={t("authorization.deploymentOwnedNamesThatYourAuthorizationAPIMayReturn")} action={<span className="heading-actions"><Badge color="violet">{t("authorization.grants", { count: definitions.length })}</Badge><Button disabled={catalogUnavailable} onClick={() => openGrant()}><Plus data-slot="icon" />{t("authorization.registerGrant")}</Button></span>} />{definitions.map((definition) => <div className="provider-row grant-definition-row" key={definition.id}><span className="settings-icon"><KeyRound /></span><span><strong>{definition.display_name}</strong><small><code>{definition.key}</code> · {definition.description || t("authorization.noDescription")}</small></span><span className="tool-badges"><Badge color={definition.risk === "critical" || definition.risk === "high" ? "red" : definition.risk === "medium" ? "amber" : "zinc"}>{definition.risk}</Badge><Badge color={definition.state === "active" ? "green" : "zinc"}>{definition.state}</Badge></span><Button outline onClick={() => openGrant(definition)}>{t("authorization.edit")}</Button></div>)}{definitions.length === 0 && <div className="empty-row">{t("authorization.registerTheFirstGrantReturnedByYourAuthorizationAPI")}</div>}</div></details>
    <Dialog open={grantOpen} onClose={setGrantOpen} title={editingGrant ? t("authorization.editGrantDefinition") : t("authorization.registerGrant")} description={t("authorization.grantKeysAreStableContractIdentifiersEditingNeverGrants")} actions={<><Button outline onClick={() => setGrantOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !grantKey.trim() || !grantName.trim()} onClick={saveGrant}>{busy ? t("common.saving") : t("authorization.saveGrant")}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>{t("authorization.grantKey")}</span><input disabled={Boolean(editingGrant)} value={grantKey} onChange={(event) => setGrantKey(event.target.value)} placeholder="customers.read" /></label><label className="auth-field"><span>{t("authorization.displayName")}</span><input value={grantName} onChange={(event) => setGrantName(event.target.value)} placeholder={t("authorization.readCustomers")} /></label><label className="auth-field"><span>{t("authorization.description")}</span><textarea value={grantDescription} onChange={(event) => setGrantDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>{t("authorization.risk")}</span><select value={grantRisk} onChange={(event) => setGrantRisk(event.target.value as APIGrantDefinition["risk"])}><option value="low">{t("authorization.low")}</option><option value="medium">{t("authorization.medium")}</option><option value="high">{t("authorization.high")}</option><option value="critical">{t("authorization.critical")}</option></select></label><label className="auth-field"><span>{t("authorization.state")}</span><select value={grantState} onChange={(event) => setGrantState(event.target.value as APIGrantDefinition["state"])}><option value="active">{t("authorization.active")}</option><option value="deprecated">{t("authorization.deprecated")}</option></select></label></div></div></Dialog>
    <Dialog open={pointOpen} onClose={setPointOpen} title={editingPoint ? t("authorization.editActionPolicy") : t("authorization.addActionPolicy")} description={t("authorization.configureADeclarativeAPIActionPolicyThereIsDeliberately")} actions={<><Button outline onClick={() => setPointOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !pointKey.trim() || !pointName.trim() || pointGrants.some((grant) => !registeredKeys.has(grant))} onClick={savePoint}>{busy ? t("common.saving") : t("authorization.savePolicy")}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>{t("authorization.policyKey")}</span><input disabled={Boolean(editingPoint)} value={pointKey} onChange={(event) => setPointKey(event.target.value)} placeholder="customers.read" /></label><label className="auth-field"><span>{t("authorization.name")}</span><input value={pointName} onChange={(event) => setPointName(event.target.value)} placeholder={t("authorization.readCustomer")} /></label></div><label className="auth-field"><span>{t("authorization.description")}</span><textarea value={pointDescription} onChange={(event) => setPointDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>{t("authorization.actionType")}</span><select value={pointAction} onChange={(event) => { const value = event.target.value as APIAuthorizationPoint["action_type"]; setPointAction(value); if (value === "destructive") setPointConfirmation(true); }}><option value="read">{t("authorization.read")}</option><option value="write">{t("authorization.write")}</option><option value="destructive">{t("authorization.destructive")}</option></select></label><label className="auth-field"><span>{t("authorization.decisionTTLSeconds")}</span><input type="number" min={0} max={3600} value={pointTTL} onChange={(event) => setPointTTL(event.target.value)} /></label></div><fieldset className="catalog-settings-section"><legend>{t("authorization.requiredRegisteredGrants")}</legend>{definitions.map((definition) => { const selected = pointGrants.includes(definition.key); return <label className="compact-check" key={definition.id}><input type="checkbox" disabled={definition.state !== "active" && !selected} checked={selected} onChange={() => setPointGrants((current) => current.includes(definition.key) ? current.filter((key) => key !== definition.key) : [...current, definition.key])} /><span>{definition.display_name}<small>{definition.key} · {definition.risk}{definition.state === "deprecated" ? t("authorization.deprecatedRemoveBeforeSaving") : ""}</small></span></label>; })}</fieldset><label className="compact-check"><input type="checkbox" disabled={pointAction === "destructive"} checked={pointConfirmation || pointAction === "destructive"} onChange={(event) => setPointConfirmation(event.target.checked)} /><span>{t("authorization.requireExplicitConfirmationForThisAction")}</span></label><label className="auth-field"><span>{t("authorization.state")}</span><select value={pointState} onChange={(event) => setPointState(event.target.value as APIAuthorizationPoint["state"])}><option value="draft">{t("authorization.draft")}</option><option value="active">{t("authorization.active")}</option><option value="deprecated">{t("authorization.deprecated")}</option></select></label></div></Dialog>
  </>;
}
