"use client";

import { CheckCircle2, KeyRound, RefreshCw, RotateCcw, ShieldCheck, TriangleAlert } from "lucide-react";
import { useCallback, useMemo, useState, useEffect } from "react";
import {
  APIError,
  APIIntegration,
  APIRuntimeAuthenticationType,
  APIRuntimeCredentialSet,
  APIRuntimeCredentialVersion,
  APIRuntimeServiceConnection,
  APIRuntimeServiceConnectionReadiness,
  APIRuntimeServiceConnectionRevision,
  APIRuntimeSetup,
  api,
} from "../../lib/api";
import { integrationPath, integrationToolBuilderPath } from "../../lib/console-routes";
import { Badge, Button, Dialog } from "../core/control";
import { PanelHeader } from "../core/layout";

type CredentialChoice = "dedicated" | "shared" | "existing";

const authenticationLabels: Record<APIRuntimeAuthenticationType, string> = {
  none: "No authentication",
  delegated_oauth: "Customer OAuth token",
  api_key_header: "API key header",
  bearer: "Bearer token",
  authorization_scheme: "Authorization scheme",
  api_key_query: "API key query parameter",
  basic: "Basic authentication",
  oauth_client_credentials: "OAuth client credentials",
  custom_header: "Custom header",
};

const commonAuthenticationTypes: APIRuntimeAuthenticationType[] = ["none", "api_key_header", "bearer", "delegated_oauth"];
const advancedAuthenticationTypes: APIRuntimeAuthenticationType[] = ["authorization_scheme", "api_key_query", "basic", "oauth_client_credentials", "custom_header"];

function errorMessage(error: unknown, fallback: string) {
  return error instanceof APIError || error instanceof Error ? error.message : fallback;
}

function needsCredential(authenticationType: APIRuntimeAuthenticationType) {
  return authenticationType !== "none" && authenticationType !== "delegated_oauth";
}

