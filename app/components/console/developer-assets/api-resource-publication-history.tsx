"use client";

import { GitBranch } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { developerAssetsApi, type APIDeveloperAssetPublication } from "../../../lib/developer-assets-api";
import { Badge } from "../../core/control";
import { PanelHeader } from "../../core/layout";
import { developerAssetError } from "./developer-asset-ui";

export function APIResourcePublicationHistory({ integrationID, live, onMessage }: { integrationID: string; live: boolean; onMessage: (message: string) => void }) {
  const [publications, setPublications] = useState<APIDeveloperAssetPublication[]>([]);

  const load = useCallback(async () => {
    if (!live) return;
    try {
      const values = await developerAssetsApi.apiResourcePublications(integrationID);
      setPublications([...values].sort((left, right) => Date.parse(right.published_at) - Date.parse(left.published_at)));
    } catch (error) {
      onMessage(developerAssetError(error, "Developer-asset publication history could not be loaded."));
    }
  }, [integrationID, live, onMessage]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void load(); }, 0);
    return () => window.clearTimeout(timeout);
  }, [load]);

  return <section className="panel api-resource-publications">
    <PanelHeader title="Developer asset snapshots" description="Exact IDs and hashes used by Query Lab, retrieval, and recipe evidence for each API publication." />
    {publications.map((publication, index) => <div className="developer-api-publication-row" key={publication.id}>
      <span><GitBranch /><span><strong>{new Date(publication.published_at).toLocaleString()}</strong><code>{publication.id}</code></span></span>
      <span>{index === 0 && <Badge color="green">latest</Badge>}<code>{publication.snapshot_hash}</code><small>{publication.documentation.length} documentation · {publication.contracts.length} contracts · {publication.sdks.length} SDKs</small></span>
    </div>)}
    {publications.length === 0 && <p className="empty-row">No immutable developer-asset snapshot has been published for this API.</p>}
  </section>;
}
