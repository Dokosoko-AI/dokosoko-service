"use client";


import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
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

function prettyAuthentication(authenticationType: APIRuntimeAuthenticationType, t: TFunction) {
  return authenticationType === "none" ? t("tools.noAuthentication")
    : authenticationType === "delegated_oauth" ? t("runtimeAccess.customerOAuthToken")
      : authenticationType === "api_key_header" ? t("tools.apiKeyHeader")
        : authenticationType === "bearer" ? t("tools.bearerToken")
          : authenticationType === "authorization_scheme" ? t("tools.authorizationScheme")
            : authenticationType === "api_key_query" ? t("tools.apiKeyQueryParameter")
              : authenticationType === "basic" ? t("tools.httpBasic")
                : authenticationType === "oauth_client_credentials" ? t("tools.oauthClientCredentials")
                  : t("tools.customSecretHeader");
}

function formatDate(value: string | undefined, t: TFunction) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : t("format.dateTime", { value: date });
}

export function IntegrationRuntimeAccess({ integration, onMessage, onNavigate, onChanged }: {
  integration: APIIntegration;
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
  onChanged?: () => void | Promise<void>;
}) {
  const { t } = useTranslation();
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
    setConnectionName(connection?.name ?? t("runtimeAccess.defaultServiceName", { name: integration.display_name }));
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
      setCredentialName(t("runtimeAccess.defaultCredentialName", { name: integration.display_name }));
      setEnvironmentVariable(defaultEnvironmentVariable(integration, "dedicated"));
      setHeaderName(nextAuthenticationType === "api_key_header" || nextAuthenticationType === "custom_header" ? "X-API-Key" : "");
    }
  }, [integration, t]);

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
      setLoadError(errorMessage(error, t("runtimeAccess.loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [environmentID, hydrateForm, integration.id, t]);

  useEffect(() => {
    let cancelled = false;
    void api.integrationRuntimeSetup(integration.id).then((value) => {
      if (cancelled) return;
      setSetup(value);
      hydrateForm(value, "");
      setLoadError("");
    }).catch((error) => {
      if (!cancelled) setLoadError(errorMessage(error, t("runtimeAccess.loadFailed")));
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });
    return () => { cancelled = true; };
  }, [hydrateForm, integration.id, t]);

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
      setCredentialName(t("runtimeAccess.defaultCredentialName", { name: integration.display_name }));
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
      setCredentialName(choice === "shared" ? t("runtimeAccess.sharedServiceCredential") : t("runtimeAccess.defaultCredentialName", { name: integration.display_name }));
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
      setFormError(t("runtimeAccess.chooseAnEnvironmentFirst"));
      return;
    }
    try {
      const parsedURL = new URL(baseURL);
      if (parsedURL.protocol !== "https:" && parsedURL.protocol !== "http:") throw new Error();
    } catch {
      setFormError(t("runtimeAccess.enterACompleteHTTPOrHTTPSServiceURL"));
      return;
    }
    if (credentialRequired && credentialChoice === "existing" && !existingCredentialSetID) {
      setFormError(t("runtimeAccess.chooseAnExistingCredentialSet"));
      return;
    }
    if (credentialRequired && credentialChoice !== "existing" && !credential.trim()) {
      setFormError(t("runtimeAccess.enterTheCredentialDokoSokoShouldStoreForThisService"));
      return;
    }
    let parsedAuthConfig: Record<string, unknown> = {};
    try {
      const value = JSON.parse(authConfig || "{}") as unknown;
      if (!value || Array.isArray(value) || typeof value !== "object") throw new Error();
      parsedAuthConfig = value as Record<string, unknown>;
    } catch {
      setFormError(t("runtimeAccess.advancedAuthenticationConfigurationMustBeAJSONObject"));
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
      onMessage(t("runtimeAccess.serviceEndpointAndAuthenticationSaved"));
    } catch (error) {
      setFormError(errorMessage(error, t("runtimeAccess.saveFailed")));
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
      onMessage(value.ready ? t("runtimeAccess.savedServiceConfigurationIsReady") : t("runtimeAccess.savedServiceConfigurationNeedsAttention"));
    } catch (error) {
      onMessage(errorMessage(error, t("runtimeAccess.checkFailed")));
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
      onMessage(t("runtimeAccess.rotatedExistingConnectionsNowUseTheNewActiveVersion", { name: String(rotateSet.name) }));
    } catch (error) {
      onMessage(errorMessage(error, t("runtimeAccess.rotateFailed")));
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
      onMessage(t("runtimeAccess.versionRevoked", { name: String(name) }));
    } catch (error) {
      onMessage(errorMessage(error, t("runtimeAccess.revokeFailed")));
    } finally {
      setCredentialBusy(false);
    }
  };

  if (loading && !setup) {
    return <section className="panel runtime-access-panel"><PanelHeader title={t("runtimeAccess.serviceConnection")} description={t("runtimeAccess.configureTheEndpointAndCredentialDokoSokoUsesWhenAn")} /><div className="runtime-access-loading"><RefreshCw /><span>{t("runtimeAccess.loadingServiceAccess")}</span></div></section>;
  }

  if (loadError && !setup) {
    return <section className="panel runtime-access-panel"><PanelHeader title={t("runtimeAccess.serviceConnection")} description={t("runtimeAccess.configureTheEndpointAndCredentialDokoSokoUsesWhenAn")} /><div className="capability-unavailable"><TriangleAlert /><span><strong>{t("runtimeAccess.serviceAccessIsUnavailable")}</strong><small>{loadError}</small></span><Button outline onClick={() => void loadSetup()}>{t("common.retry")}</Button></div></section>;
  }

  if (!setup) return null;

  return <>
    <section className="panel runtime-access-panel">
      <PanelHeader
        title={t("runtimeAccess.serviceConnection")}
        description={t("runtimeAccess.addTheServiceURLChooseHowItAuthenticatesAnd")}
        action={<span className="heading-actions">{current.revision ? <Badge color="green">{t("runtimeAccess.configured")}</Badge> : <Badge color="amber">{t("runtimeAccess.setupRequired")}</Badge>}{current.connection && <Button outline disabled={checkingConfiguration} onClick={() => void checkConfiguration()}>{checkingConfiguration ? t("runtimeAccess.checking") : t("runtimeAccess.checkConfiguration")}</Button>}{current.revision && <Button color="indigo" onClick={() => onNavigate(integrationToolBuilderPath(integration.id))}>{t("runtimeAccess.createAPITool")}</Button>}</span>}
      />
      {current.revision && <div className="runtime-current-summary">
        <span className="settings-icon"><CheckCircle2 /></span>
        <span><strong>{current.connection?.name ?? t("runtimeAccess.serviceConnection")}</strong><small>{current.revision.base_url}</small></span>
        <span><small>{t("runtimeAccess.authentication")}</small><strong>{prettyAuthentication(current.revision.authentication_type, t)}</strong></span>
        <span><small>{t("runtimeAccess.credential")}</small><strong>{selectedCurrentCredential?.environment_variable ?? t("runtimeAccess.notRequired")}</strong></span>
      </div>}
      {configurationCheck && <div className={`runtime-configuration-check ${configurationCheck.ready ? "ready" : "needs-attention"}`}>
        <div><span className={`health-icon ${configurationCheck.ready ? "ready" : ""}`}>{configurationCheck.ready ? <CheckCircle2 /> : <TriangleAlert />}</span><span><strong>{configurationCheck.ready ? t("runtimeAccess.configurationReady") : t("runtimeAccess.configurationNeedsAttention")}</strong><small>{t("runtimeAccess.thisChecksSavedEndpointAndCredentialMetadataOnlyLive")}</small></span></div>
        <div>{configurationCheck.checks.map((check) => <span key={`${check.key}:${check.environment_id ?? "all"}`}><span className={`health-icon ${check.ready ? "ready" : ""}`}>{check.ready ? <CheckCircle2 /> : <TriangleAlert />}</span><span><strong>{check.label}</strong><small>{check.message}</small></span></span>)}</div>
        {configurationCheck.ready && <div className="heading-actions"><Button color="indigo" onClick={() => onNavigate(integrationToolBuilderPath(integration.id))}>{t("runtimeAccess.createAPITool")}</Button><Button outline onClick={() => onNavigate(integrationPath(integration.id, "test"))}>{t("runtimeAccess.openTest")}</Button></div>}
      </div>}
      <div className="runtime-access-form">
        <div className="two-fields">
          <label className="auth-field"><span>{t("runtimeAccess.environment")}</span><select value={environmentID} onChange={(event) => selectEnvironment(event.target.value)}>{setup.environments.map((environment) => <option key={environment.id} value={environment.id}>{environment.name}{environment.is_production ? t("runtimeAccess.production") : ""}</option>)}</select></label>
          <label className="auth-field"><span>{t("runtimeAccess.serviceURL")}</span><input type="url" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder="https://api.example.com" autoComplete="url" /></label>
        </div>
        <label className="auth-field"><span>{t("runtimeAccess.authentication")}</span><select value={authenticationType} onChange={(event) => selectAuthentication(event.target.value as APIRuntimeAuthenticationType)}><optgroup label={t("runtimeAccess.common")}>{commonAuthenticationTypes.map((value) => <option key={value} value={value}>{prettyAuthentication(value, t)}</option>)}</optgroup><optgroup label={t("runtimeAccess.advanced")}>{advancedAuthenticationTypes.map((value) => <option key={value} value={value}>{prettyAuthentication(value, t)}</option>)}</optgroup></select><small>{t("runtimeAccess.chooseTheCredentialTheUpstreamAPIExpectsCustomerSign")}</small></label>

        {credentialRequired && <fieldset className="runtime-credential-choice">
          <legend>{t("runtimeAccess.credential")}</legend>
          <div className="runtime-choice-grid">
            <label className={credentialChoice === "dedicated" ? "selected" : ""} aria-label={t("runtimeAccess.onlyThisAPI")}><input type="radio" name={`runtime-credential-${integration.id}`} checked={credentialChoice === "dedicated"} onChange={() => selectCredentialChoice("dedicated")} /><span><strong>{t("runtimeAccess.onlyThisAPI")}</strong><small>{t("runtimeAccess.dedicatedSecret", { variable: defaultEnvironmentVariable(integration, "dedicated") })}</small></span></label>
            <label className={credentialChoice === "shared" ? "selected" : ""} aria-label={t("runtimeAccess.shareAcrossAPIs")}><input type="radio" name={`runtime-credential-${integration.id}`} checked={credentialChoice === "shared"} onChange={() => selectCredentialChoice("shared")} /><span><strong>{t("runtimeAccess.shareAcrossAPIs")}</strong><small>{t("runtimeAccess.aReusableSERVICEAPIKEYForThisEnvironment")}</small></span></label>
            <label className={credentialChoice === "existing" ? "selected" : ""} aria-label={t("runtimeAccess.useExistingCredential")} aria-disabled={eligibleExistingCredentials.length === 0}><input type="radio" name={`runtime-credential-${integration.id}`} checked={credentialChoice === "existing"} disabled={eligibleExistingCredentials.length === 0} onChange={() => selectCredentialChoice("existing")} /><span><strong>{t("runtimeAccess.useExisting")}</strong><small>{eligibleExistingCredentials.length > 0 ? t("runtimeAccess.compatibleCredentials", { count: eligibleExistingCredentials.length }) : t("runtimeAccess.noCompatibleCredentialYet")}</small></span></label>
          </div>
          {credentialChoice === "existing" ? <label className="auth-field"><span>{t("runtimeAccess.existingCredential")}</span><select value={existingCredentialSetID} onChange={(event) => setExistingCredentialSetID(event.target.value)}><option value="">{t("runtimeAccess.chooseACredential")}</option>{eligibleExistingCredentials.map((credentialSet) => <option key={credentialSet.id} value={credentialSet.id}>{credentialSet.name} · {credentialSet.environment_variable} · {credentialSet.scope}</option>)}</select><small>{selectedExistingCredential?.active_fingerprint ? t("runtimeAccess.activeFingerprint2", { active_fingerprint: String(selectedExistingCredential.active_fingerprint) }) : t("runtimeAccess.onlyMaskedMetadataIsVisibleHere")}</small></label> : <label className="auth-field"><span>{current.revision && selectedCurrentCredential ? t("runtimeAccess.newCredential") : t("runtimeAccess.credentialValue")}</span><input type="password" value={credential} onChange={(event) => setCredential(event.target.value)} placeholder={selectedCurrentCredential?.credential_present ? "••••••••••••" : t("runtimeAccess.pasteTheCredential")} autoComplete="new-password" /><small>{selectedCurrentCredential?.credential_present ? t("runtimeAccess.aCredentialIsAlreadyStoredLeaveThisPathBy") : t("runtimeAccess.encryptedAtRestAndOmittedFromEveryResponse")}</small></label>}
        </fieldset>}

        <details className="advanced-details inline-advanced runtime-advanced">
          <summary>{t("runtimeAccess.advancedConnectionSettings")}</summary>
          <div className="advanced-details-body">
            <div className="two-fields"><label className="auth-field"><span>{t("runtimeAccess.connectionName")}</span><input value={connectionName} onChange={(event) => setConnectionName(event.target.value)} /></label><label className="auth-field"><span>{t("runtimeAccess.description")}</span><input value={connectionDescription} onChange={(event) => setConnectionDescription(event.target.value)} placeholder={t("runtimeAccess.optionalOperatorNote")} /></label></div>
            {credentialRequired && credentialChoice !== "existing" && <><div className="two-fields"><label className="auth-field"><span>{t("runtimeAccess.credentialName")}</span><input value={credentialName} onChange={(event) => setCredentialName(event.target.value)} /></label><label className="auth-field"><span>{t("runtimeAccess.environmentVariable")}</span><input value={environmentVariable} onChange={(event) => setEnvironmentVariable(event.target.value.toUpperCase())} /></label></div><div className="two-fields"><label className="auth-field"><span>{t("runtimeAccess.headerName")}</span><input value={headerName} onChange={(event) => setHeaderName(event.target.value)} disabled={authenticationType !== "api_key_header" && authenticationType !== "custom_header"} placeholder={t("runtimeAccess.xAPIKey")} /></label><label className="auth-field"><span>{t("runtimeAccess.credentialExpiresOptional")}</span><input type="datetime-local" value={credentialExpiresAt} onChange={(event) => setCredentialExpiresAt(event.target.value)} /></label></div></>}
            <label className="auth-field"><span>{t("runtimeAccess.authenticationConfigurationJSON")}</span><textarea className="code-input" value={authConfig} onChange={(event) => setAuthConfig(event.target.value)} spellCheck={false} /><small>{t("runtimeAccess.useOnlyForProviderSpecificNonSecretAuthenticationOptions")}</small></label>
          </div>
        </details>
        {formError && <div className="auth-problem"><TriangleAlert /><span>{formError}</span></div>}
        <div className="runtime-save-row"><span>{current.revision ? t("runtimeAccess.currentRevision", { revision: String(current.revision.revision) }) : t("runtimeAccess.noConnectionHasBeenSavedForThisEnvironment")}</span><Button color="indigo" disabled={saving || setup.environments.length === 0 || !baseURL.trim()} onClick={() => void saveSetup()}>{saving ? t("common.saving") : current.revision ? t("runtimeAccess.saveChanges") : t("runtimeAccess.connectService")}</Button></div>
      </div>
    </section>

    <details className="panel advanced-details runtime-credential-management" onToggle={(event) => { if (event.currentTarget.open) void loadUsage(); }}>
      <summary>{t("runtimeAccess.credentialLifecycleAndConnectionMetadataAdvanced")}</summary>
      <div className="advanced-details-body">
        <PanelHeader title={t("runtimeAccess.storedCredentials")} description={t("runtimeAccess.rotateOrRevokeEncryptedVersionsWithoutChangingAPITool")} />
        {setup.credential_sets.map((credentialSet) => <div className="runtime-credential-set" key={credentialSet.id}>
          <div className="runtime-credential-heading"><span className="settings-icon"><KeyRound /></span><span><strong>{credentialSet.name}</strong><small>{credentialSet.environment_variable} · {credentialSet.scope === "shared" ? t("runtimeAccess.sharedAcrossAPIs") : t("runtimeAccess.dedicatedToThisAPI")} · {usageCounts[credentialSet.id] === undefined ? t("runtimeAccess.usageLoading") : usageCounts[credentialSet.id] < 0 ? t("runtimeAccess.usageUnavailable") : t("runtimeAccess.connections", { count: usageCounts[credentialSet.id] })}</small></span><Badge color={credentialSet.state === "active" && credentialSet.credential_present ? "green" : "amber"}>{credentialSet.credential_present ? credentialSet.state === "active" ? t("runtimeAccess.active") : t("runtimeAccess.revoked") : t("runtimeAccess.missing")}</Badge><Button outline onClick={() => { setRotateSet(credentialSet); setRotationCredential(""); setRotationExpiresAt(""); }}><RotateCcw data-slot="icon" />{t("runtimeAccess.rotate")}</Button></div>
          <dl className="runtime-metadata-grid"><div><dt>{t("runtimeAccess.authentication")}</dt><dd>{prettyAuthentication(credentialSet.authentication_type, t)}</dd></div><div><dt>{t("runtimeAccess.header")}</dt><dd>{credentialSet.header_name || "—"}</dd></div><div><dt>{t("runtimeAccess.activeFingerprint")}</dt><dd><code>{credentialSet.active_fingerprint || "—"}</code></dd></div><div><dt>{t("runtimeAccess.revision")}</dt><dd>{credentialSet.revision}</dd></div></dl>
          {(credentialSet.versions ?? []).length > 0 && <div className="runtime-version-list">{credentialSet.versions?.map((version) => <div key={version.id}><span><strong>{version.state === "revoked" ? t("runtimeAccess.revoked") : t("runtimeAccess.active")}</strong><small>{t("runtimeAccess.fingerprint")} {version.fingerprint} {t("runtimeAccess.created")} {formatDate(version.created_at, t)}{version.expires_at ? t("runtimeAccess.expires", { value1: formatDate(version.expires_at, t) }) : ""}</small></span>{version.state !== "revoked" && <Button outline disabled={credentialBusy} onClick={() => setRevokeTarget({ credentialSet, version })}>{t("runtimeAccess.revoke")}</Button>}</div>)}</div>}
        </div>)}
        {setup.credential_sets.length === 0 && <div className="empty-row">{t("runtimeAccess.noRuntimeCredentialsHaveBeenCreatedForThisAPI")}</div>}

        {current.connection && current.revision && <><PanelHeader title={t("runtimeAccess.currentConnectionMetadata")} description={t("runtimeAccess.immutableRevisionIdentifiersForAuditingAndSupport")} /><dl className="entity-detail-grid runtime-connection-details"><div><dt>{t("runtimeAccess.connectionID")}</dt><dd>{current.connection.id}</dd></div><div><dt>{t("runtimeAccess.connectionRevision")}</dt><dd>{current.revision.id}</dd></div><div><dt>{t("runtimeAccess.configurationRevision")}</dt><dd>{current.revision.revision}</dd></div><div><dt>{t("runtimeAccess.contentHash")}</dt><dd>{current.revision.content_hash}</dd></div></dl></>}
      </div>
    </details>

    <Dialog open={Boolean(rotateSet)} onClose={(open) => { if (!open && !credentialBusy) setRotateSet(null); }} title={t("runtimeAccess.rotate2", { value1: String(rotateSet?.name ?? "credential") })} description={t("runtimeAccess.theNewEncryptedVersionBecomesActiveForEveryConnection")} actions={<><Button outline disabled={credentialBusy} onClick={() => setRotateSet(null)}>{t("common.cancel")}</Button><Button color="indigo" disabled={credentialBusy || !rotationCredential.trim()} onClick={() => void rotateCredential()}>{credentialBusy ? t("runtimeAccess.rotating") : t("runtimeAccess.rotateCredential")}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>{t("runtimeAccess.newCredential")}</span><input type="password" value={rotationCredential} onChange={(event) => setRotationCredential(event.target.value)} placeholder={t("runtimeAccess.pasteTheNewCredential")} autoComplete="new-password" /></label><label className="auth-field"><span>{t("runtimeAccess.expiresOptional")}</span><input type="datetime-local" value={rotationExpiresAt} onChange={(event) => setRotationExpiresAt(event.target.value)} /></label><div className="private-default-note"><ShieldCheck /><span>{t("runtimeAccess.theExistingSecretIsNeverReturnedToThisBrowser")}</span></div></div></Dialog>

    <Dialog open={Boolean(revokeTarget)} onClose={(open) => { if (!open && !credentialBusy) setRevokeTarget(null); }} title={t("runtimeAccess.revokeCredentialVersion")} description={t("runtimeAccess.revocationIsImmediateAndCannotBeUndoneRotateFirst")} actions={<><Button outline disabled={credentialBusy} onClick={() => setRevokeTarget(null)}>{t("common.cancel")}</Button><Button color="red" disabled={credentialBusy} onClick={() => void revokeCredentialVersion()}>{credentialBusy ? t("runtimeAccess.revoking") : t("runtimeAccess.revokeVersion")}</Button></>}><div className="runtime-revoke-summary"><TriangleAlert /><span><strong>{revokeTarget?.credentialSet.name}</strong><small>{t("runtimeAccess.fingerprint")} {revokeTarget?.version.fingerprint}</small></span></div></Dialog>
  </>;
}
