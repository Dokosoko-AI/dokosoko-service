"use client";

import { lazy, Suspense, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { APIIntegration } from "../../../lib/api";
import { developerAssetsApi, type SDKPackage } from "../../../lib/developer-assets-api";
import { parseSDKSetupSelection, sdkSetupPath } from "../../../lib/sdk-setup";
import { Badge, Button } from "../../core/control";
import { PageHeader } from "../../core/layout";
import { ConsoleLink } from "../console-link";
import { developerAssetError, LoadingPanel, ProblemPanel } from "./developer-asset-ui";
import { KnowledgeNavigation } from "./developer-asset-navigation";
import { SDKPackageImportDialog } from "./sdk-package-import-dialog";
import { SDKSetupWorkspace } from "./sdk-setup-workspace";

const SDKAdvancedCatalogView = lazy(() => import("./sdk-advanced-catalog-view").then((value) => ({ default: value.SDKAdvancedCatalogView })));
type Props = { reviewerID?: string; setupSearch?: string; live: boolean; integrations: APIIntegration[]; onMessage: (message: string) => void; onNavigate: (path: string) => void };
export function SDKCatalogView(props: Props) {
  const { t } = useTranslation();
  const selection = useMemo(() => parseSDKSetupSelection(props.setupSearch), [props.setupSearch]);
  if (new URLSearchParams(props.setupSearch).get("advanced") === "1") return <><ConsoleLink path={sdkSetupPath(selection)} onNavigate={props.onNavigate}>{t("sdkSetup.backToGuidance")}</ConsoleLink><Suspense fallback={<LoadingPanel label={t("sdkSetup.loading")} />}><SDKAdvancedCatalogView {...props} /></Suspense></>;
  if (selection.package && props.live) return <SDKSetupWorkspace key={`${selection.package}:${selection.release}:${selection.api}`} selection={selection} reviewerID={props.reviewerID ?? ""} integrations={props.integrations} onNavigate={props.onNavigate} onMessage={props.onMessage} />;
  return <SDKPackageDirectory {...props} />;
}
function SDKPackageDirectory({ live, onNavigate, onMessage }: Props) {
  const { t } = useTranslation();
  const [packages, setPackages] = useState<SDKPackage[]>([]);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(live);
  const [problem, setProblem] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [importOpen, setImportOpen] = useState(false);
  useEffect(() => {
    if (!live) return;
    let cancelled = false;
    queueMicrotask(() => { if (!cancelled) { setLoading(true); setProblem(""); } });
    developerAssetsApi.sdkPackages().then((values) => { if (!cancelled) setPackages(values); }).catch((error) => { if (!cancelled) setProblem(developerAssetError(error, t("sdkCatalog.sdkPackagesCouldNotBeLoaded"))); }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [attempt, live, t]);
  const visible = packages.filter((pkg) => `${pkg.name} ${pkg.display_coordinate} ${pkg.ecosystem}`.toLowerCase().includes(query.toLowerCase()));
  return <><PageHeader eyebrow={t("navigation.knowledge")} title={t("sdkSetup.packageGuidance")} description={t("sdkSetup.catalogHelp")} action={<Button onClick={() => setImportOpen(true)}>{t("sdkImport.importPackage")}</Button>} />
    <KnowledgeNavigation active="sdks" onNavigate={onNavigate} />
    {loading ? <LoadingPanel label={t("sdkCatalog.loadingSDKPackages")} /> : problem ? <ProblemPanel message={problem} onRetry={() => setAttempt((value) => value + 1)} /> : <section className="panel sdk-setup-panel">
      <label className="auth-field"><span>{t("sdkSetup.findPackage")}</span><input value={query} onChange={(event) => setQuery(event.target.value)} /></label>
      {visible.map((pkg) => <div className="sdk-package-row" key={pkg.id}><span><strong>{pkg.name}</strong><small>{pkg.display_coordinate} · {pkg.ecosystem}</small></span><Badge>{pkg.visibility}</Badge><Badge>{pkg.lifecycle}</Badge><ConsoleLink path={sdkSetupPath({ package: pkg.id })} onNavigate={onNavigate}>{t("sdkSetup.openPackage")}</ConsoleLink></div>)}
      {visible.length === 0 && <p>{t(packages.length ? "sdkSetup.noMatchingPackages" : "sdkSetup.emptyCatalog")}</p>}
    </section>}
    <SDKPackageImportDialog open={importOpen} onClose={setImportOpen} onMessage={onMessage} onImported={(result) => onNavigate(sdkSetupPath({ package: result.package.id, release: result.release.id }))} />
  </>;
}
