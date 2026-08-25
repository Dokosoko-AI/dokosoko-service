import {
  AlertCircle, ArrowLeft, BookOpen, Check, CheckCircle2, ChevronRight, Clock3, Copy,
  Database, ExternalLink, Globe2, KeyRound, LockKeyhole, MessageSquareText,
  MoreHorizontal, Plus, RefreshCw, Search, ShieldCheck, TriangleAlert, Users, XCircle,
} from "lucide-react";
import { useEffect, useState } from "react";

import {
  APICustomerAccount, APIError, APIIntegration, APIRecipe, APIVisibility, APIWidget, APIWidgetInput,
  APIWidgetSecret, APIWidgetSession, Distribution, api,
} from "../../lib/api";
import { entityPath, sectionPath } from "../../lib/console-routes";
import { Badge, Button, Switch } from "../core/control";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader as PageHeading, PanelHeader, SectionHeader, SegmentedControl } from "../core/layout";
import {
  ConsoleLink, CopyButton, EntityLink, Source, SummaryItem, agentClients,
  widgetOriginLabel,
} from "./shared";

type Visibility = APIVisibility;

export function WidgetsView({ widgets, integrations, onCreate, onNavigate }: { widgets: APIWidget[]; integrations: APIIntegration[]; onCreate: () => void; onNavigate: (path: string) => void }) {
  const integrationName = (id: string) => integrations.find((integration) => integration.id === id)?.display_name ?? id;
  return <>
    <PageHeading eyebrow="Agent access" title="Widgets" action={<Button color="indigo" onClick={onCreate}><Plus data-slot="icon" />Create widget</Button>} />
    <div className="widget-principle"><ShieldCheck /><span><strong>One identity boundary.</strong> Your backend authenticates the user; DokoSoko limits every session to the APIs configured here.</span></div>
    <DataTable label="Widgets" className="widget-directory">
      <DataTableHeader className="widget-columns"><span>Widget</span><span>Application</span><span>APIs</span><span>Status</span><span>Open</span></DataTableHeader>
      {widgets.map((widget) => <DataTableRow className="widget-columns" key={widget.id}>
        <span className="resource-name"><span className="resource-icon"><MessageSquareText /></span><span><ConsoleLink path={entityPath("widget", widget.id)} onNavigate={onNavigate} className="entity-link"><strong>{widget.name}</strong></ConsoleLink><small>{widget.id}</small></span></span>
        <span><strong className="cell-value">{widget.allowed_origins[0] ? widgetOriginLabel(widget.allowed_origins[0]) : "Not configured"}</strong><small className="cell-note">{widget.allowed_origins.length === 1 ? "1 allowed origin" : `${widget.allowed_origins.length} allowed origins`}</small></span>
        <span><strong className="cell-value">{widget.integration_ids.length}</strong><small className="cell-note">{widget.integration_ids.slice(0, 2).map(integrationName).join(", ") || "No access"}</small></span>
        <Badge color={widget.state === "active" ? "green" : widget.state === "disabled" ? "red" : "zinc"}>{widget.state}</Badge>
        <span className="table-open-cell"><ConsoleLink path={entityPath("widget", widget.id)} onNavigate={onNavigate} className="row-arrow" ariaLabel={`Open ${widget.name}`}><ChevronRight /></ConsoleLink></span>
      </DataTableRow>)}
      {widgets.length === 0 && <DataTableEmpty columns={5}><div className="widget-empty"><span className="entity-missing-icon"><MessageSquareText /></span><div><h2>No widgets yet</h2><p>Create one authenticated widget, connect the APIs it needs, and verify the installation before going live.</p></div><Button color="indigo" onClick={onCreate}><Plus data-slot="icon" />Create widget</Button></div></DataTableEmpty>}
    </DataTable>
  </>;
}

