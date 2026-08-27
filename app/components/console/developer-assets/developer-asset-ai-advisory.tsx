"use client";


import { useTranslation } from "react-i18next";
import { Clock3, History, Quote, ShieldAlert, Sparkles, TriangleAlert } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { APIError } from "../../../lib/api-client";
import {
  developerAssetsApi,
  type DeveloperAssetAIAdvisoryInput,
  type DeveloperAssetAIAdvisoryRun,
  type DeveloperAssetAIApplicabilityResult,
  type DeveloperAssetAIMapResult,
  type DeveloperAssetAISampleReviewResult,
} from "../../../lib/developer-assets-api";
import { Badge, Button, Dialog } from "../../core/control";
import { developerAssetError } from "./developer-asset-ui";

type Availability = "unknown" | "configured" | "unavailable";

function advisoryScopeID(input: DeveloperAssetAIAdvisoryInput) {
  if (input.prompt_key === "documentation.map_enrichment") return input.source_publication_id;
  if (input.prompt_key === "sdk.map_enrichment") return input.sdk_content_publication_id;
  if (input.prompt_key === "sdk.applicability_suggestion") return input.api_sdk_binding_id;
  return input.sdk_code_sample_id;
}

function EvidenceIDs({ ids, empty }: { ids: string[]; empty?: string }) {
  const { t } = useTranslation();
  return <div className="developer-ai-evidence-ids">{ids.map((id) => <code key={id}>{id}</code>)}{ids.length === 0 && <small>{empty ?? t("assetAdvisory.noEvidenceIDsReturned")}</small>}</div>;
}

function MapResult({ result }: { result: DeveloperAssetAIMapResult }) {
  const { t } = useTranslation();
  return <div className="developer-ai-result">
    <div className="developer-ai-result-status"><Badge color={result.status === "ready" ? "blue" : "amber"}>{result.status}</Badge><small>{t("assetAdvisory.advisoryMapStatusOnly")}</small></div>
    <div className="developer-ai-entry-list">{result.entries.map((entry, index) => <article key={`${entry.kind}-${entry.title}-${index}`}><header><Badge color="violet">{entry.kind}</Badge><strong>{entry.title}</strong></header><p>{entry.summary}</p><EvidenceIDs ids={entry.evidence_ids} /></article>)}{result.entries.length === 0 && <p className="empty-row">{t("assetAdvisory.noAdvisoryEntriesWereReturned")}</p>}</div>
    <GapList gaps={result.gaps} />
  </div>;
}

function ApplicabilityResult({ result }: { result: DeveloperAssetAIApplicabilityResult }) {
  const { t } = useTranslation();
  const color = result.status === "suggested" ? "blue" : result.status === "unsupported" ? "red" : "amber";
  return <div className="developer-ai-result">
    <div className="developer-ai-result-status"><Badge color={color}>{result.status}</Badge><Badge color="zinc">{t("assetAdvisory.coverage")} {result.coverage}</Badge><small>{t("assetAdvisory.suggestionOnlyTheAPIBindingIsUnchanged")}</small></div>
    <div className="developer-ai-selector-list">{result.selectors.map((selector, index) => <article key={`${selector.kind}-${selector.value}-${index}`}><header><Badge color="violet">{selector.kind}</Badge><code>{selector.value}</code></header><EvidenceIDs ids={selector.evidence_ids} /></article>)}{result.selectors.length === 0 && <p className="empty-row">{t("assetAdvisory.noApplicabilitySelectorsWereSuggested")}</p>}</div>
    <GapList gaps={result.gaps} />
  </div>;
}

function SampleReviewResult({ result }: { result: DeveloperAssetAISampleReviewResult }) {
  const { t } = useTranslation();
  const color = result.recommendation === "pass" ? "blue" : result.recommendation === "revise" ? "red" : "amber";
  return <div className="developer-ai-result">
    <div className="developer-ai-result-status"><Badge color={color}>{result.recommendation}</Badge><small>{t("assetAdvisory.advisoryRecommendationOnlyItIsNotAReviewDecision")}</small></div>
    <section className="developer-ai-findings"><h4>{t("assetAdvisory.closedFindingCodes")}</h4>{result.findings.map((finding, index) => <article key={`${finding.code}-${index}`}><Badge color={finding.code === "insufficient_evidence" || finding.code === "uncited_claim" ? "amber" : "red"}>{finding.code}</Badge><EvidenceIDs ids={finding.evidence_ids} /></article>)}{result.findings.length === 0 && <p className="empty-row">{t("assetAdvisory.noAdvisoryFindingsWereReturnedThisDoesNotValidate")}</p>}</section>
  </div>;
}

