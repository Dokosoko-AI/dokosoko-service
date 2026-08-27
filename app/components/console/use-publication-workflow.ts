"use client";


import { useTranslation } from "react-i18next";
import { useState, type Dispatch, type SetStateAction } from "react";

import { APIError, api, type APIProduct } from "../../lib/api";
import type { Source } from "./shared";

type PendingPublication = {
  kind: "source";
  id: string;
  name: string;
  detail: string;
};

export function usePublicationWorkflow({ product, setProduct, apiConnected, sources, setSources, setPublicMCPEnabled, showToast }: {
  product: APIProduct;
  setProduct: Dispatch<SetStateAction<APIProduct>>;
  apiConnected: boolean;
  sources: Source[];
  setSources: Dispatch<SetStateAction<Source[]>>;
  setPublicMCPEnabled: Dispatch<SetStateAction<boolean>>;
  showToast: (message: string) => void;
}) {
  const { t } = useTranslation();
  const [pendingPublication, setPendingPublication] = useState<PendingPublication | null>(null);
  const [pendingMCPEnable, setPendingMCPEnable] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);
  const [productRevision, setProductRevision] = useState(product.revision);

  async function requestVisibility(kind: "source", id: string) {
    const item = sources.find((candidate) => candidate.id === id);
    if (!item) return;
    if (item.visibility === "public") {
      try {
        if (apiConnected) {
          const updated = await api.setSourceVisibility(product.id, id, "private", item.revision, false);
          setSources((items) => items.map((candidate) => candidate.id === id ? { ...candidate, visibility: updated.visibility, revision: updated.revision } : candidate));
        } else {
          setSources((items) => items.map((candidate) => candidate.id === id ? { ...candidate, visibility: "private" } : candidate));
        }
        showToast(t("publicationWorkflow.isPrivateAnonymousAccessWasRemovedImmediately", { name: String(item.name) }));
      } catch (error) {
        showToast(error instanceof APIError ? error.message : t("publicationWorkflow.couldNotUpdateVisibility"));
      }
      return;
    }
    setAcknowledged(false);
    setPendingPublication({ kind, id, name: item.name, detail: t("publicationWorkflow.publishedKnowledgeBecomesSearchable") });
  }

  async function confirmPublication() {
    if (!pendingPublication || !acknowledged) return;
    const current = sources.find((item) => item.id === pendingPublication.id);
    if (!current) return;
    try {
      if (apiConnected) {
        const updated = await api.setSourceVisibility(product.id, current.id, "public", current.revision, true);
        setSources((items) => items.map((item) => item.id === current.id ? { ...item, visibility: updated.visibility, revision: updated.revision } : item));
      } else {
        setSources((items) => items.map((item) => item.id === current.id ? { ...item, visibility: "public" } : item));
      }
      showToast(t("publicationWorkflow.isNowPublicTheChangeWasAddedToAudit", { name: String(pendingPublication.name) }));
      setPendingPublication(null);
      setAcknowledged(false);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : t("publicationWorkflow.couldNotPublishThisResource"));
    }
  }

  async function requestMCPChange(enabled: boolean) {
    if (enabled) {
      setAcknowledged(false);
      setPendingMCPEnable(true);
      return;
    }
    try {
      if (apiConnected) {
        const updated = await api.setPublicMCP(product.id, false, productRevision, false);
        setProductRevision(updated.revision);
        setProduct(updated);
      }
      setPublicMCPEnabled(false);
      showToast(t("publicationWorkflow.publicMCPIsOffAnonymousRequestsAreNoLonger"));
    } catch (error) {
      showToast(error instanceof APIError ? error.message : t("publicationWorkflow.couldNotDisablePublicMCP"));
    }
  }

  async function confirmMCPEnable() {
    if (!acknowledged) return;
    try {
      if (apiConnected) {
        const updated = await api.setPublicMCP(product.id, true, productRevision, true);
        setProductRevision(updated.revision);
        setProduct(updated);
      }
      setPublicMCPEnabled(true);
      setPendingMCPEnable(false);
      setAcknowledged(false);
      showToast(t("publicationWorkflow.publicMCPIsEnabledAndAuditLogged"));
    } catch (error) {
      showToast(error instanceof APIError ? error.message : t("publicationWorkflow.couldNotEnablePublicMCP"));
    }
  }

  return {
    pendingPublication, setPendingPublication,
    pendingMCPEnable, setPendingMCPEnable,
    acknowledged, setAcknowledged,
    productRevision, setProductRevision,
    requestVisibility,
    confirmPublication,
    requestMCPChange,
    confirmMCPEnable,
  };
}