export function WidgetDetailView({ widget, integrations, recipes, assistantAvailable, busy, onUpdate, onSetState, onRotateSecret, onConfigureAssistant, onMessage, onNavigate }: { widget: APIWidget | null; integrations: APIIntegration[]; recipes: APIRecipe[]; assistantAvailable: boolean; busy: boolean; onUpdate: (widget: APIWidget, input: APIWidgetInput) => Promise<APIWidget | null>; onSetState: (widget: APIWidget, state: "active" | "disabled") => Promise<APIWidget | null>; onRotateSecret: (widget: APIWidget) => void | Promise<void>; onConfigureAssistant: () => void; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [name, setName] = useState(widget?.name ?? "");
  const [origins, setOrigins] = useState(widget?.allowed_origins.join("\n") ?? "");
  const [integrationIDs, setIntegrationIDs] = useState<string[]>(widget?.integration_ids ?? []);
  const [theme, setTheme] = useState<"auto" | "light" | "dark">(widget?.appearance.theme ?? "auto");
  const [accent, setAccent] = useState(widget?.appearance.accentColour ?? "");
  const [greeting, setGreeting] = useState(widget?.appearance.greeting ?? "");
  const [secrets, setSecrets] = useState<APIWidgetSecret[]>([]);
  const [sessions, setSessions] = useState<APIWidgetSession[]>([]);
  const [securityRefresh, setSecurityRefresh] = useState(0);
  const [securityObservedAt, setSecurityObservedAt] = useState(0);
  useEffect(() => {
    let cancelled = false;
    if (!widget) return;
    Promise.all([api.widgetSecrets(widget.id), api.widgetSessions(widget.id)])
      .then(([nextSecrets, nextSessions]) => { if (!cancelled) { setSecrets(nextSecrets); setSessions(nextSessions); setSecurityObservedAt(Date.now()); } })
      .catch(() => { if (!cancelled) { setSecrets([]); setSessions([]); } });
    return () => { cancelled = true; };
  }, [widget, securityRefresh]);
  if (!widget) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>Widget unavailable</h1><p>This widget does not exist or is still loading.</p></div><ConsoleLink path={sectionPath("widgets")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to widgets</ConsoleLink></section>;
  const input: APIWidgetInput = {
    name: name.trim(),
    allowed_origins: origins.split(/[\n,]/).map((value) => value.trim()).filter(Boolean).sort(),
    integration_ids: [...integrationIDs].sort(),
    appearance: { theme, accent_colour: accent.trim() || undefined, launcher_position: widget.appearance.launcherPosition ?? "right", greeting: greeting.trim() || undefined },
  };
  const persistedInput: APIWidgetInput = {
    name: widget.name,
    allowed_origins: [...widget.allowed_origins].sort(),
    integration_ids: [...widget.integration_ids].sort(),
    appearance: { theme: widget.appearance.theme, accent_colour: widget.appearance.accentColour || undefined, launcher_position: widget.appearance.launcherPosition, greeting: widget.appearance.greeting || undefined },
  };
  const dirty = JSON.stringify(input) !== JSON.stringify(persistedInput);
  const activeSecrets = secrets.filter((secret) => !secret.revoked_at);
  const activeSessions = sessions.filter((session) => !session.revoked_at && new Date(session.expires_at).getTime() > securityObservedAt);
  const scopedGuidance = recipes.filter((recipe) => recipe.dependencies.some((dependency) => (dependency.kind === "integration" || dependency.kind === "integration_scope") && input.integration_ids.includes(dependency.resource_id)));
  const availableGuidance = scopedGuidance.filter((recipe) => recipe.state === "published" && !recipe.needs_attention);
  const guidanceCount = widget.state === "active" ? widget.knowledge_bindings.length : availableGuidance.length;
  const guidanceNeedsReview = scopedGuidance.some((recipe) => recipe.needs_attention || recipe.state === "outdated");
  const guidanceChanged = widget.state === "active" && availableGuidance.length > 0 && JSON.stringify(availableGuidance.map((recipe) => `${recipe.id}:${recipe.current_revision_id}`).sort()) !== JSON.stringify(widget.knowledge_bindings.map((binding) => `${binding.recipe_id}:${binding.recipe_revision_id}`).sort());
  const frontendSnippet = `import { mountWidget } from "@dokosoko/widget";\n\nmountWidget({\n  widgetId: "${widget.id}",\n  getToken: async () => {\n    const response = await fetch("/api/dokosoko/widget-token", {\n      method: "POST",\n      credentials: "same-origin",\n    });\n    if (!response.ok) throw new Error("Sign in required");\n    return response.json();\n  },\n});`;
  const backendSnippet = `import DokoSokoWidgetBackend from "@dokosoko/widget-backend";\n\nconst dokosoko = new DokoSokoWidgetBackend({\n  widgetSecret: process.env.DOKOSOKO_WIDGET_SECRET!,\n});\n\nexport async function POST(request: Request) {\n  const user = await requireAuthenticatedUser(request);\n  const token = await dokosoko.widgetSessions.create({\n    widgetId: "${widget.id}",\n    userId: user.id,\n    organizationId: user.organizationId,\n    context: {\n      view: "profile",\n      title: "Your profile",\n      facts: [\n        { label: "Plan", value: user.planName },\n        { label: "Account status", value: user.statusLabel },\n      ],\n    },\n    origin: new URL(request.url).origin,\n  }, { idempotencyKey: crypto.randomUUID() });\n\n  return Response.json(token, {\n    headers: { "cache-control": "no-store" },\n  });\n}`;
  const save = () => onUpdate(widget, input);
  const activate = async () => {
    const saved = dirty ? await onUpdate(widget, input) : widget;
    if (saved) await onSetState(saved, "active");
  };
  const refreshGuidance = async () => {
    const saved = dirty ? await onUpdate(widget, input) : widget;
    if (saved) await onSetState(saved, "active");
  };
  const rotateSecret = async () => { await onRotateSecret(widget); setSecurityRefresh((value) => value + 1); };
  const revokeSecret = async (secret: APIWidgetSecret) => {
    try {
      const updated = await api.revokeWidgetSecret(widget.id, secret.id);
      setSecrets((values) => values.map((value) => value.id === updated.id ? updated : value));
      onMessage("Widget secret revoked.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Could not revoke the widget secret."); }
  };
  const revokeSession = async (session: APIWidgetSession) => {
    try {
      const updated = await api.revokeWidgetSession(widget.id, session.id);
      setSessions((values) => values.map((value) => value.id === updated.id ? updated : value));
      onMessage("Widget session revoked.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Could not revoke the widget session."); }
  };
  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("widgets")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Back to widgets</ConsoleLink><code>/widget/{widget.id}</code></div>
    <PageHeading eyebrow="Authenticated widget" title={widget.name} action={widget.state === "active" ? <>{guidanceNeedsReview ? <Button outline onClick={() => onNavigate(sectionPath("recipes"))}>Review guidance</Button> : guidanceChanged && <Button outline disabled={busy} onClick={refreshGuidance}>Refresh guidance</Button>}<Button outline disabled={busy} onClick={() => onSetState(widget, "disabled")}>Disable</Button></> : !assistantAvailable ? <Button outline onClick={onConfigureAssistant}>Configure assistant</Button> : <Button color="indigo" disabled={busy || !input.name || input.allowed_origins.length === 0 || input.integration_ids.length === 0} onClick={activate}>{busy ? "Saving…" : dirty ? "Save and activate" : "Activate widget"}</Button>} />
    <div className="widget-status-line"><Badge color={widget.state === "active" ? "green" : widget.state === "disabled" ? "red" : "zinc"}>{widget.state}</Badge><code>{widget.id}</code><span>Revision {widget.revision}</span></div>
    <ol className="widget-setup-steps">
      <li className={input.allowed_origins.length ? "complete" : ""}><span>{input.allowed_origins.length ? <Check /> : "1"}</span><div><strong>Allow your application</strong><small>{input.allowed_origins.length ? `${input.allowed_origins.length} exact origin${input.allowed_origins.length === 1 ? "" : "s"}` : "Add the domains that may embed this widget."}</small></div></li>
      <li className="complete"><span><Check /></span><div><strong>Authenticate users</strong><small>A server-only widget secret was created. Use it only through the backend SDK.</small></div></li>
      <li className={input.integration_ids.length ? "complete" : ""}><span>{input.integration_ids.length ? <Check /> : "3"}</span><div><strong>Connect APIs</strong><small>{input.integration_ids.length ? `${input.integration_ids.length} API${input.integration_ids.length === 1 ? "" : "s"} allowed` : "No API access is granted by default."}</small></div></li>
      <li className={guidanceNeedsReview ? "attention" : guidanceCount ? "complete" : ""}><span>{guidanceNeedsReview ? <TriangleAlert /> : guidanceCount ? <Check /> : "4"}</span><div><strong>{guidanceNeedsReview ? "Review guidance" : "Publish guidance"}</strong><small>{guidanceNeedsReview ? `${guidanceCount || scopedGuidance.length} setup recipe${(guidanceCount || scopedGuidance.length) === 1 ? "" : "s"} changed after publication. Review it before refreshing this widget.` : guidanceCount ? `${guidanceCount} setup recipe${guidanceCount === 1 ? "" : "s"} ${widget.state === "active" ? "pinned" : "ready to pin"}` : "Publish a setup recipe for an allowed API."}</small></div></li>
      <li className={assistantAvailable ? "complete" : ""}><span>{assistantAvailable ? <Check /> : "5"}</span><div><strong>Connect assistant</strong><small>{assistantAvailable ? "The grounded assistant runtime is ready." : "Configure an assistant model in Settings."}</small></div></li>
      <li className={widget.state === "active" ? "complete" : ""}><span>{widget.state === "active" ? <Check /> : "6"}</span><div><strong>Go live</strong><small>{widget.state === "active" ? "New authenticated sessions are accepted." : "Activate after testing the complete answer path."}</small></div></li>
    </ol>
    <section className="panel widget-settings-panel"><PanelHeader title="Access and appearance" action={<Button color="indigo" disabled={busy || !dirty || !input.name || input.allowed_origins.length === 0 || (widget.state === "active" && input.integration_ids.length === 0)} onClick={save}>Save changes</Button>} /><div className="widget-settings-grid"><div className="auth-form compact-form"><label className="auth-field"><span>Name</span><input value={name} maxLength={120} onChange={(event) => setName(event.target.value)} /></label><label className="auth-field"><span>Allowed origins</span><textarea value={origins} onChange={(event) => setOrigins(event.target.value)} /><small>Exact origins only; one per line.</small></label><fieldset className="widget-api-picker"><legend>Allowed APIs</legend>{integrations.filter((integration) => integration.lifecycle === "active").map((integration) => <label key={integration.id}><input aria-label={`Allow ${integration.display_name}`} type="checkbox" checked={integrationIDs.includes(integration.id)} onChange={(event) => setIntegrationIDs((values) => event.target.checked ? [...values, integration.id] : values.filter((id) => id !== integration.id))} /><span><strong>{integration.display_name}</strong><small>{integration.family_key} · {integration.version_key}</small></span></label>)}{integrations.filter((integration) => integration.lifecycle === "active").length === 0 && <p className="empty-picker">Publish an API before activating this widget.</p>}</fieldset></div><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Theme</span><select value={theme} onChange={(event) => setTheme(event.target.value as typeof theme)}><option value="auto">Automatic</option><option value="light">Light</option><option value="dark">Dark</option></select></label><label className="auth-field"><span>Accent</span><input value={accent} placeholder="#5b5cf0" onChange={(event) => setAccent(event.target.value)} /></label></div><label className="auth-field"><span>Greeting</span><input value={greeting} placeholder="How can I help?" maxLength={160} onChange={(event) => setGreeting(event.target.value)} /></label><div className="widget-live-preview" style={{ "--widget-accent": accent || "#5b5cf0" } as React.CSSProperties}><span>D</span><div><strong>{name || "Customer assistant"}</strong><small>{greeting || "How can I help?"}</small></div></div></div></div></section>
    <section className="panel widget-install-panel"><PanelHeader title="Install" /><div className="install-snippets"><article><div><strong>1. Browser</strong><CopyButton text={frontendSnippet} label="Copy browser code" onCopied={() => onMessage("Browser code copied.")} /></div><pre>{frontendSnippet}</pre></article><article><div><strong>2. Backend</strong><CopyButton text={backendSnippet} label="Copy backend code" onCopied={() => onMessage("Backend code copied.")} /></div><pre>{backendSnippet}</pre></article></div></section>
    <section className="panel widget-security-panel"><PanelHeader title="Security" action={<Button outline disabled={busy} onClick={rotateSecret}>Create new secret</Button>} /><div className="widget-security-grid"><article><div className="widget-security-title"><KeyRound /><span><strong>Backend secrets</strong><small>{activeSecrets.length} active</small></span></div><div className="widget-security-list">{secrets.map((secret) => <div key={secret.id}><span><code>••••{secret.fingerprint}</code><small>{secret.last_used_at ? `Last used ${new Date(secret.last_used_at).toLocaleString()}` : `Created ${new Date(secret.created_at).toLocaleString()}`}</small></span>{secret.revoked_at ? <Badge color="zinc">Revoked</Badge> : <Button outline disabled={busy || activeSecrets.length < 2} onClick={() => revokeSecret(secret)}>Revoke</Button>}</div>)}{secrets.length === 0 && <p className="empty-picker">Credential metadata is unavailable.</p>}</div></article><article><div className="widget-security-title"><ShieldCheck /><span><strong>Recent sessions</strong><small>{activeSessions.length} active</small></span></div><div className="widget-security-list">{sessions.slice(0, 8).map((session) => { const expired = new Date(session.expires_at).getTime() <= securityObservedAt; const preview = session.kind === "admin_preview"; return <div key={session.id}><span><strong>{preview ? "Admin preview" : session.user_id}</strong><small>{preview ? "Preview" : "Customer"} · {widgetOriginLabel(session.origin)} · expires {new Date(session.expires_at).toLocaleString()}</small></span>{session.revoked_at || expired ? <Badge color="zinc">{session.revoked_at ? "Revoked" : "Expired"}</Badge> : <Button outline disabled={busy} onClick={() => revokeSession(session)}>Revoke</Button>}</div>; })}{sessions.length === 0 && <p className="empty-picker">No widget sessions yet.</p>}</div></article></div></section>
  </>;
}

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
  widgetsEnabled,
  widgetCount,
  onOpenWidgets,
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
  widgetsEnabled: boolean;
  widgetCount: number;
  onOpenWidgets: () => void;
}) {
  return <>
    <PageHeading eyebrow="Delivery" title="Agent access" action={<Button outline disabled={!privateAgentSetup.available} onClick={() => window.open(privateAgentSetup.url, "_blank", "noopener,noreferrer")}><ExternalLink data-slot="icon" />Private MCP setup</Button>} />
    <section className={`public-mcp-card ${enabled ? "enabled" : ""}`}>
      <div className="public-mcp-copy"><div className="icon-tile"><Globe2 /></div><div><div className="title-row"><h2>Public MCP</h2><Badge color={enabled ? "green" : "zinc"}>{enabled ? "Live" : "Off"}</Badge></div><p>Offer an authentication-free, read-only MCP endpoint. Its server-side policy can retrieve only published sources that you explicitly mark public.</p><div className="endpoint"><code>{publicEndpoint}</code><button type="button" aria-label="Copy public MCP endpoint" onClick={() => { navigator.clipboard.writeText(publicEndpoint); onCopied("Public MCP endpoint copied."); }}><Copy />Copy</button></div></div></div>
      <div className="switch-stack"><Switch checked={enabled} onChange={onEnabledChange} label="Enable Public MCP" /><small>{enabled ? "Accepting anonymous requests" : "Disabled by default"}</small></div>
    </section>

    <section className="section-block agent-setup-section">
      <SectionHeader title="Copy MCP button" />
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
          <span className="visibility-control"><Badge color={resource.visibility === "public" ? "green" : "zinc"}>{resource.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{resource.visibility[0].toUpperCase() + resource.visibility.slice(1)}</Badge><Switch checked={resource.visibility === "public"} onChange={() => onVisibilityChange(resource.resourceType, resource.id)} label={`Make ${resource.name} ${resource.visibility === "public" ? "private" : "public"}`} /></span>
          <span className="table-actions"><button type="button" className="more" aria-label={`Actions for ${resource.name}`}><MoreHorizontal /></button></span>
        </DataTableRow>)}
        {resources.length === 0 && <DataTableEmpty columns={4}>No resources match this filter.</DataTableEmpty>}
      </DataTable>
    </section>

    {widgetsEnabled && <section className="section-block widget-channel-card"><span className="icon-tile"><MessageSquareText /></span><div><h2>Embedded widgets</h2><p>Authenticated assistants for customer applications. Each widget has its own origins, server secret, and API allow-list.</p></div><Badge color={widgetCount > 0 ? "violet" : "zinc"}>{widgetCount}</Badge><Button outline onClick={onOpenWidgets}>{widgetCount > 0 ? "Manage widgets" : "Create widget"}<ChevronRight data-slot="icon" /></Button></section>}
  </>;
}

