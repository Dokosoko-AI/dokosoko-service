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
  return <>
    <PageHeading eyebrow="Delivery" title="Agent access" action={<Button outline disabled={!privateAgentSetup.available} onClick={() => window.open(privateAgentSetup.url, "_blank", "noopener,noreferrer")}><ExternalLink data-slot="icon" />Private MCP setup</Button>} />
    <section className={`public-mcp-card ${enabled ? "enabled" : ""}`}>
      <div className="public-mcp-copy"><div className="icon-tile"><Globe2 /></div><div><div className="title-row"><h2>Public MCP</h2><Badge color={enabled ? "green" : "zinc"}>{enabled ? "Live" : "Off"}</Badge></div><p>Offer an authentication-free, read-only MCP endpoint containing only explicitly published resources.</p><div className="endpoint"><code>{publicEndpoint}</code><button type="button" aria-label="Copy public MCP endpoint" onClick={() => { navigator.clipboard.writeText(publicEndpoint); onCopied("Public MCP endpoint copied."); }}><Copy />Copy</button></div></div></div>
      <div className="switch-stack"><Switch checked={enabled} onChange={onEnabledChange} label="Enable Public MCP" /><small>{enabled ? "Accepting anonymous requests" : "Disabled by default"}</small></div>
    </section>

    <section className="section-block agent-setup-section">
      <SectionHeader title="MCP setup buttons" />
      <div className="agent-setup-grid">
        <AgentSetupCard kind="public" tenantName={tenantName} setup={publicAgentSetup} onCopied={onCopied} onConfigureIdentity={onConfigureIdentity} />
        <AgentSetupCard kind="private" tenantName={tenantName} setup={privateAgentSetup} onCopied={onCopied} onConfigureIdentity={onConfigureIdentity} />
      </div>
    </section>

    <CustomerAccessPanel accounts={customerAccounts} status={customerAccountsStatus} hasMore={customerAccountsHaveMore} onUpdate={onUpdateCustomerAccount} onLoadMore={onLoadMoreCustomerAccounts} />

    <section className="section-block">
      <SectionHeader title="Resource visibility" action={<Button outline onClick={onOpenSources}>Manage sources</Button>} />
      <SegmentedControl label="Filter resources" items={[{ id: "all", label: "All" }, { id: "public", label: "Public" }, { id: "private", label: "Private" }]} value={resourceFilter} onChange={setResourceFilter} />
      <DataTable label="Resource visibility">
        <DataTableHeader className="resource-columns"><span>Resource</span><span>Type</span><span>Visibility</span><span>Actions</span></DataTableHeader>
        {resources.map((resource) => <DataTableRow className="resource-columns" key={`${resource.resourceType}-${resource.id}`}>
          <span className="resource-name"><span className="resource-icon"><BookOpen /></span><span><strong>{resource.name}</strong><small>{resource.detail}</small></span></span>
          <span>{resource.type}</span>
          <span className="visibility-control"><Badge color={resource.visibility === "public" ? "green" : "zinc"}>{resource.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{resource.visibility}</Badge><Switch checked={resource.visibility === "public"} onChange={() => onVisibilityChange(resource.resourceType, resource.id)} label={`Make ${resource.name} ${resource.visibility === "public" ? "private" : "public"}`} /></span>
          <span className="table-actions"><button type="button" className="more" aria-label={`Actions for ${resource.name}`}><MoreHorizontal /></button></span>
        </DataTableRow>)}
        {resources.length === 0 && <DataTableEmpty columns={4}>No resources match this filter.</DataTableEmpty>}
      </DataTable>
    </section>
  </>;
}