function GapList({ gaps }: { gaps: DeveloperAssetAIMapResult["gaps"] }) {
  const { t } = useTranslation();
  return <section className="developer-ai-gaps"><h4>{t("assetAdvisory.evidenceGaps")}</h4>{gaps.map((gap, index) => <article key={`${gap.code}-${index}`}><Badge color="amber">{gap.code}</Badge><EvidenceIDs ids={gap.evidence_ids} /></article>)}{gaps.length === 0 && <p className="empty-row">{t("assetAdvisory.noGapsWereReturnedByThisAdvisoryRunHuman")}</p>}</section>;
}

function AdvisoryResult({ run }: { run: DeveloperAssetAIAdvisoryRun }) {
  if (run.prompt_key === "sdk.applicability_suggestion") return <ApplicabilityResult result={run.result as DeveloperAssetAIApplicabilityResult} />;
  if (run.prompt_key === "sdk.sample_review") return <SampleReviewResult result={run.result as DeveloperAssetAISampleReviewResult} />;
  return <MapResult result={run.result as DeveloperAssetAIMapResult} />;
}

function AdvisoryDetail({ run }: { run: DeveloperAssetAIAdvisoryRun }) {
  const { t } = useTranslation();
  const identities = [
    [t("assetAdvisory.sourcePublication"), run.source_publication_id],
    [t("assetAdvisory.sdkPackage"), run.sdk_package_id],
    [t("assetAdvisory.exactSDKRelease"), run.sdk_release_id],
    [t("assetAdvisory.sdkContentCandidate"), run.sdk_content_candidate_id],
    [t("assetAdvisory.sdkContentPublication"), run.sdk_content_publication_id],
    [t("assetAdvisory.api"), run.api_id],
    [t("assetAdvisory.apiResourcePublication"), run.api_developer_asset_publication_id],
    [t("assetAdvisory.apiSDKBinding"), run.api_sdk_binding_id],
    [t("assetAdvisory.sdkSample"), run.sdk_code_sample_id],
    [t("assetAdvisory.ingestionRun"), run.ingestion_run_id],
  ].filter((entry): entry is [string, string] => Boolean(entry[1]));

  return <div className="developer-ai-detail">
    <div className="developer-ai-detail-heading"><span><Badge color="violet">{run.prompt_key}</Badge><Badge color="zinc">{run.scope_visibility}</Badge></span><small><Clock3 />{t("format.dateTime", { value: new Date(run.created_at) })}</small></div>
    <dl className="entity-detail-grid"><div><dt>{t("assetAdvisory.promptVersion")}</dt><dd><code>{run.prompt_version}</code></dd></div><div><dt>{t("assetAdvisory.advisoryRunID")}</dt><dd><code>{run.id}</code></dd></div><div><dt>{t("assetAdvisory.scope")}</dt><dd>{run.scope_kind}<br /><code>{run.scope_id}</code></dd></div><div><dt>{t("assetAdvisory.resultHash")}</dt><dd><code>{run.result_hash}</code></dd></div></dl>
    <AdvisoryResult run={run} />
    <details className="advanced-details"><summary>{t("assetAdvisory.exactEvidenceAndCitationIdentity")}</summary><div className="developer-ai-exact-evidence"><h4>{t("assetAdvisory.allowedEvidenceIDs")}</h4><EvidenceIDs ids={run.allowed_evidence_ids} /><dl className="entity-detail-grid">{identities.map(([label, id]) => <div key={label}><dt>{label}</dt><dd><code>{id}</code></dd></div>)}<div><dt>{t("assetAdvisory.evidenceHash")}</dt><dd><code>{run.evidence_hash}</code></dd></div><div><dt>{t("assetAdvisory.inputHash")}</dt><dd><code>{run.input_hash}</code></dd></div></dl></div></details>
  </div>;
}

