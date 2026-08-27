import { useTranslation } from "react-i18next";
import {
  AlertCircle, BookOpen, CheckCircle2, Clock3, Copy, Database, ExternalLink,
  Globe2, LockKeyhole, MoreHorizontal, Plus, RefreshCw, TriangleAlert, XCircle,
} from "lucide-react";
import Image from "next/image";

import { type APIVisibility, type Distribution } from "../../lib/api";
import { Badge, Button, Switch } from "../core/control";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader as PageHeading, SectionHeader, SegmentedControl } from "../core/layout";
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
  privateEndpoint,
  tenantName,
  publicAgentSetup,
  privateAgentSetup,
  onConfigureIdentity,
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
  privateEndpoint: string;
  tenantName: string;
  publicAgentSetup: Distribution["agent_setup"]["public"];
  privateAgentSetup: Distribution["agent_setup"]["private"];
  onConfigureIdentity: () => void;
  onOpenSources: () => void;
}) {
  const { t } = useTranslation();
  return <>
    <PageHeading eyebrow={t("agentAccess.delivery")} title={t("navigation.agentAccess")} />

    <section className="section-block agent-setup-section">
      <div className="agent-setup-grid">
        <AgentSetupCard kind="public" tenantName={tenantName} endpoint={publicEndpoint} setup={publicAgentSetup} enabled={enabled} onEnabledChange={onEnabledChange} onCopied={onCopied} onConfigureIdentity={onConfigureIdentity} />
        <AgentSetupCard kind="private" tenantName={tenantName} endpoint={privateEndpoint} setup={privateAgentSetup} onCopied={onCopied} onConfigureIdentity={onConfigureIdentity} />
      </div>
    </section>

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

function AgentSetupCard({ kind, tenantName, endpoint, setup, enabled = false, onEnabledChange, onCopied, onConfigureIdentity }: {
  kind: "public" | "private";
  tenantName: string;
  endpoint: string;
  setup: Distribution["agent_setup"]["public"];
  enabled?: boolean;
  onEnabledChange?: (enabled: boolean) => void;
  onCopied: (label: string) => void;
  onConfigureIdentity: () => void;
}) {
  const { t } = useTranslation();
  const isPublic = kind === "public";
  const kindLabel = isPublic ? t("agentAccess.publicMCP") : t("settings.privateMCP");
  const connectLabel = t("agentAccess.connectYourAgentToName", { name: tenantName });
  const previewLabel = isPublic ? `[${t("agentAccess.public")}] ${connectLabel}` : connectLabel;
  return <article className={`agent-setup-card ${!setup.available ? "agent-setup-disabled" : ""}`}>
    <div className={`agent-setup-preview ${isPublic ? "public-agent-preview" : "private-agent-preview"}`}><a href={setup.available ? setup.url : undefined} target="_blank" rel="noopener noreferrer" aria-disabled={!setup.available} onClick={(event) => { if (!setup.available) event.preventDefault(); }}><span className="agent-setup-label">{previewLabel}</span>{agentClients.map((client) => <Image key={client.id} className="agent-client-mark" src={`/agent-client-icons/${client.file}`} alt={client.name} width={20} height={20} />)}</a></div>
    <div className="agent-setup-copy">
      <div className="agent-setup-card-heading"><span className="agent-setup-kind"><Badge color={isPublic ? "blue" : "violet"}>{isPublic ? <Globe2 /> : <LockKeyhole />}{isPublic ? t("agentAccess.public") : t("agentAccess.private")}</Badge><h2 aria-label={`${kindLabel}: ${t("agentAccess.mcpButton")}`}>{t("agentAccess.mcpButton")}</h2></span><span className="agent-setup-state"><Badge color={setup.available ? "green" : "zinc"}>{isPublic ? enabled ? t("agentAccess.live") : t("agentAccess.off") : setup.available ? t("agentAccess.active") : t("agentAccess.off")}</Badge>{isPublic && onEnabledChange && <Switch checked={enabled} onChange={onEnabledChange} label={t("agentAccess.enablePublicMCP")} />}</span></div>
      <div className="agent-setup-description-slot"><p className="agent-setup-description">{isPublic ? t("agentAccess.offerAnAuthenticationFreeReadOnlyMCPEndpointContaining") : t("agentAccess.offerAnAuthenticatedMCPEndpointWithPrivateResourcesAndCustomerScopedAccess")}</p></div>
      <div className="endpoint agent-setup-endpoint"><code>{endpoint}</code><button type="button" aria-label={`${kindLabel}: ${t("agentAccess.copy")}`} onClick={() => { void navigator.clipboard.writeText(endpoint); onCopied(`${kindLabel}: ${t("queryLab.copied")}`); }}><Copy />{t("agentAccess.copy")}</button></div>
      <CopyButton text={setup.embed_code} label={t("agentAccess.copyMCPButton", { kind: isPublic ? t("agentAccess.public") : t("agentAccess.private") })} disabled={!setup.available} onCopied={() => onCopied(isPublic ? t("agentAccess.publicMCPButtonCopied") : t("agentAccess.privateMCPButtonCopied"))} />
      <div className="agent-setup-guide-slot" aria-hidden={!setup.available && isPublic}>{setup.available ? <a className="agent-setup-guide-link" href={setup.url} target="_blank" rel="noopener noreferrer"><ExternalLink />{t("agentAccess.openSetupInstructions")}</a> : !isPublic && <><div className="inline-warning"><TriangleAlert />{t("agentAccess.configureCustomerIdentityFirst")}</div><Button outline className="agent-identity-action" onClick={onConfigureIdentity}>{t("agentAccess.configureIdentity")}</Button></>}</div>
    </div>
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
