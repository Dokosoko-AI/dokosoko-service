"use client";


import { useTranslation } from "react-i18next";
import { Check, Copy, Eye, EyeOff, KeyRound, LockKeyhole, ShieldCheck, TriangleAlert } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { APIDeployment, APIError, APIOrganisation, APIUser, SetupEnrollment, api } from "../lib/api";
import type { ConsoleFixtures } from "../dev/console-fixtures";
import { Button } from "./core/control";
import { ConsoleApp } from "./ConsoleApp";

type Gate = "loading" | "console" | "setup" | "login" | "onboarding" | "error";
type SetupStep = "identity" | "mfa" | "recovery";

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof APIError ? error.message : fallback;
}

export function RootGate() {
  const { t } = useTranslation();
  const [gate, setGate] = useState<Gate>("loading");
  const [consoleMode, setConsoleMode] = useState<"live" | "fixtures">("live");
  const [consoleFixtures, setConsoleFixtures] = useState<ConsoleFixtures | null>(null);
  const [user, setUser] = useState<APIUser | null>(null);
  const [deployment, setDeployment] = useState<APIDeployment | null>(null);
  const [onboardingOrganisation, setOnboardingOrganisation] = useState<APIOrganisation | null>(null);
  const [problem, setProblem] = useState("");

  const openWorkspace = useCallback(async (value: APIUser) => {
    setUser(value);
    try {
      const organisations = await api.organisations();
      if (organisations.length === 0) {
        setOnboardingOrganisation(null);
        setGate("onboarding");
        return;
      }
      try {
        const currentDeployment = await api.deployment();
        setDeployment(currentDeployment);
        setGate("console");
      } catch (error) {
        if (!(error instanceof APIError) || error.status !== 404) throw error;
        setOnboardingOrganisation(organisations[0]);
        setGate("onboarding");
      }
    } catch (error) {
      setProblem(errorMessage(error, t("auth.requestFailed")));
      setGate("error");
    }
  }, [t]);

  useEffect(() => {
    if (
      process.env.NODE_ENV === "development" &&
      new URLSearchParams(window.location.search).get("preview") === "fixtures"
    ) {
      // Fixture mode is URL-driven browser state. Load its data only in a
      // development build so production bundles contain no sample tenant.
      let cancelled = false;
      import("../dev/console-fixtures").then(({ consoleFixtures: fixtures }) => {
        if (cancelled) return;
        setConsoleFixtures(fixtures);
        setConsoleMode("fixtures");
        setGate("console");
      }).catch((error) => {
        if (cancelled) return;
        setProblem(errorMessage(error, t("auth.requestFailed")));
        setGate("error");
      });
      return () => { cancelled = true; };
    }

    let cancelled = false;
    api.setupStatus().then(async (status) => {
      if (cancelled) return;
      if (!status.setup_complete) {
        setGate("setup");
        return;
      }
      try {
        const session = await api.session();
        if (!cancelled) {
          await openWorkspace(session.user);
        }
      } catch (error) {
        if (!cancelled) {
          if (error instanceof APIError && error.status === 401) {
            setGate("login");
          } else {
            setProblem(errorMessage(error, t("auth.requestFailed")));
            setGate("error");
          }
        }
      }
    }).catch((error) => {
      if (!cancelled) {
        setProblem(errorMessage(error, t("auth.requestFailed")));
        setGate("error");
      }
    });
    return () => { cancelled = true; };
  }, [openWorkspace, t]);

  async function logout() {
    try {
      await api.logout();
    } finally {
      setUser(null);
      setDeployment(null);
      setGate("login");
    }
  }

  if (gate === "loading") return <ConsoleLoadingShell />;
  if (gate === "setup") return <SetupScreen onComplete={openWorkspace} />;
  if (gate === "login") return <LoginScreen onComplete={openWorkspace} />;
  if (gate === "onboarding") return <WorkspaceSetup existingOrganisation={onboardingOrganisation} onComplete={(value) => { setDeployment(value); setGate("console"); }} />;
  if (gate === "error") return <AuthShell icon={<TriangleAlert />} title={t("auth.deploymentNeedsAttention")} description={problem || t("auth.authenticationIsNotConfiguredCheckTheSetupTokenMaster")} />;
  return <ConsoleApp key={deployment?.id ?? "fixture-preview"} mode={consoleMode} fixtures={consoleFixtures} currentUser={user} currentDeployment={deployment} onLogout={user ? logout : undefined} />;
}

