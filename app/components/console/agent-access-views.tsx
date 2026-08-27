import { useTranslation } from "react-i18next";
import {
  AlertCircle, BookOpen, CheckCircle2, Clock3, Copy, Database, ExternalLink,
  Globe2, LockKeyhole, MoreHorizontal, Plus, RefreshCw, TriangleAlert, Users, XCircle,
} from "lucide-react";
import { useState } from "react";
import Image from "next/image";

import { type APICustomerAccount, type APIVisibility, type Distribution } from "../../lib/api";
import { Badge, Button, Switch } from "../core/control";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader as PageHeading, PanelHeader, SectionHeader, SegmentedControl } from "../core/layout";
import { DeveloperAssetAIAdvisoryButton } from "./developer-assets/developer-asset-ai-advisory";
import { CopyButton, EntityLink, Source, SummaryItem, agentClients } from "./shared";

type Visibility = APIVisibility;

export function DistributionView({
  enabled,
  onEnabledChange,
  resources,
  resourceFilter,
  setResourceFilter,
  onVisibilityChange,
  onCopied,
  publicEndpoint,
  tenantName,
  publicAgentSetup,
  privateAgentSetup,
  onConfigureIdentity,
  customerAccounts,
  customerAccountsStatus,
  customerAccountsHaveMore,
  onUpdateCustomerAccount,
  onLoadMoreCustomerAccounts,
  onOpenSources,
}: {
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
  resources: Array<{ id: string; name: string; resourceType: "source"; type: string; detail: string; visibility: Visibility }>;
  resourceFilter: "all" | "public" | "private";
  setResourceFilter: (filter: "all" | "public" | "private") => void;
  onVisibilityChange: (kind: "source", id: string) => void;
  onCopied: (label: string) => void;
  publicEndpoint: string;
  tenantName: string;
  publicAgentSetup: Distribution["agent_setup"]["public"];
  privateAgentSetup: Distribution["agent_setup"]["private"];
  onConfigureIdentity: () => void;
  customerAccounts: APICustomerAccount[];
  customerAccountsStatus: "loading" | "ready" | "unavailable";
  customerAccountsHaveMore: boolean;
  onUpdateCustomerAccount: (account: APICustomerAccount, state: APICustomerAccount["state"]) => Promise<boolean>;
  onLoadMoreCustomerAccounts: () => Promise<boolean>;
  onOpenSources: () => void;
}) {
  const { t } = useTranslation();
  return <>
    <PageHeading eyebrow={t("agentAccess.delivery")} title={t("navigation.agentAccess")} action={<Button outline disabled={!privateAgentSetup.available} onClick={() => window.open(privateAgentSetup.url, "_blank", "noopener,noreferrer")}><ExternalLink data-slot="icon" />{t("agentAccess.privateMCPSetup")}</Button>} />
    <section className={`public-mcp-card ${enabled ? "enabled" : ""}`}>
      <div className="public-mcp-copy"><div className="icon-tile"><Globe2 /></div><div><div className="title-row"><h2>{t("agentAccess.publicMCP")}</h2><Badge color={enabled ? "green" : "zinc"}>{enabled ? t("agentAccess.live") : t("agentAccess.off")}</Badge></div><p>{t("agentAccess.offerAnAuthenticationFreeReadOnlyMCPEndpointContaining")}</p><div className="endpoint"><code>{publicEndpoint}</code><button type="button" aria-label={t("agentAccess.copyPublicMCPEndpoint")} onClick={() => { navigator.clipboard.writeText(publicEndpoint); onCopied(t("agentAccess.publicMCPEndpointCopied")); }}><Copy />{t("agentAccess.copy")}</button></div></div></div>
      <div className="switch-stack"><Switch checked={enabled} onChange={onEnabledChange} label={t("agentAccess.enablePublicMCP")} /><small>{enabled ? t("agentAccess.acceptingAnonymousRequests") : t("agentAccess.disabledByDefault")}</small></div>
    </section>

    <section className="section-block agent-setup-section">
      <SectionHeader title={t("agentAccess.mcpSetupButtons")} />
      <div className="agent-setup-grid">
        <AgentSetupCard kind="public" tenantName={tenantName} setup={publicAgentSetup} onCopied={onCopied} onConfigureIdentity={onConfigureIdentity} />
        <AgentSetupCard kind="private" tenantName={tenantName} setup={privateAgentSetup} onCopied={onCopied} onConfigureIdentity={onConfigureIdentity} />
      </div>
    </section>

    <CustomerAccessPanel accounts={customerAccounts} status={customerAccountsStatus} hasMore={customerAccountsHaveMore} onUpdate={onUpdateCustomerAccount} onLoadMore={onLoadMoreCustomerAccounts} />

    <section className="section-block">
      <SectionHeader title={t("agentAccess.resourceVisibility")} action={<Button outline onClick={onOpenSources}>{t("agentAccess.manageSources")}</Button>} />
      <SegmentedControl label={t("agentAccess.filterResources")} items={[{ id: "all", label: t("common.all") }, { id: "public", label: t("common.public") }, { id: "private", label: t("common.private") }]} value={resourceFilter} onChange={setResourceFilter} />
      <DataTable label={t("agentAccess.resourceVisibility")}>
        <DataTableHeader className="resource-columns"><span>{t("agentAccess.resource")}</span><span>{t("agentAccess.type")}</span><span>{t("agentAccess.visibility")}</span><span>{t("agentAccess.actions")}</span></DataTableHeader>
        {resources.map((resource) => <DataTableRow className="resource-columns" key={`${resource.resourceType}-${resource.id}`}>
          <span className="resource-name"><span className="resource-icon"><BookOpen /></span><span><strong>{resource.name}</strong><small>{resource.detail}</small></span></span>
          <span>{resource.type}</span>
          <span className="visibility-control"><Badge color={resource.visibility === "public" ? "green" : "zinc"}>{resource.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{resource.visibility === "public" ? t("common.public") : t("common.private")}</Badge><Switch checked={resource.visibility === "public"} onChange={() => onVisibilityChange(resource.resourceType, resource.id)} label={resource.visibility === "public" ? t("agentAccess.makePrivate", { name: resource.name }) : t("agentAccess.makePublic", { name: resource.name })} /></span>
          <span className="table-actions"><button type="button" className="more" aria-label={t("agentAccess.actionsFor", { name: String(resource.name) })}><MoreHorizontal /></button></span>
        </DataTableRow>)}
        {resources.length === 0 && <DataTableEmpty columns={4}>{t("agentAccess.noResourcesMatchThisFilter")}</DataTableEmpty>}
      </DataTable>
    </section>
  </>;
}

function CustomerAccessPanel({ accounts, status, hasMore, onUpdate, onLoadMore }: { accounts: APICustomerAccount[]; status: "loading" | "ready" | "unavailable"; hasMore: boolean; onUpdate: (account: APICustomerAccount, state: APICustomerAccount["state"]) => Promise<boolean>; onLoadMore: () => Promise<boolean> }) {
  const { t } = useTranslation();
  const [busyAccount, setBusyAccount] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);

  async function updateAccount(account: APICustomerAccount, state: APICustomerAccount["state"]) {
    if (state === "suspended" && !window.confirm(t("agentAccess.suspendCustomerMCPAccessWillFailClosedImmediately", { external_id: String(account.external_id) }))) return;
    setBusyAccount(account.id);
    try { await onUpdate(account, state); } finally { setBusyAccount(null); }
  }

  return <section className="panel customer-access-panel">
    <PanelHeader title={t("agentAccess.customerAccess")} description={t("agentAccess.suspendACompromisedCustomerAccountWithoutChangingTheShared")} action={status === "ready" ? <Badge color="zinc">{t("agentAccess.accounts", { count: accounts.length })}</Badge> : undefined} />
    {status === "loading" && <div className="customer-access-state"><RefreshCw /><span><strong>{t("agentAccess.loadingCustomerAccounts")}</strong></span></div>}
    {status === "unavailable" && <div className="customer-access-state unavailable"><TriangleAlert /><span><strong>{t("agentAccess.customerAccountsUnavailable")}</strong></span></div>}
    {status === "ready" && accounts.length === 0 && <div className="customer-access-empty"><Users /><span><strong>{t("agentAccess.noCustomerAccountsYet")}</strong><small>{t("agentAccess.accountsAppearAfterTheFirstSuccessfulCustomerSignIn")}</small></span></div>}
    {status === "ready" && accounts.length > 0 && <div className="customer-access-list">{accounts.map((account) => <article className="customer-access-row" key={account.id}><span className="customer-access-identity"><strong>{account.external_id}</strong><small>{t("agentAccess.lastSignIn")} {t("format.dateTime", { value: new Date(account.last_authenticated_at) })}</small></span><Badge color={account.state === "active" ? "green" : "red"}>{account.state === "active" ? t("agentAccess.active") : t("agentAccess.suspended")}</Badge><Button outline disabled={busyAccount !== null} onClick={() => void updateAccount(account, account.state === "active" ? "suspended" : "active")}>{busyAccount === account.id ? t("common.saving") : account.state === "active" ? t("agentAccess.suspend") : t("agentAccess.reactivate")}</Button></article>)}{hasMore && <Button outline disabled={loadingMore} onClick={async () => { setLoadingMore(true); try { await onLoadMore(); } finally { setLoadingMore(false); } }}>{loadingMore ? t("common.loading") : t("agentAccess.loadMore")}</Button>}</div>}
  </section>;
}

