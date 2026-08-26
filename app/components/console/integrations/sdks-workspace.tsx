"use client";

import { BookOpen, ExternalLink, Package, Pencil, Plus, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";

import { APIError, api, type APIIntegration, type APISDKReference, type APISDKReferenceInput } from "../../../lib/api";
import { Badge, Button, Dialog } from "../../core/control";
import { PanelHeader } from "../../core/layout";

const emptyInput: APISDKReferenceInput = {
  ecosystem: "npm",
  coordinate: "",
  exact_version: "",
  install_command: "",
  visibility: "private",
};

export function IntegrationSDKsWorkspace({ integration, onMessage }: {
  integration: APIIntegration;
  onMessage: (message: string) => void;
}) {
  const [references, setReferences] = useState<APISDKReference[]>(integration.sdks ?? []);
  const [editing, setEditing] = useState<APISDKReference | null>(null);
  const [input, setInput] = useState<APISDKReferenceInput>(emptyInput);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api.integrationSDKs(integration.id).then((items) => {
      if (!cancelled) setReferences(items);
    }).catch(() => {
      if (!cancelled) setReferences(integration.sdks ?? []);
    });
    return () => { cancelled = true; };
  }, [integration.id, integration.sdks]);

  function openEditor(reference?: APISDKReference) {
    setEditing(reference ?? null);
    setInput(reference ? {
      ecosystem: reference.ecosystem,
      coordinate: reference.coordinate,
      exact_version: reference.exact_version,
      install_command: reference.install_command,
      documentation_url: reference.documentation_url,
      source_url: reference.source_url,
      checksum: reference.checksum,
      visibility: reference.visibility,
      revision: reference.revision,
    } : emptyInput);
    setOpen(true);
  }

  async function save() {
    setBusy(true);
    try {
      const value = editing
        ? await api.replaceIntegrationSDK(integration.id, editing.id, { ...input, revision: editing.revision })
        : await api.createIntegrationSDK(integration.id, input);
      setReferences((items) => editing ? items.map((item) => item.id === value.id ? value : item) : [...items, value]);
      setOpen(false);
      onMessage(editing ? "SDK reference updated." : "Exact SDK reference added.");
    } catch (error) {
      onMessage(error instanceof APIError ? error.message : "SDK reference could not be saved.");
    } finally {
      setBusy(false);
    }
  }

  async function remove(reference: APISDKReference) {
    if (!window.confirm(`Remove ${reference.coordinate}@${reference.exact_version} from this API?`)) return;
    setBusy(true);
    try {
      await api.deleteIntegrationSDK(integration.id, reference.id);
      setReferences((items) => items.filter((item) => item.id !== reference.id));
      onMessage("SDK reference removed.");
    } catch (error) {
      onMessage(error instanceof APIError ? error.message : "SDK reference could not be removed.");
    } finally {
      setBusy(false);
    }
  }

  const ready = input.coordinate.trim() && input.exact_version.trim() && input.install_command.trim();

  return <>
    <div className="notice"><Package /><span><strong>Exact API-owned SDK references.</strong> Each entry names one installable coordinate and immutable version. There is no global package catalogue or release workflow.</span></div>
    <section className="panel">
      <PanelHeader title="SDKs" description="References are published with this API revision." action={<Button onClick={() => openEditor()}><Plus data-slot="icon" />Add SDK</Button>} />
      {references.map((reference) => <div className="provider-row" key={reference.id}>
        <span className="settings-icon"><Package /></span>
        <span><strong>{reference.coordinate}</strong><small>{reference.ecosystem} · exact version {reference.exact_version}</small><code>{reference.install_command}</code></span>
        <span className="tool-badges"><Badge color={reference.visibility === "public" ? "blue" : "zinc"}>{reference.visibility}</Badge><Badge color="green">exact</Badge></span>
        <span className="table-actions">{reference.documentation_url && <a href={reference.documentation_url} target="_blank" rel="noreferrer" className="row-arrow" aria-label={`Open documentation for ${reference.coordinate}`}><BookOpen /></a>}{reference.source_url && <a href={reference.source_url} target="_blank" rel="noreferrer" className="row-arrow" aria-label={`Open source for ${reference.coordinate}`}><ExternalLink /></a>}<Button outline disabled={busy} onClick={() => openEditor(reference)}><Pencil data-slot="icon" />Edit</Button><Button outline disabled={busy} onClick={() => void remove(reference)}><Trash2 data-slot="icon" />Remove</Button></span>
      </div>)}
      {references.length === 0 && <div className="empty-row">No SDK reference is attached to this API.</div>}
    </section>

    <Dialog open={open} onClose={setOpen} title={editing ? "Edit SDK reference" : "Add SDK reference"} description="Use one exact version. Version ranges and latest tags are rejected." actions={<><Button outline onClick={() => setOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !ready} onClick={save}>{busy ? "Saving…" : "Save SDK"}</Button></>}>
      <div className="auth-form compact-form">
        <div className="two-fields"><label className="auth-field"><span>Ecosystem</span><input value={input.ecosystem} onChange={(event) => setInput((value) => ({ ...value, ecosystem: event.target.value }))} placeholder="npm" /></label><label className="auth-field"><span>Exact version</span><input value={input.exact_version} onChange={(event) => setInput((value) => ({ ...value, exact_version: event.target.value }))} placeholder="3.2.1" /></label></div>
        <label className="auth-field"><span>Package coordinate</span><input value={input.coordinate} onChange={(event) => setInput((value) => ({ ...value, coordinate: event.target.value }))} placeholder="@acme/platform-sdk" /></label>
        <label className="auth-field"><span>Install command</span><input value={input.install_command} onChange={(event) => setInput((value) => ({ ...value, install_command: event.target.value }))} placeholder="npm install @acme/platform-sdk@3.2.1" /></label>
        <div className="two-fields"><label className="auth-field"><span>Documentation URL</span><input type="url" value={input.documentation_url ?? ""} onChange={(event) => setInput((value) => ({ ...value, documentation_url: event.target.value || undefined }))} /></label><label className="auth-field"><span>Source URL</span><input type="url" value={input.source_url ?? ""} onChange={(event) => setInput((value) => ({ ...value, source_url: event.target.value || undefined }))} /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Checksum</span><input value={input.checksum ?? ""} onChange={(event) => setInput((value) => ({ ...value, checksum: event.target.value || undefined }))} placeholder="sha256:…" /></label><label className="auth-field"><span>Visibility</span><select value={input.visibility} onChange={(event) => setInput((value) => ({ ...value, visibility: event.target.value as APISDKReferenceInput["visibility"] }))}><option value="private">Private</option><option value="public">Public</option></select></label></div>
      </div>
    </Dialog>
  </>;
}