function defaultEnvironmentVariable(integration: APIIntegration, scope: Exclude<CredentialChoice, "existing">) {
  if (scope === "shared") return "SERVICE_API_KEY";
  const family = integration.family_key
    .toUpperCase()
    .replace(/[^A-Z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .replace(/_API$/, "");
  return `${family || "SERVICE"}_API_KEY`;
}

function currentConnectionForEnvironment(setup: APIRuntimeSetup, environmentID: string): {
  connection?: APIRuntimeServiceConnection;
  revision?: APIRuntimeServiceConnectionRevision;
} {
  for (const connection of setup.service_connections) {
    const revision = connection.current_revisions?.find((candidate) => candidate.current && candidate.environment_id === environmentID);
    if (revision) return { connection, revision };
  }
  return {};
}

function currentCredential(setup: APIRuntimeSetup, revision?: APIRuntimeServiceConnectionRevision) {
  return revision?.credential_set_id ? setup.credential_sets.find((credentialSet) => credentialSet.id === revision.credential_set_id) : undefined;
}

function prettyAuthentication(authenticationType: APIRuntimeAuthenticationType) {
  return authenticationLabels[authenticationType] ?? authenticationType;
}

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export function IntegrationRuntimeAccess({ integration, onMessage, onNavigate, onChanged }: {
  integration: APIIntegration;
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
  onChanged?: () => void | Promise<void>;
}) {
  const [setup, setSetup] = useState<APIRuntimeSetup | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);
  const [environmentID, setEnvironmentID] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [authenticationType, setAuthenticationType] = useState<APIRuntimeAuthenticationType>("api_key_header");
  const [credentialChoice, setCredentialChoice] = useState<CredentialChoice>("dedicated");
  const [existingCredentialSetID, setExistingCredentialSetID] = useState("");
  const [credential, setCredential] = useState("");
  const [headerName, setHeaderName] = useState("X-API-Key");
  const [connectionName, setConnectionName] = useState("");
  const [connectionDescription, setConnectionDescription] = useState("");
  const [credentialName, setCredentialName] = useState("");
  const [environmentVariable, setEnvironmentVariable] = useState("");
  const [credentialExpiresAt, setCredentialExpiresAt] = useState("");
  const [authConfig, setAuthConfig] = useState("{}");
  const [usageCounts, setUsageCounts] = useState<Record<string, number>>({});
  const [usageLoaded, setUsageLoaded] = useState(false);
  const [rotateSet, setRotateSet] = useState<APIRuntimeCredentialSet | null>(null);
  const [rotationCredential, setRotationCredential] = useState("");
  const [rotationExpiresAt, setRotationExpiresAt] = useState("");
  const [revokeTarget, setRevokeTarget] = useState<{ credentialSet: APIRuntimeCredentialSet; version: APIRuntimeCredentialVersion } | null>(null);
  const [credentialBusy, setCredentialBusy] = useState(false);
  const [configurationCheck, setConfigurationCheck] = useState<APIRuntimeServiceConnectionReadiness | null>(null);
  const [checkingConfiguration, setCheckingConfiguration] = useState(false);

  const hydrateForm = useCallback((value: APIRuntimeSetup, selectedEnvironmentID: string) => {
    const selected = selectedEnvironmentID || value.environments.find((environment) => environment.is_production)?.id || value.environments[0]?.id || "";
    const { connection, revision } = currentConnectionForEnvironment(value, selected);
    const credentialSet = currentCredential(value, revision);
    const nextAuthenticationType = revision?.authentication_type ?? "api_key_header";
    setEnvironmentID(selected);
    setBaseURL(revision?.base_url ?? "");
    setAuthenticationType(nextAuthenticationType);
    setConnectionName(connection?.name ?? `${integration.display_name} service`);
    setConnectionDescription(connection?.description ?? "");
    setAuthConfig(JSON.stringify(revision?.auth_config ?? {}, null, 2));
    setCredential("");
    setCredentialExpiresAt("");
    if (credentialSet) {
      setCredentialChoice("existing");
      setExistingCredentialSetID(credentialSet.id);
      setCredentialName(credentialSet.name);
      setEnvironmentVariable(credentialSet.environment_variable);
      setHeaderName(credentialSet.header_name ?? (nextAuthenticationType === "api_key_header" || nextAuthenticationType === "custom_header" ? "X-API-Key" : ""));
    } else {
      setCredentialChoice("dedicated");
      setExistingCredentialSetID("");
      setCredentialName(`${integration.display_name} credential`);
      setEnvironmentVariable(defaultEnvironmentVariable(integration, "dedicated"));
      setHeaderName(nextAuthenticationType === "api_key_header" || nextAuthenticationType === "custom_header" ? "X-API-Key" : "");
    }
  }, [integration]);

  const loadSetup = useCallback(async () => {
    setLoading(true);
    setLoadError("");
    try {
      const value = await api.integrationRuntimeSetup(integration.id);
      setSetup(value);
      hydrateForm(value, environmentID);
      setUsageLoaded(false);
      setUsageCounts({});
    } catch (error) {
      setLoadError(errorMessage(error, "Runtime service access could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [environmentID, hydrateForm, integration.id]);

  useEffect(() => {
    let cancelled = false;
    void api.integrationRuntimeSetup(integration.id).then((value) => {
      if (cancelled) return;
      setSetup(value);
      hydrateForm(value, "");
      setLoadError("");
    }).catch((error) => {
      if (!cancelled) setLoadError(errorMessage(error, "Runtime service access could not be loaded."));
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });
    return () => { cancelled = true; };
  }, [hydrateForm, integration.id]);

  const current = setup ? currentConnectionForEnvironment(setup, environmentID) : {};
  const selectedCurrentCredential = setup ? currentCredential(setup, current.revision) : undefined;
  const credentialRequired = needsCredential(authenticationType);
  const eligibleExistingCredentials = useMemo(() => setup?.credential_sets.filter((credentialSet) =>
    credentialSet.environment_id === environmentID
    && credentialSet.authentication_type === authenticationType
    && credentialSet.state === "active"
    && credentialSet.credential_present,
  ) ?? [], [authenticationType, environmentID, setup]);
  const selectedExistingCredential = setup?.credential_sets.find((credentialSet) => credentialSet.id === existingCredentialSetID);

  const selectEnvironment = (nextEnvironmentID: string) => {
    if (!setup) return;
    setFormError("");
    hydrateForm(setup, nextEnvironmentID);
  };

  const selectAuthentication = (nextAuthenticationType: APIRuntimeAuthenticationType) => {
    setAuthenticationType(nextAuthenticationType);
    setFormError("");
    setCredential("");
    const existing = setup?.credential_sets.find((credentialSet) => credentialSet.id === existingCredentialSetID);
    if (!existing || existing.authentication_type !== nextAuthenticationType || existing.environment_id !== environmentID) {
      setExistingCredentialSetID("");
      setCredentialChoice("dedicated");
      setCredentialName(`${integration.display_name} credential`);
      setEnvironmentVariable(defaultEnvironmentVariable(integration, "dedicated"));
    }
    setHeaderName(nextAuthenticationType === "api_key_header" || nextAuthenticationType === "custom_header" ? existing?.header_name ?? "X-API-Key" : "");
  };

  const selectCredentialChoice = (choice: CredentialChoice) => {
    setCredentialChoice(choice);
    setCredential("");
    setFormError("");
    if (choice === "dedicated" || choice === "shared") {
      setExistingCredentialSetID("");
      setCredentialName(choice === "shared" ? "Shared service credential" : `${integration.display_name} credential`);
      setEnvironmentVariable(defaultEnvironmentVariable(integration, choice));
      return;
    }
    const selected = eligibleExistingCredentials[0];
    setExistingCredentialSetID(selected?.id ?? "");
    if (selected) {
      setCredentialName(selected.name);
      setEnvironmentVariable(selected.environment_variable);
      setHeaderName(selected.header_name ?? headerName);
    }
  };

  const saveSetup = async () => {
    setFormError("");
    if (!environmentID) {
      setFormError("Choose an environment first.");
      return;
    }
    try {
      const parsedURL = new URL(baseURL);
      if (parsedURL.protocol !== "https:" && parsedURL.protocol !== "http:") throw new Error();
    } catch {
      setFormError("Enter a complete HTTP or HTTPS service URL.");
      return;
    }
    if (credentialRequired && credentialChoice === "existing" && !existingCredentialSetID) {
      setFormError("Choose an existing credential set.");
      return;
    }
    if (credentialRequired && credentialChoice !== "existing" && !credential.trim()) {
      setFormError("Enter the credential DokoSoko should store for this service.");
      return;
    }
    let parsedAuthConfig: Record<string, unknown> = {};
    try {
      const value = JSON.parse(authConfig || "{}") as unknown;
      if (!value || Array.isArray(value) || typeof value !== "object") throw new Error();
      parsedAuthConfig = value as Record<string, unknown>;
    } catch {
      setFormError("Advanced authentication configuration must be a JSON object.");
      return;
    }
    setSaving(true);
    try {
      const value = await api.configureIntegrationRuntimeSetup(integration.id, {
        environment_id: environmentID,
        connection_name: connectionName.trim(),
        connection_description: connectionDescription.trim(),
        base_url: baseURL.trim(),
        authentication_type: authenticationType,
        auth_config: parsedAuthConfig,
        ...(credentialRequired && credentialChoice === "existing" ? { existing_credential_set_id: existingCredentialSetID } : {}),
        ...(credentialRequired && credentialChoice !== "existing" ? {
          credential_scope: credentialChoice,
          credential_name: credentialName.trim(),
          environment_variable: environmentVariable.trim(),
          header_name: headerName.trim(),
          credential: credential.trim(),
          ...(credentialExpiresAt ? { credential_expires_at: new Date(credentialExpiresAt).toISOString() } : {}),
        } : {}),
      });
      setSetup(value);
      hydrateForm(value, environmentID);
      setUsageLoaded(false);
      setUsageCounts({});
      setConfigurationCheck(null);
      await onChanged?.();
      onMessage("Service endpoint and authentication saved.");
    } catch (error) {
      setFormError(errorMessage(error, "Service access could not be saved."));
    } finally {
      setSaving(false);
    }
  };

  const loadUsage = async () => {
    if (!setup || usageLoaded) return;
    setUsageLoaded(true);
    const results = await Promise.all(setup.credential_sets.map(async (credentialSet) => {
      try {
        const usage = await api.runtimeCredentialUsage(credentialSet.id);
        return [credentialSet.id, usage.count] as const;
      } catch {
        return [credentialSet.id, -1] as const;
      }
    }));
    setUsageCounts(Object.fromEntries(results));
  };

  const checkConfiguration = async () => {
    if (!current.connection) return;
    setCheckingConfiguration(true);
    try {
      const value = await api.checkRuntimeServiceConnection(current.connection.id);
      setConfigurationCheck(value);
      onMessage(value.ready ? "Saved service configuration is ready." : "Saved service configuration needs attention.");
    } catch (error) {
      onMessage(errorMessage(error, "Service configuration could not be checked."));
    } finally {
      setCheckingConfiguration(false);
    }
  };

  const rotateCredential = async () => {
    if (!rotateSet || !rotationCredential.trim()) return;
    setCredentialBusy(true);
    try {
      await api.rotateRuntimeCredential(rotateSet.id, rotationCredential.trim(), rotationExpiresAt ? new Date(rotationExpiresAt).toISOString() : undefined);
      setRotateSet(null);
      setRotationCredential("");
      setRotationExpiresAt("");
      await loadSetup();
      await onChanged?.();
      onMessage(`${rotateSet.name} rotated. Existing connections now use the new active version.`);
    } catch (error) {
      onMessage(errorMessage(error, "Credential could not be rotated."));
    } finally {
      setCredentialBusy(false);
    }
  };

  const revokeCredentialVersion = async () => {
    if (!revokeTarget) return;
    setCredentialBusy(true);
    try {
      await api.revokeRuntimeCredentialVersion(revokeTarget.credentialSet.id, revokeTarget.version.id);
      const name = revokeTarget.credentialSet.name;
      setRevokeTarget(null);
      await loadSetup();
      await onChanged?.();
      onMessage(`${name} version revoked.`);
    } catch (error) {
      onMessage(errorMessage(error, "Credential version could not be revoked."));
    } finally {
      setCredentialBusy(false);
    }
  };

  if (loading && !setup) {
    return <section className="panel runtime-access-panel"><PanelHeader title="Service connection" description="Configure the endpoint and credential DokoSoko uses when an agent calls this API." /><div className="runtime-access-loading"><RefreshCw /><span>Loading service access…</span></div></section>;
  }

  if (loadError && !setup) {
    return <section className="panel runtime-access-panel"><PanelHeader title="Service connection" description="Configure the endpoint and credential DokoSoko uses when an agent calls this API." /><div className="capability-unavailable"><TriangleAlert /><span><strong>Service access is unavailable</strong><small>{loadError}</small></span><Button outline onClick={() => void loadSetup()}>Retry</Button></div></section>;
  }

  if (!setup) return null;

  return <>
    <section className="panel runtime-access-panel">
      <PanelHeader
        title="Service connection"
        description="Add the service URL, choose how it authenticates, and save. DokoSoko encrypts credentials and never shows them again."
        action={<span className="heading-actions">{current.revision ? <Badge color="green">Configured</Badge> : <Badge color="amber">Setup required</Badge>}{current.connection && <Button outline disabled={checkingConfiguration} onClick={() => void checkConfiguration()}>{checkingConfiguration ? "Checking…" : "Check configuration"}</Button>}{current.revision && <Button color="indigo" onClick={() => onNavigate(integrationToolBuilderPath(integration.id))}>Create API tool</Button>}</span>}
      />
      {current.revision && <div className="runtime-current-summary">
        <span className="settings-icon"><CheckCircle2 /></span>
        <span><strong>{current.connection?.name ?? "Service connection"}</strong><small>{current.revision.base_url}</small></span>
        <span><small>Authentication</small><strong>{prettyAuthentication(current.revision.authentication_type)}</strong></span>
        <span><small>Credential</small><strong>{selectedCurrentCredential?.environment_variable ?? "Not required"}</strong></span>
      </div>}
      {configurationCheck && <div className={`runtime-configuration-check ${configurationCheck.ready ? "ready" : "needs-attention"}`}>
        <div><span className={`health-icon ${configurationCheck.ready ? "ready" : ""}`}>{configurationCheck.ready ? <CheckCircle2 /> : <TriangleAlert />}</span><span><strong>{configurationCheck.ready ? "Configuration ready" : "Configuration needs attention"}</strong><small>This checks saved endpoint and credential metadata only. Live upstream behavior is tested from an attached tool.</small></span></div>
        <div>{configurationCheck.checks.map((check) => <span key={`${check.key}:${check.environment_id ?? "all"}`}><span className={`health-icon ${check.ready ? "ready" : ""}`}>{check.ready ? <CheckCircle2 /> : <TriangleAlert />}</span><span><strong>{check.label}</strong><small>{check.message}</small></span></span>)}</div>
        {configurationCheck.ready && <div className="heading-actions"><Button color="indigo" onClick={() => onNavigate(integrationToolBuilderPath(integration.id))}>Create API tool</Button><Button outline onClick={() => onNavigate(integrationPath(integration.id, "test"))}>Open Test</Button></div>}
      </div>}
      <div className="runtime-access-form">
        <div className="two-fields">
          <label className="auth-field"><span>Environment</span><select value={environmentID} onChange={(event) => selectEnvironment(event.target.value)}>{setup.environments.map((environment) => <option key={environment.id} value={environment.id}>{environment.name}{environment.is_production ? " · Production" : ""}</option>)}</select></label>
          <label className="auth-field"><span>Service URL</span><input type="url" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder="https://api.example.com" autoComplete="url" /></label>
        </div>
        <label className="auth-field"><span>Authentication</span><select value={authenticationType} onChange={(event) => selectAuthentication(event.target.value as APIRuntimeAuthenticationType)}><optgroup label="Common">{commonAuthenticationTypes.map((value) => <option key={value} value={value}>{authenticationLabels[value]}</option>)}</optgroup><optgroup label="Advanced">{advancedAuthenticationTypes.map((value) => <option key={value} value={value}>{authenticationLabels[value]}</option>)}</optgroup></select><small>Choose the credential the upstream API expects. Customer sign-in is configured separately below.</small></label>

        {credentialRequired && <fieldset className="runtime-credential-choice">
          <legend>Credential</legend>
          <div className="runtime-choice-grid">
            <label className={credentialChoice === "dedicated" ? "selected" : ""} aria-label="Only this API"><input type="radio" name={`runtime-credential-${integration.id}`} checked={credentialChoice === "dedicated"} onChange={() => selectCredentialChoice("dedicated")} /><span><strong>Only this API</strong><small>A dedicated {defaultEnvironmentVariable(integration, "dedicated")} secret.</small></span></label>
            <label className={credentialChoice === "shared" ? "selected" : ""} aria-label="Share across APIs"><input type="radio" name={`runtime-credential-${integration.id}`} checked={credentialChoice === "shared"} onChange={() => selectCredentialChoice("shared")} /><span><strong>Share across APIs</strong><small>A reusable SERVICE_API_KEY for this environment.</small></span></label>
            <label className={credentialChoice === "existing" ? "selected" : ""} aria-label="Use existing credential" aria-disabled={eligibleExistingCredentials.length === 0}><input type="radio" name={`runtime-credential-${integration.id}`} checked={credentialChoice === "existing"} disabled={eligibleExistingCredentials.length === 0} onChange={() => selectCredentialChoice("existing")} /><span><strong>Use existing</strong><small>{eligibleExistingCredentials.length > 0 ? `${eligibleExistingCredentials.length} compatible credential${eligibleExistingCredentials.length === 1 ? "" : "s"}.` : "No compatible credential yet."}</small></span></label>
          </div>
          {credentialChoice === "existing" ? <label className="auth-field"><span>Existing credential</span><select value={existingCredentialSetID} onChange={(event) => setExistingCredentialSetID(event.target.value)}><option value="">Choose a credential</option>{eligibleExistingCredentials.map((credentialSet) => <option key={credentialSet.id} value={credentialSet.id}>{credentialSet.name} · {credentialSet.environment_variable} · {credentialSet.scope}</option>)}</select><small>{selectedExistingCredential?.active_fingerprint ? `Active fingerprint ${selectedExistingCredential.active_fingerprint}` : "Only masked metadata is visible here."}</small></label> : <label className="auth-field"><span>{current.revision && selectedCurrentCredential ? "New credential" : "Credential value"}</span><input type="password" value={credential} onChange={(event) => setCredential(event.target.value)} placeholder={selectedCurrentCredential?.credential_present ? "••••••••••••" : "Paste the credential"} autoComplete="new-password" /><small>{selectedCurrentCredential?.credential_present ? "A credential is already stored. Leave this path by selecting Use existing, or enter a new value to create a separate credential set." : "Encrypted at rest and omitted from every response."}</small></label>}
        </fieldset>}

        <details className="advanced-details inline-advanced runtime-advanced">
          <summary>Advanced connection settings</summary>
          <div className="advanced-details-body">
            <div className="two-fields"><label className="auth-field"><span>Connection name</span><input value={connectionName} onChange={(event) => setConnectionName(event.target.value)} /></label><label className="auth-field"><span>Description</span><input value={connectionDescription} onChange={(event) => setConnectionDescription(event.target.value)} placeholder="Optional operator note" /></label></div>
            {credentialRequired && credentialChoice !== "existing" && <><div className="two-fields"><label className="auth-field"><span>Credential name</span><input value={credentialName} onChange={(event) => setCredentialName(event.target.value)} /></label><label className="auth-field"><span>Environment variable</span><input value={environmentVariable} onChange={(event) => setEnvironmentVariable(event.target.value.toUpperCase())} /></label></div><div className="two-fields"><label className="auth-field"><span>Header name</span><input value={headerName} onChange={(event) => setHeaderName(event.target.value)} disabled={authenticationType !== "api_key_header" && authenticationType !== "custom_header"} placeholder="X-API-Key" /></label><label className="auth-field"><span>Credential expires (optional)</span><input type="datetime-local" value={credentialExpiresAt} onChange={(event) => setCredentialExpiresAt(event.target.value)} /></label></div></>}
            <label className="auth-field"><span>Authentication configuration (JSON)</span><textarea className="code-input" value={authConfig} onChange={(event) => setAuthConfig(event.target.value)} spellCheck={false} /><small>Use only for provider-specific, non-secret authentication options. Never place credentials in this object.</small></label>
          </div>
        </details>
        {formError && <div className="auth-problem"><TriangleAlert /><span>{formError}</span></div>}
        <div className="runtime-save-row"><span>{current.revision ? `Current revision ${current.revision.revision}` : "No connection has been saved for this environment."}</span><Button color="indigo" disabled={saving || setup.environments.length === 0 || !baseURL.trim()} onClick={() => void saveSetup()}>{saving ? "Saving…" : current.revision ? "Save changes" : "Connect service"}</Button></div>
      </div>
    </section>

    <details className="panel advanced-details runtime-credential-management" onToggle={(event) => { if (event.currentTarget.open) void loadUsage(); }}>
      <summary>Credential lifecycle and connection metadata — Advanced</summary>
      <div className="advanced-details-body">
        <PanelHeader title="Stored credentials" description="Rotate or revoke encrypted versions without changing API tool definitions." />
        {setup.credential_sets.map((credentialSet) => <div className="runtime-credential-set" key={credentialSet.id}>
          <div className="runtime-credential-heading"><span className="settings-icon"><KeyRound /></span><span><strong>{credentialSet.name}</strong><small>{credentialSet.environment_variable} · {credentialSet.scope === "shared" ? "Shared across APIs" : "Dedicated to this API"} · {usageCounts[credentialSet.id] === undefined ? "Usage loading…" : usageCounts[credentialSet.id] < 0 ? "Usage unavailable" : `${usageCounts[credentialSet.id]} connection${usageCounts[credentialSet.id] === 1 ? "" : "s"}`}</small></span><Badge color={credentialSet.state === "active" && credentialSet.credential_present ? "green" : "amber"}>{credentialSet.credential_present ? credentialSet.state : "Missing"}</Badge><Button outline onClick={() => { setRotateSet(credentialSet); setRotationCredential(""); setRotationExpiresAt(""); }}><RotateCcw data-slot="icon" />Rotate</Button></div>
          <dl className="runtime-metadata-grid"><div><dt>Authentication</dt><dd>{prettyAuthentication(credentialSet.authentication_type)}</dd></div><div><dt>Header</dt><dd>{credentialSet.header_name || "—"}</dd></div><div><dt>Active fingerprint</dt><dd><code>{credentialSet.active_fingerprint || "—"}</code></dd></div><div><dt>Revision</dt><dd>{credentialSet.revision}</dd></div></dl>
          {(credentialSet.versions ?? []).length > 0 && <div className="runtime-version-list">{credentialSet.versions?.map((version) => <div key={version.id}><span><strong>{version.state}</strong><small>Fingerprint {version.fingerprint} · created {formatDate(version.created_at)}{version.expires_at ? ` · expires ${formatDate(version.expires_at)}` : ""}</small></span>{version.state !== "revoked" && <Button outline disabled={credentialBusy} onClick={() => setRevokeTarget({ credentialSet, version })}>Revoke</Button>}</div>)}</div>}
        </div>)}
        {setup.credential_sets.length === 0 && <div className="empty-row">No runtime credentials have been created for this API or shared into its environments.</div>}

        {current.connection && current.revision && <><PanelHeader title="Current connection metadata" description="Immutable revision identifiers for auditing and support." /><dl className="entity-detail-grid runtime-connection-details"><div><dt>Connection ID</dt><dd>{current.connection.id}</dd></div><div><dt>Connection revision</dt><dd>{current.revision.id}</dd></div><div><dt>Configuration revision</dt><dd>{current.revision.revision}</dd></div><div><dt>Content hash</dt><dd>{current.revision.content_hash}</dd></div></dl></>}
      </div>
    </details>

    <Dialog open={Boolean(rotateSet)} onClose={(open) => { if (!open && !credentialBusy) setRotateSet(null); }} title={`Rotate ${rotateSet?.name ?? "credential"}`} description="The new encrypted version becomes active for every connection that references this credential set." actions={<><Button outline disabled={credentialBusy} onClick={() => setRotateSet(null)}>Cancel</Button><Button color="indigo" disabled={credentialBusy || !rotationCredential.trim()} onClick={() => void rotateCredential()}>{credentialBusy ? "Rotating…" : "Rotate credential"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>New credential</span><input type="password" value={rotationCredential} onChange={(event) => setRotationCredential(event.target.value)} placeholder="Paste the new credential" autoComplete="new-password" /></label><label className="auth-field"><span>Expires (optional)</span><input type="datetime-local" value={rotationExpiresAt} onChange={(event) => setRotationExpiresAt(event.target.value)} /></label><div className="private-default-note"><ShieldCheck /><span>The existing secret is never returned to this browser.</span></div></div></Dialog>

    <Dialog open={Boolean(revokeTarget)} onClose={(open) => { if (!open && !credentialBusy) setRevokeTarget(null); }} title="Revoke credential version?" description="Revocation is immediate and cannot be undone. Rotate first if any active connection still depends on this version." actions={<><Button outline disabled={credentialBusy} onClick={() => setRevokeTarget(null)}>Cancel</Button><Button color="red" disabled={credentialBusy} onClick={() => void revokeCredentialVersion()}>{credentialBusy ? "Revoking…" : "Revoke version"}</Button></>}><div className="runtime-revoke-summary"><TriangleAlert /><span><strong>{revokeTarget?.credentialSet.name}</strong><small>Fingerprint {revokeTarget?.version.fingerprint}</small></span></div></Dialog>
  </>;
}