function CustomerAccessPanel({ accounts, status, hasMore, onUpdate, onLoadMore }: { accounts: APICustomerAccount[]; status: "loading" | "ready" | "unavailable"; hasMore: boolean; onUpdate: (account: APICustomerAccount, state: APICustomerAccount["state"]) => Promise<boolean>; onLoadMore: () => Promise<boolean> }) {
  const [pendingSuspension, setPendingSuspension] = useState<string | null>(null);
  const [busyAccount, setBusyAccount] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);

  async function updateAccount(account: APICustomerAccount, state: APICustomerAccount["state"]) {
    setBusyAccount(account.id);
    try {
      if (await onUpdate(account, state)) setPendingSuspension(null);
    } finally {
      setBusyAccount(null);
    }
  }

  async function loadMore() {
    setLoadingMore(true);
    try {
      await onLoadMore();
    } finally {
      setLoadingMore(false);
    }
  }

  return <section className="panel customer-access-panel">
    <PanelHeader title="Customer access" description="Suspend a compromised customer account without changing the shared OIDC connection." action={status === "ready" ? <Badge color="zinc">{hasMore ? `${accounts.length} loaded` : `${accounts.length} account${accounts.length === 1 ? "" : "s"}`}</Badge> : undefined} />
    {status === "loading" && <div className="customer-access-state" aria-live="polite"><RefreshCw /><span><strong>Loading customer accounts</strong><small>Suspension controls stay unavailable until live account state is verified.</small></span></div>}
    {status === "unavailable" && <div className="customer-access-state unavailable" role="status"><TriangleAlert /><span><strong>Customer accounts unavailable</strong><small>Live account state could not be verified. No suspension controls are shown; reload the page to try again.</small></span></div>}
    {status === "ready" && accounts.length === 0 && <div className="customer-access-empty"><Users /><span><strong>No customer accounts yet</strong><small>Accounts appear after the first successful customer sign-in.</small></span></div>}
    {status === "ready" && accounts.length > 0 && <div className="customer-access-list">{accounts.map((account) => {
      const confirming = pendingSuspension === account.id;
      const busy = busyAccount === account.id;
      return <article className="customer-access-row" key={account.id}>
        <span className="customer-access-identity"><strong>{account.external_id}</strong><small>Issuer {account.issuer} · Last sign-in {account.last_authenticated_at ? new Date(account.last_authenticated_at).toLocaleString() : "never"}</small></span>
        <Badge color={account.state === "active" ? "green" : "red"}>{account.state === "active" ? "Active" : "Suspended"}</Badge>
        {account.state === "active" ? confirming ? null : <Button outline disabled={busyAccount !== null} onClick={() => setPendingSuspension(account.id)}>Suspend</Button> : <Button outline disabled={busyAccount !== null} onClick={() => void updateAccount(account, "active")}>{busy ? "Reactivating…" : "Reactivate"}</Button>}
        {confirming && <div className="customer-access-confirm" role="alert"><TriangleAlert /><span><strong>Suspend {account.external_id}?</strong><small>New sign-ins and existing customer access will fail closed immediately.</small></span><span className="heading-actions"><Button outline disabled={busy} onClick={() => setPendingSuspension(null)}>Cancel</Button><Button color="red" disabled={busy} onClick={() => void updateAccount(account, "suspended")}>{busy ? "Suspending…" : "Suspend customer"}</Button></span></div>}
      </article>;
    })}{hasMore && <div className="customer-access-more"><Button outline disabled={loadingMore || busyAccount !== null} onClick={() => void loadMore()}>{loadingMore ? "Loading…" : "Load more"}</Button></div>}</div>}
  </section>;
}

