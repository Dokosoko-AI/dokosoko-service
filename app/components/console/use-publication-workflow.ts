"use client";

import { useState, type Dispatch, type SetStateAction } from "react";

import {
  APIError,
  api,
  type APIProduct,
  type APIProductBuild,
  type APIProductBuildInput,
  type APIProductDefinition,
} from "../../lib/api";
import type { ConsoleFixtures } from "../../dev/console-fixtures";
import type { Source } from "./shared";

type PendingPublication = {
  kind: "source";
  id: string;
  name: string;
  detail: string;
};

export function usePublicationWorkflow({ product, setProduct, fixtures, apiConnected, sources, setSources, setPublicMCPEnabled, refreshCatalog, navigateToProduct, showToast }: {
  product: APIProduct;
  setProduct: Dispatch<SetStateAction<APIProduct>>;
  fixtures?: ConsoleFixtures;
  apiConnected: boolean;
  sources: Source[];
  setSources: Dispatch<SetStateAction<Source[]>>;
  setPublicMCPEnabled: Dispatch<SetStateAction<boolean>>;
  refreshCatalog: () => Promise<void>;
  navigateToProduct: () => void;
  showToast: (message: string) => void;
}) {
  const [productDefinition, setProductDefinition] = useState<APIProductDefinition | null>(fixtures?.definition ?? null);
  const [latestProductBuild, setLatestProductBuild] = useState<APIProductBuild | null>(fixtures?.productBuild ?? null);
  const [productBuilderOpen, setProductBuilderOpen] = useState(false);
  const [productBuildReviewOpen, setProductBuildReviewOpen] = useState(false);
  const [productBuilderBusy, setProductBuilderBusy] = useState(false);
  const [productBuilderInputs, setProductBuilderInputs] = useState("");
  const [pendingPublication, setPendingPublication] = useState<PendingPublication | null>(null);
  const [pendingMCPEnable, setPendingMCPEnable] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);
  const [productRevision, setProductRevision] = useState(1);

  async function buildProductAutomatically() {
    setProductBuilderBusy(true);
    const additionalInputs: APIProductBuildInput[] = productBuilderInputs
      .split(/\r?\n/)
      .map((location) => location.trim())
      .filter(Boolean)
      .map((location) => ({ kind: "auto", location }));
    try {
      const fallbackBuildID = `build_${Date.now()}`;
      const value = apiConnected
        ? await api.buildProduct(product.id, additionalInputs)
        : { ...fixtures!.productBuild, id: fallbackBuildID, state: "review" as const, created_at: new Date().toISOString(), completed_at: new Date().toISOString(), inputs: [...fixtures!.productBuild.inputs, ...additionalInputs], proposal: { ...fixtures!.definition, state: "draft" as const, source_build_id: fallbackBuildID } };
      setLatestProductBuild(value);
      setProductBuilderOpen(false);
      setProductBuildReviewOpen(true);
      setProductBuilderInputs("");
      showToast(`${value.inputs.length} sources scanned. Review ${value.unresolved.length || "no"} exception${value.unresolved.length === 1 ? "" : "s"}.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The APIs could not be imported.");
    } finally {
      setProductBuilderBusy(false);
    }
  }

  async function publishImportedAPIs() {
    if (!latestProductBuild || latestProductBuild.state !== "review") return;
    setProductBuilderBusy(true);
    try {
      const definition = apiConnected
        ? await api.publishProductBuild(product.id, latestProductBuild.id)
        : { ...latestProductBuild.proposal, state: "published" as const, revision: latestProductBuild.proposal.revision + 1, published_at: new Date().toISOString() };
      setProductDefinition(definition);
      setLatestProductBuild({ ...latestProductBuild, state: "published", proposal: definition, completed_at: latestProductBuild.completed_at ?? new Date().toISOString() });
      setProductBuildReviewOpen(false);
      await refreshCatalog().catch(() => {});
      navigateToProduct();
      showToast(`${definition.components.length} API proposal${definition.components.length === 1 ? "" : "s"} published to the catalogue.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The imported API proposal could not be published.");
    } finally {
      setProductBuilderBusy(false);
    }
  }

  async function requestVisibility(kind: "source", id: string) {
    const item = sources.find((candidate) => candidate.id === id);
    if (!item) return;
    if (item.visibility === "public") {
      try {
        if (apiConnected) {
          const updated = await api.setSourceVisibility(product.id, id, "private", item.revision, false);
          setSources((items) => items.map((candidate) => candidate.id === id ? { ...candidate, visibility: updated.visibility, revision: updated.revision } : candidate));
        } else setSources((items) => items.map((candidate) => candidate.id === id ? { ...candidate, visibility: "private" } : candidate));
      } catch (error) {
        showToast(error instanceof APIError ? error.message : "Could not update visibility.");
        return;
      }
      showToast(`${item.name} is private. Anonymous access was removed immediately.`);
      return;
    }
    setAcknowledged(false);
    setPendingPublication({ kind, id, name: item.name, detail: "Its currently published knowledge will become anonymously searchable." });
  }

  async function confirmPublication() {
    if (!pendingPublication || !acknowledged) return;
    const { id, name } = pendingPublication;
    const current = sources.find((item) => item.id === id);
    if (!current) return;
    try {
      if (apiConnected) {
        const updated = await api.setSourceVisibility(product.id, id, "public", current.revision, true);
        setSources((items) => items.map((item) => item.id === id ? { ...item, visibility: updated.visibility, revision: updated.revision } : item));
      } else setSources((items) => items.map((item) => item.id === id ? { ...item, visibility: "public" } : item));
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not publish this resource.");
      return;
    }
    setPendingPublication(null);
    setAcknowledged(false);
    showToast(`${name} is now public. The change was added to audit.`);
  }

  async function requestMCPChange(enabled: boolean) {
    if (!enabled) {
      try {
        if (apiConnected) {
          const updated = await api.setPublicMCP(product.id, false, productRevision, false);
          setProductRevision(updated.revision);
          setProduct(updated);
        }
        setPublicMCPEnabled(false);
      } catch (error) {
        showToast(error instanceof APIError ? error.message : "Could not disable Public MCP.");
        return;
      }
      showToast("Public MCP is off. Anonymous requests are no longer accepted.");
      return;
    }
    setAcknowledged(false);
    setPendingMCPEnable(true);
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
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not enable Public MCP.");
      return;
    }
    setPendingMCPEnable(false);
    setAcknowledged(false);
    showToast("Public MCP is enabled and audit logged.");
  }

  return {
    productDefinition, setProductDefinition,
    latestProductBuild, setLatestProductBuild,
    productBuilderOpen, setProductBuilderOpen,
    productBuildReviewOpen, setProductBuildReviewOpen,
    productBuilderBusy,
    productBuilderInputs, setProductBuilderInputs,
    pendingPublication, setPendingPublication,
    pendingMCPEnable, setPendingMCPEnable,
    acknowledged, setAcknowledged,
    productRevision, setProductRevision,
    buildProductAutomatically,
    publishImportedAPIs,
    requestVisibility,
    confirmPublication,
    requestMCPChange,
    confirmMCPEnable,
  };
}