function AgentSetupCard({ kind, tenantName, setup, onCopied, onConfigureIdentity }: { kind: "public" | "private"; tenantName: string; setup: Distribution["agent_setup"]["public"]; onCopied: (label: string) => void; onConfigureIdentity: () => void }) {
  const { t } = useTranslation();
  const isPublic = kind === "public";
  return <article className={`agent-setup-card ${!setup.available ? "agent-setup-disabled" : ""}`}>
    <div className={`agent-setup-preview ${isPublic ? "public-agent-preview" : "private-agent-preview"}`}><a href={setup.available ? setup.url : undefined} target="_blank" rel="noopener noreferrer" aria-disabled={!setup.available} onClick={(event) => { if (!setup.available) event.preventDefault(); }}><span className="agent-setup-label">{t("agentAccess.connectYourAgentTo")} {tenantName}</span><span className={`agent-access-chip ${kind}`}>{isPublic ? t("agentAccess.public") : t("agentAccess.private")}</span>{agentClients.map((client) => <Image key={client.id} className="agent-client-mark" src={`/agent-client-icons/${client.file}`} alt={client.name} width={20} height={20} />)}</a></div>
    <div className="agent-setup-copy"><Badge color={isPublic ? "blue" : "violet"}>{isPublic ? <Globe2 /> : <LockKeyhole />}{isPublic ? t("agentAccess.public") : t("agentAccess.private")}</Badge><h3>{isPublic ? t("agentAccess.publicMCPButton") : t("agentAccess.privateMCPButton")}</h3>{setup.available ? <a className="agent-setup-guide-link" href={setup.url} target="_blank" rel="noopener noreferrer"><ExternalLink />{t("agentAccess.openSetupInstructions")}</a> : <div className="inline-warning"><TriangleAlert />{isPublic ? t("agentAccess.enablePublicMCPFirst") : t("agentAccess.configureCustomerIdentityFirst")}</div>}{!isPublic && !setup.available && <Button outline onClick={onConfigureIdentity}>{t("agentAccess.configureIdentity")}</Button>}<CopyButton text={setup.embed_html} label={t("agentAccess.copyMCPButton", { kind: isPublic ? t("agentAccess.public") : t("agentAccess.private") })} disabled={!setup.available} onCopied={() => onCopied(isPublic ? t("agentAccess.publicMCPButtonCopied") : t("agentAccess.privateMCPButtonCopied"))} /></div>
  </article>;
}