function AgentSetupCard({ kind, tenantName, setup, onCopied, onConfigureIdentity }: { kind: "public" | "private"; tenantName: string; setup: Distribution["agent_setup"]["public"]; onCopied: (label: string) => void; onConfigureIdentity: () => void }) {
  const isPublic = kind === "public";
  const title = isPublic ? "Public MCP button" : "Private MCP button";
  return <article className={`agent-setup-card ${!setup.available ? "agent-setup-disabled" : ""}`}>
    <div className={`agent-setup-preview ${isPublic ? "public-agent-preview" : "private-agent-preview"}`}>
      <a href={setup.available ? setup.url : undefined} target="_blank" rel="noopener noreferrer" aria-disabled={!setup.available} aria-label={`Connect your agent to ${tenantName} using ${kind} MCP`} onClick={(event) => { if (!setup.available) event.preventDefault(); }}>
        <span className="agent-setup-label">Connect your agent to {tenantName}</span>
        <span className={`agent-access-chip ${kind}`}>{isPublic ? "Public" : "Private"}</span>
        {/* eslint-disable-next-line @next/next/no-img-element -- These tiny vendor SVG marks are served unchanged from the public asset contract. */}
        {agentClients.map((client) => <img key={client.id} className="agent-client-mark" src={`/agent-client-icons/${client.file}`} alt={client.name} title={client.name} data-agent-client={client.id} />)}
      </a>
    </div>
    <div className="agent-setup-copy">
      <Badge color={isPublic ? "blue" : "violet"}>{isPublic ? <Globe2 /> : <LockKeyhole />}{isPublic ? "Public" : "Private"}</Badge>
      <h3>{title}</h3>
      {setup.available ? <a className="agent-setup-guide-link" href={setup.url} target="_blank" rel="noopener noreferrer"><ExternalLink />Open setup instructions</a> : <div className="inline-warning"><TriangleAlert />{isPublic ? "Enable Public MCP before distributing this button." : "Configure and activate customer identity before distributing this button."}</div>}
      {!isPublic && !setup.available && <Button outline className="agent-identity-action" onClick={onConfigureIdentity}>Configure identity</Button>}
      <CopyButton text={setup.embed_html} label={`Copy ${kind} MCP button`} disabled={!setup.available} onCopied={() => onCopied(`${isPublic ? "Public" : "Private"} MCP button copied.`)} />
    </div>
  </article>;
}

