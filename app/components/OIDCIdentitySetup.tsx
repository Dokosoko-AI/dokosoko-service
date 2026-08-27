"use client";


import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  CheckCircle2,
  Copy,
  ExternalLink,
  KeyRound,
  LockKeyhole,
  Pencil,
  RefreshCw,
  ShieldCheck,
  TriangleAlert,
  XCircle,
} from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { APIError, APIIdentity, APIIdentityTest, api } from "../lib/api";
import { Badge, Button } from "./core/control";
import { PageHeader, PanelHeader } from "./core/layout";

type IdentityOperation = "save" | "test" | "activate" | "disable" | "disconnect" | null;

type OIDCIdentitySetupProps = {
  identity: APIIdentity | null;
  loading: boolean;
  loadError: string;
  navigation?: ReactNode;
  onChanged: (identity: APIIdentity) => void;
  onMessage: (message: string) => void;
};

function isLocalDevelopmentHostname(hostname: string) {
  return hostname === "localhost" || hostname.endsWith(".localhost") || hostname === "127.0.0.1" || hostname === "[::1]" || hostname === "::1";
}

function validOIDCIssuer(raw: string) {
  try {
    const value = new URL(raw.trim());
    const localDevelopment = isLocalDevelopmentHostname(value.hostname);
    if (!value.host || value.username || value.password || value.search || value.hash) return false;
    if (value.protocol !== "https:" && !(value.protocol === "http:" && localDevelopment)) return false;
    return true;
  } catch {
    return false;
  }
}

function validAuthorizationAPIOrigin(raw: string) {
  try {
    const value = new URL(raw.trim());
    const localDevelopment = isLocalDevelopmentHostname(value.hostname);
    if (!value.host || value.username || value.password || value.search || value.hash || value.pathname !== "/") return false;
    if (value.protocol !== "https:" && !(value.protocol === "http:" && localDevelopment)) return false;
    if (value.protocol === "https:" && value.port) return false;
    return true;
  } catch {
    return false;
  }
}

function validOAuthResourceIdentifier(raw: string) {
  try {
    const value = new URL(raw.trim());
    return Boolean(value.protocol && !value.username && !value.password && !value.hash);
  } catch {
    return false;
  }
}

function oidcDiscoveryURL(issuer: string) {
  return `${issuer.replace(/\/$/, "")}/.well-known/openid-configuration`;
}

function identityConfigurationNeedsReview(identity: APIIdentity | null) {
  if (!identity?.configured) return false;
  const validStoredIssuer = validOIDCIssuer(identity.issuer);
  const validAuthorizationOrigin = validAuthorizationAPIOrigin(identity.authorization_api_origin) && !identity.authorization_api_origin.trim().endsWith("/");
  const validOAuthResource = !identity.oauth_resource.trim() || validOAuthResourceIdentifier(identity.oauth_resource);
  const hasOpenIDScope = identity.scopes.some((scope) => scope.trim() === "openid");
  return !validStoredIssuer || !identity.client_id.trim() || !identity.credential_present || !identity.customer_account_claim.trim() || !validAuthorizationOrigin || !validOAuthResource || !hasOpenIDScope;
}

function splitScopes(value: string) {
  const scopes = value.split(/[\s,]+/).map((scope) => scope.trim()).filter(Boolean);
  return [...new Set(["openid", ...scopes])];
}

function accessEvaluationURL(origin: string) {
  const trimmed = origin.trim().replace(/\/$/, "");
  return trimmed ? `${trimmed}/v1/access/evaluations` : "";
}

function identityTestIDFromLocation() {
  if (typeof window === "undefined") return "";
  try {
    return new URL(window.location.href).searchParams.get("identity_test_id")?.trim() ?? "";
  } catch {
    return "";
  }
}

function clearIdentityTestQuery(testID: string) {
  const url = new URL(window.location.href);
  if (url.searchParams.get("identity_test_id")?.trim() !== testID) return;
  url.searchParams.delete("identity_test_id");
  const query = url.searchParams.toString();
  window.history.replaceState(window.history.state, "", `${url.pathname}${query ? `?${query}` : ""}${url.hash}`);
}

