"use client";

import { CheckCircle2, ChevronRight, ExternalLink, RefreshCw, ShieldCheck, TerminalSquare, XCircle } from "lucide-react";
import { useState } from "react";

import { APIError, api, type APIIntegration, type APIIntegrationPreflight, type Distribution } from "../../../lib/api";
import { integrationValidationPath, sectionPath } from "../../../lib/console-routes";
import { Badge, Button } from "../../core/control";
import { PanelHeader } from "../../core/layout";
import { ConsoleLink } from "../shared";

export function IntegrationTestWorkspace({ integration, distribution, onNavigate }: { integration: APIIntegration; distribution: Distribution | null; onNavigate: (path: string) => void }) {
  const [preflight, setPreflight] = useState<APIIntegrationPreflight | null>(null);
  const [running, setRunning] = useState(false);
  const [preflightError, setPreflightError] = useState("");

  async function runPreflight() {
    setRunning(true);
    setPreflightError("");
    try {
      setPreflight(await api.preflightIntegration(integration.id));
    } catch (error) {
      setPreflight(null);
      setPreflightError(error instanceof APIError ? error.message : "Server preflight could not run.");
    } finally { setRunning(false); }
  }

  const pathForTab = (tab: string) => integrationValidationPath(integration.id, tab);
  const requiredChecks = preflight?.checks.filter((check) => check.required) ?? [];
  const passed = requiredChecks.filter((check) => check.status === "pass").length;
  return <div className="integration-tab-content">
    <div className="notice"><TerminalSquare /><span><strong>Vendor-neutral acceptance suite.</strong> Use the same OAuth + Stateless MCPv2 client against every integration; never special-case a vendor.</span></div>
    <section className="panel"><PanelHeader title="Configuration preflight" description="Server-backed checks over the exact candidate manifest and immutable bindings. This does not impersonate a user or call the vendor backend." action={<Button disabled={running} onClick={() => void runPreflight()}><RefreshCw data-slot="icon" />{running ? "Running…" : "Run preflight"}</Button>} />{preflightError && <div className="publish-validation error"><span><XCircle /></span><span><strong>Preflight failed</strong><small>{preflightError}</small></span></div>}{preflight?.checks.map((check) => { const ready = check.status === "pass"; const optional = check.status === "optional"; return <ConsoleLink key={check.code} path={pathForTab(check.tab)} onNavigate={onNavigate} className="integration-health-check"><span className={`health-icon ${ready ? "ready" : ""}`}>{ready ? <CheckCircle2 /> : optional ? <ShieldCheck /> : <XCircle />}</span><span><strong>{check.label}</strong><small>{check.message}</small></span><Badge color={ready ? "green" : optional ? "zinc" : "red"}>{ready ? "Pass" : optional ? "Optional" : "Missing"}</Badge><ChevronRight /></ConsoleLink>; })}{!preflight && !preflightError && <div className="empty-row">Run preflight to ask the server to verify the current candidate.</div>}<div className="preflight-summary"><span><strong>{preflight ? `${passed}/${requiredChecks.length} required checks pass` : "Server result pending"}</strong><small>{preflight ? `Candidate r${preflight.candidate_revision} · ${preflight.candidate_manifest_hash} · ${new Date(preflight.generated_at).toLocaleString()}` : "No browser-only assumptions are used"}</small></span><Badge color={preflight?.ready ? "green" : "amber"}>{preflight?.ready ? preflight.matches_latest_published ? "Published & ready" : "Ready to publish" : "Action required"}</Badge></div></section>
    <section className="panel"><PanelHeader title="MCP client acceptance" description="Complete these live scenarios with a test tenant before publication." action={<ConsoleLink path={sectionPath("runs")} onNavigate={onNavigate} className="entity-back-link">Open activity</ConsoleLink>} /><ol className="acceptance-scenarios"><li><span>1</span><div><strong>Discover metadata and register a public client</strong><small>RFC 8414, protected-resource metadata, DCR, exact resource and PKCE S256.</small></div></li><li><span>2</span><div><strong>Authorize a real test customer</strong><small>OIDC callback, live vendor access evaluation, one-time code and bound token.</small></div></li><li><span>3</span><div><strong>List resources and tools</strong><small>Only published resources and grant-authorized exact tool revisions appear.</small></div></li><li><span>4</span><div><strong>Exercise positive and negative calls</strong><small>Valid call, missing grant, invalid schema, absent confirmation, revoked access and upstream drift.</small></div></li><li><span>5</span><div><strong>Verify widget and support isolation</strong><small>Origin allowlist, API allowlist, redaction, retention, delivery receipt and audit correlation.</small></div></li></ol>{distribution?.agent_setup.private.available ? <a className="panel-footer-link" href={distribution.agent_setup.private.url} target="_blank" rel="noreferrer">Open private MCP test-client setup <ExternalLink /></a> : <div className="empty-row">Private test-client setup becomes available after customer identity is active.</div>}</section>
  </div>;
}