export function SourcesView({ sources, onAdd, onCrawl, onPublish, onVisibilityChange, onNavigate }: { sources: Source[]; onAdd: () => void; onCrawl: (id: string) => void; onPublish: (source: Source) => void; onVisibilityChange: (id: string) => void; onNavigate: (path: string) => void }) {
  return <>
    <PageHeading eyebrow="Knowledge" title="Sources" action={<Button onClick={onAdd}><Plus data-slot="icon" />Add source</Button>} />
    <div className="summary-strip"><SummaryItem label="Pages indexed" value="378" icon={<Database />} /><SummaryItem label="Healthy sources" value="1 of 3" icon={<CheckCircle2 />} /><SummaryItem label="Needs attention" value="2" icon={<AlertCircle />} /></div>
    <div className="toolbar"><div className="search-field"><Search /><input aria-label="Search sources" placeholder="Search sources…" /></div><Button outline onClick={() => sources.forEach((source) => onCrawl(source.id))}><RefreshCw data-slot="icon" />Crawl all</Button></div>
    <DataTable label="Sources">
      <DataTableHeader className="source-columns"><span>Source</span><span>Crawl state</span><span>Content</span><span>Visibility</span><span>Actions</span></DataTableHeader>
      {sources.map((source) => <DataTableRow className="source-columns" key={source.id}>
        <span className="resource-name"><span className="resource-icon"><BookOpen /></span><span><EntityLink entity="source" uid={source.id} onNavigate={onNavigate} className="entity-link"><strong>{source.name}</strong></EntityLink><small>{source.location} · {source.kind}</small></span></span>
        <span><CrawlBadge state={source.crawlState} /><small className="cell-note">{source.lastCrawl}</small></span>
        <span><strong className="cell-value">{source.pages}</strong><small className="cell-note">pages</small></span>
        <span className="visibility-control"><Badge color={source.visibility === "public" ? "green" : "zinc"}>{source.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{source.visibility}</Badge><Switch checked={source.visibility === "public"} onChange={() => onVisibilityChange(source.id)} label={`Make ${source.name} ${source.visibility === "public" ? "private" : "public"}`} /></span>
        <span className="table-actions">{source.crawlState === "review" && <Button outline onClick={() => onPublish(source)}>{source.quarantined ? "Inspect" : "Review"}</Button>}<button type="button" className="more" aria-label={`Crawl ${source.name}`} title="Queue crawl" onClick={() => onCrawl(source.id)}><RefreshCw /></button></span>
      </DataTableRow>)}
    </DataTable>
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
