"use client";

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

function EvidenceIDs({ ids, empty = "No evidence IDs returned" }: { ids: string[]; empty?: string }) {
  return <div className="developer-ai-evidence-ids">{ids.map((id) => <code key={id}>{id}</code>)}{ids.length === 0 && <small>{empty}</small>}</div>;
}

function MapResult({ result }: { result: DeveloperAssetAIMapResult }) {
  return <div className="developer-ai-result">
    <div className="developer-ai-result-status"><Badge color={result.status === "ready" ? "blue" : "amber"}>{result.status}</Badge><small>Advisory map status only</small></div>
    <div className="developer-ai-entry-list">{result.entries.map((entry, index) => <article key={`${entry.kind}-${entry.title}-${index}`}><header><Badge color="violet">{entry.kind}</Badge><strong>{entry.title}</strong></header><p>{entry.summary}</p><EvidenceIDs ids={entry.evidence_ids} /></article>)}{result.entries.length === 0 && <p className="empty-row">No advisory entries were returned.</p>}</div>
    <GapList gaps={result.gaps} />
  </div>;
}

function ApplicabilityResult({ result }: { result: DeveloperAssetAIApplicabilityResult }) {
  const color = result.status === "suggested" ? "blue" : result.status === "unsupported" ? "red" : "amber";
  return <div className="developer-ai-result">
    <div className="developer-ai-result-status"><Badge color={color}>{result.status}</Badge><Badge color="zinc">coverage: {result.coverage}</Badge><small>Suggestion only; the API binding is unchanged</small></div>
    <div className="developer-ai-selector-list">{result.selectors.map((selector, index) => <article key={`${selector.kind}-${selector.value}-${index}`}><header><Badge color="violet">{selector.kind}</Badge><code>{selector.value}</code></header><EvidenceIDs ids={selector.evidence_ids} /></article>)}{result.selectors.length === 0 && <p className="empty-row">No applicability selectors were suggested.</p>}</div>
    <GapList gaps={result.gaps} />
  </div>;
}

function SampleReviewResult({ result }: { result: DeveloperAssetAISampleReviewResult }) {
  const color = result.recommendation === "pass" ? "blue" : result.recommendation === "revise" ? "red" : "amber";
  return <div className="developer-ai-result">
    <div className="developer-ai-result-status"><Badge color={color}>{result.recommendation}</Badge><small>Advisory recommendation only; it is not a review decision</small></div>
    <section className="developer-ai-findings"><h4>Closed finding codes</h4>{result.findings.map((finding, index) => <article key={`${finding.code}-${index}`}><Badge color={finding.code === "insufficient_evidence" || finding.code === "uncited_claim" ? "amber" : "red"}>{finding.code}</Badge><EvidenceIDs ids={finding.evidence_ids} /></article>)}{result.findings.length === 0 && <p className="empty-row">No advisory findings were returned. This does not validate or approve the sample.</p>}</section>
  </div>;
}

function GapList({ gaps }: { gaps: DeveloperAssetAIMapResult["gaps"] }) {
  return <section className="developer-ai-gaps"><h4>Evidence gaps</h4>{gaps.map((gap, index) => <article key={`${gap.code}-${index}`}><Badge color="amber">{gap.code}</Badge><EvidenceIDs ids={gap.evidence_ids} /></article>)}{gaps.length === 0 && <p className="empty-row">No gaps were returned by this advisory run. Human verification is still required.</p>}</section>;
}

function AdvisoryResult({ run }: { run: DeveloperAssetAIAdvisoryRun }) {
  if (run.prompt_key === "sdk.applicability_suggestion") return <ApplicabilityResult result={run.result as DeveloperAssetAIApplicabilityResult} />;
  if (run.prompt_key === "sdk.sample_review") return <SampleReviewResult result={run.result as DeveloperAssetAISampleReviewResult} />;
  return <MapResult result={run.result as DeveloperAssetAIMapResult} />;
}

