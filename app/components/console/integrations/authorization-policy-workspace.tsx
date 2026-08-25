"use client";

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
      await loadAuthorization(); setGrantOpen(false); onMessage(editingGrant ? "Grant definition updated." : "Grant registered for policy use.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Grant definition could not be saved."); } finally { setBusy(false); }
  }

  function openPoint(value?: APIAuthorizationPoint) {
    setEditingPoint(value ?? null); setPointKey(value?.key ?? ""); setPointName(value?.name ?? ""); setPointDescription(value?.description ?? ""); setPointAction(value?.action_type ?? "read"); setPointGrants(value?.required_grants ?? []); setPointConfirmation(value?.confirmation_required ?? false); setPointTTL(String(value?.decision_ttl_seconds ?? 300)); setPointState(value?.state ?? "draft"); setPointOpen(true);
  }

  async function savePoint() {
    setBusy(true);
    try {
      const input = { key: pointKey.trim(), name: pointName.trim(), description: pointDescription.trim(), action_type: pointAction, required_grants: pointGrants, confirmation_required: pointAction === "destructive" ? true : pointConfirmation, decision_ttl_seconds: Number(pointTTL), state: pointState };
      if (editingPoint) await api.updateAuthorizationPoint(integration.id, editingPoint.id, { ...input, revision: editingPoint.revision }); else await api.createAuthorizationPoint(integration.id, input);
      await loadAuthorization(); setPointOpen(false); onMessage(editingPoint ? "Action policy updated." : "Action policy created.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Action policy could not be saved."); } finally { setBusy(false); }
  }

  return <>
    <SectionHeader title="Action policies" description={`Define the exact actions and grants ${integration.display_name} tools require.`} />
    <div className="notice authorization-policy-notice"><ShieldCheck /><span><strong>Policies do not authenticate customers.</strong> The configured identity provider resolves the customer first; these exact grant requirements narrow which published tools the customer may call.</span></div>
    {catalogUnavailable && <div className="capability-unavailable"><TriangleAlert /><span><strong>Authorization catalogue unavailable.</strong><small>Existing tool policies remain visible, but grant and action-policy changes cannot be saved by this deployment.</small></span></div>}
    <section className="panel"><PanelHeader title="API action policies" description="Each tool binding pins one exact active policy revision so publication and runtime checks fail closed." action={<Button disabled={catalogUnavailable || definitions.every((definition) => definition.state !== "active")} onClick={() => openPoint()}><Plus data-slot="icon" />Add policy</Button>} />{points.map((point) => <div className="provider-row authorization-point-row" key={point.id}><span className="settings-icon"><ShieldCheck /></span><span><strong>{point.name}</strong><small><code>{point.key}</code> · {point.required_grants.join(", ") || "no grants"} · TTL {point.decision_ttl_seconds}s</small></span><span className="tool-badges"><Badge color={point.action_type === "destructive" ? "red" : point.action_type === "write" ? "amber" : "blue"}>{point.action_type}</Badge>{point.confirmation_required && <Badge color="violet">confirmation</Badge>}<Badge color={point.state === "active" ? "green" : "zinc"}>{point.state}</Badge></span><Button outline onClick={() => openPoint(point)}>Edit</Button></div>)}{points.length === 0 && <div className="empty-row">No action policy has been configured for {integration.display_name}. Register a grant in Advanced, then add the first policy.</div>}</section>
    <details className="panel advanced-details"><summary>Deployment grant registry — Advanced</summary><div className="advanced-details-body"><PanelHeader title="Grant registry" description="Deployment-owned names that your authorization API may return. Registering a name never grants access." action={<span className="heading-actions"><Badge color="violet">{definitions.length} grants</Badge><Button disabled={catalogUnavailable} onClick={() => openGrant()}><Plus data-slot="icon" />Register grant</Button></span>} />{definitions.map((definition) => <div className="provider-row grant-definition-row" key={definition.id}><span className="settings-icon"><KeyRound /></span><span><strong>{definition.display_name}</strong><small><code>{definition.key}</code> · {definition.description || "No description"}</small></span><span className="tool-badges"><Badge color={definition.risk === "critical" || definition.risk === "high" ? "red" : definition.risk === "medium" ? "amber" : "zinc"}>{definition.risk}</Badge><Badge color={definition.state === "active" ? "green" : "zinc"}>{definition.state}</Badge></span><Button outline onClick={() => openGrant(definition)}>Edit</Button></div>)}{definitions.length === 0 && <div className="empty-row">Register the first grant returned by your authorization API.</div>}</div></details>
    <Dialog open={grantOpen} onClose={setGrantOpen} title={editingGrant ? "Edit grant definition" : "Register grant"} description="Grant keys are stable contract identifiers. Editing never grants a user access." actions={<><Button outline onClick={() => setGrantOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !grantKey.trim() || !grantName.trim()} onClick={saveGrant}>{busy ? "Saving…" : "Save grant"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Grant key</span><input disabled={Boolean(editingGrant)} value={grantKey} onChange={(event) => setGrantKey(event.target.value)} placeholder="customers.read" /></label><label className="auth-field"><span>Display name</span><input value={grantName} onChange={(event) => setGrantName(event.target.value)} placeholder="Read customers" /></label><label className="auth-field"><span>Description</span><textarea value={grantDescription} onChange={(event) => setGrantDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Risk</span><select value={grantRisk} onChange={(event) => setGrantRisk(event.target.value as APIGrantDefinition["risk"])}><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option></select></label><label className="auth-field"><span>State</span><select value={grantState} onChange={(event) => setGrantState(event.target.value as APIGrantDefinition["state"])}><option value="active">Active</option><option value="deprecated">Deprecated</option></select></label></div></div></Dialog>
    <Dialog open={pointOpen} onClose={setPointOpen} title={editingPoint ? "Edit action policy" : "Add action policy"} description="Configure a declarative API action policy. There is deliberately no hook URL or credential field." actions={<><Button outline onClick={() => setPointOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !pointKey.trim() || !pointName.trim() || pointGrants.some((grant) => !registeredKeys.has(grant))} onClick={savePoint}>{busy ? "Saving…" : "Save policy"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Policy key</span><input disabled={Boolean(editingPoint)} value={pointKey} onChange={(event) => setPointKey(event.target.value)} placeholder="customers.read" /></label><label className="auth-field"><span>Name</span><input value={pointName} onChange={(event) => setPointName(event.target.value)} placeholder="Read customer" /></label></div><label className="auth-field"><span>Description</span><textarea value={pointDescription} onChange={(event) => setPointDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Action type</span><select value={pointAction} onChange={(event) => { const value = event.target.value as APIAuthorizationPoint["action_type"]; setPointAction(value); if (value === "destructive") setPointConfirmation(true); }}><option value="read">Read</option><option value="write">Write</option><option value="destructive">Destructive</option></select></label><label className="auth-field"><span>Decision TTL (seconds)</span><input type="number" min={0} max={3600} value={pointTTL} onChange={(event) => setPointTTL(event.target.value)} /></label></div><fieldset className="catalog-settings-section"><legend>Required registered grants</legend>{definitions.map((definition) => { const selected = pointGrants.includes(definition.key); return <label className="compact-check" key={definition.id}><input type="checkbox" disabled={definition.state !== "active" && !selected} checked={selected} onChange={() => setPointGrants((current) => current.includes(definition.key) ? current.filter((key) => key !== definition.key) : [...current, definition.key])} /><span>{definition.display_name}<small>{definition.key} · {definition.risk}{definition.state === "deprecated" ? " · deprecated (remove before saving)" : ""}</small></span></label>; })}</fieldset><label className="compact-check"><input type="checkbox" disabled={pointAction === "destructive"} checked={pointConfirmation || pointAction === "destructive"} onChange={(event) => setPointConfirmation(event.target.checked)} /><span>Require explicit confirmation for this action</span></label><label className="auth-field"><span>State</span><select value={pointState} onChange={(event) => setPointState(event.target.value as APIAuthorizationPoint["state"])}><option value="draft">Draft</option><option value="active">Active</option><option value="deprecated">Deprecated</option></select></label></div></Dialog>
  </>;
}
