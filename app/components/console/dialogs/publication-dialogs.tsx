"use client";


import { useTranslation } from "react-i18next";
import { ShieldCheck } from "lucide-react";

import { Button, Dialog } from "../../core/control";
import type { usePublicationWorkflow } from "../use-publication-workflow";

export function PublicationDialogs({ workspace }: {
  workspace: ReturnType<typeof usePublicationWorkflow>;
}) {
  const { t } = useTranslation();
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
      title={t("publicationDialogs.makePublic", { value1: String(pendingPublication?.name ?? "source") })}
      description={pendingPublication?.detail ?? t("publicationDialogs.confirmPublicVisibility")}
      actions={<><Button outline onClick={() => setPendingPublication(null)}>{t("common.cancel")}</Button><Button color="indigo" disabled={!acknowledged} onClick={confirmPublication}>{t("publicationDialogs.publish")}</Button></>}
    >
      <label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{t("publicationDialogs.iUnderstandThisReviewedContentWillBeAvailableAnonymously")}</span></label>
    </Dialog>

    <Dialog
      open={pendingMCPEnable}
      onClose={setPendingMCPEnable}
      title={t("publicationDialogs.enablePublicMCP")}
      description={t("publicationDialogs.anonymousClientsWillBeAbleToDiscoverPublicAPIs")}
      actions={<><Button outline onClick={() => setPendingMCPEnable(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={!acknowledged} onClick={confirmMCPEnable}>{t("publicationDialogs.enablePublicMCP2")}</Button></>}
    >
      <div className="private-default-note"><ShieldCheck />{t("publicationDialogs.privateToolsCustomerIdentityRuntimeCredentialsAndPrivateSources")}</div>
      <label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{t("publicationDialogs.iUnderstandThePublicReadOnlyCatalogBecomesAnonymously")}</span></label>
    </Dialog>
  </>;
}
