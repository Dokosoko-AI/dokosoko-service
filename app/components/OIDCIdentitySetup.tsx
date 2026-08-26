"use client";

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
import { useEffect, useState } from "react";
import { APIError, APIIdentity, APIIdentityTest, api } from "../lib/api";
import { Badge, Button } from "./core/control";
import { PageHeader, PanelHeader } from "./core/layout";

type IdentityOperation = "save" | "test" | "activate" | "disable" | "disconnect" | null;

type OIDCIdentitySetupProps = {
  identity: APIIdentity | null;
  loading: boolean;
  loadError: string;
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

function testStatusLabel(status?: APIIdentityTest["status"]) {
  if (status === "passed") return "Passed";
  if (status === "failed") return "Failed";
  if (status === "expired") return "Expired";
  if (status === "pending") return "Waiting for sign-in";
  return "Not tested";
}

function identityTestMessage(test: APIIdentityTest | undefined, customerClaim: string) {
  if (!test) return "Complete one real sign-in before activation.";
  if (test.status === "passed") return `Customer ${test.customer_id || "claim resolved"}${test.completed_at ? ` · ${new Date(test.completed_at).toLocaleString()}` : ""}`;
  if (test.status === "pending") return "Finish the provider sign-in in this browser to complete the test.";
  if (test.status === "expired" || test.failure_code === "test_expired") return "This test expired. Start a new test and complete the provider sign-in within ten minutes.";
  if (test.failure_code === "configuration_changed") return "The saved configuration changed during the test. Start a fresh test for the current draft.";
  if (test.failure_code === "authorization_denied") return "The provider sign-in was cancelled or denied. Start the test again when the test user can continue.";
  if (test.failure_code === "authorization_code_missing") return "The provider returned without an authorization code. Check the exact callback URL and try again.";
  if (test.failure_code === "oidc_authorization_failed") return "DokoSoko could not open the OIDC authorization flow. Check the issuer and client settings.";
  if (test.failure_code === "oidc_verification_failed") return "The OIDC callback could not be verified. Check the client secret, callback URL, and optional audience or resource.";
  if (test.failure_code === "client_authentication_unsupported") return "The confidential OIDC client must support client_secret_basic or client_secret_post.";
  if (test.failure_code === "issuer_mismatch") return "The provider returned a different issuer. Use the exact issuer advertised by its discovery document.";
  if (test.failure_code === "subject_missing") return "The ID token did not include a subject claim. Check the provider application and token configuration.";
  if (test.failure_code === "customer_claim_missing") return `The ID token did not include ${customerClaim || "the customer account claim"}. Add that claim at the identity provider and test again.`;
  if (test.failure_code === "access_token_missing") return "The provider did not issue an API access token. Check the optional audience or resource and user consent.";
  if (test.failure_code === "access_token_expired") return "The provider issued an already-expired access token. Check its token settings and test again.";
  return "The OIDC sign-in could not be verified. Review the connection settings and start a new test.";
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

export function OIDCIdentitySetup({ identity, loading, loadError, onChanged, onMessage }: OIDCIdentitySetupProps) {
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
      if (!cancelled) setCallbackTestFailure(caught instanceof APIError ? caught.message : "The returned OIDC test could not be loaded.");
    }).finally(() => {
      if (cancelled) return;
      setCallbackTestLoading(false);
    });
    return () => { cancelled = true; };
  }, [callbackTestID, canLoadCallbackTest]);

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
      onMessage(active ? "Changes saved as a disabled draft. Test again before activating." : "OIDC draft saved. Test a real sign-in next.");
    } catch (caught) {
      setError(caught instanceof APIError ? caught.message : "The OIDC draft could not be saved.");
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
      setError(caught instanceof APIError || caught instanceof Error ? caught.message : "The OIDC test could not be started.");
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
      setCallbackTestFailure(caught instanceof APIError ? caught.message : "The returned OIDC test could not be loaded.");
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
      onMessage("OIDC customer sign-in is active.");
    } catch (caught) {
      setError(caught instanceof APIError ? caught.message : "OIDC could not be activated.");
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
      onMessage("Customer sign-in is disabled.");
    } catch (caught) {
      setError(caught instanceof APIError ? caught.message : "Customer sign-in could not be disabled.");
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
      onMessage("The OIDC provider is disconnected.");
    } catch (caught) {
      setError(caught instanceof APIError ? caught.message : "The OIDC provider could not be disconnected.");
    } finally {
      setOperation(null);
    }
  }

  const pageActions = active && !editing ? <span className="heading-actions">
    <Button outline disabled={operation !== null} onClick={() => setEditing(true)}><Pencil data-slot="icon" />Edit</Button>
    <Button outline disabled={operation !== null || callbackTestLoading} onClick={() => void beginTest()}><RefreshCw data-slot="icon" />{operation === "test" ? "Opening provider…" : "Test again"}</Button>
    <Button color="red" disabled={operation !== null} onClick={() => setConfirmingDisable(true)}>Disable</Button>
  </span> : undefined;

  if (loading) return <><PageHeader eyebrow="Identity" title="Customer sign-in" description="Connect one OIDC provider for all private MCP clients." /><section className="panel identity-loading"><RefreshCw /><span><strong>Loading identity settings</strong><small>Getting the exact callback URL and current OIDC connection.</small></span></section></>;

  if (!identity || loadError) return <><PageHeader eyebrow="Identity" title="Customer sign-in" description="Connect one OIDC provider for all private MCP clients." /><section className="panel identity-load-error"><TriangleAlert /><span><strong>Identity settings are unavailable</strong><small>{loadError || "The server did not return identity setup metadata."}</small></span></section></>;

  return <>
    <PageHeader eyebrow="Identity" title="Customer sign-in" description="Connect an OIDC provider, verify a real customer login, then activate customer sign-in." action={pageActions} />

    {(error || loadError) && <div className="identity-error" role="alert"><XCircle /><span>{error || loadError}</span></div>}

    {callbackTestError && <div className="identity-callback-warning" role="alert"><TriangleAlert /><span><strong>This OIDC test could not be completed</strong><small>The callback was expired, already used, or invalid. It did not pass or change the saved test result. Start a sign-in test again.</small></span></div>}

    {reviewRequired && <div className="identity-review-notice" role="status"><TriangleAlert /><span><strong>Review migrated settings</strong><small>Complete the required OIDC values and save this draft before testing. {identity.credential_present ? "The encrypted client secret is reused unless you replace it." : "Enter the OIDC client secret before saving."}</small></span></div>}

    {editing ? <div className="identity-setup-grid">
      <section className="panel identity-setup-card">
        <PanelHeader title="1 · Connect your provider" description="Register one confidential OIDC web application for DokoSoko customer sign-in." action={<Badge color="violet">Confidential web app</Badge>} />
        <div className="identity-instructions">
          <span>1</span><p>Register a <strong>confidential web application</strong> with your OIDC provider.</p>
          <span>2</span><p>Add this exact URL to the application&apos;s allowed callback URLs.</p>
          <span>3</span><p>Copy the exact issuer from OIDC discovery, then the application&apos;s Client ID and Client Secret.</p>
        </div>
        <div className="identity-copy-field">
          <span><small>Allowed callback URL</small><code>{callbackURL}</code></span>
          <Button outline disabled={!callbackURL} aria-label="Copy OIDC callback URL" onClick={() => void copyText(callbackURL).then(() => onMessage("Callback URL copied."))}><Copy data-slot="icon" />Copy</Button>
        </div>
        <div className="auth-form compact-form identity-form">
          <label className="auth-field"><span>Issuer URL</span><input type="url" autoCapitalize="none" autoCorrect="off" spellCheck={false} value={issuerInput} aria-invalid={!issuerReady} onChange={(event) => setIssuerInput(event.target.value)} placeholder="https://identity.example.com/" /><small className={!issuerReady ? "identity-field-error" : undefined}>{issuerReady ? <>Use the exact issuer value advertised by the provider. <a className="identity-discovery-link" href={discoveryURL} target="_blank" rel="noreferrer">Open discovery document <ExternalLink /></a></> : "Enter an exact credential-free HTTPS issuer URL; local HTTP is allowed."}</small></label>
          <div className="two-fields"><label className="auth-field"><span>Client ID</span><input autoCapitalize="none" autoCorrect="off" spellCheck={false} value={clientID} onChange={(event) => setClientID(event.target.value)} /></label><label className="auth-field"><span>{identity.credential_present ? "New client secret (optional)" : "Client secret"}</span><input type="password" autoComplete="new-password" placeholder={identity.credential_present ? "************" : undefined} value={clientSecret} onChange={(event) => setClientSecret(event.target.value)} /><small>{identity.credential_present ? "Encrypted secret stored. Type a new value only to replace it." : "Stored encrypted and never returned."}</small></label></div>
        </div>
      </section>

      <section className="panel identity-setup-card">
        <PanelHeader title="2 · Map your customer" description="Tell DokoSoko which ID-token claim identifies the customer account." />
        <div className="auth-form compact-form identity-form">
          <label className="auth-field"><span>Customer account claim</span><input autoCapitalize="none" autoCorrect="off" spellCheck={false} value={customerClaim} onChange={(event) => setCustomerClaim(event.target.value)} placeholder="https://your-company.com/customer_id" /><small>Required string claim in the ID token. Prefer a stable, namespaced claim configured at your identity provider.</small></label>
          <div className="identity-separate-field">
            <span className="identity-separate-icon"><ShieldCheck /></span>
            <label className="auth-field"><span>Authorization API origin</span><input type="url" autoCapitalize="none" autoCorrect="off" spellCheck={false} value={authorizationAPIOrigin} aria-invalid={!authorizationAPIOriginReady} onChange={(event) => setAuthorizationAPIOrigin(event.target.value)} placeholder="https://api.your-company.com" /><small className={!authorizationAPIOriginReady ? "identity-field-error" : undefined}>{authorizationAPIOriginReady ? <>This is your API, separate from the identity provider. During sign-in DokoSoko calls <code>{accessEvaluationURL(authorizationAPIOrigin) || "/v1/access/evaluations"}</code> with the customer token.</> : "Use a credential-free HTTPS origin on the default port; local HTTP is allowed."}</small></label>
          </div>
          <details className="advanced-details identity-advanced">
            <summary>Advanced OIDC settings</summary>
            <div className="advanced-details-body auth-form compact-form">
              <label className="auth-field"><span>Scopes</span><input value={scopes} aria-invalid={!openIDScopeReady} onChange={(event) => setScopes(event.target.value)} /><small className={!openIDScopeReady ? "identity-field-error" : undefined}>{openIDScopeReady ? <>Keep <code>openid</code>; add only scopes your authorization API needs.</> : <><code>openid</code> is required.</>}</small></label>
              <div className="two-fields"><label className="auth-field"><span>Audience (optional)</span><input autoCapitalize="none" autoCorrect="off" spellCheck={false} value={audience} onChange={(event) => setAudience(event.target.value)} placeholder="https://api.your-company.com" /><small>Provider-specific audience parameter for the access token.</small></label><label className="auth-field"><span>OAuth resource (optional)</span><input autoCapitalize="none" autoCorrect="off" spellCheck={false} value={oauthResource} aria-invalid={!oauthResourceReady} onChange={(event) => setOAuthResource(event.target.value)} placeholder="urn:example:customer-api" /><small className={!oauthResourceReady ? "identity-field-error" : undefined}>{oauthResourceReady ? "RFC 8707 absolute URI resource indicator, when required by your provider." : "Enter an absolute URI without a fragment."}</small></label></div>
              <label className="auth-field"><span>Installation claim (optional)</span><input value={installationClaim} onChange={(event) => setInstallationClaim(event.target.value)} placeholder="https://your-company.com/installation_id" /></label>
            </div>
          </details>
        </div>
      </section>

      {active && <div className="identity-edit-warning"><TriangleAlert /><span><strong>Saving changes disables customer sign-in.</strong><small>The new revision must pass a real OIDC sign-in test before it can be activated.</small></span></div>}
      <div className="identity-form-actions">
        {configured && !reviewRequired && <Button outline disabled={operation !== null} onClick={cancelEditing}>Cancel</Button>}
        <Button color="indigo" disabled={operation !== null || !formReady} onClick={() => void saveDraft()}><KeyRound data-slot="icon" />{operation === "save" ? "Saving draft…" : reviewRequired ? "Save reviewed draft" : "Save draft"}</Button>
      </div>
    </div> : <>
      <section className="panel identity-summary">
        <PanelHeader title="OIDC connection" description={active ? "Customer sign-in is active for private MCP clients." : "The configuration is saved but cannot serve private clients until it passes a test and is activated."} action={<Badge color={active ? "green" : draftTestPassed ? "blue" : "amber"}>{active ? "Active" : draftTestPassed ? "Tested draft" : "Draft"}</Badge>} />
        <dl className="identity-summary-grid">
          <div><dt>Issuer</dt><dd>{identity.issuer}</dd></div>
          <div><dt>Token target</dt><dd>{identity.audience || identity.oauth_resource || "Provider default"}</dd></div>
          <div><dt>Customer account claim</dt><dd>{identity.customer_account_claim}</dd></div>
          <div><dt>Authorization API</dt><dd>{identity.authorization_api_origin}</dd></div>
        </dl>
        {!active && <div className="panel-footer-actions"><small>Need to change a value?</small><Button outline disabled={operation !== null} onClick={() => setEditing(true)}><Pencil data-slot="icon" />Edit draft</Button></div>}
      </section>

      <div className="identity-verification-grid">
        <section className="panel identity-verification-card">
          <header className="identity-verification-header">
            <div className="identity-verification-copy"><h2 className="type-heading">Test sign-in</h2><p className="type-body">This verifies only the OIDC sign-in and mapped customer claim. Use each API&apos;s Test tab to verify end-to-end authorization.</p></div>
            <Button color={testPassedForCurrentState ? "dark" : "indigo"} outline={testPassedForCurrentState} disabled={operation !== null || callbackTestLoading} onClick={() => void beginTest()}>{operation === "test" ? "Opening provider…" : testPassedForCurrentState ? "Test again" : "Test sign-in"}<ExternalLink data-slot="icon" /></Button>
          </header>
          <div className={`identity-test-result ${callbackTestLoading ? "pending" : callbackTestFailure ? "failed" : testStaleForCurrentRevision ? "stale" : draftTestExpiredForActivation ? "expired" : lastTest?.status ?? "untested"}`}>
            {callbackTestLoading ? <RefreshCw /> : callbackTestFailure ? <XCircle /> : testStaleForCurrentRevision || draftTestExpiredForActivation ? <TriangleAlert /> : lastTest?.status === "passed" ? <CheckCircle2 /> : lastTest?.status === "failed" || lastTest?.status === "expired" ? <XCircle /> : <RefreshCw />}
            <span><strong>{callbackTestLoading ? "Loading returned OIDC test" : callbackTestFailure ? "Could not load returned OIDC test" : testStaleForCurrentRevision ? "Not tested for this revision" : draftTestExpiredForActivation ? "Test expired for activation" : testStatusLabel(lastTest?.status)}</strong><small>{callbackTestLoading ? "Retrieving the exact test transaction from this OIDC callback." : callbackTestFailure || (testStaleForCurrentRevision ? `Run Test sign-in for revision ${identity.revision}.` : draftTestExpiredForActivation ? "Run Test sign-in again, then activate before the new test expires." : identityTestMessage(lastTest, identity.customer_account_claim))}</small>{callbackTestFailure && <Button outline className="identity-test-retry" disabled={callbackTestLoading || operation !== null} onClick={() => void retryCallbackTest()}>Retry</Button>}</span>
          </div>
        </section>

        <section className="panel identity-verification-card">
          <header className="identity-verification-header">
            <div className="identity-verification-copy"><h2 className="type-heading">Activate customer sign-in</h2><p className="type-body">Activation is allowed only for the exact configuration revision that passed the OIDC sign-in test.</p></div>
            {!active && <Button color="indigo" disabled={operation !== null || !draftTestPassed} onClick={() => void activate()}>{operation === "activate" ? "Activating…" : "Activate"}</Button>}
          </header>
          {active && <div className="identity-active-state"><LockKeyhole /><span><strong>Private identity is active</strong><small>Revision {identity.revision}. Disabling this connection fails closed.</small></span></div>}
          {!active && !draftTestPassed && <small className="identity-action-hint">{draftTestExpiredForActivation ? "The passed test expired. Test sign-in again to activate this draft." : <>Pass the OIDC sign-in test for revision {identity.revision} first.</>}</small>}
        </section>
      </div>

      {configured && !active && <section className="panel identity-disconnect-zone">
        <PanelHeader title="Disconnect provider" description="Remove this disabled OIDC connection when it is no longer used." action={!confirmingDisconnect ? <Button color="red" outline disabled={operation !== null} onClick={() => setConfirmingDisconnect(true)}>Disconnect provider</Button> : undefined} />
        {confirmingDisconnect && <div className="identity-disconnect-confirm" role="alert"><TriangleAlert /><span><strong>Disconnect this OIDC provider permanently?</strong><small>This removes the saved OIDC configuration, encrypted client secret, and test history. Customer-account and audit records remain.</small></span><span className="heading-actions"><Button outline disabled={operation !== null} onClick={() => setConfirmingDisconnect(false)}>Cancel</Button><Button color="red" disabled={operation !== null} onClick={() => void disconnect()}>{operation === "disconnect" ? "Disconnecting…" : "Disconnect provider"}</Button></span></div>}
      </section>}
    </>}

    {confirmingDisable && <section className="panel identity-disable-confirm" aria-labelledby="disable-identity-title">
      <TriangleAlert />
      <span><strong id="disable-identity-title">Disable customer sign-in?</strong><small>New private MCP sessions will fail immediately. Your OIDC configuration stays saved.</small></span>
      <span className="heading-actions"><Button outline disabled={operation !== null} onClick={() => setConfirmingDisable(false)}>Cancel</Button><Button color="red" disabled={operation !== null} onClick={() => void disable()}>{operation === "disable" ? "Disabling…" : "Disable sign-in"}</Button></span>
    </section>}
  </>;
}
