import { useTranslation } from "react-i18next";
import {
  ArrowLeft, BookOpen, Check, CheckCircle2, ChevronRight,
  GitBranch, Plus, RefreshCw, Search,
  ShieldCheck, TriangleAlert, XCircle,
} from "lucide-react";
import { useEffect, useState } from "react";

import {
  APIError, APIIdentity, APIIntegration, APIIntegrationAnalysis, APIProduct,
  APIIntegrationPublishStatus, APIIntegrationRevision, APIResourceSet, APISourcePublication,
  APIRuntimeAuthenticationType, APIRuntimeCredentialSet, APITool, Distribution, api,
} from "../../lib/api";
import { developerAssetsApi } from "../../lib/developer-assets-api";
import {
  IntegrationResourceTab, IntegrationTab,
  integrationPath, integrationValidationPath, sectionPath,
} from "../../lib/console-routes";
import { Badge, Button, Dialog } from "../core/control";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader as PageHeading, PanelHeader } from "../core/layout";
import { IntegrationNavigation } from "../integrations/IntegrationNavigation";
import { IntegrationQuickStart } from "../integrations/IntegrationQuickStart";
import { IntegrationAuthorization } from "../integrations/IntegrationAuthorization";
import { AuthorizationHeaderManager, authorizationHeaderDraft } from "../integrations/AuthorizationHeaderManager";
import type { AuthorizationHeaderDraft } from "../integrations/AuthorizationHeaderManager";
import {
  ConsoleLink, DocumentationAttachmentResult, Source, apiFamilyKeyFromName,
} from "./shared";
import { AuthorizationPolicyWorkspace } from "./integrations/authorization-policy-workspace";
import { IntegrationTestWorkspace } from "./integrations/test-workspace";
import { IntegrationToolsWorkspace } from "./integrations/tools-workspace";
import { APIResourcesWorkspace } from "./developer-assets/api-resources-workspace";
import { APIResourcePublicationHistory } from "./developer-assets/api-resource-publication-history";

function apiContractSlugFromIdentity(name: string, version: string) {
  const versionSlug = version.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 20) || "v1";
  const family = apiFamilyKeyFromName(name).slice(0, 62 - versionSlug.length).replace(/-+$/g, "") || "api";
  return `${family}-${versionSlug}`;
}

function authorizationEnvironmentVariableFromName(name: string) {
  const family = apiFamilyKeyFromName(name).toUpperCase().replaceAll("-", "_").replace(/_API$/g, "");
  return `${family || "SERVICE"}_API_KEY`;
}

function localDevelopmentHostname(hostname: string) {
  const normalized = hostname.toLowerCase().replace(/^\[|\]$/g, "");
  return normalized === "localhost" || normalized.endsWith(".localhost") || normalized === "::1" || /^127(?:\.\d{1,3}){3}$/.test(normalized);
}

function validConfiguratorURL(value: string, kind: "service" | "management") {
  try {
    const parsed = new URL(value);
    if (parsed.username || parsed.password || parsed.search || parsed.hash || !parsed.hostname) return false;
    const local = localDevelopmentHostname(parsed.hostname);
    if (parsed.protocol !== "https:" && !(parsed.protocol === "http:" && local)) return false;
    if (kind === "service" && (local ? parsed.protocol !== "http:" : parsed.protocol !== "https:" || parsed.port !== "" && parsed.port !== "443")) return false;
    return true;
  } catch {
    return false;
  }
}