function identityTestErrorFromLocation() {
  if (typeof window === "undefined") return "";
  try {
    const marker = new URL(window.location.href).searchParams.get("identity_test_error")?.trim() ?? "";
    return marker === "invalid_or_expired" ? marker : "";
  } catch {
    return "";
  }
}

function clearIdentityTestErrorQuery(marker: string) {
  const url = new URL(window.location.href);
  if (url.searchParams.get("identity_test_error")?.trim() !== marker) return;
  url.searchParams.delete("identity_test_error");
  const query = url.searchParams.toString();
  window.history.replaceState(window.history.state, "", `${url.pathname}${query ? `?${query}` : ""}${url.hash}`);
}

function testStatusLabel(status: APIIdentityTest["status"] | undefined, t: TFunction) {
  if (status === "passed") return t("identity.passed");
  if (status === "failed") return t("identity.failed");
  if (status === "expired") return t("identity.expired");
  if (status === "pending") return t("identity.waitingForSignIn");
  return t("identity.notTested");
}

function identityTestMessage(test: APIIdentityTest | undefined, customerClaim: string, t: TFunction) {
  if (!test) return t("identity.completeOneRealSignInBeforeActivation");
  if (test.status === "passed") return test.completed_at
    ? t("identity.customerTestPassedAt", { customer: test.customer_id || t("identity.claimResolved"), completedAt: new Date(test.completed_at) })
    : t("identity.customerTestPassed", { customer: test.customer_id || t("identity.claimResolved") });
  if (test.status === "pending") return t("identity.finishProviderSignIn");
  if (test.status === "expired" || test.failure_code === "test_expired") return t("identity.testExpiredStartAgain");
  if (test.failure_code === "configuration_changed") return t("identity.configurationChangedDuringTest");
  if (test.failure_code === "authorization_denied") return t("identity.authorizationDeniedDuringTest");
  if (test.failure_code === "authorization_code_missing") return t("identity.authorizationCodeMissing");
  if (test.failure_code === "oidc_authorization_failed") return t("identity.oidcAuthorizationFailed");
  if (test.failure_code === "oidc_verification_failed") return t("identity.oidcVerificationFailed");
  if (test.failure_code === "client_authentication_unsupported") return t("identity.clientAuthenticationUnsupported");
  if (test.failure_code === "issuer_mismatch") return t("identity.issuerMismatch");
  if (test.failure_code === "subject_missing") return t("identity.subjectMissing");
  if (test.failure_code === "customer_claim_missing") return t("identity.customerClaimMissing", { claim: customerClaim || t("identity.customerAccountClaim") });
  if (test.failure_code === "access_token_missing") return t("identity.accessTokenMissing");
  if (test.failure_code === "access_token_expired") return t("identity.accessTokenExpired");
  return t("identity.signInCouldNotBeVerified");
}

async function copyText(value: string) {
  try {
    await navigator.clipboard.writeText(value);
  } catch {
    const area = document.createElement("textarea");
    area.value = value;
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    document.execCommand("copy");
    area.remove();
  }
}