function ConsoleLoadingShell() {
  return (
    <div className="app-shell console-loading-shell" aria-hidden="true">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark">D</span>
          <span className="console-loading-line console-loading-brand" />
        </div>
        <nav>
          {Array.from({ length: 7 }, (_, index) => (
            <span className="console-loading-nav-row" key={index}>
              <i />
              <b />
            </span>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <span className="console-loading-preferences" />
          <span className="console-loading-account" />
        </div>
      </aside>
      <div className="console-loading-workspace">
        <header className="topbar">
          <div className="topbar-inner">
            <span className="console-loading-product" />
          </div>
        </header>
        <main>
          <div className="content">
            <div className="console-loading-heading">
              <span />
              <strong />
            </div>
            <div className="console-loading-panel" />
            <div className="console-loading-grid">
              <span />
              <span />
            </div>
          </div>
        </main>
      </div>
    </div>
  );
}

function slugify(value: string): string {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "").slice(0, 63);
}

function WorkspaceSetup({ existingOrganisation, onComplete }: { existingOrganisation: APIOrganisation | null; onComplete: (deployment: APIDeployment) => void }) {
  const { t } = useTranslation();
  const [organisationName, setOrganisationName] = useState(existingOrganisation?.name ?? "");
  const [deploymentName, setDeploymentName] = useState("");
  const [environmentName, setEnvironmentName] = useState("Production");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");

  async function create(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setProblem("");
    try {
      const organisation = existingOrganisation ?? await api.createOrganisation(organisationName, slugify(organisationName));
      const deployment = await api.createDeployment(organisation.id, deploymentName, slugify(deploymentName));
      await api.createDeploymentEnvironment(organisation.id, environmentName, slugify(environmentName), true);
      onComplete(deployment);
    } catch (error) {
      setProblem(errorMessage(error, t("auth.requestFailed")));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell icon={<ShieldCheck />} title={t("auth.configureThisDokoSokoDeployment")} description={t("auth.nameThisConnectorDeploymentAndCreateTheFirstEnvironment")}>
      <form className="auth-form" onSubmit={create}>
        <Field label={t("auth.organisation")}><input required disabled={Boolean(existingOrganisation)} value={organisationName} onChange={(event) => setOrganisationName(event.target.value)} /></Field>
        <Field label={t("auth.deploymentName")} hint={t("auth.theIdentityOfThisDokoSokoInstallationAPIsAndVersions")}><input required value={deploymentName} onChange={(event) => setDeploymentName(event.target.value)} placeholder={t("auth.developerPlatformConnector")} /></Field>
        <Field label={t("auth.firstEnvironment")} hint={t("auth.theDeploymentStageAgentsShouldTargetFirstSuchAs")}><input required value={environmentName} onChange={(event) => setEnvironmentName(event.target.value)} /></Field>
        {problem && <AuthProblem>{problem}</AuthProblem>}
        <Button type="submit" color="indigo" disabled={busy}>{busy ? t("auth.creatingWorkspace") : t("auth.createAndOpenConsole")}</Button>
      </form>
    </AuthShell>
  );
}

function SetupScreen({ onComplete }: { onComplete: (user: APIUser) => void }) {
  const { t } = useTranslation();
  const [step, setStep] = useState<SetupStep>("identity");
  const [setupToken, setSetupToken] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [enrollment, setEnrollment] = useState<SetupEnrollment | null>(null);
  const [code, setCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [createdUser, setCreatedUser] = useState<APIUser | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");

  async function begin(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setProblem("");
    try {
      const value = await api.beginSetup(setupToken, { email, password });
      setEnrollment(value);
      setStep("mfa");
    } catch (error) {
      setProblem(errorMessage(error, t("auth.requestFailed")));
    } finally {
      setBusy(false);
    }
  }

  async function complete(event: FormEvent) {
    event.preventDefault();
    if (!enrollment) return;
    setBusy(true);
    setProblem("");
    try {
      const result = await api.completeSetup(enrollment.enrollment_id, code);
      setCreatedUser(result.user);
      setRecoveryCodes(result.recovery_codes);
      setStep("recovery");
    } catch (error) {
      setProblem(errorMessage(error, t("auth.requestFailed")));
    } finally {
      setBusy(false);
    }
  }

  async function copyRecoveryCodes() {
    await navigator.clipboard.writeText(recoveryCodes.join("\n"));
  }

  if (step === "mfa" && enrollment) {
    return (
      <AuthShell icon={<ShieldCheck />} title={t("auth.secureTheRootAccount")} description={t("auth.scanTheQRCodeWithGoogleAuthenticatorThenEnter")}>
        <div className="setup-progress"><span className="done">1</span><i /><span className="active">2</span><i /><span>3</span></div>
        <div className="authenticator-setup">
          <strong>{t("auth.setupGoogleAuthenticator")}</strong>
          <div className="authenticator-qr">
            <QRCodeSVG value={enrollment.provisioning_uri} size={176} level="M" marginSize={2} title={t("auth.googleAuthenticatorSetupQRCode")} />
          </div>
          <div className="authenticator-secret">
            <small>{t("auth.alternativelyManuallySetUpWithThisSecret")}</small>
            <code>{enrollment.totp_secret}</code>
            <a href={enrollment.provisioning_uri}>{t("auth.openAuthenticatorApp")}</a>
          </div>
        </div>
        <form className="auth-form" onSubmit={complete}>
          <Field label={t("auth.sixDigitCode")}><input required inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} placeholder="123456" value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, ""))} /></Field>
          {problem && <AuthProblem>{problem}</AuthProblem>}
          <Button type="submit" color="indigo" disabled={busy || code.length !== 6}>{busy ? t("common.verifying") : t("auth.verifyAndCreateRootUser")}</Button>
        </form>
      </AuthShell>
    );
  }

  if (step === "recovery" && createdUser) {
    return (
      <AuthShell icon={<Check />} title={t("auth.saveYourRecoveryCodes")} description={t("auth.theseOneTimeCodesAreTheOnlyFallbackIf")}>
        <div className="setup-progress"><span className="done">1</span><i /><span className="done">2</span><i /><span className="active">3</span></div>
        <div className="recovery-grid">{recoveryCodes.map((value) => <code key={value}>{value}</code>)}</div>
        <div className="auth-actions"><Button type="button" outline onClick={copyRecoveryCodes}><Copy data-slot="icon" />{t("auth.copyCodes")}</Button><Button type="button" color="indigo" onClick={() => onComplete(createdUser)}>{t("auth.iSavedThemOpenConsole")}</Button></div>
      </AuthShell>
    );
  }

  return (
    <AuthShell icon={<KeyRound />} title={t("auth.createTheFirstRootUser")} description={t("auth.useTheOneTimeSetupTokenFromYourDeployment")}>
      <div className="setup-progress"><span className="active">1</span><i /><span>2</span><i /><span>3</span></div>
      <form className="auth-form" onSubmit={begin}>
        <Field label={t("auth.setupToken")}><input required type="password" autoComplete="off" value={setupToken} onChange={(event) => setSetupToken(event.target.value)} /></Field>
        <Field label={t("auth.rootEmail")}><input required type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} /></Field>
        <Field label={t("auth.password")} hint={t("auth.n14CharactersWithUpperLowerAndANumber")}>
          <div className="password-input">
            <input required type={showPassword ? "text" : "password"} autoComplete="new-password" minLength={14} value={password} onChange={(event) => setPassword(event.target.value)} />
            <button type="button" className="password-visibility" aria-label={showPassword ? t("auth.hidePassword") : t("auth.showPassword")} aria-pressed={showPassword} onClick={() => setShowPassword((visible) => !visible)}>
              {showPassword ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
            </button>
          </div>
        </Field>
        {problem && <AuthProblem>{problem}</AuthProblem>}
        <Button type="submit" color="indigo" disabled={busy}>{busy ? t("auth.preparingMFA") : t("auth.continueToMFA")}</Button>
      </form>
    </AuthShell>
  );
}

