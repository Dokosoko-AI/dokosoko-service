import { Check, Copy, Sparkles, TriangleAlert } from "lucide-react";
import type { TFunction } from "i18next";
import type { ReactNode } from "react";

import type {
  APIAIProviderConnection,
  APIAIWorkloadProfile,
} from "../../lib/api";
import { Badge, Button } from "../core/control";

export * from "../../lib/console-domain";
export { ConsoleLink, EntityLink } from "./console-link";

export type AIWorkload = APIAIWorkloadProfile["workload"];

export type EntityDetail = {
  eyebrow: string;
  title: string;
  description: string;
  fields: Array<{ label: string; value: string }>;
};

export type DocumentationAttachmentResult = {
  attached: boolean;
  resourceSetName: string;
  revision: number;
};

export const aiWorkloads: Array<{
  role: AIWorkload;
  icon: typeof Sparkles;
}> = [
  { role: "analysis", icon: Sparkles },
];

export function aiWorkloadName(role: AIWorkload, t: TFunction) {
  return role === "analysis" ? t("settings.analysis") : role;
}

export function aiWorkloadDescription(role: AIWorkload, t: TFunction) {
  return role === "analysis" ? t("settings.analysisWorkloadDescription") : role;
}

export const aiModelDefaults: Record<APIAIProviderConnection["provider"], Record<AIWorkload, string>> = {
  openai: { analysis: "gpt-5.6-terra" },
  google: { analysis: "gemini-3.5-flash" },
  anthropic: { analysis: "claude-sonnet-5" },
  digitalocean: { analysis: "openai-gpt-5.6-terra" },
  xai: { analysis: "grok-4.6" },
  deepseek: { analysis: "deepseek-v4-pro" },
  "openai-compatible": { analysis: "" },
};

export const aiModelOptions: Record<APIAIProviderConnection["provider"], string[]> = {
  openai: ["gpt-5.6-terra", "gpt-5.6-sol", "gpt-5.6-luna"],
  google: ["gemini-3.5-flash", "gemini-3.6-flash", "gemini-3.5-flash-lite"],
  anthropic: ["claude-sonnet-5", "claude-opus-5", "claude-fable-5", "claude-haiku-4-5"],
  digitalocean: ["openai-gpt-5.6-terra", "openai-gpt-5.6-sol", "openai-gpt-5.6-luna", "deepseek-v4-pro", "deepseek-4-flash", "qwen3.8-max", "nvidia-nemotron-3-super-120b", "glm-5.2"],
  xai: ["grok-4.6", "grok-4.3", "grok-build-0.1"],
  deepseek: ["deepseek-v4-pro", "deepseek-v4-flash"],
  "openai-compatible": [],
};

export const aiProviders: Array<{ id: APIAIProviderConnection["provider"] }> = [
  { id: "openai" },
  { id: "google" },
  { id: "anthropic" },
  { id: "digitalocean" },
  { id: "xai" },
  { id: "deepseek" },
  { id: "openai-compatible" },
];

export function aiProviderLabel(provider: string, t?: TFunction) {
  return provider === "openai" ? "OpenAI" : provider === "google" ? "Google" : provider === "anthropic" ? "Anthropic" : provider === "digitalocean" ? "DigitalOcean" : provider === "xai" ? "xAI" : provider === "deepseek" ? "DeepSeek" : provider === "openai-compatible" ? (t?.("settings.otherOpenAPICompatibleProviders") ?? provider) : provider;
}

export function aiProviderDescription(provider: APIAIProviderConnection["provider"], t: TFunction) {
  return provider === "openai" ? t("settings.openAIProviderDescription")
    : provider === "google" ? t("settings.googleProviderDescription")
      : provider === "anthropic" ? t("settings.anthropicProviderDescription")
        : provider === "digitalocean" ? t("settings.digitalOceanProviderDescription")
          : provider === "xai" ? t("settings.xAIProviderDescription")
            : provider === "deepseek" ? t("settings.deepSeekProviderDescription")
              : t("settings.openAICompatibleProviderDescription");
}

export function aiProviderOrigin(provider: APIAIProviderConnection["provider"]) {
  return provider === "openai" ? "https://api.openai.com" : provider === "google" ? "https://generativelanguage.googleapis.com" : provider === "anthropic" ? "https://api.anthropic.com" : provider === "digitalocean" ? "https://inference.do-ai.run" : provider === "xai" ? "https://api.x.ai" : provider === "deepseek" ? "https://api.deepseek.com" : "";
}

export function CopyButton({ text, label, disabled = false, onCopied }: { text: string; label: string; disabled?: boolean; onCopied: (label: string) => void }) {
  async function copy() {
    if (disabled) return;
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const area = document.createElement("textarea");
      area.value = text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      document.execCommand("copy");
      area.remove();
    }
    onCopied(label);
  }

  return <Button outline className="full" disabled={disabled} onClick={copy}><Copy data-slot="icon" />{label}</Button>;
}

export function WarningContent({ children }: { children: ReactNode }) { return <div className="warning-content"><div className="warning-icon"><TriangleAlert /></div><div>{children}</div></div>; }
export function Confirmation({ checked, onChange, children }: { checked: boolean; onChange: (checked: boolean) => void; children: ReactNode }) { return <label className="confirmation"><input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} /><span className="check-box">{checked && <Check />}</span><span>{children}</span></label>; }
export function SummaryItem({ label, value, icon }: { label: string; value: string; icon: ReactNode }) { return <div className="summary-item"><span>{icon}</span><div><small>{label}</small><strong>{value}</strong></div></div>; }
export function Metric({ label, value, detail, positive }: { label: string; value: string; detail: string; positive?: boolean }) { return <article className="metric"><span>{label}</span><strong>{value}</strong><small className={positive ? "positive" : ""}>{detail}</small></article>; }
export function SettingsCard({ icon, title, detail, status, statusColor = "green" }: { icon: ReactNode; title: string; detail: string; status: string; statusColor?: "amber" | "zinc" | "green" }) {
  return <span className="panel settings-card"><span className="settings-icon">{icon}</span><span className="settings-card-copy"><span className="settings-card-title">{title}</span><span className="settings-card-detail">{detail}</span></span><Badge color={statusColor}>{status}</Badge></span>;
}
