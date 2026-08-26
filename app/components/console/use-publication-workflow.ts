"use client";

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
        showToast(`${item.name} is private. Anonymous access was removed immediately.`);
      } catch (error) {
        showToast(error instanceof APIError ? error.message : "Could not update visibility.");
      }
      return;
    }
    setAcknowledged(false);
    setPendingPublication({ kind, id, name: item.name, detail: "Its currently published knowledge will become anonymously searchable." });
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
      showToast(`${pendingPublication.name} is now public. The change was added to audit.`);
      setPendingPublication(null);
      setAcknowledged(false);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not publish this resource.");
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
      showToast("Public MCP is off. Anonymous requests are no longer accepted.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not disable Public MCP.");
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
      showToast("Public MCP is enabled and audit logged.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not enable Public MCP.");
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
