"use client";

import { Check, GitBranch, LockKeyhole, PackageSearch, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import {
  developerAssetsApi,
  type SDKPackageImportInput,
  type SDKPackageImportResult,
} from "../../../lib/developer-assets-api";
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
      <fieldset className="source-kind-selector">
        <legend>{t("sdkImport.ecosystem")}</legend>
        <div className="source-kind-options sdk-import-ecosystems">
          {ecosystems.map((option) => <label className={ecosystem === option.id ? "selected" : ""} key={option.id}>
            <input type="radio" name="sdk-import-ecosystem" value={option.id} checked={ecosystem === option.id} onChange={() => setEcosystem(option.id)} />
            <span className="source-kind-icon sdk-import-ecosystem-mark" aria-hidden="true">{option.mark}</span>
            <strong>{t(`sdkImport.ecosystems.${option.id}`)}</strong>
            <Check className="source-kind-check" />
          </label>)}
        </div>
      </fieldset>

      <fieldset className="source-kind-selector">
        <legend>{t("sdkImport.sourceType")}</legend>
        <div className="source-kind-options sdk-import-source-types">
          <label className={sourceKind === "registry" ? "selected" : ""}>
            <input type="radio" name="sdk-import-source" value="registry" checked={sourceKind === "registry"} onChange={() => setSourceKind("registry")} />
            <span className="source-kind-icon"><PackageSearch /></span>
            <strong>{t("sdkImport.registry")}</strong>
            <Check className="source-kind-check" />
          </label>
          <label className={sourceKind === "git" ? "selected" : ""}>
            <input type="radio" name="sdk-import-source" value="git" checked={sourceKind === "git"} onChange={() => setSourceKind("git")} />
            <span className="source-kind-icon"><GitBranch /></span>
            <strong>{t("sdkImport.gitRepository")}</strong>
            <Check className="source-kind-check" />
          </label>
        </div>
      </fieldset>

      <label className="auth-field">
        <span>{sourceKind === "git" ? t("sdkImport.repositoryURL") : t("sdkImport.registryURL")}</span>
        <input type="url" value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} placeholder={sourceKind === "git" ? "https://github.com/owner/repository" : registryPlaceholder(ecosystem)} />
        <small>{sourceKind === "git" ? t("sdkImport.gitURLHelp") : t("sdkImport.registryURLHelp")}</small>
      </label>
      <div className="two-fields">
        <label className="auth-field"><span>{t("sdkImport.packageCoordinate")}</span><input value={coordinate} onChange={(event) => setCoordinate(event.target.value)} placeholder={ecosystem === "npm" ? "@scope/package" : ecosystem === "go" ? "github.com/owner/module" : "package-name"} /></label>
        <label className="auth-field"><span>{t("sdkImport.exactVersion")}</span><input value={exactVersion} onChange={(event) => setExactVersion(event.target.value)} placeholder={ecosystem === "go" ? "v1.2.3" : "1.2.3"} /></label>
      </div>
      {sourceKind === "git" && <label className="auth-field"><span>{t("sdkImport.gitRef")}</span><input value={sourceRef} onChange={(event) => setSourceRef(event.target.value)} placeholder={exactVersion || t("sdkImport.exactVersionOrCommit")} /><small>{t("sdkImport.gitRefHelp")}</small></label>}

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
