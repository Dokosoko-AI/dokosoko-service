"use client";

import { CheckCircle2, KeyRound, RefreshCw, RotateCcw, TriangleAlert } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import {
  APIError,
  APIIntegration,
  APIRuntimeAuthenticationType,
  APIRuntimeCredentialVersion,
  APIRuntimeSetup,
  api,
} from "../../lib/api";
import { Badge, Button, Dialog } from "../core/control";
import { PanelHeader } from "../core/layout";
import { AuthorizationHeaderManager, authorizationHeaderDraft } from "./AuthorizationHeaderManager";
import type { AuthorizationHeaderDraft } from "./AuthorizationHeaderManager";

type AuthorizationMethod = Exclude<APIRuntimeAuthenticationType, "none" | "delegated_oauth" | "authorization_scheme" | "api_key_query">;
type Mode = "current" | "existing" | "new";

const methods: Array<{ value: AuthorizationMethod; label: string }> = [
  { value: "api_key_header", label: "API key header" },
  { value: "bearer", label: "Bearer token" },
  { value: "custom_header", label: "Custom header" },
  { value: "basic", label: "Basic Auth" },
  { value: "oauth_client_credentials", label: "OAuth 2.0 client credentials" },
];

function message(error: unknown, fallback: string) {
  return error instanceof APIError || error instanceof Error ? error.message : fallback;
}

function productionEnvironment(setup: APIRuntimeSetup) {
  return setup.environments.find((environment) => environment.is_production) ?? setup.environments[0];
}

function currentBinding(setup: APIRuntimeSetup) {
  const environment = productionEnvironment(setup);
  if (!environment) return {};
  for (const connection of setup.endpoint_bindings) {
    const revision = connection.current_revisions?.find((candidate) => candidate.current && candidate.environment_id === environment.id);
    if (revision) {
      return {
        environment,
        connection,
        revision,
        authorization: setup.authorizations.find((candidate) => candidate.id === revision.authorization_id),
      };
    }
  }
  return { environment };
}

function validURL(value: string, fetched = false) {
  try {
    const parsed = new URL(value);
    const hostname = parsed.hostname.toLowerCase();
    const local = hostname === "localhost" || hostname.endsWith(".localhost") || hostname === "::1" || /^127(?:\.\d{1,3}){3}$/.test(hostname);
    if (parsed.username || parsed.password || parsed.search || parsed.hash || !hostname) return false;
    if (!fetched) return parsed.protocol === "https:" || parsed.protocol === "http:" && local;
    return local ? parsed.protocol === "http:" : parsed.protocol === "https:" && (parsed.port === "" || parsed.port === "443");
  } catch {
    return false;
  }
}

function methodLabel(value: APIRuntimeAuthenticationType) {
  return methods.find((method) => method.value === value)?.label ?? value.replaceAll("_", " ");
}

function configFor(method: AuthorizationMethod, prefix: string, username: string, clientID: string, tokenURL: string, scopes: string) {
  if (method === "custom_header") return prefix.trim() ? { prefix: prefix.trim() } : {};
  if (method === "basic") return { username: username.trim() };
  if (method === "oauth_client_credentials") return {
    client_id: clientID.trim(),
    token_url: tokenURL.trim(),
    token_endpoint_auth_method: "client_secret_basic",
    scopes: scopes.split(/[\s,]+/).map((scope) => scope.trim()).filter(Boolean),
  };
  return {};
}

function secretLabel(method: AuthorizationMethod) {
  if (method === "oauth_client_credentials") return "Client secret";
  if (method === "basic") return "Password";
  if (method === "bearer") return "Bearer token";
  return "API key";
}

function usesPrimaryHeader(method: AuthorizationMethod) {
  return method === "api_key_header" || method === "custom_header";
}

const forbiddenAuthorizationHeaders = new Set(["authorization", "proxy-authorization", "cookie", "set-cookie", "host", "content-length", "transfer-encoding", "connection", "upgrade", "te", "trailer", "forwarded", "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto", "x-forwarded-uri", "x-http-method", "x-http-method-override", "x-method-override", "x-original-url", "x-original-uri", "x-rewrite-url", "x-envoy-original-path"]);