function validAuthorizationHeaderName(value: string) {
  if (!/^[!#$%&'*+.^_`|~0-9A-Za-z-]{1,100}$/.test(value)) return false;
  return !new Set([
    "authorization", "proxy-authorization", "cookie", "set-cookie", "host", "content-length", "transfer-encoding", "connection", "upgrade", "te", "trailer", "forwarded",
    "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto", "x-forwarded-uri", "x-http-method", "x-http-method-override", "x-method-override", "x-original-url", "x-original-uri", "x-rewrite-url", "x-envoy-original-path",
  ]).has(value.toLowerCase());
}

function IntegrationDirectoryView({ integrations, query, onQueryChange, onCreate, onNavigate }: { integrations: APIIntegration[]; query: string; onQueryChange: (query: string) => void; onCreate: () => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const [showRetired, setShowRetired] = useState(false);
  const normalizedQuery = query.trim().toLowerCase();
  const retiredCount = integrations.filter((integration) => integration.lifecycle === "retired").length;
  const setupIssueCount = (integration: APIIntegration) => Number((integration.resources?.length ?? 0) === 0);
  const filteredIntegrations = integrations
    .filter((integration) => showRetired || integration.lifecycle !== "retired")
    .filter((integration) => !normalizedQuery || [integration.display_name, integration.family_key, integration.version_key, integration.description].some((value) => value.toLowerCase().includes(normalizedQuery)))
    .sort((left, right) => left.display_name.localeCompare(right.display_name) || left.version_key.localeCompare(right.version_key, undefined, { numeric: true }));

  return <>
    <PageHeading eyebrow={t("navigation.apis")} title={t("integrations.apiDirectory")} action={<Button onClick={onCreate}><Plus data-slot="icon" />{t("integrations.addAPI")}</Button>} />
    <div className="toolbar integration-toolbar">
      <div className="search-field"><Search /><input aria-label={t("integrations.searchAPIs")} placeholder={t("integrations.searchAPIs2")} value={query} onChange={(event) => onQueryChange(event.target.value)} /></div>
      <span className="toolbar-count">{filteredIntegrations.length} API{filteredIntegrations.length === 1 ? "" : t("integrations.s")}</span>
    </div>
    <div className="integration-directory-wrap">
      <DataTable label={t("navigation.apis")} className="integration-directory">
        <DataTableHeader className="integration-directory-columns"><span>API</span><span>{t("integrations.lifecycle")}</span><span>{t("integrations.setup")}</span><span>{t("integrations.resources")}</span><span>{t("integrations.open")}</span></DataTableHeader>
        {filteredIntegrations.map((integration) => { const issues = setupIssueCount(integration); return <DataTableRow key={integration.id} className="integration-directory-columns integration-directory-row">
          <span className="resource-name"><span className="resource-icon"><GitBranch /></span><span><ConsoleLink path={integrationPath(integration.id)} onNavigate={onNavigate} className="entity-link"><strong>{integration.display_name}</strong></ConsoleLink><small>{integration.version_key}</small></span></span>
          <span><Badge color={integration.lifecycle === "active" ? "green" : integration.lifecycle === "deprecated" ? "amber" : "zinc"}>{integration.lifecycle}</Badge> <Badge color={integration.visibility === "public" ? "blue" : "zinc"}>{integration.visibility}</Badge></span>
          <span><Badge color={issues === 0 ? "green" : "amber"}>{issues === 0 ? t("integrations.ready") : t("integrations.stepsLeft", { count: issues })}</Badge></span>
          <span><strong className="cell-value">{integration.resources?.length ?? 0}</strong><small className="cell-note">{t("integrations.attachedSets")}</small></span>
          <span className="table-open-cell"><ConsoleLink path={integrationPath(integration.id)} onNavigate={onNavigate} className="row-arrow" ariaLabel={`Open ${integration.display_name}`}><ChevronRight /></ConsoleLink></span>
        </DataTableRow>; })}
        {filteredIntegrations.length === 0 && <DataTableEmpty columns={5}>{integrations.length === 0 ? t("integrations.noAPIsYetAddOneManuallyOrImportYour") : retiredCount === integrations.length && !showRetired && !normalizedQuery ? t("integrations.noCurrentAPIsShowRetiredAPIsToViewThe") : t("integrations.noAPIsMatchThisSearch")}</DataTableEmpty>}
      </DataTable>
      {retiredCount > 0 && <button type="button" className="retired-directory-toggle" aria-pressed={showRetired} onClick={() => setShowRetired((visible) => !visible)}>{showRetired ? t("integrations.hideRetired") : t("integrations.showRetired", { retiredCount: String(retiredCount) })}</button>}
    </div>
  </>;
}

type IntegrationWorkspaceViewProps = {
  integration: APIIntegration | null;
  analyses: APIIntegrationAnalysis[];
  tools: APITool[];
  activeTab: IntegrationTab;
  activeResourceTab: IntegrationResourceTab;
  live: boolean;
  loading: boolean;
  revisions: APIIntegrationRevision[];
  publishStatus: APIIntegrationPublishStatus | null;
  identity: APIIdentity | null;
  resourceSets: APIResourceSet[];
  sources: Source[];
  distribution: Distribution | null;
  busy: boolean;
  onEdit: (integration: APIIntegration) => void;
  onPublish: (integration: APIIntegration) => void;
  onAttach: (integration: APIIntegration, kind?: APIResourceSet["kind"]) => void;
  onCreateResource: () => void;
  onAddSource: () => void;
  onCrawlSource: (sourceID: string) => void;
  onPublishSource: (source: Source, attachIntegrationID?: string) => void;
  onAttachPublishedSource: (integration: APIIntegration, source: Source) => Promise<void>;
  onGenerateSetupGuide: (integrationID: string) => Promise<APIIntegrationAnalysis>;
  onEditResource: (resource: APIResourceSet) => void;
  onDuplicateResource: (resource: APIResourceSet) => void;
  onDetachResource: (integrationID: string, resourceSetID: string) => void;
  onInspectRevision: (revision: APIIntegrationRevision) => void;
  onRuntimeChanged: () => void | Promise<void>;
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
};

function IntegrationWorkspaceView({ integration, tools, activeTab, loading, revisions, publishStatus, identity, distribution, busy, onEdit, onPublish, onInspectRevision, onRuntimeChanged, onMessage, onNavigate, live }: IntegrationWorkspaceViewProps) {
  const { t } = useTranslation();
  if (loading && !integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><RefreshCw /></span><div><p className="eyebrow">API</p><h1>{t("integrations.loadingAPI")}</h1><p>{t("integrations.retrievingItsConfigurationAndPublishedHistory")}</p></div></section>;
  if (!integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><p className="eyebrow">API</p><h1>{t("integrations.apiUnavailable")}</h1><p>{t("integrations.thisAPIIsNotAvailableInTheCurrentDeployment")}</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{t("integrations.returnToAPIs")}</ConsoleLink></section>;

  const attachedResources = integration.resources ?? [];
  const documentationResources = attachedResources.filter((resource) => resource.kind === "documentation");
  const contractResources = attachedResources.filter((resource) => resource.kind === "api");
  const sortedRevisions = [...revisions].sort((left, right) => right.revision - left.revision);
  const publishValidationCodes = new Set(publishStatus?.validations.map((validation) => validation.code) ?? []);
  const setupSteps: Array<{ label: string; detail: string; ready: boolean; path: string }> = [
    { label: "Connect Authorization", detail: "Bind one reusable Authorization to this API.", ready: Boolean(publishStatus && !publishValidationCodes.has("access_missing")), path: integrationPath(integration.id, "authorization") },
    { label: t("integrations.addTrustedDocumentation"), detail: t("integrations.addTrustedDocumentationDetail"), ready: documentationResources.length > 0, path: integrationPath(integration.id, "documentation") },
    { label: t("integrations.attachAPIContract"), detail: t("integrations.attachAPIContractDetail"), ready: contractResources.length > 0, path: integrationPath(integration.id, "documentation") },
    { label: t("integrations.configureCustomerAccess"), detail: t("integrations.configureCustomerAccessDetail"), ready: Boolean(identity?.configured && identity.state === "active" && publishStatus && !publishValidationCodes.has("authorization_missing")), path: integrationPath(integration.id, "tools") },
    { label: t("integrations.exposeTools"), detail: t("integrations.exposeToolsDetail"), ready: Boolean(publishStatus && !publishValidationCodes.has("tools_missing")), path: integrationPath(integration.id, "tools") },
    { label: t("integrations.validateConfiguration"), detail: t("integrations.validateConfigurationDetail"), ready: Boolean(publishStatus?.ready), path: integrationPath(integration.id, "test") },
  ];
  const setupValidationCodes = new Set(["resources_missing", "authorization_missing", "tools_missing", "access_missing", "support_inherited"]);
  const actionableValidations = publishStatus?.validations.filter((validation) => !setupValidationCodes.has(validation.code)) ?? [];
  const hasChanges = Boolean(publishStatus?.has_changes);
  const canPublish = Boolean(publishStatus?.ready && hasChanges);
  const setupComplete = setupSteps.filter((step) => step.ready).length;
  const validationPath = (tab: string) => integrationValidationPath(integration.id, tab);
  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{t("integrations.allAPIs")}</ConsoleLink></div>
    <PageHeading eyebrow={t("integrations.copy", { family_key: String(integration.family_key), version_key: String(integration.version_key) })} title={integration.display_name} action={<span className="heading-actions"><Button outline onClick={() => onEdit(integration)}>{t("integrations.edit")}</Button>{!publishStatus ? <span className="published-state checking"><RefreshCw />{t("integrations.checking")}</span> : canPublish ? <Button color="indigo" disabled={busy} onClick={() => onPublish(integration)}><GitBranch data-slot="icon" />{t("integrations.publish")}</Button> : hasChanges && !publishStatus.ready ? <Badge color="amber">{t("integrations.setupRequired")}</Badge> : <span className="published-state"><CheckCircle2 />{t("integrations.published")}</span>}</span>} />
    <IntegrationNavigation integrationID={integration.id} integrationName={integration.display_name} activeTab={activeTab} onNavigate={onNavigate} />

    {activeTab === "overview" && <IntegrationQuickStart
      lifecycle={integration.lifecycle}
      status={!publishStatus ? "checking" : publishStatus.ready ? hasChanges ? "ready" : "published" : "setup"}
      statusDetail={!publishStatus ? t("integrations.loadingLatestPublicationState") : hasChanges ? t("integrations.setupStepsWithUnpublishedChanges", { count: publishStatus.changes.length, completed: setupComplete, total: setupSteps.length }) : t("integrations.setupStepsReady", { completed: setupComplete, total: setupSteps.length })}
      steps={setupSteps}
      validations={actionableValidations.map((validation) => ({ ...validation, path: validationPath(validation.tab) }))}
      onNavigate={onNavigate}
      advanced={<>
        <div className="integration-overview-grid"><ConsoleLink path={integrationPath(integration.id, "documentation")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><BookOpen /></span><span><strong>{t("integrations.resources")}</strong><small>{t("integrations.exactDocumentationAPIContractAndSDKAttachments")}</small></span><ChevronRight /></ConsoleLink><ConsoleLink path={sectionPath("identity")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><ShieldCheck /></span><span><strong>{t("integrations.customerIdentity")}</strong><small>{identity?.configured && identity.state === "active" ? t("integrations.oidcCustomerSignInActive") : identity?.configured ? t("integrations.oidcDraftNotActive") : t("integrations.oidcNotConfigured")}</small></span><ChevronRight /></ConsoleLink></div>
        <section className="panel"><PanelHeader title={t("integrations.apiDetails")} /><dl className="entity-detail-grid"><div><dt>{t("integrations.apiID")}</dt><dd>{integration.id}</dd></div><div><dt>{t("integrations.family")}</dt><dd>{integration.family_key}</dd></div><div><dt>{t("integrations.version")}</dt><dd>{integration.version_key}</dd></div><div><dt>{t("integrations.draftRevision")}</dt><dd>{integration.revision}</dd></div><div><dt>{t("integrations.replacement")}</dt><dd>{integration.replacement_integration_id ?? "—"}</dd></div><div><dt>{t("integrations.sunset")}</dt><dd>{integration.sunset_at ? t("format.date", { value: new Date(integration.sunset_at) }) : "—"}</dd></div></dl></section>
      </>}
    />}

    {activeTab === "documentation" && <div className="integration-tab-content"><APIResourcesWorkspace integration={integration} live={live} onMessage={onMessage} onNavigate={onNavigate} /></div>}

    {activeTab === "authorization" && <div className="integration-tab-content">
      <IntegrationAuthorization integration={integration} key={integration.id} onMessage={onMessage} onChanged={onRuntimeChanged} />
    </div>}

    {activeTab === "tools" && <div className="integration-tab-content"><IntegrationToolsWorkspace integration={integration} tools={tools} onMessage={onMessage} onNavigate={onNavigate} /><AuthorizationPolicyWorkspace integration={integration} onMessage={onMessage} /></div>}

    {activeTab === "test" && <IntegrationTestWorkspace key={`${integration.id}:${publishStatus?.current_manifest_hash ?? ""}`} integration={integration} distribution={distribution} onNavigate={onNavigate} />}

    {activeTab === "history" && <div className="integration-tab-content"><div className="notice"><GitBranch /><span><strong>{t("integrations.publishedHistoryIsImmutable")}</strong> {t("integrations.eachEntryPreservesTheExactDocumentationSDKsAccessAnd")}</span></div><section className="panel"><PanelHeader title={t("integrations.publishedHistory")} />{sortedRevisions.map((revision) => <button type="button" className="integration-revision-row" key={revision.id} onClick={() => onInspectRevision(revision)}><span className="revision-number">r{revision.revision}</span><span><strong>{revision.state}</strong><small>{revision.published_at || revision.created_at ? t("format.dateTime", { value: new Date(revision.published_at ?? revision.created_at) }) : t("integrations.dateUnavailable")}</small></span><ChevronRight /></button>)}{sortedRevisions.length === 0 && <div className="empty-row">{t("integrations.nothingHasBeenPublishedYet")}</div>}</section><APIResourcePublicationHistory integrationID={integration.id} live={live} onMessage={onMessage} /></div>}
  </>;
}

type IntegrationsViewProps = {
  live?: boolean;
  product: APIProduct;
  integrations: APIIntegration[];
  analyses: APIIntegrationAnalysis[];
  tools: APITool[];
  resourceSets: APIResourceSet[];
  sources: Source[];
  identity: APIIdentity | null;
  distribution: Distribution | null;
  selectedIntegrationID?: string;
  activeTab?: IntegrationTab;
  activeResourceTab?: IntegrationResourceTab;
  onAddSource: () => void;
  onCrawlSource: (sourceID: string) => void;
  onPublishSource: (source: Source, attachIntegrationID?: string) => void;
  onAttachPublishedSource: (integrationID: string, source: Source, publication: APISourcePublication) => Promise<DocumentationAttachmentResult>;
  onGenerateSetupGuide: (integrationID: string) => Promise<APIIntegrationAnalysis>;
  onChanged: () => Promise<void>;
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
};

export function IntegrationsView({ live = true, product, integrations, analyses, tools, resourceSets, sources, identity, distribution, selectedIntegrationID, activeTab = "overview", activeResourceTab = "documentation", onAddSource, onCrawlSource, onPublishSource, onAttachPublishedSource, onGenerateSetupGuide, onChanged, onMessage, onNavigate }: IntegrationsViewProps) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [selectedDetail, setSelectedDetail] = useState<APIIntegration | null>(null);
  const [selectedRevisions, setSelectedRevisions] = useState<APIIntegrationRevision[]>([]);
  const [selectedPublishStatus, setSelectedPublishStatus] = useState<APIIntegrationPublishStatus | null>(null);
  const [loadedIntegrationID, setLoadedIntegrationID] = useState("");
  const [integrationOpen, setIntegrationOpen] = useState(false);
  const [integrationStep, setIntegrationStep] = useState<1 | 2 | 3>(1);
  const [editingIntegration, setEditingIntegration] = useState<APIIntegration | null>(null);
  const [familyKey, setFamilyKey] = useState("");
  const [versionKey, setVersionKey] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");
  const [openAPIFile, setOpenAPIFile] = useState<File | null>(null);
  const [openAPIFileError, setOpenAPIFileError] = useState("");
  const [serviceURL, setServiceURL] = useState("");
  const [deploymentEnvironments, setDeploymentEnvironments] = useState<Array<{ id: string; name: string; is_production: boolean }>>([]);
  const [authorizationMode, setAuthorizationMode] = useState<"existing" | "new">("new");
  const [authorizationProfiles, setAuthorizationProfiles] = useState<APIRuntimeCredentialSet[]>([]);
  const [authorizationProfileID, setAuthorizationProfileID] = useState("");
  const [authorizationType, setAuthorizationType] = useState<Exclude<APIRuntimeAuthenticationType, "none" | "delegated_oauth">>("api_key_header");
  const [keyManagementURL, setKeyManagementURL] = useState("");
  const [accessEvaluationURL, setAccessEvaluationURL] = useState("");
  const [usageURL, setUsageURL] = useState("");
  const [environmentVariable, setEnvironmentVariable] = useState("API_KEY_ENV");
  const [environmentVariableTouched, setEnvironmentVariableTouched] = useState(false);
  const [authorizationCredential, setAuthorizationCredential] = useState("");
  const [authorizationHeaders, setAuthorizationHeaders] = useState<AuthorizationHeaderDraft[]>(() => [authorizationHeaderDraft("X-API-Key")]);
  const [authorizationPrefix, setAuthorizationPrefix] = useState("");
  const [basicUsername, setBasicUsername] = useState("");
  const [oauthClientID, setOAuthClientID] = useState("");
  const [oauthTokenURL, setOAuthTokenURL] = useState("");
  const [oauthScopes, setOAuthScopes] = useState("");
  const [integrationVisibility, setIntegrationVisibility] = useState<APIIntegration["visibility"]>("private");
  const [integrationPublicAcknowledged, setIntegrationPublicAcknowledged] = useState(false);
  const [lifecycle, setLifecycle] = useState<APIIntegration["lifecycle"]>("draft");
  const [replacementID, setReplacementID] = useState("");
  const [sunsetAt, setSunsetAt] = useState("");
  const [resourceOpen, setResourceOpen] = useState(false);
  const [editingSet, setEditingSet] = useState<APIResourceSet | null>(null);
  const [setKind, setSetKind] = useState<APIResourceSet["kind"]>("documentation");
  const [setName, setSetName] = useState("");
  const [resourceDescription, setResourceDescription] = useState("");
  const [setManifest, setSetManifest] = useState("[]");
  const [selectedSourcePublicationIDs, setSelectedSourcePublicationIDs] = useState<string[]>([]);
  const [duplicateSet, setDuplicateSet] = useState<APIResourceSet | null>(null);
  const [duplicateName, setDuplicateName] = useState("");
  const [attachIntegration, setAttachIntegration] = useState<APIIntegration | null>(null);
  const [attachSetID, setAttachSetID] = useState("");
  const [attachKind, setAttachKind] = useState<APIResourceSet["kind"] | "">("");
  const [pinAttachedSet, setPinAttachedSet] = useState(false);
  const [publishCandidate, setPublishCandidate] = useState<APIIntegration | null>(null);
  const [inspectedRevision, setInspectedRevision] = useState<APIIntegrationRevision | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!selectedIntegrationID) return;
    let cancelled = false;
    api.integration(selectedIntegrationID).then((value) => {
      if (cancelled) return;
      setSelectedDetail(value.integration);
      setSelectedRevisions(value.revisions);
      setSelectedPublishStatus(value.publish_status);
      setLoadedIntegrationID(selectedIntegrationID);
    }).catch(() => {
      if (cancelled) return;
      setSelectedDetail(null);
      setSelectedRevisions([]);
      setSelectedPublishStatus(null);
      setLoadedIntegrationID(selectedIntegrationID);
    });
    return () => { cancelled = true; };
  }, [selectedIntegrationID]);

  async function refreshSelectedIntegration(integrationID = selectedIntegrationID) {
    if (!integrationID) return;
    try {
      const value = await api.integration(integrationID);
      setSelectedDetail(value.integration);
      setSelectedRevisions(value.revisions);
      setSelectedPublishStatus(value.publish_status);
      setLoadedIntegrationID(integrationID);
    } catch {
      setSelectedDetail(null);
      setSelectedRevisions([]);
      setSelectedPublishStatus(null);
      setLoadedIntegrationID(integrationID);
    }
  }

  async function openIntegration(value?: APIIntegration) {
    setEditingIntegration(value ?? null);
    setFamilyKey(value?.family_key ?? ""); setVersionKey(value?.version_key ?? "v1"); setDisplayName(value?.display_name ?? ""); setDescription(value?.description ?? ""); setIntegrationVisibility(value?.visibility ?? "private"); setIntegrationPublicAcknowledged(false); setLifecycle(value?.lifecycle ?? "draft"); setReplacementID(value?.replacement_integration_id ?? ""); setSunsetAt(value?.sunset_at?.slice(0, 10) ?? "");
    setIntegrationStep(1);
    setOpenAPIFile(null); setOpenAPIFileError(""); setServiceURL("");
    setAuthorizationMode("new"); setAuthorizationProfileID(""); setAuthorizationType("api_key_header"); setKeyManagementURL(""); setAccessEvaluationURL(""); setUsageURL(""); setEnvironmentVariable("API_KEY_ENV"); setEnvironmentVariableTouched(false); setAuthorizationCredential(""); setAuthorizationHeaders([authorizationHeaderDraft("X-API-Key")]); setAuthorizationPrefix(""); setBasicUsername(""); setOAuthClientID(""); setOAuthTokenURL(""); setOAuthScopes("");
    setIntegrationOpen(true);
    if (!value && live) {
      try {
        const [profiles, environments] = await Promise.all([api.authorizations(), api.deploymentEnvironments()]);
        setDeploymentEnvironments(environments);
        const environment = environments.find((candidate) => candidate.is_production) ?? environments[0];
        const reusable = environment ? profiles.filter((profile) => profile.environment_id === environment.id) : [];
        setAuthorizationProfiles(reusable);
        if (reusable.length > 0) {
          setAuthorizationMode("existing");
          setAuthorizationProfileID(reusable[0].id);
        }
      } catch (error) {
        setAuthorizationProfiles([]);
        onMessage(error instanceof APIError ? error.message : t("integrations.apiCouldNotBeSaved"));
      }
    }
  }

  function validateOpenAPIFile(file: File | null) {
    if (!file) return "Choose an OpenAPI JSON or YAML file.";
    if (!/\.(json|ya?ml)$/i.test(file.name)) return "OpenAPI must be a .json, .yaml, or .yml file.";
    if (file.size > 5_000_000) return "OpenAPI must be smaller than 5 MB.";
    return "";
  }

  function newAuthorizationConfig(): Record<string, unknown> {
    switch (authorizationType) {
      case "basic": return { username: basicUsername.trim() };
      case "oauth_client_credentials": return {
        client_id: oauthClientID.trim(), token_url: oauthTokenURL.trim(), token_endpoint_auth_method: "client_secret_basic",
        scopes: oauthScopes.split(/[\s,]+/).map((value) => value.trim()).filter(Boolean),
      };
      case "custom_header": return authorizationPrefix.trim() ? { prefix: authorizationPrefix.trim() } : {};
      default: return {};
    }
  }

  function openAPIStepReady() {
    if (!displayName.trim() || displayName.trim().length > 120 || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(versionKey.trim())) return false;
    return !validateOpenAPIFile(openAPIFile) && validConfiguratorURL(serviceURL.trim(), "service");
  }

  function authorizationStepReady() {
    if (authorizationMode === "existing") return Boolean(authorizationProfiles.some((profile) => profile.id === authorizationProfileID));
    if (!validConfiguratorURL(keyManagementURL.trim(), "management") || !validConfiguratorURL(accessEvaluationURL.trim(), "service") || !validConfiguratorURL(usageURL.trim(), "service") || !/^[A-Z][A-Z0-9_]{0,127}$/.test(environmentVariable.trim())) return false;
    const headerAuthentication = authorizationType === "api_key_header" || authorizationType === "custom_header";
    if (!headerAuthentication && (!authorizationCredential.trim() || authorizationCredential.length > 16384 || /[\r\n\0]/.test(authorizationCredential))) return false;
    if (headerAuthentication && authorizationHeaders.length === 0) return false;
    const seenHeaders = new Set<string>();
    for (const header of authorizationHeaders) {
      const name = header.name.trim();
      const key = name.toLowerCase();
      if (!validAuthorizationHeaderName(name) || seenHeaders.has(key) || !header.value || header.value.length > 16384 || /[\r\n\0]/.test(header.value)) return false;
      seenHeaders.add(key);
    }
    if (authorizationType === "custom_header" && (authorizationPrefix.length > 64 || /[\r\n\0]/.test(authorizationPrefix))) return false;
    if (authorizationType === "basic" && (!basicUsername.trim() || basicUsername.trim().length > 255 || /[:\r\n\0]/.test(basicUsername))) return false;
    if (authorizationType !== "oauth_client_credentials") return true;
    const scopes = oauthScopes.split(/[\s,]+/).map((value) => value.trim()).filter(Boolean);
    return Boolean(oauthClientID.trim() && oauthClientID.trim().length <= 255 && !/[\r\n\0]/.test(oauthClientID) && validConfiguratorURL(oauthTokenURL.trim(), "service") && scopes.length <= 32 && scopes.every((scope) => scope.length <= 200 && !/[\r\n\0]/.test(scope)));
  }

  async function continueIntegrationConfigurator() {
    if (integrationStep === 1) {
      const fileError = validateOpenAPIFile(openAPIFile);
      setOpenAPIFileError(fileError);
      if (!openAPIStepReady()) return;
      if (!validConfiguratorURL(serviceURL.trim(), "service")) {
        setOpenAPIFileError("Use a credential-free public HTTPS base URL, or HTTP localhost for development.");
        return;
      }
      if (deploymentEnvironments.length === 0) {
        setOpenAPIFileError("Create a deployment environment before adding an API.");
        return;
      }
      try {
        new TextDecoder("utf-8", { fatal: true }).decode(await openAPIFile!.arrayBuffer());
      } catch {
        setOpenAPIFileError("OpenAPI must be valid UTF-8 text.");
        return;
      }
      setIntegrationStep(2);
      return;
    }
    if (integrationStep === 2 && authorizationStepReady()) {
      if (authorizationMode === "new") {
        if (!validConfiguratorURL(keyManagementURL.trim(), "management") || !validConfiguratorURL(accessEvaluationURL.trim(), "service") || !validConfiguratorURL(usageURL.trim(), "service") || (authorizationType === "oauth_client_credentials" && !validConfiguratorURL(oauthTokenURL.trim(), "service"))) {
          onMessage("Enter complete HTTPS URLs for Authorization management, access evaluation, usage delivery, and OAuth token exchange when applicable.");
          return;
        }
      }
      setIntegrationStep(3);
    }
  }

  async function saveIntegration() {
    setBusy(true);
    let savedDraft: APIIntegration | null = null;
    let failedStage = "API draft creation";
    try {
      const base = { family_key: editingIntegration ? familyKey : apiFamilyKeyFromName(displayName), version_key: versionKey, display_name: displayName, description: editingIntegration ? description : "", visibility: editingIntegration ? integrationVisibility : "private" as const, acknowledge_public: editingIntegration ? integrationPublicAcknowledged : false, lifecycle: editingIntegration ? lifecycle : "draft" as const };
      const saved = editingIntegration
        ? await api.updateIntegration(editingIntegration.id, { ...base, replacement_integration_id: replacementID || undefined, sunset_at: sunsetAt ? new Date(`${sunsetAt}T00:00:00Z`).toISOString() : undefined, revision: editingIntegration.revision })
        : await api.createIntegration(base);
      savedDraft = saved;
      if (!editingIntegration) {
        if (!openAPIFile) throw new Error("OpenAPI file is required.");
        const environment = deploymentEnvironments.find((value) => value.is_production) ?? deploymentEnvironments[0];
        if (!environment) throw new Error("A deployment environment is required.");
        failedStage = "OpenAPI upload";
        const source = await api.uploadSource(product.id, product.organisation_id, openAPIFile, `${displayName.trim()} ${versionKey.trim()} OpenAPI`);
        failedStage = "OpenAPI contract registration";
        const contract = await developerAssetsApi.createAPIContract({ name: `${displayName.trim()} ${versionKey.trim()}`, slug: apiContractSlugFromIdentity(displayName, versionKey), description: `OpenAPI contract for ${displayName.trim()} ${versionKey.trim()}.`, visibility: "private", lifecycle: "active" });
        await developerAssetsApi.attachAPIContractSource(contract.id, source.id, "primary");
        failedStage = "OpenAPI ingestion queue";
        await api.queueCrawl(product.id, source.id);
        const selectedProfile = authorizationProfiles.find((profile) => profile.id === authorizationProfileID);
        failedStage = "Authorization connection";
        const headerAuthentication = authorizationType === "api_key_header" || authorizationType === "custom_header";
        const primaryHeader = headerAuthentication ? authorizationHeaders[0] : undefined;
        const additionalHeaders = headerAuthentication ? authorizationHeaders.slice(1) : authorizationHeaders;
        await api.configureIntegrationAuthorization(saved.id, authorizationMode === "existing" && selectedProfile ? {
          environment_id: environment.id,
          connection_name: "Default",
          base_url: serviceURL.trim(),
          authentication_type: selectedProfile.authentication_type,
          authorization_id: selectedProfile.id,
        } : {
          environment_id: environment.id,
          connection_name: "Default",
          base_url: serviceURL.trim(),
          authentication_type: authorizationType,
          auth_config: newAuthorizationConfig(),
          environment_variable: environmentVariable.trim(),
          header_name: primaryHeader?.name.trim(),
          key_management_url: keyManagementURL.trim(),
          access_evaluation_url: accessEvaluationURL.trim(),
          usage_url: usageURL.trim(),
          credential: primaryHeader?.value ?? authorizationCredential,
          additional_headers: additionalHeaders.map((header) => ({ name: header.name.trim(), value: header.value })),
        });
      }
      setSelectedDetail(saved);
      await onChanged();
      if (editingIntegration) await refreshSelectedIntegration(saved.id);
      setIntegrationOpen(false);
      onMessage(editingIntegration ? t("integrations.apiUpdated") : "API created. OpenAPI ingestion is queued for review and Authorization is connected.");
      if (!editingIntegration) onNavigate(integrationPath(saved.id));
    } catch (error) {
      const message = error instanceof APIError || error instanceof Error ? error.message : t("integrations.apiCouldNotBeSaved");
      if (!editingIntegration && savedDraft) {
        await onChanged().catch(() => undefined);
        setSelectedDetail(savedDraft);
        setIntegrationOpen(false);
        onNavigate(integrationPath(savedDraft.id));
        onMessage(`API draft created, but ${failedStage} failed: ${message}. Finish that step from the API workspace; do not create a duplicate API.`);
      } else {
        onMessage(message);
      }
    } finally { setBusy(false); }
  }

  async function publishIntegration() {
    if (!publishCandidate) return;
    setBusy(true);
    try {
      const preflight = await api.preflightIntegration(publishCandidate.id);
      if (!preflight.ready) {
        const failed = preflight.checks.find((check) => check.required && check.status !== "pass");
        throw new Error(failed?.message ?? t("integrations.theServerPreflightFoundARequiredConfigurationGap"));
      }
      await api.publishIntegration(publishCandidate.id, preflight.candidate_revision, preflight.candidate_manifest_hash);
      await onChanged();
      await refreshSelectedIntegration(publishCandidate.id);
      setPublishCandidate(null);
      onMessage(t("integrations.apiPublishedFromTheExactPreflightCandidate"));
    } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : t("integrations.apiCouldNotBePublished")); } finally { setBusy(false); }
  }

  function openResource(value?: APIResourceSet) {
    const manifest = value?.latest_revision?.manifest ?? [];
    setEditingSet(value ?? null); setSetKind(value?.kind ?? "documentation"); setSetName(value?.name ?? ""); setResourceDescription(value?.description ?? ""); setSetManifest(JSON.stringify(manifest, null, 2)); setSelectedSourcePublicationIDs(manifest.map((entry) => typeof entry.source_publication_id === "string" ? entry.source_publication_id : "").filter(Boolean)); setResourceOpen(true);
  }

  async function saveResourceSet() {
    setBusy(true);
    try {
      const latestPublicationEntries = sources.flatMap((source) => source.latestPublication ? [{ source_publication_id: source.latestPublication.id, source_id: source.id, revision: source.latestPublication.revision, content_hash: source.latestPublication.content_hash, name: source.name }] : []);
      const parsedManifest = JSON.parse(setManifest) as unknown;
      if (!Array.isArray(parsedManifest)) throw new Error(t("integrations.manifestMustBeAJSONArray"));
      const existingEntries = parsedManifest as Array<Record<string, unknown>>;
      const options = new Map([...existingEntries, ...latestPublicationEntries].map((entry) => [String(entry.source_publication_id ?? ""), entry]));
      const manifest = setKind === "documentation" ? selectedSourcePublicationIDs.map((id) => options.get(id)).filter((entry): entry is Record<string, unknown> => Boolean(entry)) : existingEntries;
      if (setKind === "documentation" && manifest.length !== selectedSourcePublicationIDs.length) throw new Error(t("integrations.everySelectedDocumentationPublicationMustStillExist"));
      if (editingSet) await api.updateResourceSet(editingSet.id, { name: setName, description: resourceDescription, state: editingSet.state, manifest, revision: editingSet.revision });
      else await api.createResourceSet({ kind: setKind, name: setName, description: resourceDescription, manifest });
      await onChanged(); setResourceOpen(false); onMessage(editingSet ? t("integrations.newImmutableResourceSetRevisionCreated") : t("integrations.reusableResourceSetCreated"));
    } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : t("integrations.resourceSetCouldNotBeSaved")); } finally { setBusy(false); }
  }

  async function duplicateResource() {
    if (!duplicateSet) return;
    setBusy(true);
    try { await api.duplicateResourceSet(duplicateSet.id, duplicateName); await onChanged(); setDuplicateSet(null); onMessage(t("integrations.independentResourceSetCopyCreated")); } catch (error) { onMessage(error instanceof APIError ? error.message : t("integrations.resourceSetCouldNotBeDuplicated")); } finally { setBusy(false); }
  }

  async function attachResource() {
    const resource = resourceSets.find((value) => value.id === attachSetID);
    if (!attachIntegration || !resource) return;
    setBusy(true);
    try { await api.attachResourceSet(attachIntegration.id, resource.id, pinAttachedSet ? resource.latest_revision?.id ?? "" : ""); await onChanged(); await refreshSelectedIntegration(attachIntegration.id); setAttachIntegration(null); onMessage(pinAttachedSet ? t("integrations.resourceRevisionPinnedToAPI") : t("integrations.resourceSetAttachedAndFollowingLatest")); } catch (error) { onMessage(error instanceof APIError ? error.message : t("integrations.resourceSetCouldNotBeAttached")); } finally { setBusy(false); }
  }

  async function attachPublishedSource(integration: APIIntegration, source: Source) {
	if (!source.latestPublication) return;
	setBusy(true);
	try {
	  const result = await onAttachPublishedSource(integration.id, source, source.latestPublication);
	  await refreshSelectedIntegration(integration.id);
	  onMessage(result.attached ? t("integrations.rWasPinnedToThisAPI", { name: String(source.name), revision: String(source.latestPublication.revision) }) : t("integrations.rIsAlreadyAttached", { name: String(source.name), revision: String(source.latestPublication.revision) }));
	} catch (error) {
	  onMessage(error instanceof APIError || error instanceof Error ? error.message : t("integrations.reviewedDocumentationCouldNotBeAttached"));
	} finally {
	  setBusy(false);
	}
  }

  async function detachResource(integrationID: string, setID: string) {
    setBusy(true);
    try { await api.detachResourceSet(integrationID, setID); await onChanged(); await refreshSelectedIntegration(integrationID); onMessage(t("integrations.resourceSetDetachedFromAPI")); } catch (error) { onMessage(error instanceof APIError ? error.message : t("integrations.resourceSetCouldNotBeDetached")); } finally { setBusy(false); }
  }

  const selectedIntegration = selectedDetail?.id === selectedIntegrationID ? selectedDetail : integrations.find((integration) => integration.id === selectedIntegrationID) ?? null;
  const selectedLoading = Boolean(selectedIntegrationID && loadedIntegrationID !== selectedIntegrationID);

  function openAttachDialog(integration: APIIntegration, kind: APIResourceSet["kind"] | "" = "") {
    const availableSets = resourceSets.filter((set) => (!kind || set.kind === kind) && !(integration.resources ?? []).some((resource) => resource.resource_set_id === set.id));
    setAttachIntegration(integration);
    setAttachKind(kind);
    setAttachSetID(availableSets[0]?.id ?? "");
    setPinAttachedSet(false);
  }

  let currentResourceManifest: Array<Record<string, unknown>> = [];
  try {
    const parsed = JSON.parse(setManifest) as unknown;
    if (Array.isArray(parsed)) currentResourceManifest = parsed as Array<Record<string, unknown>>;
  } catch {
    currentResourceManifest = [];
  }
  const documentationPublicationOptions = Array.from(new Map([
    ...currentResourceManifest.filter((entry) => typeof entry.source_publication_id === "string"),
    ...sources.flatMap((source) => source.latestPublication ? [{ source_publication_id: source.latestPublication.id, source_id: source.id, revision: source.latestPublication.revision, content_hash: source.latestPublication.content_hash, name: source.name }] : []),
  ].map((entry) => [String(entry.source_publication_id), entry])).values());
  const selectedAuthorizationProfile = authorizationProfiles.find((profile) => profile.id === authorizationProfileID);
  const authorizationCredentialLabel = authorizationType === "basic" ? "Password" : authorizationType === "oauth_client_credentials" ? "Client secret" : authorizationType === "bearer" ? "Bearer token" : "Secret value";

  return <>
    {selectedIntegrationID ? <IntegrationWorkspaceView live={live} integration={selectedIntegration} analyses={analyses} tools={tools} activeTab={activeTab} activeResourceTab={activeResourceTab} loading={selectedLoading} revisions={selectedRevisions} publishStatus={selectedPublishStatus} identity={identity} resourceSets={resourceSets} sources={sources} distribution={distribution} busy={busy} onEdit={openIntegration} onPublish={setPublishCandidate} onAttach={openAttachDialog} onCreateResource={() => openResource()} onAddSource={onAddSource} onCrawlSource={onCrawlSource} onPublishSource={onPublishSource} onAttachPublishedSource={attachPublishedSource} onGenerateSetupGuide={onGenerateSetupGuide} onEditResource={openResource} onDuplicateResource={(set) => { setDuplicateSet(set); setDuplicateName(t("integrations.duplicateName", { name: set.name })); }} onDetachResource={detachResource} onInspectRevision={setInspectedRevision} onRuntimeChanged={async () => { await onChanged(); await refreshSelectedIntegration(selectedIntegrationID); }} onMessage={onMessage} onNavigate={onNavigate} /> : <IntegrationDirectoryView integrations={integrations} query={query} onQueryChange={setQuery} onCreate={() => openIntegration()} onNavigate={onNavigate} />}

    <Dialog
      open={integrationOpen}
      onClose={setIntegrationOpen}
      title={editingIntegration ? t("integrations.editAPI") : t("integrations.addAPI")}
      description={editingIntegration ? t("integrations.updateThisAPISIdentityPublishingAndLifecycleSettings") : "Add the OpenAPI contract, then reuse or create the Authorization shared by this API."}
      actions={editingIntegration ? <><Button outline onClick={() => setIntegrationOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !versionKey.trim() || !displayName.trim() || !familyKey.trim() || Boolean(integrationVisibility === "public" && editingIntegration.visibility !== "public" && !integrationPublicAcknowledged)} onClick={saveIntegration}>{busy ? t("common.saving") : t("integrations.saveChanges")}</Button></> : <><Button outline onClick={() => setIntegrationOpen(false)}>{t("common.cancel")}</Button>{integrationStep > 1 && <Button outline disabled={busy} onClick={() => setIntegrationStep((integrationStep - 1) as 1 | 2)}>Back</Button>}{integrationStep < 3 ? <Button color="indigo" disabled={busy || (integrationStep === 1 ? !openAPIStepReady() : !authorizationStepReady())} onClick={() => void continueIntegrationConfigurator()}>Continue</Button> : <Button color="indigo" disabled={busy} onClick={saveIntegration}>{busy ? "Creating API…" : t("integrations.createAPI")}</Button>}</>}
    >
      <div className="auth-form compact-form">
        {!editingIntegration ? <>
          <ol className="api-configurator-progress" aria-label="API configuration progress"><li className={integrationStep >= 1 ? "active" : ""}><span>1</span>OpenAPI</li><li className={integrationStep >= 2 ? "active" : ""}><span>2</span>Authorization</li><li className={integrationStep >= 3 ? "active" : ""}><span>3</span>Review</li></ol>
          {integrationStep === 1 && <div className="api-configurator-step">
            <div className="two-fields"><label className="auth-field"><span>{t("integrations.apiName")}</span><input value={displayName} maxLength={120} onChange={(event) => { const value = event.target.value; setDisplayName(value); if (!environmentVariableTouched) setEnvironmentVariable(authorizationEnvironmentVariableFromName(value)); }} placeholder={t("integrations.exVoiceAPI")} /></label><label className="auth-field"><span>{t("integrations.version")}</span><input value={versionKey} maxLength={64} onChange={(event) => setVersionKey(event.target.value)} placeholder="v1" /></label></div>
            <label className="auth-field"><span>OpenAPI file</span><input type="file" accept=".json,.yaml,.yml,application/json,application/yaml,text/yaml" onChange={(event) => { const file = event.target.files?.[0] ?? null; setOpenAPIFile(file); setOpenAPIFileError(validateOpenAPIFile(file)); }} /><small>JSON or YAML, up to 5 MB. It is ingested into a reviewable contract candidate before publication.</small></label>
            <label className="auth-field"><span>API base URL</span><input type="url" value={serviceURL} onChange={(event) => setServiceURL(event.target.value)} placeholder="https://api.example.com" autoComplete="url" /><small>The fixed credential-free origin DokoSoko uses for generated HTTP tools.</small></label>
            {openAPIFileError && <div className="auth-problem"><TriangleAlert /><span>{openAPIFileError}</span></div>}
          </div>}
          {integrationStep === 2 && <div className="api-configurator-step">
            <fieldset className="runtime-credential-choice"><legend>Authorization</legend><div className="runtime-choice-grid two-choice-grid"><label aria-label="Import existing Authorization" className={authorizationMode === "existing" ? "selected" : ""} aria-disabled={authorizationProfiles.length === 0}><input type="radio" name="api-authorization-mode" checked={authorizationMode === "existing"} disabled={authorizationProfiles.length === 0} onChange={() => { setAuthorizationMode("existing"); setAuthorizationProfileID(authorizationProfiles[0]?.id ?? ""); }} /><span><strong>Import existing</strong><small>Reuse one Authorization already connected to another API.</small></span></label><label aria-label="Configure new Authorization" className={authorizationMode === "new" ? "selected" : ""}><input type="radio" name="api-authorization-mode" checked={authorizationMode === "new"} onChange={() => setAuthorizationMode("new")} /><span><strong>Configure new</strong><small>Create one reusable Authorization for this and future APIs.</small></span></label></div></fieldset>
            {authorizationMode === "existing" ? <label className="auth-field"><span>Existing Authorization</span><select value={authorizationProfileID} onChange={(event) => setAuthorizationProfileID(event.target.value)}><option value="">Choose Authorization</option>{authorizationProfiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.name} · {profile.environment_variable} · {profile.authentication_type.replaceAll("_", " ")}</option>)}</select>{selectedAuthorizationProfile && <small>{selectedAuthorizationProfile.key_management_url || "No key management URL recorded"} · credential {selectedAuthorizationProfile.active_fingerprint}</small>}</label> : <>
              <label className="auth-field"><span>Method</span><select value={authorizationType} onChange={(event) => { const value = event.target.value as Exclude<APIRuntimeAuthenticationType, "none" | "delegated_oauth">; const hadPrimaryHeader = authorizationType === "api_key_header" || authorizationType === "custom_header"; const hasPrimaryHeader = value === "api_key_header" || value === "custom_header"; setAuthorizationType(value); if (hasPrimaryHeader && !hadPrimaryHeader) setAuthorizationHeaders([authorizationHeaderDraft(value === "custom_header" ? "X-Custom-Auth" : "X-API-Key")]); if (!hasPrimaryHeader && hadPrimaryHeader) setAuthorizationHeaders([]); }}><option value="api_key_header">API key header</option><option value="bearer">Bearer token</option><option value="custom_header">Custom header</option><option value="basic">Basic Auth</option><option value="oauth_client_credentials">OAuth 2.0 client credentials</option></select></label>
              <label className="auth-field"><span>Key management URL</span><input type="url" value={keyManagementURL} onChange={(event) => setKeyManagementURL(event.target.value)} placeholder="https://dashboard.example.com/api-keys" /><small>Stored with this Authorization for operators. DokoSoko never fetches it.</small></label>
              <div className="two-fields"><label className="auth-field"><span>Access evaluation URL</span><input type="url" value={accessEvaluationURL} onChange={(event) => setAccessEvaluationURL(event.target.value)} placeholder="https://api.example.com/hooks/access-evaluation" /><small>Synchronous and fail-closed.</small></label><label className="auth-field"><span>Usage URL</span><input type="url" value={usageURL} onChange={(event) => setUsageURL(event.target.value)} placeholder="https://api.example.com/hooks/usage" /><small>Delivered asynchronously.</small></label></div>
              <div className="two-fields"><label className="auth-field"><span>Environment variable</span><input value={environmentVariable} maxLength={128} onChange={(event) => { setEnvironmentVariableTouched(true); setEnvironmentVariable(event.target.value.toUpperCase()); }} placeholder="API_KEY_ENV" pattern="[A-Z][A-Z0-9_]*" /><small>This binding belongs to the Authorization and follows it to every API.</small></label>{authorizationType !== "api_key_header" && authorizationType !== "custom_header" && <label className="auth-field"><span>{authorizationCredentialLabel}</span><input type="password" value={authorizationCredential} maxLength={16384} onChange={(event) => setAuthorizationCredential(event.target.value)} placeholder="************" autoComplete="new-password" /></label>}</div>
              {authorizationType === "basic" && <label className="auth-field"><span>Username</span><input value={basicUsername} onChange={(event) => setBasicUsername(event.target.value)} autoComplete="username" /></label>}
              {authorizationType === "oauth_client_credentials" && <><div className="two-fields"><label className="auth-field"><span>Client ID</span><input value={oauthClientID} onChange={(event) => setOAuthClientID(event.target.value)} /></label><label className="auth-field"><span>Token URL</span><input type="url" value={oauthTokenURL} onChange={(event) => setOAuthTokenURL(event.target.value)} placeholder="https://identity.example.com/oauth/token" /></label></div><label className="auth-field"><span>Scopes</span><input value={oauthScopes} onChange={(event) => setOAuthScopes(event.target.value)} placeholder="read write" /><small>Separate scopes with spaces or commas.</small></label></>}
              <AuthorizationHeaderManager headers={authorizationHeaders} onChange={setAuthorizationHeaders} required={authorizationType === "api_key_header" || authorizationType === "custom_header"} />
            </>}
          </div>}
          {integrationStep === 3 && <div className="api-configurator-step api-configurator-review"><div><span>API</span><strong>{displayName} {versionKey}</strong><small>{openAPIFile?.name}</small></div><div><span>Service</span><strong>{serviceURL}</strong><small>{deploymentEnvironments.find((value) => value.is_production)?.name ?? deploymentEnvironments[0]?.name}</small></div><div><span>Authorization</span><strong>{authorizationMode === "existing" ? selectedAuthorizationProfile?.name : displayName}</strong><small>{authorizationMode === "existing" ? selectedAuthorizationProfile?.environment_variable : `${environmentVariable} · ${authorizationType.replaceAll("_", " ")}`}</small></div><div><span>Hooks</span><strong>{authorizationMode === "existing" ? selectedAuthorizationProfile?.access_evaluation_url : accessEvaluationURL}</strong><small>{authorizationMode === "existing" ? selectedAuthorizationProfile?.usage_url : usageURL}</small></div><div className="notice"><ShieldCheck /><span><strong>Secrets and hooks stay on Authorization.</strong> The API stores only a reference to the reusable profile; OpenAPI ingestion remains review-gated before publication.</span></div></div>}
        </> : <>
          <div className="two-fields"><label className="auth-field"><span>{t("integrations.apiFamilyKey")}</span><input value={familyKey} onChange={(event) => setFamilyKey(event.target.value)} /></label><label className="auth-field"><span>{t("integrations.apiVersion")}</span><input value={versionKey} onChange={(event) => setVersionKey(event.target.value)} /></label></div>
          <label className="auth-field"><span>{t("integrations.displayName")}</span><input value={displayName} onChange={(event) => setDisplayName(event.target.value)} /></label>
          <label className="auth-field"><span>{t("integrations.description")}</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} /></label>
          <div className="two-fields"><label className="auth-field"><span>{t("integrations.visibility")}</span><select value={integrationVisibility} onChange={(event) => { setIntegrationVisibility(event.target.value as APIIntegration["visibility"]); setIntegrationPublicAcknowledged(false); }}><option value="private">{t("integrations.private")}</option><option value="public">{t("integrations.public")}</option></select><small>{t("integrations.publicExposesOnlyThePublishedReadOnlyAPIManifest")}</small></label><label className="auth-field"><span>{t("integrations.lifecycle")}</span><select value={lifecycle} onChange={(event) => setLifecycle(event.target.value as APIIntegration["lifecycle"])}><option value="draft">{t("integrations.draft")}</option><option value="active">{t("integrations.active")}</option><option value="deprecated">{t("integrations.deprecated")}</option><option value="retired">{t("integrations.retired")}</option></select></label></div>
          {integrationVisibility === "public" && editingIntegration.visibility !== "public" && <label className="compact-check"><input type="checkbox" checked={integrationPublicAcknowledged} onChange={(event) => setIntegrationPublicAcknowledged(event.target.checked)} /><span>{t("integrations.iUnderstandThisPublishedAPIMetadataWillBeAnonymously")}</span></label>}
          <label className="auth-field"><span>{t("integrations.replacement")}</span><select disabled={lifecycle !== "deprecated" && lifecycle !== "retired"} value={replacementID} onChange={(event) => setReplacementID(event.target.value)}><option value="">{t("integrations.none")}</option>{integrations.filter((value) => value.id !== editingIntegration.id).map((value) => <option key={value.id} value={value.id}>{value.display_name} {value.version_key}</option>)}</select></label>
          {(lifecycle === "deprecated" || lifecycle === "retired") && <label className="auth-field"><span>{t("integrations.sunsetDate")}</span><input type="date" value={sunsetAt} onChange={(event) => setSunsetAt(event.target.value)} /></label>}
        </>}
      </div>
    </Dialog>
    <Dialog open={resourceOpen} onClose={setResourceOpen} title={editingSet ? t("integrations.createRevisionFor", { name: String(editingSet.name) }) : t("integrations.createReusableResourceSet")} description={t("integrations.setsAreReusableByExplicitAttachmentEachSaveCreates")} actions={<><Button outline onClick={() => setResourceOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !setName.trim() || (setKind === "documentation" && selectedSourcePublicationIDs.length === 0)} onClick={saveResourceSet}>{busy ? t("common.saving") : editingSet ? t("integrations.createRevision") : t("integrations.createSet")}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>{t("integrations.kind")}</span><select disabled={Boolean(editingSet)} value={setKind} onChange={(event) => setSetKind(event.target.value as APIResourceSet["kind"])}><option value="documentation">{t("integrations.documentation")}</option><option value="api">{t("integrations.apiContract")}</option></select></label><label className="auth-field"><span>{t("integrations.name")}</span><input value={setName} onChange={(event) => setSetName(event.target.value)} /></label></div><label className="auth-field"><span>{t("integrations.description")}</span><textarea value={resourceDescription} onChange={(event) => setResourceDescription(event.target.value)} /></label>{setKind === "documentation" ? <div className="auth-field"><span>{t("integrations.reviewedSourcePublications")}</span><div className="catalog-list">{documentationPublicationOptions.map((entry) => { const id = String(entry.source_publication_id); const selected = selectedSourcePublicationIDs.includes(id); return <label className="catalog-tool" key={id}><input type="checkbox" checked={selected} onChange={(event) => setSelectedSourcePublicationIDs((items) => event.target.checked ? [...items, id] : items.filter((value) => value !== id))} /><span className="check-box">{selected && <Check />}</span><span><strong>{String(entry.name ?? entry.source_id)}</strong><code>{id}</code><small>{t("integrations.publicationR")}{String(entry.revision)} · {String(entry.content_hash)}</small></span><Badge color="green">{t("integrations.reviewed")}</Badge></label>; })}{documentationPublicationOptions.length === 0 && <div className="empty-row">{t("integrations.publishAReviewedDocumentationGenerationBeforeCreatingThisSet")}</div>}</div><small>{t("integrations.eachSelectionPinsOneImmutableSourcePublicationRevisionAnd")}</small></div> : <label className="auth-field"><span>{t("integrations.apiContractManifestJSONArray")}</span><textarea className="code-input" value={setManifest} onChange={(event) => setSetManifest(event.target.value)} spellCheck={false} /></label>}</div></Dialog>
    <Dialog open={Boolean(duplicateSet)} onClose={(open) => { if (!open) setDuplicateSet(null); }} title={t("integrations.duplicateResourceSet")} description={t("integrations.createsAnIndependentCopySoLaterEditsDoNot")} actions={<><Button outline onClick={() => setDuplicateSet(null)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !duplicateName.trim()} onClick={duplicateResource}>{t("integrations.duplicate")}</Button></>}><label className="auth-field"><span>{t("integrations.newSetName")}</span><input value={duplicateName} onChange={(event) => setDuplicateName(event.target.value)} /></label></Dialog>
    <Dialog open={Boolean(attachIntegration)} onClose={(open) => { if (!open) setAttachIntegration(null); }} title={t("integrations.attachResourcesTo", { value1: String(attachIntegration?.display_name ?? "API") })} description={t("integrations.followLatestForDeliberateSharingOrPinTheCurrent")} actions={<><Button outline onClick={() => setAttachIntegration(null)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !attachSetID} onClick={attachResource}>{t("integrations.attach")}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>{t("integrations.resourceSet")}</span><select value={attachSetID} onChange={(event) => setAttachSetID(event.target.value)}><option value="">{t("integrations.selectASet")}</option>{resourceSets.filter((set) => (!attachKind || set.kind === attachKind) && !(attachIntegration?.resources ?? []).some((link) => link.resource_set_id === set.id)).map((set) => <option key={set.id} value={set.id}>{set.kind === "api" ? t("integrations.apiContract") : t("integrations.documentation2")} · {set.name}</option>)}</select></label><label className="compact-check"><input type="checkbox" checked={pinAttachedSet} onChange={(event) => setPinAttachedSet(event.target.checked)} /><span>{t("integrations.pinTheCurrentRevisionInsteadOfFollowingLatest")}</span></label></div></Dialog>
    <Dialog open={Boolean(publishCandidate)} onClose={(open) => { if (!open) setPublishCandidate(null); }} title={t("integrations.publish2", { value1: String(publishCandidate?.display_name ?? "API") })} description={t("integrations.reviewWhatChangedBeforeCreatingANewImmutableVersion")} actions={<><Button outline onClick={() => setPublishCandidate(null)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !selectedPublishStatus?.ready || !selectedPublishStatus.has_changes} onClick={publishIntegration}>{busy ? t("integrations.publishing") : t("integrations.publish")}</Button></>}><div className="publish-review">{selectedPublishStatus?.validations.map((validation) => <div key={validation.code} className={`publish-validation ${validation.level}`}><span>{validation.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{validation.level}</strong><small>{validation.message}</small></span></div>)}<div className="publish-diff-list">{selectedPublishStatus?.changes.map((change) => <div className="publish-diff" key={change.field}><strong>{change.field}</strong><span><small>{t("integrations.published")}</small><code>{change.before === undefined ? "—" : JSON.stringify(change.before)}</code></span><ChevronRight /><span><small>{t("integrations.draft")}</small><code>{change.after === undefined ? "—" : JSON.stringify(change.after)}</code></span></div>)}</div><details className="advanced-details"><summary>{t("integrations.technicalDetails")}</summary><code>{selectedPublishStatus?.current_manifest_hash ?? "—"}</code></details></div></Dialog>
    <Dialog open={Boolean(inspectedRevision)} onClose={(open) => { if (!open) setInspectedRevision(null); }} title={t("integrations.publishedVersionR", { value1: String(inspectedRevision?.revision ?? "") })} description={t("integrations.thisImmutableTechnicalSnapshotIsKeptForAuditAnd")} actions={<Button outline onClick={() => setInspectedRevision(null)}>{t("common.close")}</Button>}><div className="revision-inspector"><dl className="entity-detail-grid"><div><dt>{t("integrations.versionID")}</dt><dd>{inspectedRevision?.id}</dd></div><div><dt>{t("integrations.state")}</dt><dd>{inspectedRevision?.state}</dd></div><div><dt>{t("integrations.published")}</dt><dd>{inspectedRevision ? t("format.dateTime", { value: new Date(inspectedRevision.published_at ?? inspectedRevision.created_at) }) : "—"}</dd></div><div><dt>{t("integrations.publishedBy")}</dt><dd>{inspectedRevision?.published_by || "—"}</dd></div><div><dt>{t("integrations.manifestHash")}</dt><dd><code>{inspectedRevision?.manifest_hash}</code></dd></div></dl><pre className="usage-contract"><code>{JSON.stringify(inspectedRevision?.snapshot ?? {}, null, 2)}</code></pre></div></Dialog>
  </>;
}
