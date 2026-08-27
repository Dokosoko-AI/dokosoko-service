"use client";


import { useTranslation } from "react-i18next";
import { Check, Copy, Filter, Search, ShieldCheck } from "lucide-react";
import { FormEvent, useState } from "react";

import type { APIIntegration } from "../../../lib/api";
import type { Section } from "../../../lib/console-routes";
import { developerAssetsApi, type DeveloperAssetKind, type DeveloperAssetScope, type QueryLabResponse } from "../../../lib/developer-assets-api";
import { Badge, Button } from "../../core/control";
import { PageHeader, PanelHeader, SegmentedControl } from "../../core/layout";
import { DocumentationNavigation } from "./developer-asset-navigation";
import { developerAssetError, PrettyJSON } from "./developer-asset-ui";

function splitFilter(value: string) {
  return [...new Set(value.split(",").map((item) => item.trim()).filter(Boolean))];
}

function score(value: number) {
  return Number.isFinite(value) ? value.toFixed(4) : "—";
}

export function QueryLabView({ live, integrations, initialResult = null, onMessage, onNavigate }: { live: boolean; integrations: APIIntegration[]; initialResult?: QueryLabResponse | null; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
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
  const assetKindOptions: Array<{ id: DeveloperAssetKind; label: string }> = [
    { id: "documentation", label: t("routes.documentation") },
    { id: "contract", label: t("common.contract") },
    { id: "sdk", label: t("routes.sdks") },
  ];

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
      onMessage(t("queryLab.queryCompletedWithTrace", { trace_id: String(response.trace_id) }));
    } catch (error) {
      setResult(null);
      setProblem(developerAssetError(error, t("queryLab.publishedDeveloperAssetsCouldNotBeQueried")));
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
    <PageHeader eyebrow={t("navigation.docs")} title={t("navigation.queryLab")} />
    <DocumentationNavigation active={active} onNavigate={onNavigate} />
    <form className="developer-query-layout" onSubmit={run}>
      <section className="panel developer-query-form">
        <PanelHeader title={t("queryLab.queryPublishedKnowledge")} description={t("queryLab.chooseTheScopeFirstThenNarrowRetrievalWithExact")} />
        <div className="developer-query-body">
          <div className="developer-query-scope"><span>{t("queryLab.scope")}</span><SegmentedControl label={t("queryLab.queryScope")} value={scope} onChange={(value) => { setScope(value); if (value === "global") setAPIID(""); }} items={[{ id: "global", label: t("common.global") }, { id: "api", label: "API" }, { id: "combined", label: t("common.combined") }]} /></div>
          {apiRequired && <label className="auth-field"><span>API</span><select value={apiID} onChange={(event) => setAPIID(event.target.value)}><option value="">{t("queryLab.selectAnAPI")}</option>{integrations.map((integration) => <option key={integration.id} value={integration.id}>{integration.display_name} · {integration.version_key}</option>)}</select><small>{t("queryLab.theServerRejectsAPublicationThatBelongsToAnother")}</small></label>}
          <label className="auth-field developer-query-input"><span>{t("queryLab.question")}</span><div><Search /><textarea value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t("queryLab.howDoIAuthenticateTheJavaScriptSDK")} maxLength={500} /></div></label>
          <fieldset className="developer-query-kinds"><legend><Filter />{t("queryLab.assetKinds")}</legend>{assetKindOptions.map((option) => <label key={option.id}><input type="checkbox" checked={assetKinds.includes(option.id)} onChange={(event) => setAssetKinds((current) => event.target.checked ? [...current, option.id] : current.filter((item) => item !== option.id))} /><span>{option.label}</span></label>)}</fieldset>
          <details className="advanced-details developer-query-filters"><summary>{t("queryLab.exactScopeAndFilters")}</summary><div className="auth-form compact-form"><label className="auth-field"><span>{t("queryLab.globalPublicationID")}</span><input value={globalPublicationID} onChange={(event) => setGlobalPublicationID(event.target.value)} placeholder={t("queryLab.resolveActiveWhenBlank")} /></label>{apiRequired && <label className="auth-field"><span>{t("queryLab.apiDeveloperAssetPublicationID")}</span><input value={apiPublicationID} onChange={(event) => setAPIPublicationID(event.target.value)} placeholder={t("queryLab.resolveExactAPIScopeWhenBlank")} /></label>}<div className="two-fields"><label className="auth-field"><span>{t("queryLab.languages")}</span><input value={languages} onChange={(event) => setLanguages(event.target.value)} placeholder={t("queryLab.typescriptMarkdown")} /></label><label className="auth-field"><span>{t("queryLab.ecosystems")}</span><input value={ecosystems} onChange={(event) => setEcosystems(event.target.value)} placeholder={t("queryLab.npmGo")} /></label></div><div className="two-fields"><label className="auth-field"><span>{t("queryLab.exactVersions")}</span><input value={exactVersions} onChange={(event) => setExactVersions(event.target.value)} placeholder="1.4.0, 2.0.1" /></label><label className="auth-field"><span>{t("queryLab.exactSDKReleaseIDs")}</span><input value={releaseIDs} onChange={(event) => setReleaseIDs(event.target.value)} placeholder={t("queryLab.sdkRelease")} /></label></div><div className="two-fields"><label className="auth-field"><span>{t("queryLab.resultLimit")}</span><input type="number" min={1} max={50} value={limit} onChange={(event) => setLimit(Number(event.target.value))} /></label><label className="auth-field"><span>{t("queryLab.contextTokenLimit")}</span><input type="number" min={256} max={32000} value={contextTokens} onChange={(event) => setContextTokens(Number(event.target.value))} /></label></div></div></details>
        </div>
        <div className="developer-query-actions"><Button type="submit" color="indigo" disabled={!live || busy || !query.trim() || (apiRequired && !apiID)}>{busy ? t("queryLab.searching") : t("queryLab.runQuery")}</Button><small>{live ? t("queryLab.hybridRetrievalIsBoundedByTheSelectedPublishedScope") : t("queryLab.queryLabIsUnavailableInFixturePreview")}</small></div>
      </section>
      <section className="panel developer-query-results" aria-live="polite">
        <PanelHeader title={t("queryLab.rankedResults")} description={result ? t("queryLab.selectedResultsWithContextTokens", { count: result.results.length, contextTokens: result.context_tokens }) : t("queryLab.scoresExcerptsAndImmutableCitationsAppearAfterAQuery")} action={result && <Button outline onClick={() => void copyTrace()}><Copy data-slot="icon" />{copied ? t("queryLab.copied") : t("queryLab.copyTraceID")}</Button>} />
        {problem && <div className="workspace-notice error"><span><strong>{t("queryLab.queryFailed")}</strong><small>{problem}</small></span></div>}
        {result && <div className="developer-query-trace"><span><ShieldCheck /><span><small>{t("queryLab.traceID")}</small><code>{result.trace_id}</code></span></span><Badge color="green"><Check />{t("queryLab.resolvedScope", { scope: result.resolved_scope.scope })}</Badge></div>}
        {result?.results.map((item) => <article className="developer-query-result" key={`${item.rank}:${item.unit.id}`}>
          <header><span className="developer-query-rank">{item.rank}</span><span><strong>{item.unit.title || item.unit.breadcrumb.at(-1) || item.unit.unit_kind}</strong><small>{item.unit.source_publication_kind} · {item.unit.breadcrumb.join(" / ")}</small></span><Badge color="blue">{item.unit.visibility}</Badge></header>
          <p>{item.excerpt}</p>
          <div className="developer-query-scores" aria-label={t("queryLab.scoresForResult", { rank: String(item.rank) })}><span><small>{t("queryLab.lexical")}</small><strong>{score(item.lexical_score)}</strong></span><span title={t("queryLab.localDeterministicFeatureHashCosineSignalNotALearned")}><small>{t("queryLab.featureHash")}</small><strong>{score(item.semantic_score)}</strong></span><span><small>{t("queryLab.rerank")}</small><strong>{score(item.rerank_score)}</strong></span></div>
          <details className="advanced-details"><summary>{t("queryLab.citationAndExactIdentity")}</summary><dl className="entity-detail-grid"><div><dt>{t("queryLab.publication")}</dt><dd><code>{item.unit.source_publication_id}</code></dd></div><div><dt>{t("queryLab.entity")}</dt><dd><code>{item.unit.source_entity_id}</code></dd></div><div><dt>{t("queryLab.contentHash")}</dt><dd><code>{item.unit.content_hash}</code></dd></div><div><dt>{t("queryLab.unitID")}</dt><dd><code>{item.unit.id}</code></dd></div></dl><PrettyJSON value={item.unit.citation} label={t("queryLab.citationForResult", { rank: String(item.rank) })} /></details>
        </article>)}
        {result && result.results.length === 0 && <div className="developer-query-empty"><Search /><strong>{t("queryLab.noPublishedResultMatched")}</strong><small>{t("queryLab.broadenFiltersOrVerifyThatTheRequestedExactScope")}</small></div>}
        {!result && !problem && <div className="developer-query-empty"><Search /><strong>{t("queryLab.noQueryYet")}</strong><small>{t("queryLab.resultsRemainEmptyUntilYouRunABoundedPublished")}</small></div>}
        {result && <details className="advanced-details developer-query-diagnostics"><summary>{t("queryLab.resolvedScopeAndDiagnostics")}</summary><PrettyJSON value={{ resolved_scope: result.resolved_scope, diagnostics: result.diagnostics }} label={t("queryLab.queryTraceDiagnostics")} /></details>}
      </section>
    </form>
  </>;
}
