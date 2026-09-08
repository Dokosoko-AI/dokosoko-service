"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { SDKIngestionFile } from "../../../lib/developer-assets-api";
import { Button } from "../../core/control";
import { AIReadinessPanel } from "../ai-readiness-panel";
import { developerAssetError } from "./developer-asset-ui";
import { maxSDKIngestionFiles, maxSDKIngestionFileBytes, maxSDKIngestionTotalBytes, sdkBufferLooksText, sdkLanguageForPath, sdkNormalizedLocalPath, sdkTextBytes } from "./sdk-catalog-helpers";

export function SDKGuidanceInput({ onSubmit, disabled = false }: { onSubmit: (files: SDKIngestionFile[]) => Promise<void>; disabled?: boolean }) {
  const { t } = useTranslation();
  const [files, setFiles] = useState<SDKIngestionFile[]>([]);
  const [path, setPath] = useState("README.md");
  const [content, setContent] = useState("");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [rejected, setRejected] = useState<string[]>([]);
  const pending = Boolean(path.trim() && content);
  const language = sdkLanguageForPath(path);
  const pendingFile: SDKIngestionFile = { source_path: path.trim(), content, language, media_type: language === "markdown" ? "text/markdown" : "text/plain", role: /readme/i.test(path) ? "readme" : /example/i.test(path) ? "example" : "guide" };
  const selected = [...files, ...(pending ? [pendingFile] : [])];
  const invalid = selected.length > maxSDKIngestionFiles || selected.some((file) => !sdkNormalizedLocalPath(file.source_path) || sdkTextBytes(file.content) > maxSDKIngestionFileBytes) || selected.reduce((sum, file) => sum + sdkTextBytes(file.content), 0) > maxSDKIngestionTotalBytes || new Set(selected.map((file) => file.source_path.toLowerCase())).size !== selected.length;
  async function choose(values: FileList | null) {
    if (!values) return;
    setBusy(true); setProblem("");
    const accepted = [...files]; const failures: string[] = [];
    const known = new Set(accepted.map((file) => file.source_path.toLowerCase()));
    let bytes = accepted.reduce((sum, file) => sum + sdkTextBytes(file.content), 0);
    try {
      for (const file of Array.from(values)) {
        const name = sdkNormalizedLocalPath(file.webkitRelativePath || file.name);
        if (!name || name.length > 500 || known.has(name.toLowerCase()) || !file.size || file.size > maxSDKIngestionFileBytes || accepted.length >= maxSDKIngestionFiles || bytes + file.size > maxSDKIngestionTotalBytes) { failures.push(file.name); continue; }
        try {
          const buffer = await file.arrayBuffer();
          if (!sdkBufferLooksText(buffer)) { failures.push(name); continue; }
          const text = new TextDecoder("utf-8", { fatal: true }).decode(buffer);
          const language = sdkLanguageForPath(name);
          accepted.push({ source_path: name, content: text, language, media_type: language === "markdown" ? "text/markdown" : language === "json" ? "application/json" : "text/plain", role: /readme/i.test(name) ? "readme" : /example/i.test(name) ? "example" : "source" });
          known.add(name.toLowerCase()); bytes += file.size;
        } catch { failures.push(name); }
      }
      setFiles(accepted); setRejected(failures);
    } finally { setBusy(false); }
  }
  async function submit() {
    if (disabled || busy || invalid || !selected.length) return;
    setBusy(true); setProblem("");
    try { await onSubmit(selected); }
    catch (error) { setProblem(developerAssetError(error, t("sdkSetup.importFailed"))); }
    finally { setBusy(false); }
  }
  return <section className="panel sdk-setup-panel">
    <h2>{t("sdkSetup.addGuidance")}</h2><p>{t("sdkSetup.guidanceHelp")}</p>
    <AIReadinessPanel compact preserveForm />
    <label className="auth-field"><span>{t("sdkSetup.uploadFiles")}</span><input type="file" multiple accept="text/*,.md,.mdx,.txt,.json,.yaml,.yml,.ts,.tsx,.js,.jsx,.py,.go,.rb,.rs,.java,.kt,.swift,.php,.cs" disabled={busy || disabled} onChange={(event) => { const input = event.currentTarget; void choose(input.files).finally(() => { input.value = ""; }); }} /></label>
    {files.length > 0 && <ul className="sdk-guidance-file-queue">{files.map((file, index) => <li key={file.source_path}><span>{file.source_path}</span><Button outline disabled={busy} onClick={() => setFiles((current) => current.filter((_, item) => item !== index))}>{t("sdkCatalog.remove")}</Button></li>)}</ul>}
    {rejected.length > 0 && <div role="alert"><p>{t("sdkSetup.rejectedFiles")}</p><ul>{rejected.map((name, index) => <li key={index}>{name}</li>)}</ul></div>}
    <details open={files.length === 0}><summary>{t("sdkSetup.pasteFile")}</summary><div className="auth-form compact-form">
      <label className="auth-field"><span>{t("sdkCatalog.relativeSourcePath")}</span><input maxLength={500} value={path} disabled={busy} onChange={(event) => setPath(event.target.value)} /></label>
      <label className="auth-field"><span>{t("sdkCatalog.textContent")}</span><textarea rows={12} maxLength={maxSDKIngestionFileBytes} spellCheck={false} value={content} disabled={busy} onChange={(event) => setContent(event.target.value)} /></label>
      <Button outline disabled={busy || invalid || !pending} onClick={() => { setFiles((values) => [...values, pendingFile]); setContent(""); setPath(""); }}>{t("sdkCatalog.queuePastedFile")}</Button>
    </div></details>
    <p>{t("sdkSetup.fileLimits")}</p>
    {invalid && <p role="alert">{t("sdkSetup.invalidFiles")}</p>}{problem && <p className="auth-problem" role="alert">{problem}</p>}
    <div className="heading-actions"><Button disabled={busy || disabled || invalid || selected.length === 0} onClick={() => void submit()}>{t(busy ? "sdkSetup.importing" : "sdkSetup.importGuidance")}</Button></div>
  </section>;
}
