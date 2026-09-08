"use client";

import { LockKeyhole, PackageSearch, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import {
  developerAssetsApi,
  type SDKPackageImportInput,
  type SDKPackageImportResult,
} from "../../../lib/developer-assets-api";
import { resolveSDKPackageLocation } from "../../../lib/sdk-package-locator";
import { Button, Dialog } from "../../core/control";
import { developerAssetError } from "./developer-asset-ui";

type Ecosystem = SDKPackageImportInput["ecosystem"];
type SourceKind = SDKPackageImportInput["source_kind"];
type AuthenticationType = SDKPackageImportInput["authentication"]["type"];

const ecosystems: Array<{ id: Ecosystem; mark: string }> = [
  { id: "npm", mark: "npm" },
  { id: "pypi", mark: "Py" },
  { id: "go", mark: "Go" },
  { id: "cargo", mark: "Rs" },
];

function registryPlaceholder(ecosystem: Ecosystem) {
  switch (ecosystem) {
    case "npm": return "https://registry.npmjs.org/@scope/package";
    case "pypi": return "https://pypi.org/project/package";
    case "go": return "https://proxy.golang.org";
    case "cargo": return "https://crates.io/crates/package";
  }
}

export function SDKPackageImportDialog({
  open,
  onClose,
  onImported,
  onMessage,
}: {
  open: boolean;
  onClose: (open: boolean) => void;
  onImported: (result: SDKPackageImportResult) => void | Promise<void>;
  onMessage: (message: string) => void;
}) {
  const { t } = useTranslation();
  const [locator, setLocator] = useState("");
  const [advanced, setAdvanced] = useState(false);
  const [ecosystem, setEcosystem] = useState<Ecosystem>("npm");
  const [sourceKind, setSourceKind] = useState<SourceKind>("registry");
  const [sourceURL, setSourceURL] = useState("");
  const [coordinate, setCoordinate] = useState("");
  const [exactVersion, setExactVersion] = useState("");
  const [sourceRef, setSourceRef] = useState("");
  const [privateSource, setPrivateSource] = useState(false);
  const [authenticationType, setAuthenticationType] = useState<Exclude<AuthenticationType, "none">>("bearer");
  const [username, setUsername] = useState("");
  const [credential, setCredential] = useState("");
  const [visibility, setVisibility] = useState<SDKPackageImportInput["visibility"]>("private");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");

  function resetForm() {
    setLocator("");
    setAdvanced(false);
    setEcosystem("npm");
    setSourceKind("registry");
    setSourceURL("");
    setCoordinate("");
    setExactVersion("");
    setSourceRef("");
    setPrivateSource(false);
    setAuthenticationType("bearer");
    setUsername("");
    setCredential("");
    setVisibility("private");
    setProblem("");
  }

  function closeDialog() {
    resetForm();
    onClose(false);
  }

  function locate(value: string, selected = ecosystem) {
    setLocator(value); setCredential(""); setSourceRef("");
    const resolved = resolveSDKPackageLocation(value, selected);
    setExactVersion(resolved?.exactVersion ?? "");
    if (resolved) {
      if (value.includes("://")) setAdvanced(false);
      setEcosystem(resolved.ecosystem); setCoordinate(resolved.coordinate); setSourceURL(resolved.sourceURL); setSourceKind("registry");
    } else {
      setCoordinate(""); setSourceURL(value.includes("://") ? value.trim() : "");
      if (value.trim()) setAdvanced(true);
    }
  }

  const authentication: SDKPackageImportInput["authentication"] = privateSource
    ? {
      type: authenticationType,
      ...(authenticationType === "basic" ? { username: username.trim() } : {}),
      credential,
    }
    : { type: "none" };
  const missingAuthentication = privateSource && (!credential || (authenticationType === "basic" && !username.trim()));
  const ready = Boolean(sourceURL.trim() && coordinate.trim() && exactVersion.trim() && !missingAuthentication);

  async function importPackage() {
    if (!ready || busy) return;
    setBusy(true);
    setProblem("");
    try {
      const result = await developerAssetsApi.importSDKPackage({
        ecosystem,
        source_kind: sourceKind,
        source_url: sourceURL.trim(),
        coordinate: coordinate.trim(),
        exact_version: exactVersion.trim(),
        ...(sourceKind === "git" && sourceRef.trim() ? { source_ref: sourceRef.trim() } : {}),
        visibility,
        authentication,
      });
      await onImported(result);
      closeDialog();
      onMessage(result.already_imported
        ? t("sdkImport.exactReleaseAlreadyImported", { version: result.release.exact_version })
        : t("sdkImport.exactReleaseImported", { version: result.release.exact_version }));
    } catch (error) {
      setProblem(developerAssetError(error, t("sdkImport.packageCouldNotBeImported")));
    } finally {
      setBusy(false);
      setCredential("");
    }
  }

  return <Dialog
    open={open}
    onClose={(next) => { if (!next && !busy) closeDialog(); }}
    title={t("sdkImport.title")}
    description={t("sdkImport.description")}
    actions={<>
      <Button outline disabled={busy} onClick={closeDialog}>{t("common.cancel")}</Button>
      <Button color="indigo" disabled={busy || !ready} onClick={() => void importPackage()}>
        <PackageSearch data-slot="icon" />{busy ? t("sdkImport.importing") : t("sdkImport.importPackage")}
      </Button>
    </>}
  >
    <div className="auth-form compact-form sdk-import-form">
      <label className="auth-field"><span>{t("sdkSetup.packageNameOrURL")}</span><input value={locator} onChange={(event) => locate(event.target.value)} placeholder="@scope/package or https://pypi.org/project/package" /><small>{t("sdkSetup.packageLocatorHelp")}</small></label>
      <div className="two-fields"><label className="auth-field"><span>{t("sdkImport.ecosystem")}</span><select value={ecosystem} onChange={(event) => { const selected = event.target.value as Ecosystem; setEcosystem(selected); locate(locator, selected); }}>{ecosystems.map((option) => <option key={option.id} value={option.id}>{t(`sdkImport.ecosystems.${option.id}`)}</option>)}</select></label><label className="auth-field"><span>{t("sdkImport.exactVersion")}</span><input value={exactVersion} onChange={(event) => setExactVersion(event.target.value)} placeholder={ecosystem === "go" ? "v1.2.3" : "1.2.3"} /></label></div>
      {coordinate && <p>{t("sdkSetup.resolvedPackage", { coordinate, ecosystem })}</p>}
      <details open={advanced} onToggle={(event) => setAdvanced(event.currentTarget.open)}><summary>{t("sdkSetup.customPackageSource")}</summary><div className="auth-form compact-form">
        <label className="auth-field"><span>{t("sdkImport.sourceType")}</span><select value={sourceKind} onChange={(event) => { setSourceKind(event.target.value as SourceKind); setCredential(""); }}><option value="registry">{t("sdkImport.registry")}</option><option value="git">{t("sdkImport.gitRepository")}</option></select></label>
        <label className="auth-field"><span>{sourceKind === "git" ? t("sdkImport.repositoryURL") : t("sdkImport.registryURL")}</span><input type="url" value={sourceURL} onChange={(event) => { setSourceURL(event.target.value); setCredential(""); }} placeholder={sourceKind === "git" ? "https://github.com/owner/repository" : registryPlaceholder(ecosystem)} /><small>{sourceKind === "git" ? t("sdkImport.gitURLHelp") : t("sdkImport.registryURLHelp")}</small></label>
        <label className="auth-field"><span>{t("sdkImport.packageCoordinate")}</span><input value={coordinate} onChange={(event) => setCoordinate(event.target.value)} /></label>
        {sourceKind === "git" && <label className="auth-field"><span>{t("sdkImport.gitRef")}</span><input value={sourceRef} onChange={(event) => setSourceRef(event.target.value)} placeholder={exactVersion || t("sdkImport.exactVersionOrCommit")} /><small>{t("sdkImport.gitRefHelp")}</small></label>}
      </div></details>

      <div className="two-fields">
        <label className="auth-field"><span>{t("sdkImport.sourceAccess")}</span><select value={privateSource ? "private" : "public"} onChange={(event) => setPrivateSource(event.target.value === "private")}><option value="public">{t("sdkImport.publicSource")}</option><option value="private">{t("sdkImport.privateSource")}</option></select></label>
        <label className="auth-field"><span>{t("sdkImport.catalogVisibility")}</span><select value={visibility} onChange={(event) => setVisibility(event.target.value as SDKPackageImportInput["visibility"])}><option value="private">{t("common.private")}</option><option value="public">{t("common.public")}</option></select></label>
      </div>

      {privateSource && <div className="sdk-import-private-access">
        <div className="private-default-note"><LockKeyhole />{t("sdkImport.credentialTransient")}</div>
        <label className="auth-field"><span>{t("sdkImport.authentication")}</span><select value={authenticationType} onChange={(event) => setAuthenticationType(event.target.value as Exclude<AuthenticationType, "none">)}><option value="bearer">{t("sdkImport.bearerToken")}</option><option value="basic">{t("sdkImport.basicAuthentication")}</option></select></label>
        {authenticationType === "basic" && <label className="auth-field"><span>{t("sdkImport.username")}</span><input autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} /></label>}
        <label className="auth-field"><span>{authenticationType === "bearer" ? t("sdkImport.patOrToken") : t("sdkImport.passwordOrToken")}</span><input type="password" autoComplete="new-password" value={credential} onChange={(event) => setCredential(event.target.value)} spellCheck={false} /></label>
      </div>}

      {problem && <div className="inline-warning" role="alert">{problem}</div>}
      <div className="notice"><ShieldCheck /><span><strong>{t("sdkImport.noCodeExecution")}</strong> {t("sdkImport.noCodeExecutionDetail")}</span></div>
    </div>
  </Dialog>;
}
