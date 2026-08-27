"use client";

import { Check, Copy, Filter, Search, ShieldCheck } from "lucide-react";
import { FormEvent, useState } from "react";

import type { APIIntegration } from "../../../lib/api";
import type { Section } from "../../../lib/console-routes";
import { developerAssetsApi, type DeveloperAssetKind, type DeveloperAssetScope, type QueryLabResponse } from "../../../lib/developer-assets-api";
import { Badge, Button } from "../../core/control";
import { PageHeader, PanelHeader, SegmentedControl } from "../../core/layout";
import { DocumentationNavigation } from "./developer-asset-navigation";
import { developerAssetError, PrettyJSON } from "./developer-asset-ui";

const assetKindOptions: Array<{ id: DeveloperAssetKind; label: string }> = [
  { id: "documentation", label: "Documentation" },
  { id: "contract", label: "Contracts" },
  { id: "sdk", label: "SDKs" },
];

function splitFilter(value: string) {
  return [...new Set(value.split(",").map((item) => item.trim()).filter(Boolean))];
}

function score(value: number) {
  return Number.isFinite(value) ? value.toFixed(4) : "—";
}

export function QueryLabView({ live, integrations, initialResult = null, onMessage, onNavigate }: { live: boolean; integrations: APIIntegration[]; initialResult?: QueryLabResponse | null; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [scope, setScope] = useState<DeveloperAssetScope>("global");
  const [apiID, setAPIID] = useState("");
  const [query, setQuery] = useState("");
  const [assetKinds, setAssetKinds] = useState<DeveloperAssetKind[]>([]);
  const [languages, setLanguages] = useState("");
  const [ecosystems, setEcosystems] = useState("");
  const [exactVersions, setExactVersions] = useState("");
  const [releaseIDs, setReleaseIDs] = useState("");
  const [globalPublicationID, setGlobalPublicationID] = useState("");
  const [apiPublicationID, setAPIPublicationID] = useState("");
  const [limit, setLimit] = useState(10);
  const [contextTokens, setContextTokens] = useState(4000);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<QueryLabResponse | null>(initialResult);
  const [problem, setProblem] = useState("");
  const [copied, setCopied] = useState(false);

  async function run(event: FormEvent) {
    event.preventDefault();
    if (!live || !query.trim() || ((scope === "api" || scope === "combined") && !apiID)) return;
    setBusy(true);
    setProblem("");
    try {
      const response = await developerAssetsApi.queryLab({
        scope,
        ...(scope !== "global" ? { api_id: apiID } : {}),
        ...(globalPublicationID.trim() ? { deployment_documentation_publication_id: globalPublicationID.trim() } : {}),
        ...(scope !== "global" && apiPublicationID.trim() ? { api_developer_asset_publication_id: apiPublicationID.trim() } : {}),
        query: query.trim(),
        ...(assetKinds.length ? { asset_kinds: assetKinds } : {}),
        ...(splitFilter(languages).length ? { languages: splitFilter(languages) } : {}),
        ...(splitFilter(ecosystems).length ? { ecosystems: splitFilter(ecosystems) } : {}),
        ...(splitFilter(exactVersions).length ? { exact_versions: splitFilter(exactVersions) } : {}),
        ...(splitFilter(releaseIDs).length ? { sdk_release_ids: splitFilter(releaseIDs) } : {}),
        limit,
        context_token_limit: contextTokens,
      });
      setResult(response);
      onMessage(`Query completed with trace ${response.trace_id}.`);
    } catch (error) {
      setResult(null);
      setProblem(developerAssetError(error, "Published developer assets could not be queried."));
    } finally {
      setBusy(false);
    }
  }

  async function copyTrace() {
    if (!result?.trace_id) return;
    await navigator.clipboard.writeText(result.trace_id);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  }

  const active: Section = "query-lab";
  const apiRequired = scope === "api" || scope === "combined";
  return <>
    <PageHeader eyebrow="Docs" title="Query Lab" />
    <DocumentationNavigation active={active} onNavigate={onNavigate} />
    <form className="developer-query-layout" onSubmit={run}>
      <section className="panel developer-query-form">
        <PanelHeader title="Query published knowledge" description="Choose the scope first, then narrow retrieval with exact filters." />
        <div className="developer-query-body">
          <div className="developer-query-scope"><span>Scope</span><SegmentedControl label="Query scope" value={scope} onChange={(value) => { setScope(value); if (value === "global") setAPIID(""); }} items={[{ id: "global", label: "Global" }, { id: "api", label: "API" }, { id: "combined", label: "Combined" }]} /></div>
          {apiRequired && <label className="auth-field"><span>API</span><select value={apiID} onChange={(event) => setAPIID(event.target.value)}><option value="">Select an API</option>{integrations.map((integration) => <option key={integration.id} value={integration.id}>{integration.display_name} · {integration.version_key}</option>)}</select><small>The server rejects a publication that belongs to another API.</small></label>}
          <label className="auth-field developer-query-input"><span>Question</span><div><Search /><textarea value={query} onChange={(event) => setQuery(event.target.value)} placeholder="How do I authenticate the JavaScript SDK?" maxLength={500} /></div></label>
          <fieldset className="developer-query-kinds"><legend><Filter />Asset kinds</legend>{assetKindOptions.map((option) => <label key={option.id}><input type="checkbox" checked={assetKinds.includes(option.id)} onChange={(event) => setAssetKinds((current) => event.target.checked ? [...current, option.id] : current.filter((item) => item !== option.id))} /><span>{option.label}</span></label>)}</fieldset>
          <details className="advanced-details developer-query-filters"><summary>Exact scope and filters</summary><div className="auth-form compact-form"><label className="auth-field"><span>Global publication ID</span><input value={globalPublicationID} onChange={(event) => setGlobalPublicationID(event.target.value)} placeholder="Resolve active when blank" /></label>{apiRequired && <label className="auth-field"><span>API developer-asset publication ID</span><input value={apiPublicationID} onChange={(event) => setAPIPublicationID(event.target.value)} placeholder="Resolve exact API scope when blank" /></label>}<div className="two-fields"><label className="auth-field"><span>Languages</span><input value={languages} onChange={(event) => setLanguages(event.target.value)} placeholder="typescript, markdown" /></label><label className="auth-field"><span>Ecosystems</span><input value={ecosystems} onChange={(event) => setEcosystems(event.target.value)} placeholder="npm, go" /></label></div><div className="two-fields"><label className="auth-field"><span>Exact versions</span><input value={exactVersions} onChange={(event) => setExactVersions(event.target.value)} placeholder="1.4.0, 2.0.1" /></label><label className="auth-field"><span>Exact SDK release IDs</span><input value={releaseIDs} onChange={(event) => setReleaseIDs(event.target.value)} placeholder="sdk-release-…" /></label></div><div className="two-fields"><label className="auth-field"><span>Result limit</span><input type="number" min={1} max={50} value={limit} onChange={(event) => setLimit(Number(event.target.value))} /></label><label className="auth-field"><span>Context token limit</span><input type="number" min={256} max={32000} value={contextTokens} onChange={(event) => setContextTokens(Number(event.target.value))} /></label></div></div></details>
        </div>
        <div className="developer-query-actions"><Button type="submit" color="indigo" disabled={!live || busy || !query.trim() || (apiRequired && !apiID)}>{busy ? "Searching…" : "Run query"}</Button><small>{live ? "Hybrid retrieval is bounded by the selected published scope." : "Query Lab is unavailable in fixture preview."}</small></div>
      </section>
      <section className="panel developer-query-results" aria-live="polite">
        <PanelHeader title="Ranked results" description={result ? `${result.results.length} selected result${result.results.length === 1 ? "" : "s"} · ${result.context_tokens} context tokens` : "Scores, excerpts, and immutable citations appear after a query."} action={result && <Button outline onClick={() => void copyTrace()}><Copy data-slot="icon" />{copied ? "Copied" : "Copy trace ID"}</Button>} />
        {problem && <div className="workspace-notice error"><span><strong>Query failed</strong><small>{problem}</small></span></div>}
        {result && <div className="developer-query-trace"><span><ShieldCheck /><span><small>Trace ID</small><code>{result.trace_id}</code></span></span><Badge color="green"><Check />{result.resolved_scope.scope} scope</Badge></div>}
        {result?.results.map((item) => <article className="developer-query-result" key={`${item.rank}:${item.unit.id}`}>
          <header><span className="developer-query-rank">{item.rank}</span><span><strong>{item.unit.title || item.unit.breadcrumb.at(-1) || item.unit.unit_kind}</strong><small>{item.unit.source_publication_kind} · {item.unit.breadcrumb.join(" / ")}</small></span><Badge color="blue">{item.unit.visibility}</Badge></header>
          <p>{item.excerpt}</p>
          <div className="developer-query-scores" aria-label={`Scores for result ${item.rank}`}><span><small>Lexical</small><strong>{score(item.lexical_score)}</strong></span><span title="Local deterministic feature-hash cosine signal; not a learned embedding model."><small>Feature hash</small><strong>{score(item.semantic_score)}</strong></span><span><small>Rerank</small><strong>{score(item.rerank_score)}</strong></span></div>
          <details className="advanced-details"><summary>Citation and exact identity</summary><dl className="entity-detail-grid"><div><dt>Publication</dt><dd><code>{item.unit.source_publication_id}</code></dd></div><div><dt>Entity</dt><dd><code>{item.unit.source_entity_id}</code></dd></div><div><dt>Content hash</dt><dd><code>{item.unit.content_hash}</code></dd></div><div><dt>Unit ID</dt><dd><code>{item.unit.id}</code></dd></div></dl><PrettyJSON value={item.unit.citation} label={`Citation for result ${item.rank}`} /></details>
        </article>)}
        {result && result.results.length === 0 && <div className="developer-query-empty"><Search /><strong>No published result matched</strong><small>Broaden filters or verify that the requested exact scope has reviewed publications.</small></div>}
        {!result && !problem && <div className="developer-query-empty"><Search /><strong>No query yet</strong><small>Results remain empty until you run a bounded published-scope query.</small></div>}
        {result && <details className="advanced-details developer-query-diagnostics"><summary>Resolved scope and diagnostics</summary><PrettyJSON value={{ resolved_scope: result.resolved_scope, diagnostics: result.diagnostics }} label="Query trace diagnostics" /></details>}
      </section>
    </form>
  </>;
}