export function OIDCIdentitySetup({ identity, loading, loadError, navigation, onChanged, onMessage }: OIDCIdentitySetupProps) {
  const { t } = useTranslation();
	const [editing, setEditing] = useState(() => !identity?.configured || identityConfigurationNeedsReview(identity));
	const [operation, setOperation] = useState<IdentityOperation>(null);
	const [error, setError] = useState("");
	const [confirmingDisable, setConfirmingDisable] = useState(false);
	const [confirmingDisconnect, setConfirmingDisconnect] = useState(false);
	const [issuerInput, setIssuerInput] = useState(() => identity?.issuer ?? "");
	const [clientID, setClientID] = useState(() => identity?.client_id ?? "");
	const [clientSecret, setClientSecret] = useState("");
	const [audience, setAudience] = useState(() => identity?.audience ?? "");
	const [customerClaim, setCustomerClaim] = useState(() => identity?.customer_account_claim ?? "");
	const [authorizationAPIOrigin, setAuthorizationAPIOrigin] = useState(() => identity?.authorization_api_origin ?? "");
	const [scopes, setScopes] = useState(() => identity?.scopes.length ? identity.scopes.join(" ") : "openid profile email");
	const [oauthResource, setOAuthResource] = useState(() => identity?.oauth_resource ?? "");
	const [installationClaim, setInstallationClaim] = useState(() => identity?.installation_claim ?? "");
	const [activationObservedAt, setActivationObservedAt] = useState(() => Date.now());
	const [callbackTestID] = useState(identityTestIDFromLocation);
	const [selectedTest, setSelectedTest] = useState<{ id: string; value?: APIIdentityTest }>(() => callbackTestID ? { id: callbackTestID } : identity?.last_test ? { id: identity.last_test.id, value: identity.last_test } : { id: "" });
	const [callbackTestLoading, setCallbackTestLoading] = useState(Boolean(callbackTestID));
	const [callbackTestFailure, setCallbackTestFailure] = useState("");
	const [callbackTestError, setCallbackTestError] = useState(identityTestErrorFromLocation);

  const issuer = issuerInput.trim();
  const issuerReady = validOIDCIssuer(issuer);
  const discoveryURL = issuerReady ? oidcDiscoveryURL(issuer) : "";
  const configured = identity?.configured === true;
  const active = configured && identity?.state === "active";
  const reviewRequired = identityConfigurationNeedsReview(identity);
  const lastTest = selectedTest.value;
  const canLoadCallbackTest = Boolean(callbackTestID && identity && !loading);

  useEffect(() => {
    if (!canLoadCallbackTest) return;
    let cancelled = false;
    api.identityTest(callbackTestID).then((test) => {
      if (cancelled) return;
      setSelectedTest({ id: callbackTestID, value: test });
      setCallbackTestFailure("");
      clearIdentityTestQuery(callbackTestID);
    }).catch((caught) => {
      if (!cancelled) setCallbackTestFailure(caught instanceof APIError ? caught.message : t("identity.returnedTestLoadFailed"));
    }).finally(() => {
      if (cancelled) return;
      setCallbackTestLoading(false);
    });
    return () => { cancelled = true; };
  }, [callbackTestID, canLoadCallbackTest, t]);

  useEffect(() => {
    if (callbackTestError && identity && !loading) clearIdentityTestErrorQuery(callbackTestError);
  }, [callbackTestError, identity, loading]);

  useEffect(() => {
    if (active || !lastTest?.expires_at) return;
    const expiresAt = Date.parse(lastTest.expires_at);
    if (!Number.isFinite(expiresAt)) return;
    const timeout = window.setTimeout(() => setActivationObservedAt(Date.now()), Math.max(0, expiresAt - Date.now()) + 1);
    return () => window.clearTimeout(timeout);
  }, [active, lastTest?.expires_at]);

  const draftTestMatchesCurrentRevision = Boolean(lastTest?.status === "passed" && lastTest.configuration_revision === identity?.revision);
  const testStaleForCurrentRevision = Boolean(!active && lastTest && lastTest.configuration_revision !== identity?.revision);
  const draftTestPassed = Boolean(draftTestMatchesCurrentRevision && lastTest && Date.parse(lastTest.expires_at) > activationObservedAt);
  const draftTestExpiredForActivation = Boolean(!active && draftTestMatchesCurrentRevision && !draftTestPassed);
  const testPassedForCurrentState = active ? true : draftTestPassed;
  const callbackURL = identity?.callback_url ?? "";
	const secretReady = Boolean(identity?.credential_present || clientSecret.trim());
  const scopeValues = scopes.split(/[\s,]+/).map((scope) => scope.trim()).filter(Boolean);
  const openIDScopeReady = scopeValues.includes("openid");
  const authorizationAPIOriginReady = validAuthorizationAPIOrigin(authorizationAPIOrigin);
  const oauthResourceReady = !oauthResource.trim() || validOAuthResourceIdentifier(oauthResource);
  const formReady = Boolean(callbackURL && issuerReady && clientID.trim() && secretReady && customerClaim.trim() && authorizationAPIOriginReady && oauthResourceReady && openIDScopeReady);

  async function saveDraft() {
    if (!identity || !formReady) return;
    setOperation("save");
    setError("");
    try {
      const saved = await api.saveIdentityDraft({
        provider: "oidc",
        issuer,
        client_id: clientID.trim(),
        client_secret: clientSecret,
        scopes: splitScopes(scopes),
        audience: audience.trim(),
        oauth_resource: oauthResource.trim(),
        customer_account_claim: customerClaim.trim(),
        installation_claim: installationClaim.trim(),
        authorization_api_origin: authorizationAPIOrigin.trim().replace(/\/$/, ""),
        revision: identity.revision,
      });
      onChanged(saved);
      setClientSecret("");
      setEditing(false);
      onMessage(active ? t("identity.changesSavedAsADisabledDraftTestAgainBefore") : t("identity.oidcDraftSavedTestARealSignInNext"));
    } catch (caught) {
      setError(caught instanceof APIError ? caught.message : t("identity.draftSaveFailed"));
    } finally {
      setOperation(null);
    }
  }

  async function beginTest() {
    if (!identity?.configured) return;
    setOperation("test");
    setError("");
    try {
      const started = await api.beginIdentityTest(identity.revision);
      setSelectedTest({ id: started.id, value: started });
      setCallbackTestFailure("");
      setCallbackTestError("");
      if (callbackTestID) clearIdentityTestQuery(callbackTestID);
      if (started.authorization_url) {
        window.location.assign(started.authorization_url);
      }
    } catch (caught) {
      setError(caught instanceof APIError || caught instanceof Error ? caught.message : t("identity.testStartFailed"));
    } finally {
      setOperation(null);
    }
  }

  async function retryCallbackTest() {
    if (!callbackTestID) return;
    setCallbackTestLoading(true);
    setCallbackTestFailure("");
    try {
      const test = await api.identityTest(callbackTestID);
      setSelectedTest({ id: callbackTestID, value: test });
      clearIdentityTestQuery(callbackTestID);
    } catch (caught) {
      setCallbackTestFailure(caught instanceof APIError ? caught.message : t("identity.returnedTestLoadFailed"));
    } finally {
      setCallbackTestLoading(false);
    }
  }

  async function activate() {
    if (!identity || !lastTest || lastTest.status !== "passed" || lastTest.configuration_revision !== identity.revision || !(Date.parse(lastTest.expires_at) > Date.now())) return;
    setOperation("activate");
    setError("");
    try {
      const activated = await api.activateIdentity(identity.revision, lastTest.id);
      onChanged(activated);
      onMessage(t("identity.oidcCustomerSignInIsActive"));
    } catch (caught) {
      setError(caught instanceof APIError ? caught.message : t("identity.activationFailed"));
    } finally {
      setOperation(null);
    }
  }

  function cancelEditing() {
    if (!identity) return;
    setIssuerInput(identity.issuer);
    setClientID(identity.client_id);
    setClientSecret("");
    setAudience(identity.audience ?? "");
    setCustomerClaim(identity.customer_account_claim);
    setAuthorizationAPIOrigin(identity.authorization_api_origin);
    setScopes(identity.scopes.length ? identity.scopes.join(" ") : "openid profile email");
    setOAuthResource(identity.oauth_resource ?? "");
    setInstallationClaim(identity.installation_claim ?? "");
    setError("");
    setEditing(false);
  }

  async function disable() {
    if (!identity?.configured) return;
    setOperation("disable");
    setError("");
    try {
      const disabled = await api.disableIdentity(identity.revision);
      onChanged(disabled);
      setConfirmingDisable(false);
      onMessage(t("identity.customerSignInIsDisabled"));
    } catch (caught) {
      setError(caught instanceof APIError ? caught.message : t("identity.disableFailed"));
    } finally {
      setOperation(null);
    }
  }

  async function disconnect() {
    if (!identity?.configured || active) return;
    setOperation("disconnect");
    setError("");
    try {
      const disconnected = await api.disconnectIdentity(identity.revision);
      onChanged(disconnected);
      setConfirmingDisconnect(false);
      onMessage(t("identity.theOIDCProviderIsDisconnected"));
    } catch (caught) {
      setError(caught instanceof APIError ? caught.message : t("identity.disconnectFailed"));
    } finally {
      setOperation(null);
    }
  }

  const pageActions = active && !editing ? <span className="heading-actions">
    <Button outline disabled={operation !== null} onClick={() => setEditing(true)}><Pencil data-slot="icon" />{t("identity.edit")}</Button>
    <Button outline disabled={operation !== null || callbackTestLoading} onClick={() => void beginTest()}><RefreshCw data-slot="icon" />{operation === "test" ? t("identity.openingProvider") : t("identity.testAgain")}</Button>
    <Button color="red" disabled={operation !== null} onClick={() => setConfirmingDisable(true)}>{t("identity.disable")}</Button>
  </span> : undefined;

  if (loading) return <><PageHeader eyebrow={t("identity.identity")} title={t("identity.customerSignIn")} />{navigation}<section className="panel identity-loading"><RefreshCw /><span><strong>{t("identity.loadingIdentitySettings")}</strong><small>{t("identity.gettingTheExactCallbackURLAndCurrentOIDCConnection")}</small></span></section></>;

  if (!identity || loadError) return <><PageHeader eyebrow={t("identity.identity")} title={t("identity.customerSignIn")} />{navigation}<section className="panel identity-load-error"><TriangleAlert /><span><strong>{t("identity.identitySettingsAreUnavailable")}</strong><small>{loadError || t("identity.theServerDidNotReturnIdentitySetupMetadata")}</small></span></section></>;

  return <>
    <PageHeader eyebrow={t("identity.identity")} title={t("identity.customerSignIn")} action={pageActions} />
    {navigation}

    {(error || loadError) && <div className="identity-error" role="alert"><XCircle /><span>{error || loadError}</span></div>}

    {callbackTestError && <div className="identity-callback-warning" role="alert"><TriangleAlert /><span><strong>{t("identity.thisOIDCTestCouldNotBeCompleted")}</strong><small>{t("identity.theCallbackWasExpiredAlreadyUsedOrInvalidIt")}</small></span></div>}

    {reviewRequired && <div className="identity-review-notice" role="status"><TriangleAlert /><span><strong>{t("identity.reviewMigratedSettings")}</strong><small>{t("identity.completeTheRequiredOIDCValuesAndSaveThisDraft")} {identity.credential_present ? t("identity.theEncryptedClientSecretIsReusedUnlessYouReplace") : t("identity.enterTheOIDCClientSecretBeforeSaving")}</small></span></div>}

    {editing ? <div className="identity-setup-grid">
      <section className="panel identity-setup-card">
        <PanelHeader title={t("identity.n1ConnectYourProvider")} description={t("identity.registerOneConfidentialOIDCWebApplicationForDokoSokoCustomer")} action={<Badge color="violet">{t("identity.confidentialWebApp")}</Badge>} />
        <div className="identity-instructions">
          <span>1</span><p>{t("identity.registerA")} <strong>{t("identity.confidentialWebApplication")}</strong> {t("identity.withYourOIDCProvider")}</p>
          <span>2</span><p>{t("identity.addThisExactURLToTheApplicationSAllowed")}</p>
          <span>3</span><p>{t("identity.copyTheExactIssuerFromOIDCDiscoveryThenThe")}</p>
        </div>
        <div className="identity-copy-field">
          <span><small>{t("identity.allowedCallbackURL")}</small><code>{callbackURL}</code></span>
          <Button outline disabled={!callbackURL} aria-label={t("identity.copyOIDCCallbackURL")} onClick={() => void copyText(callbackURL).then(() => onMessage(t("identity.callbackURLCopied")))}><Copy data-slot="icon" />{t("identity.copy")}</Button>
        </div>
        <div className="auth-form compact-form identity-form">
          <label className="auth-field"><span>{t("identity.issuerURL")}</span><input type="url" autoCapitalize="none" autoCorrect="off" spellCheck={false} value={issuerInput} aria-invalid={!issuerReady} onChange={(event) => setIssuerInput(event.target.value)} placeholder="https://identity.example.com/" /><small className={!issuerReady ? "identity-field-error" : undefined}>{issuerReady ? <>{t("identity.useTheExactIssuerValueAdvertisedByTheProvider")} <a className="identity-discovery-link" href={discoveryURL} target="_blank" rel="noreferrer">{t("identity.openDiscoveryDocument")} <ExternalLink /></a></> : t("identity.enterAnExactCredentialFreeHTTPSIssuerURLLocal")}</small></label>
          <div className="two-fields"><label className="auth-field"><span>{t("identity.clientID")}</span><input autoCapitalize="none" autoCorrect="off" spellCheck={false} value={clientID} onChange={(event) => setClientID(event.target.value)} /></label><label className="auth-field"><span>{identity.credential_present ? t("identity.newClientSecretOptional") : t("identity.clientSecret")}</span><input type="password" autoComplete="new-password" placeholder={identity.credential_present ? "************" : undefined} value={clientSecret} onChange={(event) => setClientSecret(event.target.value)} /><small>{identity.credential_present ? t("identity.encryptedSecretStoredTypeANewValueOnlyTo") : t("identity.storedEncryptedAndNeverReturned")}</small></label></div>
        </div>
      </section>

      <section className="panel identity-setup-card">
        <PanelHeader title={t("identity.n2MapYourCustomer")} description={t("identity.tellDokoSokoWhichIDTokenClaimIdentifiesTheCustomer")} />
        <div className="auth-form compact-form identity-form">
          <label className="auth-field"><span>{t("identity.customerAccountClaim")}</span><input autoCapitalize="none" autoCorrect="off" spellCheck={false} value={customerClaim} onChange={(event) => setCustomerClaim(event.target.value)} placeholder="https://your-company.com/customer_id" /><small>{t("identity.requiredStringClaimInTheIDTokenPreferA")}</small></label>
          <div className="identity-separate-field">
            <span className="identity-separate-icon"><ShieldCheck /></span>
            <label className="auth-field"><span>{t("identity.authorizationAPIOrigin")}</span><input type="url" autoCapitalize="none" autoCorrect="off" spellCheck={false} value={authorizationAPIOrigin} aria-invalid={!authorizationAPIOriginReady} onChange={(event) => setAuthorizationAPIOrigin(event.target.value)} placeholder="https://api.your-company.com" /><small className={!authorizationAPIOriginReady ? "identity-field-error" : undefined}>{authorizationAPIOriginReady ? <>{t("identity.thisIsYourAPISeparateFromTheIdentityProvider")} <code>{accessEvaluationURL(authorizationAPIOrigin) || "/v1/access/evaluations"}</code> {t("identity.withTheCustomerToken")}</> : t("identity.useACredentialFreeHTTPSOriginOnTheDefault")}</small></label>
          </div>
          <details className="advanced-details identity-advanced">
            <summary>{t("identity.advancedOIDCSettings")}</summary>
            <div className="advanced-details-body auth-form compact-form">
              <label className="auth-field"><span>{t("identity.scopes")}</span><input value={scopes} aria-invalid={!openIDScopeReady} onChange={(event) => setScopes(event.target.value)} /><small className={!openIDScopeReady ? "identity-field-error" : undefined}>{openIDScopeReady ? <>{t("identity.keep")} <code>openid</code>{t("identity.addOnlyScopesYourAuthorizationAPINeeds")}</> : <><code>openid</code> {t("identity.isRequired")}</>}</small></label>
              <div className="two-fields"><label className="auth-field"><span>{t("identity.audienceOptional")}</span><input autoCapitalize="none" autoCorrect="off" spellCheck={false} value={audience} onChange={(event) => setAudience(event.target.value)} placeholder="https://api.your-company.com" /><small>{t("identity.providerSpecificAudienceParameterForTheAccessToken")}</small></label><label className="auth-field"><span>{t("identity.oauthResourceOptional")}</span><input autoCapitalize="none" autoCorrect="off" spellCheck={false} value={oauthResource} aria-invalid={!oauthResourceReady} onChange={(event) => setOAuthResource(event.target.value)} placeholder={t("identity.urnExampleCustomerApi")} /><small className={!oauthResourceReady ? "identity-field-error" : undefined}>{oauthResourceReady ? t("identity.rfcN8707AbsoluteURIResourceIndicatorWhenRequiredBy") : t("identity.enterAnAbsoluteURIWithoutAFragment")}</small></label></div>
              <label className="auth-field"><span>{t("identity.installationClaimOptional")}</span><input value={installationClaim} onChange={(event) => setInstallationClaim(event.target.value)} placeholder="https://your-company.com/installation_id" /></label>
            </div>
          </details>
        </div>
      </section>

      {active && <div className="identity-edit-warning"><TriangleAlert /><span><strong>{t("identity.savingChangesDisablesCustomerSignIn")}</strong><small>{t("identity.theNewRevisionMustPassARealOIDCSign")}</small></span></div>}
      <div className="identity-form-actions">
        {configured && !reviewRequired && <Button outline disabled={operation !== null} onClick={cancelEditing}>{t("common.cancel")}</Button>}
        <Button color="indigo" disabled={operation !== null || !formReady} onClick={() => void saveDraft()}><KeyRound data-slot="icon" />{operation === "save" ? t("identity.savingDraft") : reviewRequired ? t("identity.saveReviewedDraft") : t("identity.saveDraft")}</Button>
      </div>
    </div> : <>
      <section className="panel identity-summary">
        <PanelHeader title={t("identity.oidcConnection")} description={active ? t("identity.customerSignInIsActiveForPrivateMCPClients") : t("identity.theConfigurationIsSavedButCannotServePrivateClients")} action={<Badge color={active ? "green" : draftTestPassed ? "blue" : "amber"}>{active ? t("identity.active") : draftTestPassed ? t("identity.testedDraft") : t("identity.draft")}</Badge>} />
        <dl className="identity-summary-grid">
          <div><dt>{t("identity.issuer")}</dt><dd>{identity.issuer}</dd></div>
          <div><dt>{t("identity.tokenTarget")}</dt><dd>{identity.audience || identity.oauth_resource || t("identity.providerDefault")}</dd></div>
          <div><dt>{t("identity.customerAccountClaim")}</dt><dd>{identity.customer_account_claim}</dd></div>
          <div><dt>{t("identity.authorizationAPI")}</dt><dd>{identity.authorization_api_origin}</dd></div>
        </dl>
        {!active && <div className="panel-footer-actions"><small>{t("identity.needToChangeAValue")}</small><Button outline disabled={operation !== null} onClick={() => setEditing(true)}><Pencil data-slot="icon" />{t("identity.editDraft")}</Button></div>}
      </section>

      <div className="identity-verification-grid">
        <section className="panel identity-verification-card">
          <header className="identity-verification-header">
            <div className="identity-verification-copy"><h2 className="type-heading">{t("identity.testSignIn")}</h2><p className="type-body">{t("identity.thisVerifiesOnlyTheOIDCSignInAndMapped")}</p></div>
            <Button color={testPassedForCurrentState ? "dark" : "indigo"} outline={testPassedForCurrentState} disabled={operation !== null || callbackTestLoading} onClick={() => void beginTest()}>{operation === "test" ? t("identity.openingProvider") : testPassedForCurrentState ? t("identity.testAgain") : t("identity.testSignIn")}<ExternalLink data-slot="icon" /></Button>
          </header>
          <div className={`identity-test-result ${callbackTestLoading ? "pending" : callbackTestFailure ? "failed" : testStaleForCurrentRevision ? "stale" : draftTestExpiredForActivation ? "expired" : lastTest?.status ?? "untested"}`}>
            {callbackTestLoading ? <RefreshCw /> : callbackTestFailure ? <XCircle /> : testStaleForCurrentRevision || draftTestExpiredForActivation ? <TriangleAlert /> : lastTest?.status === "passed" ? <CheckCircle2 /> : lastTest?.status === "failed" || lastTest?.status === "expired" ? <XCircle /> : <RefreshCw />}
            <span><strong>{callbackTestLoading ? t("identity.loadingReturnedOIDCTest") : callbackTestFailure ? t("identity.couldNotLoadReturnedOIDCTest") : testStaleForCurrentRevision ? t("identity.notTestedForThisRevision") : draftTestExpiredForActivation ? t("identity.testExpiredForActivation") : testStatusLabel(lastTest?.status, t)}</strong><small>{callbackTestLoading ? t("identity.retrievingTheExactTestTransactionFromThisOIDCCallback") : callbackTestFailure || (testStaleForCurrentRevision ? t("identity.runTestSignInForRevision", { revision: String(identity.revision) }) : draftTestExpiredForActivation ? t("identity.runTestSignInAgainThenActivateBeforeThe") : identityTestMessage(lastTest, identity.customer_account_claim, t))}</small>{callbackTestFailure && <Button outline className="identity-test-retry" disabled={callbackTestLoading || operation !== null} onClick={() => void retryCallbackTest()}>{t("common.retry")}</Button>}</span>
          </div>
        </section>

        <section className="panel identity-verification-card">
          <header className="identity-verification-header">
            <div className="identity-verification-copy"><h2 className="type-heading">{t("identity.activateCustomerSignIn")}</h2><p className="type-body">{t("identity.activationIsAllowedOnlyForTheExactConfigurationRevision")}</p></div>
            {!active && <Button color="indigo" disabled={operation !== null || !draftTestPassed} onClick={() => void activate()}>{operation === "activate" ? t("identity.activating") : t("identity.activate")}</Button>}
          </header>
          {active && <div className="identity-active-state"><LockKeyhole /><span><strong>{t("identity.privateIdentityIsActive")}</strong><small>{t("identity.revision")} {identity.revision}{t("identity.disablingThisConnectionFailsClosed")}</small></span></div>}
          {!active && !draftTestPassed && <small className="identity-action-hint">{draftTestExpiredForActivation ? t("identity.thePassedTestExpiredTestSignInAgainTo") : t("identity.passTheOIDCSignInTestForRevisionFirst", { revision: identity.revision })}</small>}
        </section>
      </div>

      {configured && !active && <section className="panel identity-disconnect-zone">
        <PanelHeader title={t("identity.disconnectProvider")} description={t("identity.removeThisDisabledOIDCConnectionWhenItIsNo")} action={!confirmingDisconnect ? <Button color="red" outline disabled={operation !== null} onClick={() => setConfirmingDisconnect(true)}>{t("identity.disconnectProvider")}</Button> : undefined} />
        {confirmingDisconnect && <div className="identity-disconnect-confirm" role="alert"><TriangleAlert /><span><strong>{t("identity.disconnectThisOIDCProviderPermanently")}</strong><small>{t("identity.thisRemovesTheSavedOIDCConfigurationEncryptedClientSecret")}</small></span><span className="heading-actions"><Button outline disabled={operation !== null} onClick={() => setConfirmingDisconnect(false)}>{t("common.cancel")}</Button><Button color="red" disabled={operation !== null} onClick={() => void disconnect()}>{operation === "disconnect" ? t("identity.disconnecting") : t("identity.disconnectProvider")}</Button></span></div>}
      </section>}
    </>}

    {confirmingDisable && <section className="panel identity-disable-confirm" aria-labelledby="disable-identity-title">
      <TriangleAlert />
      <span><strong id="disable-identity-title">{t("identity.disableCustomerSignIn")}</strong><small>{t("identity.newPrivateMCPSessionsWillFailImmediatelyYourOIDC")}</small></span>
      <span className="heading-actions"><Button outline disabled={operation !== null} onClick={() => setConfirmingDisable(false)}>{t("common.cancel")}</Button><Button color="red" disabled={operation !== null} onClick={() => void disable()}>{operation === "disable" ? t("identity.disabling") : t("identity.disableSignIn")}</Button></span>
    </section>}
  </>;
}
