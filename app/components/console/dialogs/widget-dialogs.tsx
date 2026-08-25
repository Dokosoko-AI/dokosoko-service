"use client";

import type { APIIntegration } from "../../../lib/api";
import { Button, Dialog } from "../../core/control";
import { KeyRound } from "lucide-react";
import type { useWidgetWorkflow } from "../use-widget-workflow";
import { CopyButton } from "../shared";

export function WidgetDialogs({
  workspace,
  integrations,
  onMessage,
}: {
  workspace: ReturnType<typeof useWidgetWorkflow>;
  integrations: APIIntegration[];
  onMessage: (message: string) => void;
}) {
  const {
    widgetCreateOpen,
    setWidgetCreateOpen,
    widgetBusy,
    widgetName,
    setWidgetName,
    widgetOrigins,
    setWidgetOrigins,
    widgetIntegrationIDs,
    setWidgetIntegrationIDs,
    widgetCredential,
    setWidgetCredential,
    createWidget,
  } = workspace;
  const activeIntegrations = integrations.filter((integration) => integration.lifecycle === "active");

  return <>
    <Dialog
      open={widgetCreateOpen}
      onClose={setWidgetCreateOpen}
      title="Create widget"
      description="Start with one authenticated widget, then connect only the APIs it should expose."
      actions={<><Button outline onClick={() => setWidgetCreateOpen(false)}>Cancel</Button><Button color="indigo" disabled={widgetBusy || !widgetName.trim() || !widgetOrigins.trim() || widgetIntegrationIDs.length === 0} onClick={createWidget}>{widgetBusy ? "Creating…" : "Create widget"}</Button></>}
    >
      <div className="auth-form compact-form">
        <label className="auth-field"><span>Name</span><input value={widgetName} maxLength={120} onChange={(event) => setWidgetName(event.target.value)} placeholder="Customer assistant" /></label>
        <label className="auth-field"><span>Allowed application origins</span><textarea value={widgetOrigins} onChange={(event) => setWidgetOrigins(event.target.value)} placeholder={"https://app.example.com\nhttp://localhost:3000"} /><small>One exact origin per line. Paths and wildcard domains are not accepted.</small></label>
        <fieldset className="widget-api-picker"><legend>APIs this widget can use</legend>{activeIntegrations.map((integration) => <label key={integration.id}><input aria-label={`Allow ${integration.display_name}`} type="checkbox" checked={widgetIntegrationIDs.includes(integration.id)} onChange={(event) => setWidgetIntegrationIDs((values) => event.target.checked ? [...values, integration.id] : values.filter((id) => id !== integration.id))} /><span><strong>{integration.display_name}</strong><small>{integration.family_key} · {integration.version_key}</small></span></label>)}{activeIntegrations.length === 0 && <p className="empty-picker">Publish an API before creating a widget.</p>}</fieldset>
      </div>
    </Dialog>

    <Dialog
      open={Boolean(widgetCredential)}
      onClose={(open) => { if (!open) setWidgetCredential(null); }}
      title="Save the widget secret"
      description="This server-only credential is shown once. DokoSoko stores only its hash."
      actions={<Button color="indigo" onClick={() => setWidgetCredential(null)}>I saved it</Button>}
    >
      <div className="one-time-secret"><div><KeyRound /><span><strong>Server only</strong><small>Never place this value in browser code or NEXT_PUBLIC variables.</small></span></div><code>{widgetCredential?.secret}</code><CopyButton text={widgetCredential?.secret ?? ""} label="Copy secret" onCopied={onMessage} /></div>
    </Dialog>
  </>;
}
