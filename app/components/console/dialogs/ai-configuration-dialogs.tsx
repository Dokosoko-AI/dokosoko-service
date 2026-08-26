"use client";

import { BookOpen, Bot, ChevronRight, KeyRound, LockKeyhole, ShieldCheck, TerminalSquare } from "lucide-react";
import type { ElementType } from "react";

import type { APIAIProviderConnection } from "../../../lib/api";
import { Input, Select, Textarea } from "../../core";
import { Button, Dialog, Switch } from "../../core/control";
import { aiModelOptions, aiProviderLabel, aiProviders, aiWorkloads } from "../shared";
import type { useAIWorkspaceState } from "../use-ai-workspace";

export function AIConfigurationDialogs({
  workspace,
  ProviderLogo,
}: {
  workspace: ReturnType<typeof useAIWorkspaceState>;
  ProviderLogo: ElementType<{ provider: APIAIProviderConnection["provider"] }>;
}) {
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
      title={`Configure ${workload?.name ?? workloadRole}`}
      description={workload?.description ?? "Choose the provider and model for this workload."}
      actions={<><Button outline onClick={() => setWorkloadOpen(false)}>Cancel</Button><Button color="indigo" disabled={workloadBusy || !workloadConnectionID || !workloadModel.trim()} onClick={saveAIWorkload}>{workloadBusy ? "Saving…" : "Save workload"}</Button></>}
    >
      <div className="auth-form compact-form ai-model-form">
        <div className="ai-dialog-workload"><span className="settings-icon">{(() => { const Icon = workload?.icon ?? Bot; return <Icon />; })()}</span><span><small>Workload</small><strong>{workload?.name ?? workloadRole}</strong></span><Switch checked={workloadEnabled} onChange={setWorkloadEnabled} label="Enabled" /></div>
        <div className="two-fields"><label className="auth-field"><span>Provider connection</span><Select name="ai-workload-connection" value={workloadConnectionID} onChange={(event) => changeAIWorkloadConnection(event.target.value)}><option value="">Choose a connection</option>{aiConnections.filter((connection) => connection.enabled && !connection.is_backup).map((connection) => <option value={connection.id} key={connection.id}>{aiProviderLabel(connection.provider)}{connection.managed_by === "environment" ? " · environment" : ""}</option>)}</Select></label><label className="auth-field"><span>Model</span>{selectedProvider && selectedProvider !== "openai-compatible" ? <Select name="ai-workload-model" value={workloadModel} onChange={(event) => setWorkloadModel(event.target.value)}>{aiModelOptions[selectedProvider].map((model) => <option key={model} value={model}>{model}</option>)}</Select> : <Input name="ai-workload-model" autoComplete="off" value={workloadModel} onChange={(event) => setWorkloadModel(event.target.value)} placeholder="Provider model ID" />}</label></div>
        {aiConnections.length === 0 && <div className="private-default-note"><KeyRound />Connect one provider before enabling a workload.</div>}
        <details className="advanced-details ai-model-advanced"><summary>Limits and budget</summary><div className="ai-model-advanced-body"><div className="two-fields"><label className="auth-field" htmlFor="ai-max-input-tokens"><span>Max input tokens</span><Input id="ai-max-input-tokens" type="number" min={256} max={1000000} value={workloadInputTokens} onChange={(event) => setWorkloadInputTokens(event.target.value)} /></label><label className="auth-field" htmlFor="ai-max-output-tokens"><span>Max output tokens</span><Input id="ai-max-output-tokens" type="number" min={1} max={32768} value={workloadOutputTokens} onChange={(event) => setWorkloadOutputTokens(event.target.value)} /></label></div><label className="auth-field" htmlFor="ai-daily-token-budget"><span>Daily token budget</span><Input id="ai-daily-token-budget" type="number" min={0} max={10000000000} value={workloadDailyBudget} onChange={(event) => setWorkloadDailyBudget(event.target.value)} /><small>Set to 0 for no daily cap. Budget reservations are atomic across concurrent jobs.</small></label></div></details>
        <div className="ai-dialog-safeguards"><span><ShieldCheck />Untrusted context</span><span><LockKeyhole />No authorization</span><span><TerminalSquare />No tool calls</span><span><BookOpen />Citations required</span></div>
      </div>
    </Dialog>

    <Dialog
      open={promptOpen}
      onClose={setPromptOpen}
      title={activePrompt ? `Edit ${activePrompt.label}` : "Edit workflow instructions"}
      description="Save a new version of the workflow-specific instructions. DokoSoko's built-in safety policy cannot be edited or disabled."
      actions={<><Button outline disabled={promptBusy || activePrompt?.source !== "override"} onClick={resetAIPromptOverride}>Reset to safe default</Button><Button outline onClick={() => setPromptOpen(false)}>Cancel</Button><Button color="indigo" disabled={promptBusy || !activePrompt || !promptInstructions.trim() || promptInstructionBytes > 32768 || promptUnchanged} onClick={saveAIPromptOverride}>{promptBusy ? "Saving…" : "Save new version"}</Button></>}
    >
      <div className="auth-form compact-form ai-model-form ai-prompt-editor">
        {activePrompt && <div className="ai-dialog-workload"><span className="settings-icon"><BookOpen /></span><span><small>{activePrompt.source === "override" ? "Custom override" : "Built-in default"}</small><strong>{activePrompt.effective_version}</strong></span></div>}
        <label className="auth-field" htmlFor="ai-workflow-instructions"><span>Workflow instructions</span><Textarea id="ai-workflow-instructions" name="ai-workflow-instructions" rows={14} maxLength={32768} value={promptInstructions} onChange={(event) => setPromptInstructions(event.target.value)} /><small>{promptInstructionBytes.toLocaleString()} / 32,768 UTF-8 bytes. These instructions shape one workflow. Evidence, output schemas, authorization boundaries, and the immutable safety policy remain server-controlled.</small></label>
        <div className="private-default-note"><ShieldCheck />The server always applies prompt-injection defenses, blocks tool calls and authorization decisions, and requires evidence-grounded output.</div>
      </div>
    </Dialog>

    <Dialog
      open={providerPickerOpen}
      onClose={setProviderPickerOpen}
      title="Add provider"
      description="Choose the provider that will own this encrypted credential. You can connect each provider once."
      actions={<Button outline onClick={() => setProviderPickerOpen(false)}>Cancel</Button>}
    >
      <div className="ai-provider-picker">
        {aiProviders.map((provider) => {
          const connected = aiConnections.some((connection) => connection.provider === provider.id);
          return <button type="button" key={provider.id} aria-label={`${provider.name}${connected ? " (connected)" : ""}`} onClick={() => openAIConnection(provider.id)}><ProviderLogo provider={provider.id} /><strong>{provider.name}</strong><ChevronRight /></button>;
        })}
      </div>
    </Dialog>

    <Dialog
      open={providerOpen}
      onClose={setProviderOpen}
      title={`Connect ${aiProviderLabel(providerKind)}`}
      description="One provider connection owns one credential. The Analysis workload reuses it without copying secrets."
      actions={<><Button outline onClick={() => setProviderOpen(false)}>Cancel</Button><Button color="indigo" disabled={providerBusy || !providerEndpoint.trim() || (providerIsBackup && !providerBackupAnalysisModel.trim()) || (providerEnabled && !providerCredential.trim() && !providerConnection) || providerManagedByEnvironment} onClick={saveAIConnection}>{providerBusy ? "Saving…" : "Save connection"}</Button></>}
    >
      <div className="auth-form compact-form ai-model-form">
        <div className="ai-dialog-workload"><ProviderLogo provider={providerKind} /><span><small>Provider</small><strong>{aiProviderLabel(providerKind)}</strong></span><Switch checked={providerEnabled} onChange={setProviderEnabled} label="Enabled" /></div>
        {providerManagedByEnvironment ? <div className="private-default-note"><TerminalSquare />This connection is managed by DOKOSOKO_AI_* environment variables. Change it in deployment configuration and restart DokoSoko.</div> : <>
          <label className="auth-field"><span>Provider origin</span><Input name="ai-provider-endpoint" type="url" autoComplete="off" readOnly={providerKind !== "openai-compatible"} value={providerEndpoint} onChange={(event) => setProviderEndpoint(event.target.value)} placeholder="https://api.provider.com" /><small>{providerKind === "openai-compatible" ? "A fixed public HTTPS origin. Private-network destinations, redirects, paths, and non-default ports are rejected." : "The native provider origin is fixed by DokoSoko."}</small></label>
          <label className="auth-field" htmlFor="ai-provider-credential"><span>API credential</span><Input id="ai-provider-credential" name="ai-provider-credential" type="password" autoComplete="new-password" value={providerCredential} onChange={(event) => setProviderCredential(event.target.value)} placeholder={providerConnection ? "Leave blank to keep the stored credential" : "Required before enabling"} /><small>Encrypted at rest, redacted from every response, and shared only with the selected provider.</small></label>
          <div className="ai-backup-control"><span><strong>Backup provider</strong><small>On a retryable failure, send the same bounded prompt and reviewed evidence to this provider once.</small></span><Switch checked={providerIsBackup} onChange={setProviderIsBackup} label="Use as backup provider" /></div>
          {providerIsBackup && <label className="auth-field"><span>Analysis backup model</span>{providerKind === "openai-compatible" ? <Input value={providerBackupAnalysisModel} onChange={(event) => setProviderBackupAnalysisModel(event.target.value)} placeholder="Provider model ID" /> : <Select value={providerBackupAnalysisModel} onChange={(event) => setProviderBackupAnalysisModel(event.target.value)}>{aiModelOptions[providerKind].map((model) => <option key={model} value={model}>{model}</option>)}</Select>}</label>}
        </>}
      </div>
    </Dialog>
  </>;
}
