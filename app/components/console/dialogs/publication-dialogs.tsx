"use client";

import { ShieldCheck } from "lucide-react";

import { Button, Dialog } from "../../core/control";
import type { usePublicationWorkflow } from "../use-publication-workflow";

export function PublicationDialogs({ workspace }: {
  workspace: ReturnType<typeof usePublicationWorkflow>;
}) {
  const {
    pendingPublication, setPendingPublication,
    pendingMCPEnable, setPendingMCPEnable,
    acknowledged, setAcknowledged,
    confirmPublication,
    confirmMCPEnable,
  } = workspace;

  return <>
    <Dialog
      open={Boolean(pendingPublication)}
      onClose={(open) => { if (!open) setPendingPublication(null); }}
      title={`Make ${pendingPublication?.name ?? "source"} public?`}
      description={pendingPublication?.detail ?? "Confirm public visibility."}
      actions={<><Button outline onClick={() => setPendingPublication(null)}>Cancel</Button><Button color="indigo" disabled={!acknowledged} onClick={confirmPublication}>Publish</Button></>}
    >
      <label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>I understand this reviewed content will be available anonymously through Public MCP.</span></label>
    </Dialog>

    <Dialog
      open={pendingMCPEnable}
      onClose={setPendingMCPEnable}
      title="Enable Public MCP?"
      description="Anonymous clients will be able to discover public APIs, documentation, recipes, and read-only resources."
      actions={<><Button outline onClick={() => setPendingMCPEnable(false)}>Cancel</Button><Button color="indigo" disabled={!acknowledged} onClick={confirmMCPEnable}>Enable Public MCP</Button></>}
    >
      <div className="private-default-note"><ShieldCheck />Private tools, customer identity, runtime credentials, and private sources remain unavailable on the public endpoint.</div>
      <label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>I understand the public, read-only catalog becomes anonymously accessible.</span></label>
    </Dialog>
  </>;
}
