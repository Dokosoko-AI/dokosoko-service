"use client";

import { BookOpen, Bot, ChevronRight, KeyRound, LockKeyhole, ShieldCheck, TerminalSquare } from "lucide-react";
import type { ElementType } from "react";

import type { APIAIProviderConnection } from "../../../lib/api";
import { Input, Select } from "../../core";
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
    llmOpen, setLLMOpen,
    llmBusy,
    llmRole,
    llmConnectionID,
    providerOpen, setProviderOpen,
    providerPickerOpen, setProviderPickerOpen,
    providerBusy,
    providerEnabled, setProviderEnabled,
    providerIsBackup, setProviderIsBackup,
    providerBackupAnalysisModel, setProviderBackupAnalysisModel,
    providerBackupAssistantModel, setProviderBackupAssistantModel,
    llmProvider,
    llmEndpoint, setLLMEndpoint,
    llmModel, setLLMModel,
    llmCredential, setLLMCredential,
    llmInputTokens, setLLMInputTokens,
    llmOutputTokens, setLLMOutputTokens,
    llmDailyBudget, setLLMDailyBudget,
    llmEnabled, setLLMEnabled,
    openAIConnection,
    changeLLMConnection,
    saveAIConnection,
    saveLLMProfile,
  } = workspace;
  const workload = aiWorkloads.find((candidate) => candidate.role === llmRole);
  const selectedProvider = aiConnections.find((connection) => connection.id === llmConnectionID)?.provider;
  const providerConnection = aiConnections.find((connection) => connection.provider === llmProvider);
  const providerManagedByEnvironment = providerConnection?.managed_by === "environment";

  return <>
    <Dialog
      open={llmOpen}
      onClose={setLLMOpen}
      title={`Configure ${workload?.name ?? llmRole}`}
      description={workload?.description ?? "Choose the provider and model for this workload."}
      actions={<><Button outline onClick={() => setLLMOpen(false)}>Cancel</Button><Button color="indigo" disabled={llmBusy || !llmConnectionID || !llmModel.trim()} onClick={saveLLMProfile}>{llmBusy ? "Saving…" : "Save workload"}</Button></>}
    >
      <div className="auth-form compact-form ai-model-form">
        <div className="ai-dialog-workload"><span className="settings-icon">{(() => { const Icon = workload?.icon ?? Bot; return <Icon />; })()}</span><span><small>Workload</small><strong>{workload?.name ?? llmRole}</strong></span><Switch checked={llmEnabled} onChange={setLLMEnabled} label="Enabled" /></div>
        <div className="two-fields"><label className="auth-field"><span>Provider connection</span><Select name="llm-connection" value={llmConnectionID} onChange={(event) => changeLLMConnection(event.target.value)}><option value="">Choose a connection</option>{aiConnections.filter((connection) => connection.enabled && !connection.is_backup).map((connection) => <option value={connection.id} key={connection.id}>{aiProviderLabel(connection.provider)}{connection.managed_by === "environment" ? " · environment" : ""}</option>)}</Select></label><label className="auth-field"><span>Model</span>{selectedProvider && selectedProvider !== "openai-compatible" ? <Select name="llm-model" value={llmModel} onChange={(event) => setLLMModel(event.target.value)}>{aiModelOptions[selectedProvider].map((model) => <option key={model} value={model}>{model}</option>)}</Select> : <Input name="llm-model" autoComplete="off" value={llmModel} onChange={(event) => setLLMModel(event.target.value)} placeholder="Provider model ID" />}</label></div>
        {aiConnections.length === 0 && <div className="private-default-note"><KeyRound />Connect one provider before enabling a workload.</div>}
        <details className="advanced-details ai-model-advanced"><summary>Limits and budget</summary><div className="ai-model-advanced-body"><div className="two-fields"><label className="auth-field" htmlFor="ai-max-input-tokens"><span>Max input tokens</span><Input id="ai-max-input-tokens" type="number" min={256} max={1000000} value={llmInputTokens} onChange={(event) => setLLMInputTokens(event.target.value)} /></label><label className="auth-field" htmlFor="ai-max-output-tokens"><span>Max output tokens</span><Input id="ai-max-output-tokens" type="number" min={1} max={32768} value={llmOutputTokens} onChange={(event) => setLLMOutputTokens(event.target.value)} /></label></div><label className="auth-field" htmlFor="ai-daily-token-budget"><span>Daily token budget</span><Input id="ai-daily-token-budget" type="number" min={0} max={10000000000} value={llmDailyBudget} onChange={(event) => setLLMDailyBudget(event.target.value)} /><small>Set to 0 for no daily cap. Budget reservations are atomic across concurrent jobs.</small></label></div></details>
        <div className="ai-dialog-safeguards"><span><ShieldCheck />Untrusted context</span><span><LockKeyhole />No authorization</span><span><TerminalSquare />No tool calls</span><span><BookOpen />Citations required</span></div>
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
      title={`Connect ${aiProviderLabel(llmProvider)}`}
      description="One provider connection owns one credential. Workloads reuse it without copying secrets."
      actions={<><Button outline onClick={() => setProviderOpen(false)}>Cancel</Button><Button color="indigo" disabled={providerBusy || !llmEndpoint.trim() || (providerIsBackup && (!providerBackupAnalysisModel.trim() || !providerBackupAssistantModel.trim())) || (providerEnabled && !llmCredential.trim() && !providerConnection) || providerManagedByEnvironment} onClick={saveAIConnection}>{providerBusy ? "Saving…" : "Save connection"}</Button></>}
    >
      <div className="auth-form compact-form ai-model-form">
        <div className="ai-dialog-workload"><ProviderLogo provider={llmProvider} /><span><small>Provider</small><strong>{aiProviderLabel(llmProvider)}</strong></span><Switch checked={providerEnabled} onChange={setProviderEnabled} label="Enabled" /></div>
        {providerManagedByEnvironment ? <div className="private-default-note"><TerminalSquare />This connection is managed by DOKOSOKO_AI_* environment variables. Change it in deployment configuration and restart DokoSoko.</div> : <>
          <label className="auth-field"><span>Provider origin</span><Input name="ai-provider-endpoint" type="url" autoComplete="off" readOnly={llmProvider !== "openai-compatible"} value={llmEndpoint} onChange={(event) => setLLMEndpoint(event.target.value)} placeholder="https://api.provider.com" /><small>{llmProvider === "openai-compatible" ? "A fixed public HTTPS origin. Private-network destinations, redirects, paths, and non-default ports are rejected." : "The native provider origin is fixed by DokoSoko."}</small></label>
          <label className="auth-field" htmlFor="ai-provider-credential"><span>API credential</span><Input id="ai-provider-credential" name="ai-provider-credential" type="password" autoComplete="new-password" value={llmCredential} onChange={(event) => setLLMCredential(event.target.value)} placeholder={providerConnection ? "Leave blank to keep the stored credential" : "Required before enabling"} /><small>Encrypted at rest, redacted from every response, and shared only with the selected provider.</small></label>
          <div className="ai-backup-control"><span><strong>Backup provider</strong><small>Retry this provider once when the selected provider times out, is rate-limited, or is unavailable.</small></span><Switch checked={providerIsBackup} onChange={setProviderIsBackup} label="Use as backup provider" /></div>
          {providerIsBackup && <div className="two-fields"><label className="auth-field"><span>Analysis backup model</span>{llmProvider === "openai-compatible" ? <Input value={providerBackupAnalysisModel} onChange={(event) => setProviderBackupAnalysisModel(event.target.value)} placeholder="Provider model ID" /> : <Select value={providerBackupAnalysisModel} onChange={(event) => setProviderBackupAnalysisModel(event.target.value)}>{aiModelOptions[llmProvider].map((model) => <option key={model} value={model}>{model}</option>)}</Select>}</label><label className="auth-field"><span>Assistant backup model</span>{llmProvider === "openai-compatible" ? <Input value={providerBackupAssistantModel} onChange={(event) => setProviderBackupAssistantModel(event.target.value)} placeholder="Provider model ID" /> : <Select value={providerBackupAssistantModel} onChange={(event) => setProviderBackupAssistantModel(event.target.value)}>{aiModelOptions[llmProvider].map((model) => <option key={model} value={model}>{model}</option>)}</Select>}</label></div>}
        </>}
      </div>
    </Dialog>
  </>;
}
