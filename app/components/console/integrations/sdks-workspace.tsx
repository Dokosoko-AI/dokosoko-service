"use client";

import { BookOpen, ExternalLink, Package } from "lucide-react";
import { useEffect, useState } from "react";

import { api, type APIIntegration, type APISDKReference } from "../../../lib/api";
import { Badge } from "../../core/control";
import { PanelHeader } from "../../core/layout";

// This component is retained only for deployments that still render the
// legacy API SDK-reference projection. New authoring happens in Catalog; an
// API owns an exact attachment, never the SDK package or release lifecycle.
export function IntegrationSDKsWorkspace({ integration }: {
  integration: APIIntegration;
  onMessage: (message: string) => void;
}) {
  const [references, setReferences] = useState<APISDKReference[]>(integration.sdks ?? []);

  useEffect(() => {
    let cancelled = false;
    api.integrationSDKs(integration.id).then((items) => {
      if (!cancelled) setReferences(items);
    }).catch(() => {
      if (!cancelled) setReferences(integration.sdks ?? []);
    });
    return () => { cancelled = true; };
  }, [integration.id, integration.sdks]);

  return <>
    <div className="notice"><Package /><span><strong>Deployment-owned SDK packages and exact releases.</strong> This API keeps only an immutable exact attachment. Create, ingest, review, and manage SDKs in Catalog.</span></div>
    <section className="panel">
      <PanelHeader title="Legacy SDK attachment projection" description="Read-only compatibility view. Use this API’s Resources tab to attach or change an exact Catalog release." action={<a className="button outline" href="/developer-assets/sdk-packages">Open Catalog SDKs</a>} />
      {references.map((reference) => <div className="provider-row" key={reference.id}>
        <span className="settings-icon"><Package /></span>
        <span><strong>{reference.coordinate}</strong><small>{reference.ecosystem} · exact version {reference.exact_version}</small><code>{reference.install_command}</code></span>
        <span className="tool-badges"><Badge color={reference.visibility === "public" ? "blue" : "zinc"}>{reference.visibility}</Badge><Badge color="green">exact</Badge></span>
        <span className="table-actions">{reference.documentation_url && <a href={reference.documentation_url} target="_blank" rel="noreferrer" className="row-arrow" aria-label={`Open documentation for ${reference.coordinate}`}><BookOpen /></a>}{reference.source_url && <a href={reference.source_url} target="_blank" rel="noreferrer" className="row-arrow" aria-label={`Open source for ${reference.coordinate}`}><ExternalLink /></a>}</span>
      </div>)}
      {references.length === 0 && <div className="empty-row">No exact Catalog SDK release is attached to this API.</div>}
    </section>
  </>;
}