function CustomerAccessPanel({ accounts, status, hasMore, onUpdate, onLoadMore }: { accounts: APICustomerAccount[]; status: "loading" | "ready" | "unavailable"; hasMore: boolean; onUpdate: (account: APICustomerAccount, state: APICustomerAccount["state"]) => Promise<boolean>; onLoadMore: () => Promise<boolean> }) {
  const [busyAccount, setBusyAccount] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);

  async function updateAccount(account: APICustomerAccount, state: APICustomerAccount["state"]) {
    if (state === "suspended" && !window.confirm(`Suspend ${account.external_id}? Customer MCP access will fail closed immediately.`)) return;
    setBusyAccount(account.id);
    try { await onUpdate(account, state); } finally { setBusyAccount(null); }
  }

  return <section className="panel customer-access-panel">
    <PanelHeader title="Customer access" description="Suspend a compromised customer account without changing the shared OIDC connection." action={status === "ready" ? <Badge color="zinc">{accounts.length} account{accounts.length === 1 ? "" : "s"}</Badge> : undefined} />
    {status === "loading" && <div className="customer-access-state"><RefreshCw /><span><strong>Loading customer accounts</strong></span></div>}
    {status === "unavailable" && <div className="customer-access-state unavailable"><TriangleAlert /><span><strong>Customer accounts unavailable</strong></span></div>}
    {status === "ready" && accounts.length === 0 && <div className="customer-access-empty"><Users /><span><strong>No customer accounts yet</strong><small>Accounts appear after the first successful customer sign-in.</small></span></div>}
    {status === "ready" && accounts.length > 0 && <div className="customer-access-list">{accounts.map((account) => <article className="customer-access-row" key={account.id}><span className="customer-access-identity"><strong>{account.external_id}</strong><small>Last sign-in {new Date(account.last_authenticated_at).toLocaleString()}</small></span><Badge color={account.state === "active" ? "green" : "red"}>{account.state}</Badge><Button outline disabled={busyAccount !== null} onClick={() => void updateAccount(account, account.state === "active" ? "suspended" : "active")}>{busyAccount === account.id ? "Saving…" : account.state === "active" ? "Suspend" : "Reactivate"}</Button></article>)}{hasMore && <Button outline disabled={loadingMore} onClick={async () => { setLoadingMore(true); try { await onLoadMore(); } finally { setLoadingMore(false); } }}>{loadingMore ? "Loading…" : "Load more"}</Button>}</div>}
  </section>;
}

function AgentSetupCard({ kind, tenantName, setup, onCopied, onConfigureIdentity }: { kind: "public" | "private"; tenantName: string; setup: Distribution["agent_setup"]["public"]; onCopied: (label: string) => void; onConfigureIdentity: () => void }) {
  const isPublic = kind === "public";
  return <article className={`agent-setup-card ${!setup.available ? "agent-setup-disabled" : ""}`}>
    <div className={`agent-setup-preview ${isPublic ? "public-agent-preview" : "private-agent-preview"}`}><a href={setup.available ? setup.url : undefined} target="_blank" rel="noopener noreferrer" aria-disabled={!setup.available} onClick={(event) => { if (!setup.available) event.preventDefault(); }}><span className="agent-setup-label">Connect your agent to {tenantName}</span><span className={`agent-access-chip ${kind}`}>{isPublic ? "Public" : "Private"}</span>{agentClients.map((client) => <Image key={client.id} className="agent-client-mark" src={`/agent-client-icons/${client.file}`} alt={client.name} width={20} height={20} />)}</a></div>
    <div className="agent-setup-copy"><Badge color={isPublic ? "blue" : "violet"}>{isPublic ? <Globe2 /> : <LockKeyhole />}{isPublic ? "Public" : "Private"}</Badge><h3>{isPublic ? "Public MCP button" : "Private MCP button"}</h3>{setup.available ? <a className="agent-setup-guide-link" href={setup.url} target="_blank" rel="noopener noreferrer"><ExternalLink />Open setup instructions</a> : <div className="inline-warning"><TriangleAlert />{isPublic ? "Enable Public MCP first." : "Configure customer identity first."}</div>}{!isPublic && !setup.available && <Button outline onClick={onConfigureIdentity}>Configure identity</Button>}<CopyButton text={setup.embed_html} label={`Copy ${kind} MCP button`} disabled={!setup.available} onCopied={() => onCopied(`${isPublic ? "Public" : "Private"} MCP button copied.`)} /></div>
  </article>;
}

