"use client";

import { Check, Copy, Eye, EyeOff, KeyRound, LockKeyhole, ShieldCheck, TriangleAlert } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { FormEvent, useEffect, useState } from "react";
import { APIDeployment, APIError, APIOrganisation, APIUser, SetupEnrollment, api } from "../lib/api";
import { Button } from "./core/control";
import { ConsoleApp } from "./ConsoleApp";

type Gate = "loading" | "console" | "setup" | "login" | "onboarding" | "error";
type SetupStep = "identity" | "mfa" | "recovery";

function errorMessage(error: unknown): string {
  return error instanceof APIError ? error.message : "DokoSoko could not complete the request.";
}

export function RootGate() {
  const [gate, setGate] = useState<Gate>("loading");
  const [user, setUser] = useState<APIUser | null>(null);
  const [deployment, setDeployment] = useState<APIDeployment | null>(null);
  const [onboardingOrganisation, setOnboardingOrganisation] = useState<APIOrganisation | null>(null);
  const [problem, setProblem] = useState("");

  useEffect(() => {
    if (
      process.env.NODE_ENV === "development" &&
      new URLSearchParams(window.location.search).get("preview") === "fixtures"
    ) {
      // Fixture mode is URL-driven browser state. Resolve it after hydration so
      // the server and first client render share the same safe loading shell.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setGate("console");
      return;
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
        if (!cancelled) setGate(error instanceof APIError && error.status === 401 ? "login" : "error");
      }
    }).catch((error) => {
      // A standalone static preview has no service API and intentionally displays
      // the fixture console. A responding, misconfigured service gets an error gate.
      if (!cancelled) {
        if (error instanceof APIError) {
          setProblem(error.message);
          setGate("error");
        } else {
          setGate("console");
        }
      }
    });
    return () => { cancelled = true; };
  }, []);

  async function openWorkspace(value: APIUser) {
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
      setProblem(errorMessage(error));
      setGate("error");
    }
  }

  async function logout() {
    try {
      await api.logout();
    } finally {
      setUser(null);
      setDeployment(null);
      setGate("login");
    }
  }

  if (gate === "loading") return <AuthShell icon={<ShieldCheck />} title="Opening DokoSoko" description="Loading the authenticated deployment…" />;
  if (gate === "setup") return <SetupScreen onComplete={openWorkspace} />;
  if (gate === "login") return <LoginScreen onComplete={openWorkspace} />;
  if (gate === "onboarding") return <WorkspaceSetup existingOrganisation={onboardingOrganisation} onComplete={(value) => { setDeployment(value); setGate("console"); }} />;
  if (gate === "error") return <AuthShell icon={<TriangleAlert />} title="Deployment needs attention" description={problem || "Authentication is not configured. Check the setup token, master key, database, and public URL."} />;
  return <ConsoleApp key={deployment?.id ?? "fixture-preview"} currentUser={user} currentDeployment={deployment} onLogout={user ? logout : undefined} />;
}

function slugify(value: string): string {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "").slice(0, 63);
}

function WorkspaceSetup({ existingOrganisation, onComplete }: { existingOrganisation: APIOrganisation | null; onComplete: (deployment: APIDeployment) => void }) {
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
      setProblem(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell icon={<ShieldCheck />} title="Configure this DokoSoko deployment" description="Name this connector deployment and create the first environment agents will target.">
      <form className="auth-form" onSubmit={create}>
        <Field label="Organisation"><input required disabled={Boolean(existingOrganisation)} value={organisationName} onChange={(event) => setOrganisationName(event.target.value)} /></Field>
        <Field label="Deployment name" hint="The identity of this DokoSoko installation. APIs and versions are added later as Integrations."><input required value={deploymentName} onChange={(event) => setDeploymentName(event.target.value)} placeholder="Developer Platform connector" /></Field>
        <Field label="First environment" hint="The deployment stage agents should target first, such as Production, Staging, or Development."><input required value={environmentName} onChange={(event) => setEnvironmentName(event.target.value)} /></Field>
        {problem && <AuthProblem>{problem}</AuthProblem>}
        <Button type="submit" color="indigo" disabled={busy}>{busy ? "Creating workspace…" : "Create and open console"}</Button>
      </form>
    </AuthShell>
  );
}