export function DeveloperAssetAIAdvisoryButton({
  input,
  subject,
  label,
  unavailableReason,
}: {
  input: DeveloperAssetAIAdvisoryInput | null;
  subject: string;
  label?: string;
  unavailableReason?: string;
}) {
  const { t } = useTranslation();
  const resolvedLabel = label ?? t("assetAdvisory.aiAdvisory");
  const resolvedUnavailableReason = unavailableReason ?? t("assetAdvisory.exactPublishedScopeRequired");
  const [open, setOpen] = useState(false);
  const [runs, setRuns] = useState<DeveloperAssetAIAdvisoryRun[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [loading, setLoading] = useState(false);
  const [running, setRunning] = useState(false);
  const [availability, setAvailability] = useState<Availability>("unknown");
  const [problem, setProblem] = useState("");
  const scopeID = input ? advisoryScopeID(input) : "";
  const selected = useMemo(() => runs.find((run) => run.id === selectedID) ?? runs[0] ?? null, [runs, selectedID]);

  const loadHistory = useCallback(async () => {
    if (!input) return;
    setLoading(true);
    setProblem("");
    try {
      const values = await developerAssetsApi.aiAdvisories({ prompt_key: input.prompt_key, scope_id: scopeID, limit: 50 });
      setRuns(values);
      setSelectedID((current) => values.some((run) => run.id === current) ? current : values[0]?.id ?? "");
    } catch (error) {
      setProblem(developerAssetError(error, t("assetAdvisory.advisoryHistoryCouldNotBeLoaded")));
    } finally { setLoading(false); }
  }, [input, scopeID, t]);

  useEffect(() => {
    if (!open || !input) return;
    const timeout = window.setTimeout(() => { void loadHistory(); }, 0);
    return () => window.clearTimeout(timeout);
  }, [input, loadHistory, open]);

  async function selectRun(runID: string) {
    setSelectedID(runID);
    setProblem("");
    try {
      const detail = await developerAssetsApi.aiAdvisory(runID);
      setRuns((current) => current.map((run) => run.id === detail.id ? detail : run));
    } catch (error) {
      setProblem(developerAssetError(error, t("assetAdvisory.theImmutableAdvisoryDetailCouldNotBeLoaded")));
    }
  }

  async function runAdvisory() {
    if (!input || availability === "unavailable") return;
    setRunning(true);
    setProblem("");
    try {
      const run = await developerAssetsApi.runAIAdvisory(input);
      setAvailability("configured");
      setRuns((current) => [run, ...current.filter((value) => value.id !== run.id)]);
      setSelectedID(run.id);
    } catch (error) {
      if (error instanceof APIError && error.code === "ai_unavailable") {
        setAvailability("unavailable");
        setProblem(t("assetAdvisory.advisoryAIIsDisabledOrUnconfiguredForTheAnalysis"));
      } else {
        setProblem(developerAssetError(error, t("assetAdvisory.advisoryAIFailedNoAdvisoryWasStoredAndNo")));
      }
    } finally { setRunning(false); }
  }

  if (!input) return <Button type="button" outline disabled title={resolvedUnavailableReason}><Sparkles data-slot="icon" />{t("assetAdvisory.aiAdvisoryUnavailable")}</Button>;

  return <>
    <Button type="button" outline onClick={() => setOpen(true)}><Sparkles data-slot="icon" />{resolvedLabel}</Button>
    <Dialog open={open} onClose={setOpen} title={t("assetAdvisory.advisoryAI", { subject: String(subject) })} description={t("assetAdvisory.inspectImmutableEvidenceBoundedSuggestionsAndTheirHistoryAdvisory")} actions={<><Button outline onClick={() => setOpen(false)}>{t("common.close")}</Button><Button color="indigo" disabled={running || availability === "unavailable"} onClick={() => void runAdvisory()}><Sparkles data-slot="icon" />{running ? t("assetAdvisory.runningAdvisory") : availability === "unavailable" ? t("assetAdvisory.aiUnconfigured") : t("assetAdvisory.runAdvisoryAI")}</Button></>}>
      <div className="developer-ai-advisory">
        <div className="notice developer-ai-non-authoritative"><ShieldAlert /><span><strong>{t("assetAdvisory.nonAuthoritativeSuggestion")}</strong> {t("assetAdvisory.aHumanMustVerifyEverySelectorFindingGapAnd")}</span></div>
        <div className="developer-ai-availability"><span><strong>{t("assetAdvisory.analysisProvider")}</strong><small>{availability === "unknown" ? t("assetAdvisory.configurationIsCheckedOnlyWhenARunIsRequested") : availability === "configured" ? t("assetAdvisory.configuredForThisSuccessfulRun") : t("assetAdvisory.disabledOrUnconfiguredRunActionIsUnavailable")}</small></span><Badge color={availability === "configured" ? "green" : availability === "unavailable" ? "red" : "zinc"}>{availability === "configured" ? t("assetAdvisory.configured") : availability === "unavailable" ? t("assetAdvisory.unconfigured") : t("assetAdvisory.notChecked")}</Badge></div>
        {problem && <div className="developer-ai-problem"><TriangleAlert /><span>{problem}</span></div>}
        {loading ? <div className="developer-ai-loading"><History /><span>{t("assetAdvisory.loadingImmutableAdvisoryHistory")}</span></div> : runs.length > 0 ? <><div className="developer-ai-history"><label><span>{t("assetAdvisory.advisoryHistory")}</span><select aria-label={t("assetAdvisory.advisoryHistoryFor", { subject: String(subject) })} value={selected?.id ?? ""} onChange={(event) => void selectRun(event.target.value)}>{runs.map((run) => <option key={run.id} value={run.id}>{t("format.dateTime", { value: new Date(run.created_at) })} · {run.prompt_version} · {run.id}</option>)}</select></label><Badge color="zinc">{runs.length} {t("assetAdvisory.immutableRun")}{runs.length === 1 ? "" : t("assetAdvisory.s")}</Badge></div>{selected && <AdvisoryDetail run={selected} />}</> : <div className="developer-ai-empty"><Quote /><strong>{t("assetAdvisory.noAdvisoryHistory")}</strong><small>{t("assetAdvisory.runAdvisoryAIToCreateAnImmutableSuggestionOver")}</small></div>}
      </div>
    </Dialog>
  </>;
}
