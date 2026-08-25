"use client";

import { BookOpen, ChevronRight, Share2, Sparkles, TerminalSquare, TriangleAlert, Wrench, XCircle } from "lucide-react";

import type { APIMCPConnection, APITool } from "../../../lib/api";
import { Badge, Button, Dialog } from "../../core/control";
import type { Source } from "../shared";
import { Confirmation, WarningContent } from "../shared";
import type { usePublicationWorkflow } from "../use-publication-workflow";

export function PublicationDialogs({
  workspace,
  sources,
  mcpConnections,
  tools,
  publicResourceCount,
}: {
  workspace: ReturnType<typeof usePublicationWorkflow>;
  sources: Source[];
  mcpConnections: APIMCPConnection[];
  tools: APITool[];
  publicResourceCount: number;
}) {
  const {
    latestProductBuild,
    productBuilderOpen, setProductBuilderOpen,
    productBuildReviewOpen, setProductBuildReviewOpen,
    productBuilderBusy,
    productBuilderInputs, setProductBuilderInputs,
    pendingPublication, setPendingPublication,
    pendingMCPEnable, setPendingMCPEnable,
    acknowledged, setAcknowledged,
    buildProductAutomatically,
    publishImportedAPIs,
    confirmPublication,
    confirmMCPEnable,
  } = workspace;

  return <>
    <Dialog
      open={Boolean(pendingPublication)}
      onClose={(open) => { if (!open) setPendingPublication(null); }}
      title={`Make ${pendingPublication?.name ?? "resource"} public?`}
      description="This is a security-sensitive publication change. Private is the default for every new source."
      actions={<><Button outline onClick={() => setPendingPublication(null)}>Keep private</Button><Button color="red" disabled={!acknowledged} onClick={confirmPublication}>Make public</Button></>}
    >
      <WarningContent>
        <p><strong>{pendingPublication?.detail}</strong> Public MCP does not require users to sign in.</p>
        <p>DokoSoko will record your identity, the prior revision, and this decision in the audit log.</p>
        <Confirmation checked={acknowledged} onChange={setAcknowledged}>I understand this published {pendingPublication?.kind} will be available without authentication.</Confirmation>
      </WarningContent>
    </Dialog>

    <Dialog
      open={productBuilderOpen}
      onClose={setProductBuilderOpen}
      title="Import APIs"
      description="Add specs, documentation, repositories, or MCP endpoints. DokoSoko will group them into reviewable APIs."
      actions={<><Button outline onClick={() => setProductBuilderOpen(false)}>Cancel</Button><Button color="indigo" disabled={productBuilderBusy} onClick={buildProductAutomatically}>{productBuilderBusy ? "Scanning…" : "Import APIs"}</Button></>}
    >
      <div className="product-builder-form">
        <div className="builder-source-summary">
          <span><BookOpen /><strong>{sources.length}</strong><small>docs & specs</small></span>
          <span><Share2 /><strong>{mcpConnections.length}</strong><small>MCP upstreams</small></span>
          <span><Wrench /><strong>{tools.length}</strong><small>tools</small></span>
        </div>
        <label className="auth-field"><span>Anything else?</span><textarea value={productBuilderInputs} onChange={(event) => setProductBuilderInputs(event.target.value)} placeholder={"Paste specs, documentation, repositories, or MCP endpoints—one per line.\nhttps://api.example.com/voice/v3/openapi.yaml\nhttps://github.com/acme/voice-examples"} /><small>Optional. DokoSoko automatically classifies each input and never retrieves credentials embedded in a URL.</small></label>
        <div className="builder-magic-note"><Sparkles /><span><strong>Review exceptions, not configuration.</strong> Exact matches are joined automatically. Ambiguous version relationships stay unresolved and cannot silently fall back.</span></div>
        {latestProductBuild?.state === "review" && <button type="button" className="panel-footer-link builder-review-link" onClick={() => { setProductBuilderOpen(false); setProductBuildReviewOpen(true); }}>Review the latest unpublished proposal <ChevronRight /></button>}
      </div>
    </Dialog>

    <Dialog
      open={productBuildReviewOpen}
      onClose={setProductBuildReviewOpen}
      title="Review imported APIs"
      description="Nothing is added to the catalogue until this exact proposal is reviewed and published."
      actions={<><Button outline onClick={() => setProductBuildReviewOpen(false)}>Keep as draft</Button><Button color="indigo" disabled={productBuilderBusy || !latestProductBuild || latestProductBuild.state !== "review" || latestProductBuild.unresolved.some((finding) => finding.level === "error")} onClick={publishImportedAPIs}>{productBuilderBusy ? "Publishing…" : "Publish proposal"}</Button></>}
    >
      {latestProductBuild ? <div className="product-build-review">
        <div className="builder-source-summary">
          <span><BookOpen /><strong>{latestProductBuild.inputs.length}</strong><small>inputs scanned</small></span>
          <span><TerminalSquare /><strong>{latestProductBuild.proposal.components.length}</strong><small>APIs proposed</small></span>
          <span><TriangleAlert /><strong>{latestProductBuild.unresolved.length}</strong><small>exceptions</small></span>
        </div>
        <div className="product-build-components">
          {latestProductBuild.proposal.components.map((component) => <section className="catalog-settings-section" key={component.id}>
            <div className="catalog-settings-heading"><span><strong>{component.name}</strong><small>{component.releases.length} independently versioned release{component.releases.length === 1 ? "" : "s"}</small></span><Badge color="violet">API</Badge></div>
            {component.releases.map((release) => <div className="build-release-row" key={release.id}><span><strong>{release.version}</strong><small>{release.bindings.map((binding) => binding.name).join(" · ") || "No bindings"}</small></span><Badge color={release.bindings.every((binding) => binding.verified) ? "green" : "amber"}>{release.bindings.filter((binding) => binding.verified).length}/{release.bindings.length} verified</Badge></div>)}
          </section>)}
        </div>
        {latestProductBuild.unresolved.length > 0 && <section className="catalog-settings-section"><div className="catalog-settings-heading"><span><strong>Resolve before publishing</strong><small>Ambiguous relationships never silently fall back.</small></span></div>{latestProductBuild.unresolved.map((finding, index) => <div className={`publish-validation ${finding.level}`} key={`${finding.code}:${index}`}><span>{finding.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{finding.code}</strong><small>{finding.message}</small></span></div>)}</section>}
      </div> : <div className="empty-row">No import proposal is ready for review.</div>}
    </Dialog>

    <Dialog
      open={pendingMCPEnable}
      onClose={setPendingMCPEnable}
      title="Enable authentication-free Public MCP?"
      description="Anyone with the endpoint can query resources that you have explicitly marked public."
      actions={<><Button outline onClick={() => setPendingMCPEnable(false)}>Cancel</Button><Button color="red" disabled={!acknowledged} onClick={confirmMCPEnable}>Enable Public MCP</Button></>}
    >
      <WarningContent>
        <p><strong>{publicResourceCount} published {publicResourceCount === 1 ? "resource is" : "resources are"} currently marked public.</strong> Private sources, API tools, provider resources, credentials, identities, and customer access data remain excluded.</p>
        <p>Anonymous requests are rate-limited and logged as aggregate security events. You can turn this endpoint off immediately.</p>
        <Confirmation checked={acknowledged} onChange={setAcknowledged}>I understand Public MCP is authentication-less and exposes public resources anonymously.</Confirmation>
      </WarningContent>
    </Dialog>
  </>;
}