export function SourcesView({ sources, navigation, onAdd, onCrawl, onPublish, onVisibilityChange, onNavigate }: { sources: Source[]; navigation?: React.ReactNode; onAdd: () => void; onCrawl: (id: string) => void; onPublish: (source: Source) => void; onVisibilityChange: (id: string) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <>
    <PageHeading eyebrow={t("navigation.docs")} title={t("navigation.sources")} action={<Button onClick={onAdd}><Plus data-slot="icon" />{t("agentAccess.addSource")}</Button>} />
    {navigation}
    <div className="summary-strip"><SummaryItem label={t("agentAccess.pagesIndexed")} value={String(sources.reduce((total, source) => total + source.pages, 0))} icon={<Database />} /><SummaryItem label={t("agentAccess.healthySources")} value={String(sources.filter((source) => source.crawlState === "synced").length)} icon={<CheckCircle2 />} /><SummaryItem label={t("agentAccess.needsAttention")} value={String(sources.filter((source) => source.crawlState === "review" || source.crawlState === "failed").length)} icon={<AlertCircle />} /></div>
    <div className="toolbar"><Button outline onClick={() => sources.forEach((source) => onCrawl(source.id))}><RefreshCw data-slot="icon" />{t("agentAccess.crawlAll")}</Button></div>
    <DataTable label={t("navigation.sources")}><DataTableHeader className="source-columns"><span>{t("agentAccess.source")}</span><span>{t("agentAccess.crawlState")}</span><span>{t("agentAccess.content")}</span><span>{t("agentAccess.visibility")}</span><span>{t("agentAccess.actions")}</span></DataTableHeader>{sources.map((source) => <DataTableRow className="source-columns" key={source.id}><span className="resource-name"><span className="resource-icon"><BookOpen /></span><span><EntityLink entity="source" uid={source.id} onNavigate={onNavigate} className="entity-link"><strong>{source.name}</strong></EntityLink><small>{source.location} · {source.kind}</small></span></span><span><CrawlBadge state={source.crawlState} /><small className="cell-note">{source.lastCrawl === "not-crawled" || source.lastCrawl === "Not crawled" ? t("agentAccess.notCrawled") : source.lastCrawl === "queued-now" || source.lastCrawl === "Queued now" ? t("agentAccess.queuedNow") : source.lastCrawl}</small></span><span><strong className="cell-value">{source.pages}</strong><small className="cell-note">{t("agentAccess.pages", { count: source.pages })}</small></span><span className="visibility-control"><Badge color={source.visibility === "public" ? "green" : "zinc"}>{source.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{source.visibility === "public" ? t("common.public") : t("common.private")}</Badge><Switch checked={source.visibility === "public"} onChange={() => onVisibilityChange(source.id)} label={source.visibility === "public" ? t("agentAccess.makePrivate", { name: source.name }) : t("agentAccess.makePublic", { name: source.name })} /></span><span className="table-actions"><DeveloperAssetAIAdvisoryButton input={source.latestPublication ? { prompt_key: "documentation.map_enrichment", source_publication_id: source.latestPublication.id } : null} subject={t("agentAccess.sourcePublicationSubject", { name: source.name })} label={t("agentAccess.aiMapAdvisory")} unavailableReason={t("agentAccess.publishReviewedSourceBeforeAI")} />{source.crawlState === "review" && <Button outline onClick={() => onPublish(source)}>{source.quarantined ? t("agentAccess.inspect") : t("agentAccess.review")}</Button>}<button type="button" className="more" aria-label={t("agentAccess.crawl", { name: String(source.name) })} onClick={() => onCrawl(source.id)}><RefreshCw /></button></span></DataTableRow>)}</DataTable>
  </>;
}

function CrawlBadge({ state }: { state: Source["crawlState"] }) {
  const { t } = useTranslation();
  if (state === "queued" || state === "running") return <Badge color="blue"><RefreshCw />{state === "queued" ? t("agentAccess.queued") : t("agentAccess.running")}</Badge>;
  if (state === "synced") return <Badge color="green"><CheckCircle2 />{t("agentAccess.synced")}</Badge>;
  if (state === "review") return <Badge color="amber"><Clock3 />{t("agentAccess.needsReview")}</Badge>;
  if (state === "draft") return <Badge color="zinc"><Clock3 />{t("agentAccess.notCrawled")}</Badge>;
  if (state === "cancelled") return <Badge color="zinc"><XCircle />{t("agentAccess.cancelled")}</Badge>;
  return <Badge color="red"><XCircle />{t("agentAccess.failed")}</Badge>;
}
