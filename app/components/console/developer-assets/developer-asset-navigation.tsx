"use client";


import { useTranslation } from "react-i18next";
import type { Section } from "../../../lib/console-routes";
import { sectionPath } from "../../../lib/console-routes";
import { PageTabs } from "../../core/layout";
import { ConsoleLink } from "../console-link";

export function DocumentationNavigation({ active, onNavigate }: { active: Section; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const items: Array<{ label: string; section: Section }> = [
    { label: t("navigation.sources"), section: "sources" },
    { label: t("navigation.documents"), section: "documents" },
    { label: t("navigation.apiContracts"), section: "contracts" },
    { label: t("navigation.queryLab"), section: "query-lab" },
  ];
  return <PageTabs label={t("developerAssets.docsAreas")}>{items.map((item) => <ConsoleLink key={item.section} path={sectionPath(item.section)} onNavigate={onNavigate} className={`page-tab${item.section === "query-lab" ? " docs-query-lab-tab" : ""}${active === item.section ? " active" : ""}`} ariaCurrent={active === item.section ? "page" : undefined}>{item.label}</ConsoleLink>)}</PageTabs>;
}
