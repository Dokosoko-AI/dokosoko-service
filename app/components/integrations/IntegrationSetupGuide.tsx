"use client";

import { BookOpen, RefreshCw, ShieldCheck, Sparkles, TerminalSquare } from "lucide-react";
import type { APIIntegrationAnalysis } from "../../lib/api";
import { Button } from "../core/button";
import { Badge } from "../core/control";
import { PanelHeader } from "../core/layout";
import { IntegrationEvidenceGaps } from "./IntegrationEvidenceGaps";

type IntegrationSetupGuideProps = {
  analysis?: APIIntegrationAnalysis;
  canGenerate: boolean;
  busy: boolean;
  onGenerate: () => void | Promise<void>;
};

export function IntegrationSetupGuide({ analysis, canGenerate, busy, onGenerate }: IntegrationSetupGuideProps) {
  if (!analysis) {
    return <section className="panel integration-setup-guide">
      <PanelHeader title="Setup guide" description="Generate a short starting path from this API's attached, reviewed evidence." action={<Button color="indigo" disabled={!canGenerate || busy} onClick={() => void onGenerate()}><Sparkles data-slot="icon" />{busy ? "Generating…" : "Generate guide"}</Button>} />
      <div className="setup-guide-empty"><span className="settings-icon"><BookOpen /></span><span><strong>No setup guide yet</strong><small>{canGenerate ? "Generate an orientation from the exact documentation and contract revisions attached to this API." : "Attach reviewed documentation or an API contract first. The attached revisions remain the source of truth."}</small></span><Badge color="zinc">Not generated</Badge></div>
    </section>;
  }

  const endpoints = analysis.plan.endpoints.slice(0, 2);
  return <section className="panel integration-setup-guide">
    <PanelHeader title="Setup guide" description="Start here before opening the full documentation and contract sets." action={<span className="heading-actions"><Badge color="violet">Advisory draft</Badge><Button outline disabled={!canGenerate || busy} onClick={() => void onGenerate()}><RefreshCw data-slot="icon" />{busy ? "Refreshing…" : "Refresh guide"}</Button></span>} />
    <div className="setup-guide-boundary"><Sparkles /><span><strong>Orientation, not published evidence</strong><small>This guide is generated from the current analysis for operator review. Agents rely only on the reviewed resources attached below.</small></span></div>
    <div className="setup-guide-summary"><p>{analysis.plan.summary}</p></div>
    <IntegrationEvidenceGaps unknowns={analysis.unknowns} />
    <ol className="setup-guide-steps">
      <li><span className="setup-guide-number">1</span><span><strong>Authenticate</strong><small>{analysis.plan.identity.explanation || `Use the ${analysis.plan.identity.mode} identity mode described by this API.`}</small></span><ShieldCheck /></li>
      {endpoints.map((endpoint, index) => <li key={`${endpoint.method}:${endpoint.path}`}><span className="setup-guide-number">{index + 2}</span><span><strong>{index === 0 ? "First request" : endpoint.name}</strong><small><code>{endpoint.method} {endpoint.path}</code> · {endpoint.purpose}</small></span><TerminalSquare /></li>)}
    </ol>
    {analysis.plan.endpoints.length > endpoints.length && <details className="advanced-details inline-advanced setup-guide-endpoints"><summary>All suggested starting endpoints</summary><div className="advanced-details-body">{analysis.plan.endpoints.map((endpoint) => <div key={`${endpoint.method}:${endpoint.path}:${endpoint.name}`}><code>{endpoint.method}</code><span><strong>{endpoint.path}</strong><small>{endpoint.purpose}</small></span></div>)}</div></details>}
  </section>;
}