function AdvisoryDetail({ run }: { run: DeveloperAssetAIAdvisoryRun }) {
  const identities = [
    ["Source publication", run.source_publication_id],
    ["SDK package", run.sdk_package_id],
    ["Exact SDK release", run.sdk_release_id],
    ["SDK content candidate", run.sdk_content_candidate_id],
    ["SDK content publication", run.sdk_content_publication_id],
    ["API", run.api_id],
    ["API resource publication", run.api_developer_asset_publication_id],
    ["API SDK binding", run.api_sdk_binding_id],
    ["SDK sample", run.sdk_code_sample_id],
    ["Ingestion run", run.ingestion_run_id],
  ].filter((entry): entry is [string, string] => Boolean(entry[1]));

  return <div className="developer-ai-detail">
    <div className="developer-ai-detail-heading"><span><Badge color="violet">{run.prompt_key}</Badge><Badge color="zinc">{run.scope_visibility}</Badge></span><small><Clock3 />{new Date(run.created_at).toLocaleString()}</small></div>
    <dl className="entity-detail-grid"><div><dt>Prompt version</dt><dd><code>{run.prompt_version}</code></dd></div><div><dt>Advisory run ID</dt><dd><code>{run.id}</code></dd></div><div><dt>Scope</dt><dd>{run.scope_kind}<br /><code>{run.scope_id}</code></dd></div><div><dt>Result hash</dt><dd><code>{run.result_hash}</code></dd></div></dl>
    <AdvisoryResult run={run} />
    <details className="advanced-details"><summary>Exact evidence and citation identity</summary><div className="developer-ai-exact-evidence"><h4>Allowed evidence IDs</h4><EvidenceIDs ids={run.allowed_evidence_ids} /><dl className="entity-detail-grid">{identities.map(([label, id]) => <div key={label}><dt>{label}</dt><dd><code>{id}</code></dd></div>)}<div><dt>Evidence hash</dt><dd><code>{run.evidence_hash}</code></dd></div><div><dt>Input hash</dt><dd><code>{run.input_hash}</code></dd></div></dl></div></details>
  </div>;
}

export function DeveloperAssetAIAdvisoryButton({
  input,
  subject,
  label = "AI advisory",
  unavailableReason = "An exact published scope is required before advisory AI can run.",
}: {
  input: DeveloperAssetAIAdvisoryInput | null;
  subject: string;
  label?: string;
  unavailableReason?: string;
}) {
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
      setProblem(developerAssetError(error, "Advisory history could not be loaded."));
    } finally { setLoading(false); }
  }, [input, scopeID]);

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
      setProblem(developerAssetError(error, "The immutable advisory detail could not be loaded."));
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
        setProblem("Advisory AI is disabled or unconfigured for the Analysis workload. No advisory was stored; existing history remains readable.");
      } else {
        setProblem(developerAssetError(error, "Advisory AI failed. No advisory was stored and no asset was changed."));
      }
    } finally { setRunning(false); }
  }

  if (!input) return <Button type="button" outline disabled title={unavailableReason}><Sparkles data-slot="icon" />AI advisory unavailable</Button>;

  return <>
    <Button type="button" outline onClick={() => setOpen(true)}><Sparkles data-slot="icon" />{label}</Button>
    <Dialog open={open} onClose={setOpen} title={`Advisory AI · ${subject}`} description="Inspect immutable, evidence-bounded suggestions and their history. Advisory AI cannot approve, validate, attach, publish, or change this asset." actions={<><Button outline onClick={() => setOpen(false)}>Close</Button><Button color="indigo" disabled={running || availability === "unavailable"} onClick={() => void runAdvisory()}><Sparkles data-slot="icon" />{running ? "Running advisory…" : availability === "unavailable" ? "AI unconfigured" : "Run advisory AI"}</Button></>}>
      <div className="developer-ai-advisory">
        <div className="notice developer-ai-non-authoritative"><ShieldAlert /><span><strong>Non-authoritative suggestion.</strong> A human must verify every selector, finding, gap, and exact evidence ID before making a separate review or publication decision.</span></div>
        <div className="developer-ai-availability"><span><strong>Analysis provider</strong><small>{availability === "unknown" ? "Configuration is checked only when a run is requested." : availability === "configured" ? "Configured for this successful run." : "Disabled or unconfigured. Run action is unavailable."}</small></span><Badge color={availability === "configured" ? "green" : availability === "unavailable" ? "red" : "zinc"}>{availability === "configured" ? "configured" : availability === "unavailable" ? "unconfigured" : "not checked"}</Badge></div>
        {problem && <div className="developer-ai-problem"><TriangleAlert /><span>{problem}</span></div>}
        {loading ? <div className="developer-ai-loading"><History /><span>Loading immutable advisory history…</span></div> : runs.length > 0 ? <><div className="developer-ai-history"><label><span>Advisory history</span><select aria-label={`Advisory history for ${subject}`} value={selected?.id ?? ""} onChange={(event) => void selectRun(event.target.value)}>{runs.map((run) => <option key={run.id} value={run.id}>{new Date(run.created_at).toLocaleString()} · {run.prompt_version} · {run.id}</option>)}</select></label><Badge color="zinc">{runs.length} immutable run{runs.length === 1 ? "" : "s"}</Badge></div>{selected && <AdvisoryDetail run={selected} />}</> : <div className="developer-ai-empty"><Quote /><strong>No advisory history</strong><small>Run advisory AI to create an immutable suggestion over this exact published scope. It will not alter the asset.</small></div>}
      </div>
    </Dialog>
  </>;
}
