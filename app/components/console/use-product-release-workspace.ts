"use client";

import { useEffect, useState } from "react";

import { APIError, api } from "../../lib/api";
import type {
  APICustomerAccount,
  APIProduct,
  APIProductDefinition,
  APIProductInstallation,
  APIProductVersion,
  APIProductVersionImpact,
  APIProductVersionPin,
  APIProductVersionPinHistory,
} from "../../lib/api";
import type { ConsoleFixtures } from "../../dev/console-fixtures";

type LoadProblemReporter = (area: string, error: unknown) => void;

export function useProductReleaseState({
  product,
  fixtures,
  fixturePreview,
  onLoadProblem,
  productDefinition,
  onProductChanged,
  showToast,
}: {
  product: APIProduct;
  fixtures?: ConsoleFixtures;
  fixturePreview: boolean;
  onLoadProblem: LoadProblemReporter;
  productDefinition: APIProductDefinition | null;
  onProductChanged: (product: APIProduct) => void;
  showToast: (message: string) => void;
}) {
  const apiConnected = !fixturePreview;
  const [productCatalogOpen, setProductCatalogOpen] = useState(false);
  const [productCatalogBusy, setProductCatalogBusy] = useState(false);
  const [productDescription, setProductDescription] = useState(product.description);
  const [defaultVersionPolicy, setDefaultVersionPolicy] = useState<"latest" | "lts">(product.default_version_policy);
  const [requirePromotionApproval, setRequirePromotionApproval] = useState(product.require_promotion_approval);
  const [productVersions, setProductVersions] = useState<APIProductVersion[]>(fixtures?.productVersions ?? []);
  const [productVersionPins, setProductVersionPins] = useState<APIProductVersionPin[]>(fixtures?.productPins ?? []);
  const [customerAccountLoad, setCustomerAccountLoad] = useState<{
    productID: string;
    status: "loading" | "ready" | "unavailable";
    items: APICustomerAccount[];
    hasMore: boolean;
  }>({ productID: product.id, status: "loading", items: [], hasMore: false });
  const [productInstallations, setProductInstallations] = useState<APIProductInstallation[]>(fixtures?.installations ?? []);
  const [pinHistory, setPinHistory] = useState<APIProductVersionPinHistory[]>([]);
  const [newProductVersion, setNewProductVersion] = useState("");
  const [newProductProfile, setNewProductProfile] = useState(fixtures?.definition.profiles[0]?.id ?? "");
  const [newVersionLatest, setNewVersionLatest] = useState(true);
  const [newVersionLTS, setNewVersionLTS] = useState(false);
  const [newVersionStage, setNewVersionStage] = useState<"preview" | "active">("active");
  const [newVersionRollout, setNewVersionRollout] = useState(100);
  const [pinScope, setPinScope] = useState<"customer" | "environment" | "installation">("customer");
  const [pinCustomerID, setPinCustomerID] = useState("");
  const [pinVersionID, setPinVersionID] = useState(fixtures?.productVersions[0]?.id ?? "");
  const [pinReason, setPinReason] = useState("");
  const [versionLifecycleOpen, setVersionLifecycleOpen] = useState(false);
  const [editingProductVersion, setEditingProductVersion] = useState<APIProductVersion | null>(null);
  const [lifecycleLatest, setLifecycleLatest] = useState(false);
  const [lifecycleLTS, setLifecycleLTS] = useState(false);
  const [lifecycleDeprecated, setLifecycleDeprecated] = useState(false);
  const [lifecycleMessage, setLifecycleMessage] = useState("");
  const [lifecycleReplacement, setLifecycleReplacement] = useState("");
  const [lifecycleSunset, setLifecycleSunset] = useState("");
  const [lifecycleRollout, setLifecycleRollout] = useState(100);
  const [lifecycleImpact, setLifecycleImpact] = useState<APIProductVersionImpact | null>(null);
  const [lifecycleImpactAcknowledged, setLifecycleImpactAcknowledged] = useState(false);
  const [installationName, setInstallationName] = useState("");
  const [installationExternalID, setInstallationExternalID] = useState("");
  const [installationCustomerID, setInstallationCustomerID] = useState("");
  const [installationEnvironmentID, setInstallationEnvironmentID] = useState(fixtures?.environment.id ?? "");

  const accountDataCurrent = customerAccountLoad.productID === product.id;

  useEffect(() => {
    let cancelled = false;
    const request = fixturePreview
      ? Promise.resolve({ items: fixtures?.customerAccounts ?? [], has_more: false })
      : api.customerAccounts(product.id);
    request.then((page) => {
      if (!cancelled) {
        setCustomerAccountLoad({
          productID: product.id,
          status: "ready",
          items: page.items,
          hasMore: page.has_more,
        });
      }
    }).catch((error) => {
      if (!cancelled) {
        setCustomerAccountLoad({
          productID: product.id,
          status: "unavailable",
          items: [],
          hasMore: false,
        });
        onLoadProblem("Customer accounts", error);
      }
    });
    return () => { cancelled = true; };
  }, [fixturePreview, fixtures?.customerAccounts, onLoadProblem, product.id]);

  useEffect(() => {
    if (fixturePreview) return;
    let cancelled = false;

    api.productVersions(product.id)
      .then((values) => {
        if (!cancelled) {
          setProductVersions(values);
          setPinVersionID(values.find((value) => value.is_latest)?.id ?? values[0]?.id ?? "");
        }
      })
      .catch((error) => onLoadProblem("Product versions", error));

    api.productVersionPins(product.id)
      .then((values) => {
        if (!cancelled) setProductVersionPins(values);
      })
      .catch((error) => onLoadProblem("Version pins", error));

    Promise.all([
      api.productInstallations(product.id),
      api.productVersionPinHistory(product.id),
    ])
      .then(([installationValues, historyValues]) => {
        if (!cancelled) {
          setProductInstallations(installationValues);
          setPinHistory(historyValues);
        }
      })
      .catch((error) => onLoadProblem("Installations", error));

    return () => { cancelled = true; };
  }, [fixturePreview, onLoadProblem, product.id]);

  function openProductCatalog() {
    setProductDescription(product.description);
    setDefaultVersionPolicy(product.default_version_policy);
	setRequirePromotionApproval(product.require_promotion_approval);
    setNewProductProfile(productDefinition?.profiles[0]?.id ?? "");
    setProductCatalogOpen(true);
  }

  async function saveProductDiscoverySettings() {
    setProductCatalogBusy(true);
    try {
      const value = apiConnected
		? await api.updateProductSettings(product.id, productDescription, defaultVersionPolicy, requirePromotionApproval, product.revision)
		: { ...product, description: productDescription.trim(), default_version_policy: defaultVersionPolicy, require_promotion_approval: requirePromotionApproval, catalog_revision: product.catalog_revision + 1, revision: product.revision + 1 };
      onProductChanged(value);
      setProductDescription(value.description);
      showToast("Agent-facing deployment description and default release policy saved.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Deployment discovery settings could not be saved.");
    } finally {
      setProductCatalogBusy(false);
    }
  }

  async function rewriteDescriptionWithAI() {
    setProductCatalogBusy(true);
    try {
      const value = apiConnected
        ? await api.rewriteProductDescription(product.id, productDescription)
        : { description: "Build reliable voice and messaging experiences with independently versioned APIs, compatible SDKs, API documentation, and policy-authorized tools." };
      setProductDescription(value.description);
      showToast("AI rewrite applied as an editable draft. Save to publish it to agents.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The assistant model could not rewrite the description.");
    } finally {
      setProductCatalogBusy(false);
    }
  }

  async function publishProductVersion() {
    if (!newProductVersion.trim() || !newProductProfile) return;
    setProductCatalogBusy(true);
    try {
      const now = new Date().toISOString();
      const profile = productDefinition?.profiles.find((candidate) => candidate.id === newProductProfile);
      const value = apiConnected
		? await api.createProductVersion(product.id, { version: newProductVersion.trim(), profile_id: newProductProfile, is_latest: newVersionLatest, is_lts: newVersionLTS, release_stage: newVersionStage, rollout_percentage: newVersionRollout })
		: { id: `version_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, version: newProductVersion.trim(), profile_id: newProductProfile, profile_name: profile?.name ?? "Compatibility profile", definition_revision: productDefinition?.revision ?? 1, manifest_hash: `sha256:preview-${Date.now()}`, diff: fixtures!.diff, release_stage: requirePromotionApproval ? "preview" as const : newVersionStage, rollout_percentage: newVersionRollout, promotion_state: requirePromotionApproval ? "pending" as const : "not_required" as const, requested_latest: newVersionLatest, requested_lts: newVersionLTS, drift_status: "healthy" as const, drift_details: [], is_latest: requirePromotionApproval ? false : newVersionLatest || productVersions.length === 0, is_lts: requirePromotionApproval ? false : newVersionLTS, revision: 1, published_at: now };
      if (apiConnected) setProductVersions(await api.productVersions(product.id));
      else setProductVersions((current) => [value, ...current.map((candidate) => value.is_latest ? { ...candidate, is_latest: false } : candidate)]);
      setPinVersionID(value.id);
      setNewProductVersion("");
      setNewVersionLatest(false);
	  setNewVersionLTS(false);
	  setNewVersionStage("active");
	  setNewVersionRollout(100);
      showToast(`Compatibility snapshot ${value.version} published.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The compatibility snapshot could not be published.");
    } finally {
      setProductCatalogBusy(false);
    }
  }

  function editProductVersion(version: APIProductVersion) {
    setEditingProductVersion(version);
    setLifecycleLatest(version.is_latest);
    setLifecycleLTS(version.is_lts);
    setLifecycleDeprecated(Boolean(version.deprecated_at));
    setLifecycleMessage(version.deprecation_message ?? "");
    setLifecycleReplacement(version.replacement_version ?? "");
    setLifecycleSunset(version.sunset_at?.slice(0, 10) ?? "");
	setLifecycleRollout(version.rollout_percentage);
	setLifecycleImpact(null);
	setLifecycleImpactAcknowledged(false);
	if (apiConnected) api.productVersionImpact(product.id, version.id).then(setLifecycleImpact).catch(() => {});
	else setLifecycleImpact({ product_version_id: version.id, product_version: version.version, customer_pins: productVersionPins.filter((pin) => pin.scope === "customer" && pin.product_version_id === version.id).length, environment_pins: productVersionPins.filter((pin) => pin.scope === "environment" && pin.product_version_id === version.id).length, installation_pins: productVersionPins.filter((pin) => pin.scope === "installation" && pin.product_version_id === version.id).length, affected_customers: [], affected_environments: [], affected_installations: [], requests_30_days: 1842, tool_calls_30_days: 327 });
    setVersionLifecycleOpen(true);
  }

  async function saveProductVersionLifecycle() {
    if (!editingProductVersion) return;
    setProductCatalogBusy(true);
    try {
	  const input = { is_latest: lifecycleDeprecated ? false : lifecycleLatest, is_lts: lifecycleDeprecated ? false : lifecycleLTS, deprecated: lifecycleDeprecated, deprecation_message: lifecycleDeprecated ? lifecycleMessage : "", replacement_version: lifecycleDeprecated ? lifecycleReplacement : "", sunset_at: lifecycleDeprecated && lifecycleSunset ? new Date(`${lifecycleSunset}T00:00:00Z`).toISOString() : undefined, rollout_percentage: lifecycleRollout, acknowledge_impact: lifecycleImpactAcknowledged, revision: editingProductVersion.revision };
      const value = apiConnected
        ? await api.updateProductVersion(product.id, editingProductVersion.id, input)
		: { ...editingProductVersion, is_latest: input.is_latest, is_lts: input.is_lts, rollout_percentage: input.rollout_percentage, deprecated_at: input.deprecated ? editingProductVersion.deprecated_at ?? new Date().toISOString() : undefined, deprecation_message: input.deprecation_message || undefined, replacement_version: input.replacement_version || undefined, sunset_at: input.sunset_at, revision: editingProductVersion.revision + 1 };
      if (apiConnected) setProductVersions(await api.productVersions(product.id));
      else setProductVersions((current) => current.map((candidate) => candidate.id === value.id ? value : value.is_latest ? { ...candidate, is_latest: false } : candidate));
      setVersionLifecycleOpen(false);
	      showToast(value.promotion_state === "pending" ? `${value.version} channel promotion is awaiting independent approval.` : `Lifecycle metadata for ${value.version} updated.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Version lifecycle settings could not be saved.");
    } finally {
      setProductCatalogBusy(false);
    }
  }

  async function pinCustomerVersion() {
    if (!pinCustomerID.trim() || !pinVersionID) return;
    setProductCatalogBusy(true);
    try {
      const selected = productVersions.find((version) => version.id === pinVersionID);
      const now = new Date().toISOString();
	  const existing = productVersionPins.find((pin) => pin.scope === pinScope && pin.scope_id === pinCustomerID.trim());
	  const installation = pinScope === "installation" ? productInstallations.find((item) => item.id === pinCustomerID) : undefined;
	  const value = apiConnected
		? await api.saveProductVersionPin(product.id, { scope: pinScope, scope_id: pinCustomerID.trim(), customer_account_id: pinScope === "customer" ? pinCustomerID.trim() : installation?.customer_account_id, product_version_id: pinVersionID, reason: pinReason.trim(), revision: existing?.revision ?? 0 })
		: { id: existing?.id ?? `pin_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, scope: pinScope, scope_id: pinCustomerID.trim(), customer_account_id: pinScope === "customer" ? pinCustomerID.trim() : installation?.customer_account_id ?? "", environment_id: pinScope === "environment" ? pinCustomerID.trim() : installation?.environment_id, installation_id: installation?.id, product_version_id: pinVersionID, product_version: selected?.version ?? "", reason: pinReason.trim(), revision: (existing?.revision ?? 0) + 1, created_at: existing?.created_at ?? now, updated_at: now };
	  setProductVersionPins((current) => [value, ...current.filter((pin) => !(pin.scope === value.scope && pin.scope_id === value.scope_id))]);
      setPinCustomerID("");
      setPinReason("");
	  showToast(`${value.scope} ${value.scope_id} pinned to ${value.product_version}.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The scoped version pin could not be saved.");
    } finally {
      setProductCatalogBusy(false);
    }
  }

  async function saveInstallation() {
	if (!installationName.trim() || !installationExternalID.trim() || !installationCustomerID.trim() || !installationEnvironmentID) return;
	setProductCatalogBusy(true);
	try {
	  const now = new Date().toISOString();
	  const value = apiConnected ? await api.saveProductInstallation(product.id, { customer_account_id: installationCustomerID.trim(), environment_id: installationEnvironmentID, external_id: installationExternalID.trim(), name: installationName.trim(), state: "active", revision: 0 }) : { id: `installation_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, customer_account_id: installationCustomerID.trim(), environment_id: installationEnvironmentID, external_id: installationExternalID.trim(), name: installationName.trim(), state: "active" as const, revision: 1, created_at: now, updated_at: now };
	  setProductInstallations((current) => [value, ...current]);
	  setInstallationName(""); setInstallationExternalID(""); setInstallationCustomerID("");
	  showToast(`${value.name} is now available for installation-scoped version resolution.`);
	} catch (error) { showToast(error instanceof APIError ? error.message : "The installation could not be saved."); }
	finally { setProductCatalogBusy(false); }
  }

  async function reconcileVersion(version: APIProductVersion) {
	setProductCatalogBusy(true);
	try {
	  const value = apiConnected ? await api.reconcileProductVersion(product.id, version.id, version.revision) : { ...version, drift_status: "healthy" as const, drift_details: [], drift_checked_at: new Date().toISOString(), revision: version.revision + 1 };
	  setProductVersions((current) => current.map((item) => item.id === value.id ? value : item));
	  setEditingProductVersion(value);
	  showToast(`Artifact health for ${value.version}: ${value.drift_status}.`);
	} catch (error) { showToast(error instanceof APIError ? error.message : "Artifact health could not be checked."); }
	finally { setProductCatalogBusy(false); }
  }

  async function promoteVersion(version: APIProductVersion, action: "request" | "approve" | "reject") {
	setProductCatalogBusy(true);
	try {
	  const note = action === "approve" ? "Generated diff and artifact health reviewed." : action === "reject" ? "Promotion rejected after review." : "Ready for independent review.";
	  const value = apiConnected ? await api.promoteProductVersion(product.id, version.id, action, note, version.revision) : { ...version, promotion_state: action === "approve" ? "approved" as const : action === "reject" ? "rejected" as const : "pending" as const, release_stage: action === "approve" ? "active" as const : "preview" as const, is_latest: action === "approve" ? version.requested_latest : false, is_lts: action === "approve" ? version.requested_lts : false, revision: version.revision + 1 };
	  setProductVersions((current) => current.map((item) => item.id === value.id ? value : value.is_latest ? { ...item, is_latest: false } : item));
	  setEditingProductVersion(value);
	  showToast(`${value.version} promotion is ${value.promotion_state}.`);
	} catch (error) { showToast(error instanceof APIError ? error.message : "Promotion state could not be changed."); }
	finally { setProductCatalogBusy(false); }
  }

  async function removeProductVersionPin(pin: APIProductVersionPin) {
    setProductCatalogBusy(true);
    try {
      if (apiConnected) await api.deleteProductVersionPin(product.id, pin.id);
      setProductVersionPins((current) => current.filter((candidate) => candidate.id !== pin.id));
	  showToast(`${pin.scope} ${pin.scope_id} will now follow the next resolution level.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The scoped version pin could not be removed.");
    } finally {
      setProductCatalogBusy(false);
    }
  }


  return {
    productCatalogOpen, setProductCatalogOpen,
    productCatalogBusy, setProductCatalogBusy,
    productDescription, setProductDescription,
    defaultVersionPolicy, setDefaultVersionPolicy,
    requirePromotionApproval, setRequirePromotionApproval,
    productVersions, setProductVersions,
    productVersionPins, setProductVersionPins,
    customerAccountLoad, setCustomerAccountLoad,
    customerAccounts: accountDataCurrent ? customerAccountLoad.items : [],
    customerAccountsStatus: accountDataCurrent ? customerAccountLoad.status : "loading" as const,
    customerAccountsHaveMore: accountDataCurrent && customerAccountLoad.hasMore,
    productInstallations, setProductInstallations,
    pinHistory, setPinHistory,
    newProductVersion, setNewProductVersion,
    newProductProfile, setNewProductProfile,
    newVersionLatest, setNewVersionLatest,
    newVersionLTS, setNewVersionLTS,
    newVersionStage, setNewVersionStage,
    newVersionRollout, setNewVersionRollout,
    pinScope, setPinScope,
    pinCustomerID, setPinCustomerID,
    pinVersionID, setPinVersionID,
    pinReason, setPinReason,
    versionLifecycleOpen, setVersionLifecycleOpen,
    editingProductVersion, setEditingProductVersion,
    lifecycleLatest, setLifecycleLatest,
    lifecycleLTS, setLifecycleLTS,
    lifecycleDeprecated, setLifecycleDeprecated,
    lifecycleMessage, setLifecycleMessage,
    lifecycleReplacement, setLifecycleReplacement,
    lifecycleSunset, setLifecycleSunset,
    lifecycleRollout, setLifecycleRollout,
    lifecycleImpact, setLifecycleImpact,
    lifecycleImpactAcknowledged, setLifecycleImpactAcknowledged,
    installationName, setInstallationName,
    installationExternalID, setInstallationExternalID,
    installationCustomerID, setInstallationCustomerID,
    installationEnvironmentID, setInstallationEnvironmentID,
    openProductCatalog,
    saveProductDiscoverySettings,
    rewriteDescriptionWithAI,
    publishProductVersion,
    editProductVersion,
    saveProductVersionLifecycle,
    pinCustomerVersion,
    saveInstallation,
    reconcileVersion,
    promoteVersion,
    removeProductVersionPin,
  };
}