function SetupScreen({ onComplete }: { onComplete: (user: APIUser) => void }) {
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
      setProblem(errorMessage(error));
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
      setProblem(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  async function copyRecoveryCodes() {
    await navigator.clipboard.writeText(recoveryCodes.join("\n"));
  }

  if (step === "mfa" && enrollment) {
    return (
      <AuthShell icon={<ShieldCheck />} title="Secure the root account" description="Scan the QR code with Google Authenticator, then enter the current six-digit code.">
        <div className="setup-progress"><span className="done">1</span><i /><span className="active">2</span><i /><span>3</span></div>
        <div className="authenticator-setup">
          <strong>Setup Google Authenticator</strong>
          <div className="authenticator-qr">
            <QRCodeSVG value={enrollment.provisioning_uri} size={176} level="M" marginSize={2} title="Google Authenticator setup QR code" />
          </div>
          <div className="authenticator-secret">
            <small>Alternatively, manually set up with this secret</small>
            <code>{enrollment.totp_secret}</code>
            <a href={enrollment.provisioning_uri}>Open authenticator app</a>
          </div>
        </div>
        <form className="auth-form" onSubmit={complete}>
          <Field label="Six-digit code"><input required inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} placeholder="123456" value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, ""))} /></Field>
          {problem && <AuthProblem>{problem}</AuthProblem>}
          <Button type="submit" color="indigo" disabled={busy || code.length !== 6}>{busy ? "Verifying…" : "Verify and create root user"}</Button>
        </form>
      </AuthShell>
    );
  }

  if (step === "recovery" && createdUser) {
    return (
      <AuthShell icon={<Check />} title="Save your recovery codes" description="These one-time codes are the only fallback if the root authenticator is unavailable.">
        <div className="setup-progress"><span className="done">1</span><i /><span className="done">2</span><i /><span className="active">3</span></div>
        <div className="recovery-grid">{recoveryCodes.map((value) => <code key={value}>{value}</code>)}</div>
        <div className="auth-actions"><Button type="button" outline onClick={copyRecoveryCodes}><Copy data-slot="icon" />Copy codes</Button><Button type="button" color="indigo" onClick={() => onComplete(createdUser)}>I saved them — open console</Button></div>
      </AuthShell>
    );
  }

  return (
    <AuthShell icon={<KeyRound />} title="Create the first root user" description="Use the one-time setup token from your deployment environment. MFA is mandatory.">
      <div className="setup-progress"><span className="active">1</span><i /><span>2</span><i /><span>3</span></div>
      <form className="auth-form" onSubmit={begin}>
        <Field label="Setup token"><input required type="password" autoComplete="off" value={setupToken} onChange={(event) => setSetupToken(event.target.value)} /></Field>
        <Field label="Root email"><input required type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} /></Field>
        <Field label="Password" hint="14+ characters with upper, lower, and a number">
          <div className="password-input">
            <input required type={showPassword ? "text" : "password"} autoComplete="new-password" minLength={14} value={password} onChange={(event) => setPassword(event.target.value)} />
            <button type="button" className="password-visibility" aria-label={showPassword ? "Hide password" : "Show password"} aria-pressed={showPassword} onClick={() => setShowPassword((visible) => !visible)}>
              {showPassword ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
            </button>
          </div>
        </Field>
        {problem && <AuthProblem>{problem}</AuthProblem>}
        <Button type="submit" color="indigo" disabled={busy}>{busy ? "Preparing MFA…" : "Continue to MFA"}</Button>
      </form>
    </AuthShell>
  );
}

function LoginScreen({ onComplete }: { onComplete: (user: APIUser) => void }) {
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
      setProblem(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell icon={<LockKeyhole />} title="Sign in to DokoSoko" description="Root administration requires your password and current authenticator code.">
      <form className="auth-form" onSubmit={login}>
        <Field label="Email"><input required type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} /></Field>
        <Field label="Password"><input required type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></Field>
        <Field label="Authenticator code"><input required inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, ""))} /></Field>
        {problem && <AuthProblem>{problem}</AuthProblem>}
        <Button type="submit" color="indigo" disabled={busy || code.length !== 6}>{busy ? "Signing in…" : "Sign in"}</Button>
      </form>
    </AuthShell>
  );
}

function AuthShell({ icon, title, description, children }: { icon: React.ReactNode; title: string; description: string; children?: React.ReactNode }) {
  return <main className="auth-shell"><section className="auth-card"><div className="auth-brand"><span className="brand-mark">D</span><strong>DokoSoko</strong></div><span className="auth-icon">{icon}</span><h1>{title}</h1><p>{description}</p>{children}</section><footer>Private by default · MFA enforced · security events audited</footer></main>;
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return <label className="auth-field"><span>{label}</span>{children}{hint && <small>{hint}</small>}</label>;
}

function AuthProblem({ children }: { children: React.ReactNode }) {
  return <div className="auth-problem" role="alert"><TriangleAlert />{children}</div>;
}
