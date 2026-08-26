"use client";

import { Check, ExternalLink, LockKeyhole, ShieldCheck, TriangleAlert, Users } from "lucide-react";

import { Badge, Button, Dialog, Switch } from "../../core/control";
import type { useMCPWorkspaceState } from "../use-mcp-workspace";

export function MCPDialogs({ workspace, connectionReady }: {
  workspace: ReturnType<typeof useMCPWorkspaceState>;
  connectionReady: boolean;
}) {
  const {
    mcpConnectionOpen, setMCPConnectionOpen,
    mcpImportOpen, setMCPImportOpen,
    mcpBusy,
    mcpCatalog,
    mcpSelectedTools, setMCPSelectedTools,
    mcpImportFailures,
    mcpName, setMCPName,
    mcpNamespace, setMCPNamespace,
    mcpEndpoint, setMCPEndpoint,
    mcpAccessToken, setMCPAccessToken,
    mcpForwardUserIdentity, setMCPForwardUserIdentity,
    mcpGrants, setMCPGrants,
    mcpConfirmationRequired, setMCPConfirmationRequired,
    createMCPConnection,
    importMCPTools,
  } = workspace;

  return <>
    <Dialog
      open={mcpConnectionOpen}
      onClose={setMCPConnectionOpen}
      title="Connect third-party MCP"
      description="Register one fixed upstream, inspect its complete catalog, then explicitly select the tools DokoSoko may expose."
      actions={<><Button outline onClick={() => setMCPConnectionOpen(false)}>Cancel</Button><Button color="indigo" disabled={mcpBusy || !connectionReady} onClick={createMCPConnection}>{mcpBusy ? "Inspecting…" : "Connect & inspect"}</Button></>}
    >
      <div className="auth-form compact-form">
        <a className="protocol-policy" href="https://blog.modelcontextprotocol.io/posts/2026-07-28/" target="_blank" rel="noreferrer"><ShieldCheck /><span><strong>Stateless MCPv2 Only</strong><small>Protocol revision 2026-07-28 · no logical live sessions</small></span><ExternalLink /></a>
        <div className="two-fields"><label className="auth-field"><span>Connection name</span><input value={mcpName} onChange={(event) => setMCPName(event.target.value)} placeholder="Support operations" /></label><label className="auth-field"><span>Local namespace</span><input value={mcpNamespace} onChange={(event) => setMCPNamespace(event.target.value)} placeholder="support" pattern="[a-z][a-z0-9_]*" /></label></div>
        <label className="auth-field"><span>Fixed HTTPS MCP endpoint</span><input type="url" value={mcpEndpoint} onChange={(event) => setMCPEndpoint(event.target.value)} placeholder="https://mcp.vendor.com/v2" /><small>Default HTTPS port only. Redirects and private-network destinations are denied.</small></label>
        <label className="auth-field"><span>Upstream access token</span><input type="password" autoComplete="off" value={mcpAccessToken} onChange={(event) => setMCPAccessToken(event.target.value)} /><small>Encrypted server-side and used only for this fixed upstream endpoint.</small></label>
        <Switch checked={mcpForwardUserIdentity} onChange={setMCPForwardUserIdentity} label="Forward signed DokoSoko user identity" />
        <div className="private-default-note"><Users />The inbound bearer token is never forwarded. When enabled, the service sends a small trusted identity envelope signed with the upstream access token.</div>
      </div>
    </Dialog>

    <Dialog
      open={mcpImportOpen}
      onClose={setMCPImportOpen}
      title={`Review tools from ${mcpCatalog?.connection.name ?? "upstream MCP"}`}
      description="Upstream names, descriptions, schemas, and annotations are untrusted input. Only checked tools become local drafts."
      actions={<><Button outline onClick={() => setMCPImportOpen(false)}>Cancel</Button><Button color="indigo" disabled={mcpBusy || mcpSelectedTools.length === 0} onClick={importMCPTools}>{mcpBusy ? "Pinning schemas…" : `Import ${mcpSelectedTools.length} draft${mcpSelectedTools.length === 1 ? "" : "s"}`}</Button></>}
    >
      <div className="mcp-import-review">
        <div className="import-summary"><Badge color="violet">Stateless MCPv2 Only</Badge><code>{mcpCatalog?.connection.namespace}.*</code><span>{mcpCatalog?.tools.length ?? 0} catalog tools</span></div>
        {Object.keys(mcpImportFailures).length > 0 && <div className="capability-unavailable mcp-import-failures" role="alert"><TriangleAlert /><span><strong>Some tools were rejected.</strong><small>Close this dialog, resolve the local conflict, then inspect the connection again.</small>{Object.entries(mcpImportFailures).map(([name, reason]) => <span key={name}><code>{name}</code><small>{reason}</small></span>)}</span></div>}
        <div className="catalog-list">{mcpCatalog?.tools.map((tool) => <label className="catalog-tool" key={tool.name}><input type="checkbox" checked={mcpSelectedTools.includes(tool.name)} onChange={(event) => setMCPSelectedTools((items) => event.target.checked ? [...items, tool.name] : items.filter((name) => name !== tool.name))} /><span className="check-box">{mcpSelectedTools.includes(tool.name) && <Check />}</span><span><strong>{tool.title || tool.name}</strong><code>{tool.name}</code><small>{tool.description || "No upstream description"}</small></span><Badge color="zinc">{tool.schema_hash.slice(0, 12)}</Badge></label>)}</div>
        <label className="auth-field"><span>Required grants</span><input value={mcpGrants} onChange={(event) => setMCPGrants(event.target.value)} placeholder="support.write, developer.pro" /><small>Evaluated before every upstream call. Access-evaluation failures deny execution.</small></label>
        <Switch checked={mcpConfirmationRequired} onChange={setMCPConfirmationRequired} label="Require user confirmation before execution" />
        <div className="private-default-note"><LockKeyhole />Import pins each schema hash. Published tools fail closed if a later catalog inspection detects drift.</div>
      </div>
    </Dialog>
  </>;
}
