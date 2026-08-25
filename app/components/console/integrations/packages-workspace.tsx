"use client";

import { Database, Plus, ShieldCheck, TriangleAlert, XCircle } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import {
  APIError,
  api,
  type APIIntegration,
  type APIIntegrationPackageBinding,
  type APIPackageArtifact,
  type APIPackageRelease,
  type APIVisibility,
} from "../../../lib/api";
import { Badge, Button, Dialog } from "../../core/control";
import { PanelHeader } from "../../core/layout";
import { unavailableConsoleCapability } from "../shared";

const packageEcosystemPattern = /^[a-z][a-z0-9._-]{0,63}$/;

function packageSunsetPassed(value?: string) {
  if (!value) return false;
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) && timestamp <= Date.now();
}

function packageArtifactCanPublish(artifact: APIPackageArtifact) {
  return (artifact.lifecycle === "draft" || artifact.lifecycle === "active") && !packageSunsetPassed(artifact.sunset_at);
}

function packageArtifactCanBind(artifact: APIPackageArtifact) {
  return artifact.lifecycle === "active" && !packageSunsetPassed(artifact.sunset_at);
}

function packageArtifactCanPublishForIntegration(artifact: APIPackageArtifact, integration: APIIntegration) {
  return packageArtifactCanPublish(artifact) && (integration.visibility !== "public" || artifact.visibility === "public");
}

function packageLifecycleDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export function IntegrationPackagesWorkspace({ integration, onMessage }: { integration: APIIntegration; onMessage: (message: string) => void }) {
  const [bindings, setBindings] = useState<APIIntegrationPackageBinding[]>([]);
  const [catalog, setCatalog] = useState<APIPackageArtifact[]>([]);
  const [loading, setLoading] = useState(true);
  const [bindingsUnavailable, setBindingsUnavailable] = useState(false);
  const [busy, setBusy] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editingArtifact, setEditingArtifact] = useState<APIPackageArtifact | null>(null);
  const [deprecateOpen, setDeprecateOpen] = useState(false);
  const [deprecatingArtifact, setDeprecatingArtifact] = useState<APIPackageArtifact | null>(null);
  const [deprecationMessage, setDeprecationMessage] = useState("");
  const [replacementArtifactID, setReplacementArtifactID] = useState("");
  const [sunsetAt, setSunsetAt] = useState("");
  const [retireOpen, setRetireOpen] = useState(false);
  const [retiringArtifact, setRetiringArtifact] = useState<APIPackageArtifact | null>(null);
  const [retirementMessage, setRetirementMessage] = useState("");
  const [retirementReplacementID, setRetirementReplacementID] = useState("");
  const [publishReleaseOpen, setPublishReleaseOpen] = useState(false);
  const [publishingArtifact, setPublishingArtifact] = useState<APIPackageArtifact | null>(null);
  const [releasePublicAcknowledged, setReleasePublicAcknowledged] = useState(false);
  const [bindOpen, setBindOpen] = useState(false);
  const [selectedReleaseID, setSelectedReleaseID] = useState("");
  const [ecosystem, setEcosystem] = useState("npm");
  const [packageName, setPackageName] = useState("");
  const [packageDescription, setPackageDescription] = useState("");
  const [packageCoordinate, setPackageCoordinate] = useState("");
  const [artifactPURL, setArtifactPURL] = useState("");
  const [releasePURL, setReleasePURL] = useState("");
  const [registryURL, setRegistryURL] = useState("");
  const [sourceURL, setSourceURL] = useState("");
  const [packageLanguage, setPackageLanguage] = useState("");
  const [packagePlatform, setPackagePlatform] = useState("");
  const [packageVisibility, setPackageVisibility] = useState<APIVisibility>("private");
  const [packagePublicAcknowledged, setPackagePublicAcknowledged] = useState(false);
  const [packageVersion, setPackageVersion] = useState("");
  const [installCommand, setInstallCommand] = useState("");
  const [integrityDigest, setIntegrityDigest] = useState("");
  const [sbomURL, setSBOMURL] = useState("");
  const [provenanceURL, setProvenanceURL] = useState("");
  const ecosystemOptionsID = `package-ecosystems-${integration.id}`;

  const loadPackages = useCallback(async () => {
    setLoading(true);
    const [bindingResult, catalogResult] = await Promise.allSettled([api.integrationPackages(integration.id), api.packageArtifacts()]);
    if (bindingResult.status === "fulfilled") {
      setBindings(bindingResult.value);
      setBindingsUnavailable(false);
    } else {
      setBindings([]);
      setBindingsUnavailable(unavailableConsoleCapability(bindingResult.reason));
    }
    if (catalogResult.status === "fulfilled") {
      const enriched = await Promise.all(catalogResult.value.map(async (artifact) => {
        try {
          const releases = await api.packageReleases(artifact.id);
          return { ...artifact, releases };
        } catch {
          return artifact;
        }
      }));
      setCatalog(enriched);
    } else setCatalog([]);
    setLoading(false);
  }, [integration.id]);

  useEffect(() => {
    const task = window.setTimeout(() => { void loadPackages(); }, 0);
    return () => window.clearTimeout(task);
  }, [loadPackages]);

  const publishedReleases = catalog.flatMap((artifact) => {
    if (!packageArtifactCanBind(artifact)) return [];
    return (artifact.releases ?? (artifact.latest_release ? [artifact.latest_release] : []))
      .filter((release) => integration.visibility !== "public" || release.visibility === "public")
      .map((release) => ({ artifact, release }));
  });
  const replacementCandidates = catalog.filter((artifact) => packageArtifactCanBind(artifact) && (artifact.latest_release || (artifact.releases?.length ?? 0) > 0));
  const ecosystemValid = packageEcosystemPattern.test(ecosystem.trim());

  function resetPackageMetadata() {
    setEcosystem("npm"); setPackageName(""); setPackageDescription(""); setPackageCoordinate(""); setArtifactPURL(""); setRegistryURL(""); setSourceURL(""); setPackageLanguage(""); setPackagePlatform(""); setPackageVisibility("private"); setPackagePublicAcknowledged(false);
  }

  function resetReleaseMetadata() {
    setPackageVersion(""); setReleasePURL(""); setInstallCommand(""); setIntegrityDigest(""); setSBOMURL(""); setProvenanceURL(""); setReleasePublicAcknowledged(false);
  }

  function packageArtifactInput() {
    return { ecosystem: ecosystem.trim().toLowerCase(), name: packageName.trim(), description: packageDescription.trim(), coordinate: packageCoordinate.trim(), purl: artifactPURL.trim(), registry_url: registryURL.trim(), source_url: sourceURL.trim() || undefined, language: packageLanguage.trim() || undefined, platform: packagePlatform.trim() || undefined, visibility: packageVisibility, acknowledge_public: packagePublicAcknowledged };
  }

  function openCreatePackage() {
    resetPackageMetadata();
    setPackageVisibility(integration.visibility);
    resetReleaseMetadata();
    setCreateOpen(true);
  }

  function openEditPackage(artifact: APIPackageArtifact) {
    setEditingArtifact(artifact);
    setEcosystem(artifact.ecosystem); setPackageName(artifact.name); setPackageDescription(artifact.description); setPackageCoordinate(artifact.coordinate); setArtifactPURL(artifact.purl); setRegistryURL(artifact.registry_url); setSourceURL(artifact.source_url ?? ""); setPackageLanguage(artifact.language ?? ""); setPackagePlatform(artifact.platform ?? ""); setPackageVisibility(artifact.visibility); setPackagePublicAcknowledged(false);
    setEditOpen(true);
  }

  function openPublishRelease(artifact: APIPackageArtifact) {
    resetReleaseMetadata();
    setPublishingArtifact(artifact);
    setPublishReleaseOpen(true);
  }

  function openDeprecatePackage(artifact: APIPackageArtifact) {
    setDeprecatingArtifact(artifact); setDeprecationMessage(""); setReplacementArtifactID(""); setSunsetAt(""); setDeprecateOpen(true);
  }

  function openRetirePackage(artifact: APIPackageArtifact) {
    setRetiringArtifact(artifact); setRetirementMessage(artifact.deprecation_message ?? ""); setRetirementReplacementID(artifact.replacement_package_artifact_id ?? ""); setRetireOpen(true);
  }

  async function bindSelectedRelease() {
    if (!selectedReleaseID) return;
    if (!publishedReleases.some(({ release }) => release.id === selectedReleaseID)) {
      onMessage("That release is no longer eligible to bind to this Integration.");
      return;
    }
    setBusy(true);
    try {
      await api.bindIntegrationPackage(integration.id, selectedReleaseID);
      await loadPackages();
      setBindOpen(false);
      setSelectedReleaseID("");
      onMessage("Exact package release bound to this API.");
    } catch (error) {
      onMessage(unavailableConsoleCapability(error) ? "Package bindings are not available in this deployment yet." : error instanceof APIError ? error.message : "Package release could not be bound.");
    } finally { setBusy(false); }
  }

  function packageFailureMessage(error: unknown, fallback: string) {
    if (unavailableConsoleCapability(error)) return "The SDK and package catalogue is not available in this deployment yet.";
    return error instanceof APIError ? error.message : fallback;
  }

  async function recoverPackageWorkflow(knownArtifact: APIPackageArtifact | null, knownRelease: APIPackageRelease | null, failure: string) {
    let artifact = knownArtifact;
    let release = knownRelease;
    try {
      const artifacts = await api.packageArtifacts();
      artifact = (knownArtifact ? artifacts.find((candidate) => candidate.id === knownArtifact.id) : undefined)
        ?? artifacts.find((candidate) => candidate.ecosystem === ecosystem.trim().toLowerCase() && candidate.coordinate === packageCoordinate.trim())
        ?? artifact;
      if (artifact) {
        const releases = await api.packageReleases(artifact.id);
        artifact = { ...artifact, releases };
        release = release ?? releases.find((candidate) => candidate.version === packageVersion.trim() && candidate.purl === releasePURL.trim()) ?? null;
      }
    } catch {
      // A best-effort refresh must not hide the original create, publish, or bind error.
    }
    await loadPackages();
    if (release && integration.visibility === "public" && release.visibility !== "public") {
      setCreateOpen(false);
      setPublishReleaseOpen(false);
      setPublishingArtifact(null);
      setSelectedReleaseID("");
      onMessage(`${failure} The private release was saved, but it cannot be bound to a public Integration; publish a public replacement artifact instead.`);
      return;
    }
    if (release) {
      setCreateOpen(false);
      setPublishReleaseOpen(false);
      setPublishingArtifact(null);
      setSelectedReleaseID(release.id);
      setBindOpen(true);
      onMessage(`${failure} The exact release was saved; finish its binding in Bind existing.`);
      return;
    }
    if (artifact) {
      setCreateOpen(false);
      if (integration.visibility === "public" && artifact.visibility !== "public") {
        setPublishReleaseOpen(false);
        setPublishingArtifact(null);
        onMessage(`${failure} The private artifact draft was saved, but it must be made public before it can publish and bind to this public Integration.`);
        return;
      }
      setPublishingArtifact(artifact);
      setReleasePublicAcknowledged(artifact.visibility === "public" && packagePublicAcknowledged);
      setPublishReleaseOpen(true);
      onMessage(`${failure} The reusable artifact draft was saved; review and retry its release.`);
      return;
    }
    onMessage(`${failure} Refreshing the catalogue did not find a saved artifact; correct the form or retry.`);
  }

  async function createPublishAndBindPackage() {
    if (integration.visibility === "public" && packageVisibility !== "public") {
      onMessage("A public Integration can only bind a public package release.");
      return;
    }
    let artifact: APIPackageArtifact | null = null;
    let release: APIPackageRelease | null = null;
    setBusy(true);
    try {
      artifact = await api.createPackageArtifact(packageArtifactInput());
      const published = await api.publishPackageRelease(artifact.id, { version: packageVersion.trim(), purl: releasePURL.trim(), install_command: installCommand.trim(), digest: integrityDigest.trim(), sbom_url: sbomURL.trim() || undefined, provenance_url: provenanceURL.trim() || undefined, artifact_revision: artifact.revision, acknowledge_public: packageVisibility === "public" && packagePublicAcknowledged });
      release = published.release;
      await api.bindIntegrationPackage(integration.id, release.id);
      await loadPackages();
      setCreateOpen(false);
      resetPackageMetadata(); resetReleaseMetadata();
      onMessage(`${artifact.name}@${release.version} published and bound to ${integration.display_name}.`);
    } catch (error) {
      await recoverPackageWorkflow(artifact, release, packageFailureMessage(error, "Package could not be created, published, and bound."));
    } finally { setBusy(false); }
  }

  async function publishAndBindExistingArtifact() {
    if (!publishingArtifact) return;
    if (!packageArtifactCanPublishForIntegration(publishingArtifact, integration)) {
      onMessage(integration.visibility === "public" && publishingArtifact.visibility !== "public" ? "A private package artifact cannot publish and bind to a public Integration." : "This package artifact cannot publish another release.");
      return;
    }
    let release: APIPackageRelease | null = null;
    setBusy(true);
    try {
      const published = await api.publishPackageRelease(publishingArtifact.id, { version: packageVersion.trim(), purl: releasePURL.trim(), install_command: installCommand.trim(), digest: integrityDigest.trim(), sbom_url: sbomURL.trim() || undefined, provenance_url: provenanceURL.trim() || undefined, artifact_revision: publishingArtifact.revision, acknowledge_public: publishingArtifact.visibility === "public" && releasePublicAcknowledged });
      release = published.release;
      await api.bindIntegrationPackage(integration.id, release.id);
      await loadPackages();
      setPublishReleaseOpen(false); setPublishingArtifact(null); resetReleaseMetadata();
      onMessage(`${published.artifact.name}@${release.version} published and bound to ${integration.display_name}.`);
    } catch (error) {
      await recoverPackageWorkflow(publishingArtifact, release, packageFailureMessage(error, "Package release could not be published and bound."));
    } finally { setBusy(false); }
  }

  async function savePackageEdits() {
    if (!editingArtifact) return;
    setBusy(true);
    try {
      const updated = await api.updatePackageArtifact(editingArtifact.id, { ...packageArtifactInput(), revision: editingArtifact.revision });
      await loadPackages(); setEditOpen(false); setEditingArtifact(null); resetPackageMetadata();
      onMessage(`${updated.name} catalogue metadata updated.`);
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Package metadata could not be updated."); } finally { setBusy(false); }
  }

  async function deprecatePackage() {
    if (!deprecatingArtifact) return;
    setBusy(true);
    try {
      const updated = await api.deprecatePackageArtifact(deprecatingArtifact.id, { replacement_package_artifact_id: replacementArtifactID || undefined, message: deprecationMessage.trim(), sunset_at: sunsetAt ? new Date(sunsetAt).toISOString() : undefined, revision: deprecatingArtifact.revision });
      await loadPackages(); setDeprecateOpen(false); setDeprecatingArtifact(null);
      onMessage(`${updated.name} marked deprecated.`);
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Package could not be deprecated."); } finally { setBusy(false); }
  }

  async function retirePackage() {
    if (!retiringArtifact) return;
    setBusy(true);
    try {
      const updated = await api.retirePackageArtifact(retiringArtifact.id, { replacement_package_artifact_id: retirementReplacementID || undefined, message: retirementMessage.trim(), revision: retiringArtifact.revision });
      await loadPackages(); setRetireOpen(false); setRetiringArtifact(null);
      onMessage(`${updated.name} retired. Existing immutable snapshots retain their exact release.`);
    } catch (error) {
      onMessage(unavailableConsoleCapability(error) ? "Package retirement is not available in this deployment yet." : error instanceof APIError ? error.message : "Package could not be retired.");
    } finally { setBusy(false); }
  }

  async function unbind(binding: APIIntegrationPackageBinding) {
    setBusy(true);
    try {
      await api.unbindIntegrationPackage(integration.id, binding.package_artifact_id);
      setBindings((items) => items.filter((item) => item.package_artifact_id !== binding.package_artifact_id));
      onMessage("Package release removed from this API draft.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Package release could not be removed."); } finally { setBusy(false); }
  }

  return <>
    <div className="notice"><Database /><span><strong>Developer artifact catalogue, not a package proxy or verifier.</strong> Registries deliver package bytes; DokoSoko records digest-identified metadata and binds exact releases to compatible API snapshots.</span></div>
    {bindingsUnavailable && <div className="capability-unavailable"><TriangleAlert /><span><strong>Package binding is not enabled on this deployment.</strong><small>The workspace remains usable and will activate automatically when the package endpoints are installed.</small></span></div>}
    <section className="panel"><PanelHeader title="Bound SDKs & packages" description="Every binding resolves to one exact published release." action={<span className="heading-actions"><Button outline disabled={loading || publishedReleases.length === 0 || bindingsUnavailable} onClick={() => setBindOpen(true)}>Bind existing</Button><Button disabled={bindingsUnavailable} onClick={openCreatePackage}><Plus data-slot="icon" />Add package</Button></span>} />
      {loading ? <div className="empty-row">Loading package catalogue…</div> : bindings.map((binding) => {
        const artifact = binding.artifact ?? catalog.find((candidate) => candidate.id === binding.package_artifact_id);
        const release = binding.release ?? artifact?.releases?.find((candidate) => candidate.id === binding.package_release_id) ?? artifact?.latest_release;
        const replacement = catalog.find((candidate) => candidate.id === artifact?.replacement_package_artifact_id);
        return <div className="provider-row package-binding-row" key={binding.id ?? `${binding.package_artifact_id}:${binding.package_release_id}`}><span className="settings-icon"><Database /></span><span><strong>{artifact?.name ?? binding.package_artifact_id}</strong><small>{artifact?.ecosystem ?? "package"} · {release?.coordinate ?? artifact?.coordinate ?? "—"}@{release?.version ?? binding.package_release_id} · compatible with {integration.version_key}</small>{artifact?.deprecation_message && <small>Lifecycle message: {artifact.deprecation_message}</small>}{artifact?.replacement_package_artifact_id && <small>Replacement: {replacement?.name ?? artifact.replacement_package_artifact_id}</small>}{artifact?.sunset_at && <small>Sunset: {packageLifecycleDate(artifact.sunset_at)}{packageSunsetPassed(artifact.sunset_at) ? " · passed" : ""}</small>}</span><span className="tool-badges"><Badge color="green">exact release</Badge>{artifact && artifact.lifecycle !== "active" && <Badge color={artifact.lifecycle === "deprecated" ? "amber" : "zinc"}>{artifact.lifecycle}</Badge>}</span><span className="table-actions">{release?.digest && <code title={release.digest}>{release.digest.slice(0, 18)}…</code>}<button type="button" className="more" disabled={busy} aria-label={`Unbind ${artifact?.name ?? "package"}`} title="Unbind package" onClick={() => unbind(binding)}><XCircle /></button></span></div>;
      })}
      {!loading && bindings.length === 0 && <div className="empty-row">No SDK or package release is bound to this API.</div>}
    </section>
    <section className="panel"><PanelHeader title="Package catalogue" description="Draft artifact metadata can be edited. Publishing the first release freezes all artifact metadata; later corrections require a replacement artifact. Exact releases are always immutable." />
      {catalog.map((artifact) => {
        const replacement = catalog.find((candidate) => candidate.id === artifact.replacement_package_artifact_id);
        const releases = artifact.releases ?? (artifact.latest_release ? [artifact.latest_release] : []);
        return <div className="provider-row package-binding-row" key={artifact.id}><span className="settings-icon"><Database /></span><span><strong>{artifact.name}</strong><small>{artifact.ecosystem} · {artifact.coordinate} · {releases.length} published release{releases.length === 1 ? "" : "s"}</small><small>Reusable PURL: {artifact.purl}</small>{artifact.deprecation_message && <small>Lifecycle message: {artifact.deprecation_message}</small>}{artifact.replacement_package_artifact_id && <small>Replacement: {replacement ? `${replacement.name} · ${replacement.coordinate}` : artifact.replacement_package_artifact_id}</small>}{artifact.sunset_at && <small>Sunset: {packageLifecycleDate(artifact.sunset_at)}{packageSunsetPassed(artifact.sunset_at) ? " · passed" : ""}</small>}</span><span className="tool-badges"><Badge color={artifact.lifecycle === "active" ? "green" : artifact.lifecycle === "deprecated" ? "amber" : "zinc"}>{artifact.lifecycle}</Badge><Badge color={artifact.visibility === "public" ? "blue" : "zinc"}>{artifact.visibility}</Badge></span><span className="table-actions">{artifact.lifecycle === "draft" && <Button outline disabled={busy} onClick={() => openEditPackage(artifact)}>Edit draft</Button>}{packageArtifactCanPublishForIntegration(artifact, integration) && <Button outline disabled={busy} onClick={() => openPublishRelease(artifact)}>Publish release</Button>}{artifact.lifecycle === "active" && <Button outline disabled={busy} onClick={() => openDeprecatePackage(artifact)}>Deprecate</Button>}{artifact.lifecycle === "deprecated" && <Button outline disabled={busy} onClick={() => openRetirePackage(artifact)}>Retire</Button>}</span></div>;
      })}
      {!loading && catalog.length === 0 && <div className="empty-row">No reusable package artifacts have been created.</div>}
    </section>
    <Dialog open={bindOpen} onClose={setBindOpen} title="Bind a published package" description="Choose an exact release from an active, non-sunset artifact. Deprecated, retired, and sunset entries are excluded." actions={<><Button outline onClick={() => setBindOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !selectedReleaseID} onClick={bindSelectedRelease}>{busy ? "Binding…" : "Bind release"}</Button></>}><label className="auth-field"><span>Package release</span><select value={selectedReleaseID} onChange={(event) => setSelectedReleaseID(event.target.value)}><option value="">Select an exact release</option>{publishedReleases.map(({ artifact, release }) => <option key={release.id} value={release.id}>{artifact.ecosystem} · {artifact.name}@{release.version}</option>)}</select></label></Dialog>
    <Dialog open={createOpen} onClose={setCreateOpen} title="Add SDK or package" description="Create a reusable artifact, publish one exact release, then bind it to this API. A saved draft remains recoverable if a later step fails." actions={<><Button outline onClick={() => setCreateOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !ecosystemValid || !packageName.trim() || !packageCoordinate.trim() || !artifactPURL.trim() || !registryURL.trim() || !packageVersion.trim() || !releasePURL.trim() || !installCommand.trim() || !integrityDigest.trim() || (packageVisibility === "public" && !packagePublicAcknowledged) || (integration.visibility === "public" && packageVisibility !== "public")} onClick={createPublishAndBindPackage}>{busy ? "Publishing…" : "Publish & bind"}</Button></>}>
      <div className="auth-form compact-form">
        <div className="two-fields"><label className="auth-field"><span>Ecosystem identifier</span><input list={ecosystemOptionsID} pattern="[a-z][a-z0-9._-]{0,63}" value={ecosystem} onChange={(event) => setEcosystem(event.target.value.toLowerCase())} placeholder="npm or vendor.ecosystem" /><small>Choose a suggestion or enter a lowercase vendor ecosystem identifier.</small></label><label className="auth-field"><span>Display name</span><input value={packageName} onChange={(event) => setPackageName(event.target.value)} placeholder="Vendor JavaScript SDK" /></label></div>
        <label className="auth-field"><span>Description</span><textarea value={packageDescription} onChange={(event) => setPackageDescription(event.target.value)} placeholder="What this developer artifact provides." /></label>
        <div className="two-fields"><label className="auth-field"><span>Registry coordinate</span><input value={packageCoordinate} onChange={(event) => setPackageCoordinate(event.target.value)} placeholder="@vendor/sdk" /></label><label className="auth-field"><span>Reusable artifact PURL</span><input value={artifactPURL} onChange={(event) => setArtifactPURL(event.target.value)} placeholder="pkg:npm/%40vendor/sdk" /><small>Stable package identity without a version, query, or fragment.</small></label></div>
        <div className="two-fields"><label className="auth-field"><span>Registry URL</span><input type="url" value={registryURL} onChange={(event) => setRegistryURL(event.target.value)} placeholder="https://registry.npmjs.org/…" /></label><label className="auth-field"><span>Source URL</span><input type="url" value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} placeholder="https://github.com/vendor/sdk" /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Language</span><input value={packageLanguage} onChange={(event) => setPackageLanguage(event.target.value)} placeholder="TypeScript" /></label><label className="auth-field"><span>Platform</span><input value={packagePlatform} onChange={(event) => setPackagePlatform(event.target.value)} placeholder="Node.js 22+" /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Exact version</span><input value={packageVersion} onChange={(event) => setPackageVersion(event.target.value)} placeholder="3.2.1" /></label><label className="auth-field"><span>Exact release PURL</span><input value={releasePURL} onChange={(event) => setReleasePURL(event.target.value)} placeholder="pkg:npm/%40vendor/sdk@3.2.1" /><small>Must equal the reusable artifact PURL plus this exact version.</small></label></div>
        <label className="auth-field"><span>Install command</span><input value={installCommand} onChange={(event) => setInstallCommand(event.target.value)} placeholder="npm install @vendor/sdk@3.2.1" /></label>
        <label className="auth-field"><span>Integrity digest</span><input value={integrityDigest} onChange={(event) => setIntegrityDigest(event.target.value)} placeholder="sha256:…" /><small>Required and syntax-validated. DokoSoko stores the supplied digest but does not download or verify package bytes.</small></label>
        <div className="two-fields"><label className="auth-field"><span>SBOM URL</span><input type="url" value={sbomURL} onChange={(event) => setSBOMURL(event.target.value)} /></label><label className="auth-field"><span>Provenance URL</span><input type="url" value={provenanceURL} onChange={(event) => setProvenanceURL(event.target.value)} /></label></div>
        <label className="auth-field"><span>Discovery visibility</span><select value={packageVisibility} onChange={(event) => { setPackageVisibility(event.target.value as APIVisibility); setPackagePublicAcknowledged(false); }}><option value="private">Private</option><option value="public">Public</option></select><small>Public makes this metadata eligible for public Integration discovery only after an exact public release is bound, the Integration is published, and Public MCP is enabled. It does not create a standalone public package catalogue.</small></label>
        {integration.visibility === "public" && packageVisibility !== "public" && <div className="capability-unavailable"><TriangleAlert /><span><strong>A public Integration requires a public package release.</strong><small>Choose Public before publishing and binding this package.</small></span></div>}
        {packageVisibility === "public" && <label className="auth-check"><input type="checkbox" checked={packagePublicAcknowledged} onChange={(event) => setPackagePublicAcknowledged(event.target.checked)} /><span>I understand public package and release metadata becomes discoverable through Public MCP only after public binding and Integration publication.</span></label>}
      </div>
    </Dialog>
    <Dialog open={publishReleaseOpen} onClose={setPublishReleaseOpen} title={`Publish release for ${publishingArtifact?.name ?? "package"}`} description={`Publish and bind a new immutable release of ${publishingArtifact?.purl ?? "this reusable artifact"}. If binding fails, the saved release remains available through Bind existing.`} actions={<><Button outline onClick={() => setPublishReleaseOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !publishingArtifact || !packageArtifactCanPublishForIntegration(publishingArtifact, integration) || !packageVersion.trim() || !releasePURL.trim() || !installCommand.trim() || !integrityDigest.trim() || (publishingArtifact.visibility === "public" && !releasePublicAcknowledged)} onClick={publishAndBindExistingArtifact}>{busy ? "Publishing…" : "Publish & bind"}</Button></>}>
      <div className="auth-form compact-form">
        {publishingArtifact && !packageArtifactCanPublish(publishingArtifact) && <div className="capability-unavailable"><TriangleAlert /><span><strong>This artifact cannot publish another release.</strong><small>Deprecated, retired, and sunset artifacts are immutable lifecycle records.</small></span></div>}
        {publishingArtifact && packageArtifactCanPublish(publishingArtifact) && integration.visibility === "public" && publishingArtifact.visibility !== "public" && <div className="capability-unavailable"><TriangleAlert /><span><strong>This private artifact cannot bind to a public Integration.</strong><small>Create a public replacement artifact instead.</small></span></div>}
        <div className="two-fields"><label className="auth-field"><span>Exact version</span><input value={packageVersion} onChange={(event) => setPackageVersion(event.target.value)} placeholder="3.2.1" /></label><label className="auth-field"><span>Exact release PURL</span><input value={releasePURL} onChange={(event) => setReleasePURL(event.target.value)} placeholder={`${publishingArtifact?.purl ?? "pkg:npm/%40vendor/sdk"}@3.2.1`} /><small>Must equal the reusable artifact PURL plus this exact version.</small></label></div>
        <label className="auth-field"><span>Install command</span><input value={installCommand} onChange={(event) => setInstallCommand(event.target.value)} placeholder="npm install @vendor/sdk@3.2.1" /></label>
        <label className="auth-field"><span>Integrity digest</span><input value={integrityDigest} onChange={(event) => setIntegrityDigest(event.target.value)} placeholder="sha256:…" /><small>DokoSoko records the supplied digest but does not download or verify package bytes.</small></label>
        <div className="two-fields"><label className="auth-field"><span>SBOM URL</span><input type="url" value={sbomURL} onChange={(event) => setSBOMURL(event.target.value)} /></label><label className="auth-field"><span>Provenance URL</span><input type="url" value={provenanceURL} onChange={(event) => setProvenanceURL(event.target.value)} /></label></div>
        {publishingArtifact?.visibility === "public" && <><div className="notice"><ShieldCheck /><span><strong>Public discovery remains gated.</strong> This exact release is only eligible for discovery after public binding, Integration publication, and Public MCP enablement.</span></div><label className="auth-check"><input type="checkbox" checked={releasePublicAcknowledged} onChange={(event) => setReleasePublicAcknowledged(event.target.checked)} /><span>I explicitly confirm this exact release may become discoverable through Public MCP after those publication gates are satisfied.</span></label></>}
      </div>
    </Dialog>
    <Dialog open={editOpen} onClose={setEditOpen} title="Edit draft package" description="Draft catalogue metadata may be replaced until the first exact release is published." actions={<><Button outline onClick={() => setEditOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !editingArtifact || !ecosystemValid || !packageName.trim() || !packageCoordinate.trim() || !artifactPURL.trim() || !registryURL.trim() || (packageVisibility === "public" && !packagePublicAcknowledged)} onClick={savePackageEdits}>{busy ? "Saving…" : "Save draft"}</Button></>}>
      <div className="auth-form compact-form">
        <div className="two-fields"><label className="auth-field"><span>Ecosystem identifier</span><input list={ecosystemOptionsID} pattern="[a-z][a-z0-9._-]{0,63}" value={ecosystem} onChange={(event) => setEcosystem(event.target.value.toLowerCase())} /></label><label className="auth-field"><span>Display name</span><input value={packageName} onChange={(event) => setPackageName(event.target.value)} /></label></div>
        <label className="auth-field"><span>Description</span><textarea value={packageDescription} onChange={(event) => setPackageDescription(event.target.value)} /></label>
        <div className="two-fields"><label className="auth-field"><span>Registry coordinate</span><input value={packageCoordinate} onChange={(event) => setPackageCoordinate(event.target.value)} /></label><label className="auth-field"><span>Reusable artifact PURL</span><input value={artifactPURL} onChange={(event) => setArtifactPURL(event.target.value)} /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Registry URL</span><input type="url" value={registryURL} onChange={(event) => setRegistryURL(event.target.value)} /></label><label className="auth-field"><span>Source URL</span><input type="url" value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Language</span><input value={packageLanguage} onChange={(event) => setPackageLanguage(event.target.value)} /></label><label className="auth-field"><span>Platform</span><input value={packagePlatform} onChange={(event) => setPackagePlatform(event.target.value)} /></label></div>
        <label className="auth-field"><span>Discovery visibility</span><select value={packageVisibility} onChange={(event) => { setPackageVisibility(event.target.value as APIVisibility); setPackagePublicAcknowledged(false); }}><option value="private">Private</option><option value="public">Public</option></select><small>Public eligibility still requires an exact public binding, a published public Integration, and Public MCP.</small></label>
        {packageVisibility === "public" && <label className="auth-check"><input type="checkbox" checked={packagePublicAcknowledged} onChange={(event) => setPackagePublicAcknowledged(event.target.checked)} /><span>I understand public metadata remains gated by public binding, Integration publication, and Public MCP.</span></label>}
      </div>
    </Dialog>
    <Dialog open={deprecateOpen} onClose={setDeprecateOpen} title={`Deprecate ${deprecatingArtifact?.name ?? "package"}`} description="Deprecation immediately blocks new releases, new bindings, and candidate publication. Historical published snapshots remain readable; an optional sunset is migration guidance only." actions={<><Button outline onClick={() => setDeprecateOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !deprecatingArtifact || !deprecationMessage.trim()} onClick={deprecatePackage}>{busy ? "Deprecating…" : "Deprecate"}</Button></>}>
      <div className="auth-form compact-form"><label className="auth-field"><span>Deprecation message</span><textarea value={deprecationMessage} onChange={(event) => setDeprecationMessage(event.target.value)} placeholder="Explain the migration path." /></label><label className="auth-field"><span>Replacement artifact</span><select value={replacementArtifactID} onChange={(event) => setReplacementArtifactID(event.target.value)}><option value="">No replacement</option>{replacementCandidates.filter((artifact) => artifact.id !== deprecatingArtifact?.id).map((artifact) => <option value={artifact.id} key={artifact.id}>{artifact.name} · {artifact.coordinate}</option>)}</select></label><label className="auth-field"><span>Optional sunset</span><input type="datetime-local" value={sunsetAt} onChange={(event) => setSunsetAt(event.target.value)} /><small>Guidance for migration timing; deprecation enforcement is immediate.</small></label></div>
    </Dialog>
    <Dialog open={retireOpen} onClose={setRetireOpen} title={`Retire ${retiringArtifact?.name ?? "package"}`} description="Retirement permanently prevents new releases and bindings. Existing immutable snapshots retain their exact release and lifecycle warning." actions={<><Button outline onClick={() => setRetireOpen(false)}>Cancel</Button><Button color="red" disabled={busy || !retiringArtifact || !retirementMessage.trim()} onClick={retirePackage}>{busy ? "Retiring…" : "Retire package"}</Button></>}>
      <div className="auth-form compact-form"><label className="auth-field"><span>Retirement message</span><textarea value={retirementMessage} onChange={(event) => setRetirementMessage(event.target.value)} placeholder="Explain why this artifact is retired and where consumers should migrate." /></label><label className="auth-field"><span>Replacement artifact</span><select value={retirementReplacementID} onChange={(event) => setRetirementReplacementID(event.target.value)}><option value="">No replacement</option>{replacementCandidates.filter((artifact) => artifact.id !== retiringArtifact?.id).map((artifact) => <option value={artifact.id} key={artifact.id}>{artifact.name} · {artifact.coordinate}</option>)}</select></label></div>
    </Dialog>
    <datalist id={ecosystemOptionsID}><option value="npm" /><option value="pypi" /><option value="maven" /><option value="nuget" /><option value="go" /><option value="docker" /><option value="oci" /></datalist>
  </>;
}
