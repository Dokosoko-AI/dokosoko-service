"use client";


import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
  if (!analysis) {
    return <section className="panel integration-setup-guide">
      <PanelHeader title={t("integrationSetupGuide.setupGuide")} description={t("integrationSetupGuide.generateAShortStartingPathFromThisAPIS")} action={<Button color="indigo" disabled={!canGenerate || busy} onClick={() => void onGenerate()}><Sparkles data-slot="icon" />{busy ? t("integrationSetupGuide.generating") : t("integrationSetupGuide.generateGuide")}</Button>} />
      <div className="setup-guide-empty"><span className="settings-icon"><BookOpen /></span><span><strong>{t("integrationSetupGuide.noSetupGuideYet")}</strong><small>{canGenerate ? t("integrationSetupGuide.generateAnOrientationFromTheExactDocumentationAndContract") : t("integrationSetupGuide.attachReviewedDocumentationOrAnAPIContractFirstThe")}</small></span><Badge color="zinc">{t("integrationSetupGuide.notGenerated")}</Badge></div>
    </section>;
  }

  const endpoints = analysis.plan.endpoints.slice(0, 2);
  return <section className="panel integration-setup-guide">
    <PanelHeader title={t("integrationSetupGuide.setupGuide")} description={t("integrationSetupGuide.startHereBeforeOpeningTheFullDocumentationAndContract")} action={<span className="heading-actions"><Badge color="violet">{t("integrationSetupGuide.advisoryDraft")}</Badge><Button outline disabled={!canGenerate || busy} onClick={() => void onGenerate()}><RefreshCw data-slot="icon" />{busy ? t("integrationSetupGuide.refreshing") : t("integrationSetupGuide.refreshGuide")}</Button></span>} />
    <div className="setup-guide-boundary"><Sparkles /><span><strong>{t("integrationSetupGuide.orientationNotPublishedEvidence")}</strong><small>{t("integrationSetupGuide.thisGuideIsGeneratedFromTheCurrentAnalysisFor")}</small></span></div>
    <div className="setup-guide-summary"><p>{analysis.plan.summary}</p></div>
    <IntegrationEvidenceGaps unknowns={analysis.unknowns} />
    <ol className="setup-guide-steps">
      <li><span className="setup-guide-number">1</span><span><strong>{t("integrationSetupGuide.authenticate")}</strong><small>{analysis.plan.identity.explanation || t("integrationSetupGuide.useTheIdentityModeDescribedByThisAPI", { mode: String(analysis.plan.identity.mode) })}</small></span><ShieldCheck /></li>
      {endpoints.map((endpoint, index) => <li key={`${endpoint.method}:${endpoint.path}`}><span className="setup-guide-number">{index + 2}</span><span><strong>{index === 0 ? t("integrationSetupGuide.firstRequest") : endpoint.name}</strong><small><code>{endpoint.method} {endpoint.path}</code> · {endpoint.purpose}</small></span><TerminalSquare /></li>)}
    </ol>
    {analysis.plan.endpoints.length > endpoints.length && <details className="advanced-details inline-advanced setup-guide-endpoints"><summary>{t("integrationSetupGuide.allSuggestedStartingEndpoints")}</summary><div className="advanced-details-body">{analysis.plan.endpoints.map((endpoint) => <div key={`${endpoint.method}:${endpoint.path}:${endpoint.name}`}><code>{endpoint.method}</code><span><strong>{endpoint.path}</strong><small>{endpoint.purpose}</small></span></div>)}</div></details>}
  </section>;
}