function LoginScreen({ onComplete }: { onComplete: (user: APIUser) => void }) {
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");

  async function login(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setProblem("");
    try {
      const session = await api.login(email, password, code);
      onComplete(session.user);
    } catch (error) {
      setProblem(errorMessage(error, t("auth.requestFailed")));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell icon={<LockKeyhole />} title={t("auth.signInToDokoSoko")} description={t("auth.rootAdministrationRequiresYourPasswordAndCurrentAuthenticatorCode")}>
      <form className="auth-form" onSubmit={login}>
        <Field label={t("auth.email")}><input required type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} /></Field>
        <Field label={t("auth.password")}><input required type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></Field>
        <Field label={t("auth.authenticatorCode")}><input required inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, ""))} /></Field>
        {problem && <AuthProblem>{problem}</AuthProblem>}
        <Button type="submit" color="indigo" disabled={busy || code.length !== 6}>{busy ? t("auth.signingIn") : t("auth.signIn")}</Button>
      </form>
    </AuthShell>
  );
}

function AuthShell({ icon, title, description, children }: { icon: React.ReactNode; title: string; description: string; children?: React.ReactNode }) {
  const { t } = useTranslation();
  return <main className="auth-shell"><section className="auth-card"><span className="auth-icon">{icon}</span><h1 className="type-section-title">{title}</h1><p className="type-body">{description}</p>{children}</section><footer className="type-caption">{t("auth.privateByDefaultMFAEnforcedSecurityEventsAudited")}</footer></main>;
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return <label className="auth-field"><span>{label}</span>{children}{hint && <small>{hint}</small>}</label>;
}

function AuthProblem({ children }: { children: React.ReactNode }) {
  return <div className="auth-problem" role="alert"><TriangleAlert />{children}</div>;
}