function validManagedHeaderName(value: string) {
  const name = value.trim();
  return /^[!#$%&'*+.^_`|~0-9A-Za-z-]{1,100}$/.test(name) && !forbiddenAuthorizationHeaders.has(name.toLowerCase());
}

function profileHeaderNames(profile: APIRuntimeSetup["authorizations"][number]) {
  const configured = Array.isArray(profile.auth_config?.headers) ? profile.auth_config.headers.filter((name): name is string => typeof name === "string") : [];
  return usesPrimaryHeader(profile.authentication_type as AuthorizationMethod) ? [profile.header_name ?? "X-API-Key", ...configured] : configured;
}

export function IntegrationAuthorization({ integration, onMessage, onChanged }: {
  integration: APIIntegration;
  onMessage: (message: string) => void;
  onChanged?: () => void | Promise<void>;
}) {
  const [setup, setSetup] = useState<APIRuntimeSetup | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [formError, setFormError] = useState("");
  const [busy, setBusy] = useState(false);
  const [mode, setMode] = useState<Mode>("current");
  const [existingID, setExistingID] = useState("");
  const [usageCount, setUsageCount] = useState<number | null>(null);

  const [method, setMethod] = useState<AuthorizationMethod>("api_key_header");
  const [environmentVariable, setEnvironmentVariable] = useState("API_KEY_ENV");
  const [prefix, setPrefix] = useState("");
  const [username, setUsername] = useState("");
  const [clientID, setClientID] = useState("");
  const [tokenURL, setTokenURL] = useState("");
  const [scopes, setScopes] = useState("");
  const [keyManagementURL, setKeyManagementURL] = useState("");
  const [accessEvaluationURL, setAccessEvaluationURL] = useState("");
  const [usageURL, setUsageURL] = useState("");
  const [credential, setCredential] = useState("");
  const [headers, setHeaders] = useState<AuthorizationHeaderDraft[]>(() => [authorizationHeaderDraft("X-API-Key")]);
  const [baseURL, setBaseURL] = useState("");

  const [rotationOpen, setRotationOpen] = useState(false);
  const [rotationCredential, setRotationCredential] = useState("");
  const [rotationExpiresAt, setRotationExpiresAt] = useState("");
  const [revokeVersion, setRevokeVersion] = useState<APIRuntimeCredentialVersion | null>(null);

  const binding = useMemo(() => setup ? currentBinding(setup) : {}, [setup]);
  const authorization = binding.authorization;
  const alternatives = setup?.authorizations.filter((candidate) => candidate.state === "active" && candidate.credential_present && candidate.id !== authorization?.id && candidate.environment_id === binding.environment?.id) ?? [];

  const hydrate = useCallback((value: APIRuntimeSetup) => {
    const selected = currentBinding(value);
    const profile = selected.authorization;
    setBaseURL(selected.revision?.base_url ?? "");
    setMode(profile ? "current" : value.authorizations.length > 0 ? "existing" : "new");
    setExistingID(value.authorizations.find((candidate) => candidate.state === "active" && candidate.credential_present && candidate.environment_id === selected.environment?.id)?.id ?? "");
    if (profile) {
      const config = profile.auth_config ?? {};
      setMethod(profile.authentication_type as AuthorizationMethod);
      setEnvironmentVariable(profile.environment_variable);
      setPrefix(typeof config.prefix === "string" ? config.prefix : "");
      setUsername(typeof config.username === "string" ? config.username : "");
      setClientID(typeof config.client_id === "string" ? config.client_id : "");
      setTokenURL(typeof config.token_url === "string" ? config.token_url : "");
      setScopes(Array.isArray(config.scopes) ? config.scopes.filter((scope): scope is string => typeof scope === "string").join(" ") : "");
      setHeaders(profileHeaderNames(profile).map((name) => authorizationHeaderDraft(name)));
      setKeyManagementURL(profile.key_management_url ?? "");
      setAccessEvaluationURL(profile.access_evaluation_url);
      setUsageURL(profile.usage_url);
    } else {
      setHeaders([authorizationHeaderDraft("X-API-Key")]);
    }
    setCredential("");
    setFormError("");
    setUsageCount(null);
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError("");
    try {
      const value = await api.integrationAuthorization(integration.id);
      setSetup(value);
      hydrate(value);
    } catch (error) {
      setLoadError(message(error, "Authorization could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [hydrate, integration.id]);

  useEffect(() => {
    const timeout = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timeout);
  }, [load]);

  useEffect(() => {
    if (!authorization) return;
    let cancelled = false;
    void api.authorizationUsage(authorization.id).then((value) => {
      if (!cancelled) setUsageCount(value.count);
    }).catch(() => {
      if (!cancelled) setUsageCount(-1);
    });
    return () => { cancelled = true; };
  }, [authorization]);

  function validateProfile() {
    if (!/^[A-Z][A-Z0-9_]{0,127}$/.test(environmentVariable.trim())) return "API_KEY_ENV must use upper-case letters, numbers, and underscores.";
    if (!validURL(keyManagementURL.trim()) || !validURL(accessEvaluationURL.trim(), true) || !validURL(usageURL.trim(), true)) return "Enter valid HTTPS URLs. Localhost HTTP is allowed for development.";
    if (usesPrimaryHeader(method) && headers.length === 0) return "Add at least one Authorization header.";
    const seenHeaders = new Set<string>();
    const storedHeaders = new Set(authorization ? profileHeaderNames(authorization).map((name) => name.toLowerCase()) : []);
    for (const header of headers) {
      const name = header.name.trim();
      const key = name.toLowerCase();
      if (!validManagedHeaderName(name)) return `Enter a safe HTTP header name for header ${headers.indexOf(header) + 1}.`;
      if (seenHeaders.has(key)) return `Header ${name} is duplicated.`;
      if (!header.value && mode === "new") return `Enter a value for header ${name}.`;
      if (!header.value && mode === "current" && !storedHeaders.has(key)) return `Enter a value for the new header ${name}.`;
      seenHeaders.add(key);
    }
    if (method === "basic" && (!username.trim() || /[:\r\n\0]/.test(username))) return "Enter a valid Basic Auth username.";
    if (method === "oauth_client_credentials" && (!clientID.trim() || !validURL(tokenURL.trim(), true))) return "Enter the OAuth client ID and token URL.";
    return "";
  }

  async function saveCurrent() {
    if (!authorization) return;
    const problem = validateProfile();
    if (problem) { setFormError(problem); return; }
    setBusy(true);
    setFormError("");
    try {
      const primaryHeader = usesPrimaryHeader(method) ? headers[0] : undefined;
      const additionalHeaders = usesPrimaryHeader(method) ? headers.slice(1) : headers;
      await api.updateAuthorization(authorization.id, {
        environment_variable: environmentVariable.trim(),
        header_name: primaryHeader?.name.trim(),
        auth_config: configFor(method, prefix, username, clientID, tokenURL, scopes),
        key_management_url: keyManagementURL.trim(),
        access_evaluation_url: accessEvaluationURL.trim(),
        usage_url: usageURL.trim(),
        credential: primaryHeader?.value || undefined,
        additional_headers: additionalHeaders.map((header) => ({ name: header.name.trim(), value: header.value })),
        state: authorization.state,
        revision: authorization.revision,
      });
      await load();
      await onChanged?.();
      onMessage("Authorization saved.");
    } catch (error) {
      setFormError(message(error, "Authorization could not be saved."));
    } finally { setBusy(false); }
  }

  async function bindExisting() {
    const profile = setup?.authorizations.find((candidate) => candidate.id === existingID);
    if (!setup || !binding.environment || !profile || !baseURL) return;
    setBusy(true);
    setFormError("");
    try {
      await api.configureIntegrationAuthorization(integration.id, {
        environment_id: binding.environment.id,
        connection_name: binding.connection?.name ?? "Default",
        connection_description: binding.connection?.description ?? "",
        base_url: baseURL,
        authentication_type: profile.authentication_type,
        authorization_id: profile.id,
      });
      await load();
      await onChanged?.();
      onMessage(`${profile.name} is now connected to this API.`);
    } catch (error) {
      setFormError(message(error, "Authorization could not be connected."));
    } finally { setBusy(false); }
  }

  async function createAndBind() {
    if (!setup || !binding.environment) { setFormError("Create a deployment environment first."); return; }
    const problem = validateProfile();
    if (problem) { setFormError(problem); return; }
    if (!usesPrimaryHeader(method) && !credential.trim()) { setFormError(`Enter the ${secretLabel(method).toLowerCase()}.`); return; }
    if (!validURL(baseURL.trim(), true)) { setFormError("Enter the API base URL before connecting Authorization."); return; }
    setBusy(true);
    setFormError("");
    try {
      const primaryHeader = usesPrimaryHeader(method) ? headers[0] : undefined;
      const additionalHeaders = usesPrimaryHeader(method) ? headers.slice(1) : headers;
      await api.configureIntegrationAuthorization(integration.id, {
        environment_id: binding.environment.id,
        connection_name: binding.connection?.name ?? "Default",
        connection_description: binding.connection?.description ?? "",
        base_url: baseURL.trim(),
        authentication_type: method,
        auth_config: configFor(method, prefix, username, clientID, tokenURL, scopes),
        environment_variable: environmentVariable.trim(),
        header_name: primaryHeader?.name.trim(),
        key_management_url: keyManagementURL.trim(),
        access_evaluation_url: accessEvaluationURL.trim(),
        usage_url: usageURL.trim(),
        credential: primaryHeader?.value ?? credential,
        additional_headers: additionalHeaders.map((header) => ({ name: header.name.trim(), value: header.value })),
      });
      await load();
      await onChanged?.();
      onMessage("Authorization created and connected.");
    } catch (error) {
      setFormError(message(error, "Authorization could not be created."));
    } finally { setBusy(false); }
  }

  async function rotate() {
    if (!authorization || !rotationCredential.trim()) return;
    setBusy(true);
    try {
      await api.rotateAuthorizationCredential(authorization.id, rotationCredential, rotationExpiresAt ? new Date(rotationExpiresAt).toISOString() : undefined);
      setRotationOpen(false);
      setRotationCredential("");
      setRotationExpiresAt("");
      await load();
      await onChanged?.();
      onMessage("Credential rotated. Connected APIs now use the new active version.");
    } catch (error) { onMessage(message(error, "Credential could not be rotated.")); }
    finally { setBusy(false); }
  }

  async function revoke() {
    if (!authorization || !revokeVersion) return;
    setBusy(true);
    try {
      await api.revokeAuthorizationCredentialVersion(authorization.id, revokeVersion.id);
      setRevokeVersion(null);
      await load();
      await onChanged?.();
      onMessage("Credential version revoked.");
    } catch (error) { onMessage(message(error, "Credential version could not be revoked.")); }
    finally { setBusy(false); }
  }

  if (loading && !setup) return <section className="panel runtime-access-panel"><PanelHeader title="Authorization" description="Loading the API Authorization binding." /><div className="runtime-access-loading"><RefreshCw /><span>Loading Authorization…</span></div></section>;
  if (loadError && !setup) return <section className="panel runtime-access-panel"><PanelHeader title="Authorization" /><div className="capability-unavailable"><TriangleAlert /><span><strong>Authorization is unavailable</strong><small>{loadError}</small></span><Button outline onClick={() => void load()}>Retry</Button></div></section>;
  if (!setup) return null;

  return <>
    <section className="panel runtime-access-panel">
      <PanelHeader
        title="Authorization"
        description="One reusable Authorization owns the authentication method, secret, API_KEY_ENV, and provider hooks. This API stores only the binding."
        action={authorization ? <span className="heading-actions"><Badge color="green">Connected</Badge><Button outline onClick={() => setMode("existing")}>Use existing</Button><Button outline onClick={() => { setMode("new"); setCredential(""); }}>Create new</Button></span> : undefined}
      />

      {binding.revision && <div className="runtime-current-summary">
        <span className="settings-icon"><CheckCircle2 /></span>
        <span><strong>{integration.display_name}</strong><small>{binding.revision.base_url}</small></span>
        <span><small>Environment</small><strong>{binding.environment?.name}</strong></span>
        <span><small>Authorization</small><strong>{authorization?.name ?? "Not connected"}</strong></span>
      </div>}

      {mode === "existing" && <div className="runtime-access-form">
        <label className="auth-field"><span>Existing Authorization</span><select value={existingID} onChange={(event) => setExistingID(event.target.value)}><option value="">Choose Authorization</option>{alternatives.map((profile) => <option key={profile.id} value={profile.id}>{profile.name} · {profile.environment_variable} · {methodLabel(profile.authentication_type)}</option>)}</select><small>Reuses the same secret and hook configuration. Nothing is copied into this API.</small></label>
        {alternatives.length === 0 && <div className="empty-row">No other active Authorization is available in this environment.</div>}
        <div className="runtime-save-row"><Button outline onClick={() => setMode("current")}>Cancel</Button><Button color="indigo" disabled={busy || !existingID} onClick={() => void bindExisting()}>{busy ? "Connecting…" : "Connect Authorization"}</Button></div>
      </div>}

      {(mode === "new" || mode === "current" && authorization) && <div className="runtime-access-form">
        <section className="runtime-form-section" aria-labelledby="runtime-authorization-section">
          <header className="runtime-form-section-heading"><h3 id="runtime-authorization-section">Authorization</h3><p>Choose the authentication method, then manage the fixed headers sent with each request.</p></header>
          <label className="auth-field"><span>Method</span><select value={method} disabled={mode === "current"} onChange={(event) => { const value = event.target.value as AuthorizationMethod; const hadPrimaryHeader = usesPrimaryHeader(method); setMethod(value); if (usesPrimaryHeader(value) && !hadPrimaryHeader) setHeaders([authorizationHeaderDraft(value === "custom_header" ? "X-Custom-Auth" : "X-API-Key")]); if (!usesPrimaryHeader(value) && hadPrimaryHeader) setHeaders([]); }} >{methods.map((value) => <option key={value.value} value={value.value}>{value.label}</option>)}</select><small>{mode === "current" ? "Create a new Authorization to change the authentication method." : "Postman-style upstream authentication."}</small></label>
          {mode === "new" && !usesPrimaryHeader(method) && <label className="auth-field"><span>{secretLabel(method)}</span><input type="password" value={credential} maxLength={16384} onChange={(event) => setCredential(event.target.value)} placeholder="************" autoComplete="new-password" /></label>}
          {method === "basic" && <label className="auth-field"><span>Username</span><input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" /></label>}
          {method === "oauth_client_credentials" && <><div className="two-fields"><label className="auth-field"><span>Client ID</span><input value={clientID} onChange={(event) => setClientID(event.target.value)} /></label><label className="auth-field"><span>Token URL</span><input type="url" value={tokenURL} onChange={(event) => setTokenURL(event.target.value)} placeholder="https://identity.example.com/oauth/token" /></label></div><label className="auth-field"><span>Scopes</span><input value={scopes} onChange={(event) => setScopes(event.target.value)} placeholder="read write" /></label></>}
          <AuthorizationHeaderManager headers={headers} onChange={setHeaders} required={usesPrimaryHeader(method)} />
        </section>

        <section className="runtime-form-section" aria-labelledby="runtime-env-section">
          <header className="runtime-form-section-heading"><h3 id="runtime-env-section">ENV</h3><p>Name the environment variable coding agents should use for this credential.</p></header>
          <label className="auth-field"><input aria-label="Environment variable" value={environmentVariable} maxLength={128} onChange={(event) => setEnvironmentVariable(event.target.value.toUpperCase())} placeholder="API_KEY_ENV" /></label>
        </section>

        <section className="runtime-form-section" aria-labelledby="runtime-urls-section">
          <header className="runtime-form-section-heading"><h3 id="runtime-urls-section">URLs</h3><p>Configure the API endpoint, operator destination, and provider hooks.</p></header>
          {mode === "new" && !binding.revision && <label className="auth-field"><span>API base URL</span><input type="url" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder="https://api.example.com" /><small>This is the API endpoint binding, not part of the reusable Authorization.</small></label>}
          <label className="auth-field"><span>Key management URL</span><input type="url" value={keyManagementURL} onChange={(event) => setKeyManagementURL(event.target.value)} placeholder="https://dashboard.example.com/api-keys" /><small>Operator link only. DokoSoko does not fetch it.</small></label>
          <div className="two-fields"><label className="auth-field"><span>Access evaluation URL</span><input type="url" value={accessEvaluationURL} onChange={(event) => setAccessEvaluationURL(event.target.value)} placeholder="https://api.example.com/hooks/access-evaluation" /><small>Synchronous. Timeout, transport, status, and malformed responses deny execution.</small></label><label className="auth-field"><span>Usage URL</span><input type="url" value={usageURL} onChange={(event) => setUsageURL(event.target.value)} placeholder="https://api.example.com/hooks/usage" /><small>Queued after execution and delivered asynchronously.</small></label></div>
        </section>

        {formError && <div className="auth-problem"><TriangleAlert /><span>{formError}</span></div>}
        <div className="runtime-save-row"><span>{authorization ? `${usageCount === null ? "Loading usage…" : usageCount < 0 ? "Usage unavailable" : `${usageCount} connected API${usageCount === 1 ? "" : "s"}`} · credential ${authorization.active_fingerprint ?? "missing"}` : "The secret is write-only and never returned."}</span><span className="heading-actions">{mode === "new" && authorization && <Button outline onClick={() => setMode("current")}>Cancel</Button>}<Button color="indigo" disabled={busy} onClick={() => void (mode === "current" ? saveCurrent() : createAndBind())}>{busy ? "Saving…" : mode === "current" ? "Save Authorization" : "Create & connect"}</Button></span></div>
      </div>}
    </section>

    {authorization && mode === "current" && <section className="panel runtime-credential-management">
      <PanelHeader title="Credential lifecycle" description="Rotate the write-only secret without changing API bindings." action={<Button outline onClick={() => setRotationOpen(true)}><RotateCcw data-slot="icon" />Rotate</Button>} />
      <div className="runtime-credential-set">
        <div className="runtime-credential-heading"><span className="settings-icon"><KeyRound /></span><span><strong>{authorization.environment_variable}</strong><small>{methodLabel(authorization.authentication_type)} · revision {authorization.revision}</small></span><Badge color={authorization.credential_present ? "green" : "amber"}>{authorization.credential_present ? "Active" : "Missing"}</Badge></div>
        <div className="runtime-version-list">{authorization.versions?.map((version) => <div key={version.id}><span><strong>{version.state}</strong><small>{version.fingerprint} · {new Date(version.created_at).toLocaleString()}</small></span>{version.state !== "revoked" && <Button outline disabled={busy} onClick={() => setRevokeVersion(version)}>Revoke</Button>}</div>)}</div>
      </div>
    </section>}

    <Dialog open={rotationOpen} onClose={(open) => { if (!open && !busy) setRotationOpen(false); }} title="Rotate credential" description="The new version becomes active for every API using this Authorization." actions={<><Button outline disabled={busy} onClick={() => setRotationOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !rotationCredential.trim()} onClick={() => void rotate()}>{busy ? "Rotating…" : "Rotate"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>New {authorization ? secretLabel(authorization.authentication_type as AuthorizationMethod).toLowerCase() : "credential"}</span><input type="password" value={rotationCredential} onChange={(event) => setRotationCredential(event.target.value)} placeholder="************" autoComplete="new-password" /></label><label className="auth-field"><span>Expires (optional)</span><input type="datetime-local" value={rotationExpiresAt} onChange={(event) => setRotationExpiresAt(event.target.value)} /></label></div></Dialog>
    <Dialog open={Boolean(revokeVersion)} onClose={(open) => { if (!open && !busy) setRevokeVersion(null); }} title="Revoke credential version" description="Revocation is immediate and cannot be undone. Rotate first if this is the active version." actions={<><Button outline disabled={busy} onClick={() => setRevokeVersion(null)}>Cancel</Button><Button color="red" disabled={busy} onClick={() => void revoke()}>{busy ? "Revoking…" : "Revoke"}</Button></>}><div className="runtime-revoke-summary"><TriangleAlert /><span><strong>{authorization?.name}</strong><small>{revokeVersion?.fingerprint}</small></span></div></Dialog>
  </>;
}