export function SourcesView({ sources, navigation, onAdd, onCrawl, onPublish, onVisibilityChange, onNavigate }: { sources: Source[]; navigation?: React.ReactNode; onAdd: () => void; onCrawl: (id: string) => void; onPublish: (source: Source) => void; onVisibilityChange: (id: string) => void; onNavigate: (path: string) => void }) {
  return <>
    <PageHeading eyebrow="Docs" title="Sources" action={<Button onClick={onAdd}><Plus data-slot="icon" />Add source</Button>} />
    {navigation}
    <div className="summary-strip"><SummaryItem label="Pages indexed" value={String(sources.reduce((total, source) => total + source.pages, 0))} icon={<Database />} /><SummaryItem label="Healthy sources" value={String(sources.filter((source) => source.crawlState === "synced").length)} icon={<CheckCircle2 />} /><SummaryItem label="Needs attention" value={String(sources.filter((source) => source.crawlState === "review" || source.crawlState === "failed").length)} icon={<AlertCircle />} /></div>
    <div className="toolbar"><Button outline onClick={() => sources.forEach((source) => onCrawl(source.id))}><RefreshCw data-slot="icon" />Crawl all</Button></div>
    <DataTable label="Sources"><DataTableHeader className="source-columns"><span>Source</span><span>Crawl state</span><span>Content</span><span>Visibility</span><span>Actions</span></DataTableHeader>{sources.map((source) => <DataTableRow className="source-columns" key={source.id}><span className="resource-name"><span className="resource-icon"><BookOpen /></span><span><EntityLink entity="source" uid={source.id} onNavigate={onNavigate} className="entity-link"><strong>{source.name}</strong></EntityLink><small>{source.location} · {source.kind}</small></span></span><span><CrawlBadge state={source.crawlState} /><small className="cell-note">{source.lastCrawl}</small></span><span><strong className="cell-value">{source.pages}</strong><small className="cell-note">pages</small></span><span className="visibility-control"><Badge color={source.visibility === "public" ? "green" : "zinc"}>{source.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{source.visibility}</Badge><Switch checked={source.visibility === "public"} onChange={() => onVisibilityChange(source.id)} label={`Make ${source.name} ${source.visibility === "public" ? "private" : "public"}`} /></span><span className="table-actions"><DeveloperAssetAIAdvisoryButton input={source.latestPublication ? { prompt_key: "documentation.map_enrichment", source_publication_id: source.latestPublication.id } : null} subject={`${source.name} source publication`} label="AI map advisory" unavailableReason="Publish a reviewed source generation before requesting advisory AI." />{source.crawlState === "review" && <Button outline onClick={() => onPublish(source)}>{source.quarantined ? "Inspect" : "Review"}</Button>}<button type="button" className="more" aria-label={`Crawl ${source.name}`} onClick={() => onCrawl(source.id)}><RefreshCw /></button></span></DataTableRow>)}</DataTable>
  </>;
}

function CrawlBadge({ state }: { state: Source["crawlState"] }) {
  if (state === "queued" || state === "running") return <Badge color="blue"><RefreshCw />{state}</Badge>;
  if (state === "synced") return <Badge color="green"><CheckCircle2 />Synced</Badge>;
  if (state === "review") return <Badge color="amber"><Clock3 />Needs review</Badge>;
  if (state === "draft") return <Badge color="zinc"><Clock3 />Not crawled</Badge>;
  if (state === "cancelled") return <Badge color="zinc"><XCircle />Cancelled</Badge>;
  return <Badge color="red"><XCircle />Failed</Badge>;
}
