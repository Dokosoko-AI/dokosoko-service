"use client";


import { useTranslation } from "react-i18next";
import { BookOpen, Bot, ChevronRight, KeyRound, LockKeyhole, ShieldCheck, TerminalSquare } from "lucide-react";
import type { ElementType } from "react";

import type { APIAIProviderConnection } from "../../../lib/api";
import { Input, Select, Textarea } from "../../core";
import { Button, Dialog, Switch } from "../../core/control";
import { aiModelOptions, aiProviderLabel, aiProviders, aiWorkloadDescription, aiWorkloadName, aiWorkloads } from "../shared";
import type { useAIWorkspaceState } from "../use-ai-workspace";

export function AIConfigurationDialogs({
  workspace,
  ProviderLogo,
}: {
  workspace: ReturnType<typeof useAIWorkspaceState>;
  ProviderLogo: ElementType<{ provider: APIAIProviderConnection["provider"] }>;
}) {
  const { t } = useTranslation();
  const {
    aiConnections,
    workloadOpen, setWorkloadOpen,
    workloadBusy,
    workloadRole,
    workloadConnectionID,
    providerOpen, setProviderOpen,
    providerPickerOpen, setProviderPickerOpen,
    providerBusy,
    providerEnabled, setProviderEnabled,
    providerIsBackup, setProviderIsBackup,
    providerBackupAnalysisModel, setProviderBackupAnalysisModel,
    providerKind,
    providerEndpoint, setProviderEndpoint,
    workloadModel, setWorkloadModel,
    providerCredential, setProviderCredential,
    workloadInputTokens, setWorkloadInputTokens,
    workloadOutputTokens, setWorkloadOutputTokens,
    workloadDailyBudget, setWorkloadDailyBudget,
    workloadEnabled, setWorkloadEnabled,
    aiPrompts,
    promptOpen, setPromptOpen,
    promptBusy,
    promptKey,
    promptInstructions, setPromptInstructions,
    openAIConnection,
    changeAIWorkloadConnection,
    saveAIConnection,
    saveAIWorkload,
    saveAIPromptOverride,
    resetAIPromptOverride,
  } = workspace;
  const workload = aiWorkloads.find((candidate) => candidate.role === workloadRole);
  const selectedProvider = aiConnections.find((connection) => connection.id === workloadConnectionID)?.provider;
  const providerConnection = aiConnections.find((connection) => connection.provider === providerKind);
  const providerManagedByEnvironment = providerConnection?.managed_by === "environment";
  const activePrompt = aiPrompts.find((prompt) => prompt.key === promptKey);
  const promptInstructionBytes = new TextEncoder().encode(promptInstructions).length;
  const promptUnchanged = promptInstructions.trim() === activePrompt?.instructions.trim();

  return <>
    <Dialog
      open={workloadOpen}
      onClose={setWorkloadOpen}
      title={t("aiDialogs.configure", { workloadRole: aiWorkloadName(workloadRole, t) })}
      description={workload ? aiWorkloadDescription(workload.role, t) : t("aiDialogs.chooseTheProviderAndModelForThisWorkload")}
      actions={<><Button outline onClick={() => setWorkloadOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={workloadBusy || !workloadConnectionID || !workloadModel.trim()} onClick={saveAIWorkload}>{workloadBusy ? t("common.saving") : t("aiDialogs.saveWorkload")}</Button></>}
    >
      <div className="auth-form compact-form ai-model-form">
        <div className="ai-dialog-workload"><span className="settings-icon">{(() => { const Icon = workload?.icon ?? Bot; return <Icon />; })()}</span><span><small>{t("aiDialogs.workload")}</small><strong>{aiWorkloadName(workloadRole, t)}</strong></span><Switch checked={workloadEnabled} onChange={setWorkloadEnabled} label={t("aiDialogs.enabled")} /></div>
        <div className="two-fields"><label className="auth-field"><span>{t("aiDialogs.providerConnection")}</span><Select name="ai-workload-connection" value={workloadConnectionID} onChange={(event) => changeAIWorkloadConnection(event.target.value)}><option value="">{t("aiDialogs.chooseAConnection")}</option>{aiConnections.filter((connection) => connection.enabled && !connection.is_backup).map((connection) => <option value={connection.id} key={connection.id}>{aiProviderLabel(connection.provider, t)}{connection.managed_by === "environment" ? t("aiDialogs.environment") : ""}</option>)}</Select></label><label className="auth-field"><span>{t("aiDialogs.model")}</span>{selectedProvider && selectedProvider !== "openai-compatible" ? <Select name="ai-workload-model" value={workloadModel} onChange={(event) => setWorkloadModel(event.target.value)}>{aiModelOptions[selectedProvider].map((model) => <option key={model} value={model}>{model}</option>)}</Select> : <Input name="ai-workload-model" autoComplete="off" value={workloadModel} onChange={(event) => setWorkloadModel(event.target.value)} placeholder={t("aiDialogs.providerModelID")} />}</label></div>
        {aiConnections.length === 0 && <div className="private-default-note"><KeyRound />{t("aiDialogs.connectOneProviderBeforeEnablingAWorkload")}</div>}
        <details className="advanced-details ai-model-advanced"><summary>{t("aiDialogs.limitsAndBudget")}</summary><div className="ai-model-advanced-body"><div className="two-fields"><label className="auth-field" htmlFor="ai-max-input-tokens"><span>{t("aiDialogs.maxInputTokens")}</span><Input id="ai-max-input-tokens" type="number" min={256} max={1000000} value={workloadInputTokens} onChange={(event) => setWorkloadInputTokens(event.target.value)} /></label><label className="auth-field" htmlFor="ai-max-output-tokens"><span>{t("aiDialogs.maxOutputTokens")}</span><Input id="ai-max-output-tokens" type="number" min={1} max={32768} value={workloadOutputTokens} onChange={(event) => setWorkloadOutputTokens(event.target.value)} /></label></div><label className="auth-field" htmlFor="ai-daily-token-budget"><span>{t("aiDialogs.dailyTokenBudget")}</span><Input id="ai-daily-token-budget" type="number" min={0} max={10000000000} value={workloadDailyBudget} onChange={(event) => setWorkloadDailyBudget(event.target.value)} /><small>{t("aiDialogs.setToN0ForNoDailyCapBudgetReservations")}</small></label></div></details>
        <div className="ai-dialog-safeguards"><span><ShieldCheck />{t("aiDialogs.untrustedContext")}</span><span><LockKeyhole />{t("aiDialogs.noAuthorization")}</span><span><TerminalSquare />{t("aiDialogs.noToolCalls")}</span><span><BookOpen />{t("aiDialogs.citationsRequired")}</span></div>
      </div>
    </Dialog>

    <Dialog
      open={promptOpen}
      onClose={setPromptOpen}
      title={activePrompt ? t("aiDialogs.edit", { label: String(activePrompt.label) }) : t("aiDialogs.editWorkflowInstructions")}
      description={t("aiDialogs.saveANewVersionOfTheWorkflowSpecificInstructions")}
      actions={<><Button outline disabled={promptBusy || activePrompt?.source !== "override"} onClick={resetAIPromptOverride}>{t("aiDialogs.resetToSafeDefault")}</Button><Button outline onClick={() => setPromptOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={promptBusy || !activePrompt || !promptInstructions.trim() || promptInstructionBytes > 32768 || promptUnchanged} onClick={saveAIPromptOverride}>{promptBusy ? t("common.saving") : t("aiDialogs.saveNewVersion")}</Button></>}
    >
      <div className="auth-form compact-form ai-model-form ai-prompt-editor">
        {activePrompt && <div className="ai-dialog-workload"><span className="settings-icon"><BookOpen /></span><span><small>{activePrompt.source === "override" ? t("aiDialogs.customOverride") : t("aiDialogs.builtInDefault")}</small><strong>{activePrompt.effective_version}</strong></span></div>}
        <label className="auth-field" htmlFor="ai-workflow-instructions"><span>{t("aiDialogs.workflowInstructions")}</span><Textarea id="ai-workflow-instructions" name="ai-workflow-instructions" rows={14} maxLength={32768} value={promptInstructions} onChange={(event) => setPromptInstructions(event.target.value)} /><small>{t("aiDialogs.workflowInstructionSize", { count: promptInstructionBytes, limit: 32768 })}</small></label>
        <div className="private-default-note"><ShieldCheck />{t("aiDialogs.theServerAlwaysAppliesPromptInjectionDefensesBlocksTool")}</div>
      </div>
    </Dialog>

    <Dialog
      open={providerPickerOpen}
      onClose={setProviderPickerOpen}
      title={t("aiDialogs.addProvider")}
      description={t("aiDialogs.chooseTheProviderThatWillOwnThisEncryptedCredential")}
      actions={<Button outline onClick={() => setProviderPickerOpen(false)}>{t("common.cancel")}</Button>}
    >
      <div className="ai-provider-picker">
        {aiProviders.map((provider) => {
          const connected = aiConnections.some((connection) => connection.provider === provider.id);
          const name = aiProviderLabel(provider.id, t);
          return <button type="button" key={provider.id} aria-label={connected ? t("aiDialogs.connectedProvider", { provider: name }) : name} onClick={() => openAIConnection(provider.id)}><ProviderLogo provider={provider.id} /><strong>{name}</strong><ChevronRight /></button>;
        })}
      </div>
    </Dialog>

    <Dialog
      open={providerOpen}
      onClose={setProviderOpen}
      title={t("aiDialogs.connect", { value1: aiProviderLabel(providerKind, t) })}
      description={t("aiDialogs.oneProviderConnectionOwnsOneCredentialTheAnalysisWorkload")}
      actions={<><Button outline onClick={() => setProviderOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={providerBusy || !providerEndpoint.trim() || (providerIsBackup && !providerBackupAnalysisModel.trim()) || (providerEnabled && !providerCredential.trim() && !providerConnection) || providerManagedByEnvironment} onClick={saveAIConnection}>{providerBusy ? t("common.saving") : t("aiDialogs.saveConnection")}</Button></>}
    >
      <div className="auth-form compact-form ai-model-form">
        <div className="ai-dialog-workload"><ProviderLogo provider={providerKind} /><span><small>{t("aiDialogs.provider")}</small><strong>{aiProviderLabel(providerKind, t)}</strong></span><Switch checked={providerEnabled} onChange={setProviderEnabled} label={t("aiDialogs.enabled")} /></div>
        {providerManagedByEnvironment ? <div className="private-default-note"><TerminalSquare />{t("aiDialogs.thisConnectionIsManagedByDOKOSOKOAIEnvironmentVariables")}</div> : <>
          <label className="auth-field"><span>{t("aiDialogs.providerOrigin")}</span><Input name="ai-provider-endpoint" type="url" autoComplete="off" readOnly={providerKind !== "openai-compatible"} value={providerEndpoint} onChange={(event) => setProviderEndpoint(event.target.value)} placeholder="https://api.provider.com" /><small>{providerKind === "openai-compatible" ? t("aiDialogs.aFixedPublicHTTPSOriginPrivateNetworkDestinationsRedirects") : t("aiDialogs.theNativeProviderOriginIsFixedByDokoSoko")}</small></label>
          <label className="auth-field" htmlFor="ai-provider-credential"><span>{t("aiDialogs.apiCredential")}</span><Input id="ai-provider-credential" name="ai-provider-credential" type="password" autoComplete="new-password" value={providerCredential} onChange={(event) => setProviderCredential(event.target.value)} placeholder={providerConnection ? t("aiDialogs.leaveBlankToKeepTheStoredCredential") : t("aiDialogs.requiredBeforeEnabling")} /><small>{t("aiDialogs.encryptedAtRestRedactedFromEveryResponseAndShared")}</small></label>
          <div className="ai-backup-control"><span><strong>{t("aiDialogs.backupProvider")}</strong><small>{t("aiDialogs.onARetryableFailureSendTheSameBoundedPrompt")}</small></span><Switch checked={providerIsBackup} onChange={setProviderIsBackup} label={t("aiDialogs.useAsBackupProvider")} /></div>
          {providerIsBackup && <label className="auth-field"><span>{t("aiDialogs.analysisBackupModel")}</span>{providerKind === "openai-compatible" ? <Input value={providerBackupAnalysisModel} onChange={(event) => setProviderBackupAnalysisModel(event.target.value)} placeholder={t("aiDialogs.providerModelID")} /> : <Select value={providerBackupAnalysisModel} onChange={(event) => setProviderBackupAnalysisModel(event.target.value)}>{aiModelOptions[providerKind].map((model) => <option key={model} value={model}>{model}</option>)}</Select>}</label>}
        </>}
      </div>
    </Dialog>
  </>;
}
