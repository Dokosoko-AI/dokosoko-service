"use client";


import { useTranslation } from "react-i18next";
import { GitBranch } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { developerAssetsApi, type APIDeveloperAssetPublication } from "../../../lib/developer-assets-api";
import { Badge } from "../../core/control";
import { PanelHeader } from "../../core/layout";
import { developerAssetError } from "./developer-asset-ui";

export function APIResourcePublicationHistory({ integrationID, live, onMessage, servingPublicationID }: { integrationID: string; live: boolean; onMessage: (message: string) => void; servingPublicationID?: string }) {
  const { t } = useTranslation();
  const [publications, setPublications] = useState<APIDeveloperAssetPublication[]>([]);

  const load = useCallback(async () => {
    if (!live) return;
    try {
      const values = await developerAssetsApi.apiResourcePublications(integrationID);
      setPublications([...values].sort((left, right) => Date.parse(right.published_at) - Date.parse(left.published_at)));
    } catch (error) {
      onMessage(developerAssetError(error, t("apiPublicationHistory.developerAssetPublicationHistoryCouldNotBeLoaded")));
    }
  }, [integrationID, live, onMessage, t]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void load(); }, 0);
    return () => window.clearTimeout(timeout);
  }, [load, servingPublicationID]);

  return <section className="panel api-resource-publications">
    <PanelHeader title={t("apiPublicationHistory.developerAssetSnapshots")} description={t("apiPublicationHistory.exactIDsAndHashesUsedByQueryLabRetrieval")} />
    {publications.map((publication) => <div className="developer-api-publication-row" key={publication.id}>
      <span><GitBranch /><span><strong>{t("format.dateTime", { value: new Date(publication.published_at) })}</strong><code>{publication.id}</code></span></span>
      <span>{publication.id === servingPublicationID && <Badge color="green">{t("publicationReview.currentlyServing")}</Badge>}<code>{publication.snapshot_hash}</code><small>{publication.documentation.length} {t("apiPublicationHistory.documentation")} {publication.contracts.length} {t("apiPublicationHistory.contracts")} {publication.sdks.length} {t("apiPublicationHistory.sdks")}</small></span>
    </div>)}
    {publications.length === 0 && <p className="empty-row">{t("apiPublicationHistory.noImmutableDeveloperAssetSnapshotHasBeenPublishedFor")}</p>}
  </section>;
}
