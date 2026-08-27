"use client";


import { useTranslation } from "react-i18next";
import { CheckCircle2, ChevronRight, ExternalLink, RefreshCw, ShieldCheck, TerminalSquare, XCircle } from "lucide-react";
import { useState } from "react";

import { APIError, api, type APIIntegration, type APIIntegrationPreflight, type Distribution } from "../../../lib/api";
import { integrationValidationPath } from "../../../lib/console-routes";
import { Badge, Button } from "../../core/control";
import { PanelHeader } from "../../core/layout";
import { ConsoleLink } from "../shared";

export function IntegrationTestWorkspace({ integration, distribution, onNavigate }: { integration: APIIntegration; distribution: Distribution | null; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
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
      setPreflightError(error instanceof APIError ? error.message : t("integrationTest.serverPreflightCouldNotRun"));
    } finally { setRunning(false); }
  }

  const pathForTab = (tab: string) => integrationValidationPath(integration.id, tab);
  const requiredChecks = preflight?.checks.filter((check) => check.required) ?? [];
  const passed = requiredChecks.filter((check) => check.status === "pass").length;
  return <div className="integration-tab-content">
    <div className="notice"><TerminalSquare /><span><strong>{t("integrationTest.vendorNeutralAcceptanceSuite")}</strong> {t("integrationTest.useTheSameOAuthStatelessMCPv2ClientAgainstEvery")}</span></div>
    <section className="panel"><PanelHeader title={t("integrationTest.configurationPreflight")} description={t("integrationTest.serverBackedChecksOverTheExactCandidateManifestAnd")} action={<Button disabled={running} onClick={() => void runPreflight()}><RefreshCw data-slot="icon" />{running ? t("integrationTest.running") : t("integrationTest.runPreflight")}</Button>} />{preflightError && <div className="publish-validation error"><span><XCircle /></span><span><strong>{t("integrationTest.preflightFailed")}</strong><small>{preflightError}</small></span></div>}{preflight?.checks.map((check) => { const ready = check.status === "pass"; const optional = check.status === "optional"; return <ConsoleLink key={check.code} path={pathForTab(check.tab)} onNavigate={onNavigate} className="integration-health-check"><span className={`health-icon ${ready ? "ready" : ""}`}>{ready ? <CheckCircle2 /> : optional ? <ShieldCheck /> : <XCircle />}</span><span><strong>{check.label}</strong><small>{check.message}</small></span><Badge color={ready ? "green" : optional ? "zinc" : "red"}>{ready ? t("integrationTest.pass") : optional ? t("integrationTest.optional") : t("integrationTest.missing")}</Badge><ChevronRight /></ConsoleLink>; })}{!preflight && !preflightError && <div className="empty-row">{t("integrationTest.runPreflightToAskTheServerToVerifyThe")}</div>}<div className="preflight-summary"><span><strong>{preflight ? t("integrationTest.requiredChecksPass", { passed: String(passed), length: String(requiredChecks.length) }) : t("integrationTest.serverResultPending")}</strong><small>{preflight ? t("integrationTest.candidateR", { candidate_revision: String(preflight.candidate_revision), candidate_manifest_hash: String(preflight.candidate_manifest_hash), value3: String(t("format.dateTime", { value: new Date(preflight.generated_at) })) }) : t("integrationTest.noBrowserOnlyAssumptionsAreUsed")}</small></span><Badge color={preflight?.ready ? "green" : "amber"}>{preflight?.ready ? preflight.matches_latest_published ? t("integrationTest.publishedReady") : t("integrationTest.readyToPublish") : t("integrationTest.actionRequired")}</Badge></div></section>
    <section className="panel"><PanelHeader title={t("integrationTest.mcpClientAcceptance")} description={t("integrationTest.completeTheseLiveScenariosWithATestTenantBefore")} /><ol className="acceptance-scenarios"><li><span>1</span><div><strong>{t("integrationTest.discoverMetadataAndRegisterAPublicClient")}</strong><small>{t("integrationTest.rfcN8414ProtectedResourceMetadataDCRExactResourceAnd")}</small></div></li><li><span>2</span><div><strong>{t("integrationTest.authorizeARealTestCustomer")}</strong><small>{t("integrationTest.oidcCallbackLiveAccessEvaluationOneTimeCodeAnd")}</small></div></li><li><span>3</span><div><strong>{t("integrationTest.listResourcesAndTools")}</strong><small>{t("integrationTest.onlyPublishedResourcesAndGrantAuthorizedExactToolRevisions")}</small></div></li><li><span>4</span><div><strong>{t("integrationTest.exercisePositiveAndNegativeCalls")}</strong><small>{t("integrationTest.validCallMissingGrantInvalidSchemaAbsentConfirmationRevoked")}</small></div></li><li><span>5</span><div><strong>{t("integrationTest.verifySupportReporting")}</strong><small>{t("integrationTest.consentSensitiveValueRejectionOutboxPersistenceAndAuditCorrelation")}</small></div></li></ol>{distribution?.agent_setup.private.available ? <a className="panel-footer-link" href={distribution.agent_setup.private.url} target="_blank" rel="noreferrer">{t("integrationTest.openPrivateMCPTestClientSetup")} <ExternalLink /></a> : <div className="empty-row">{t("integrationTest.privateTestClientSetupBecomesAvailableAfterCustomerIdentity")}</div>}</section>
  </div>;
}
