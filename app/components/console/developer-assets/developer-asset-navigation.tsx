"use client";

import type { Section } from "../../../lib/console-routes";
import { sectionPath } from "../../../lib/console-routes";
import { PageTabs } from "../../core/layout";
import { ConsoleLink } from "../console-link";

const documentationSections = new Set<Section>(["sources", "documents", "collections", "contracts"]);

export function CatalogNavigation({ active, onNavigate }: { active: Section; onNavigate: (path: string) => void }) {
  const items: Array<{ label: string; section: Section; active: boolean }> = [
    { label: "APIs", section: "product", active: active === "product" },
    { label: "Documentation", section: "sources", active: documentationSections.has(active) },
    { label: "SDKs", section: "sdks", active: active === "sdks" },
    { label: "Query Lab", section: "query-lab", active: active === "query-lab" },
  ];
  return <PageTabs label="Catalog areas">{items.map((item) => <ConsoleLink key={item.section} path={sectionPath(item.section)} onNavigate={onNavigate} className={`page-tab ${item.active ? "active" : ""}`} ariaCurrent={item.active ? "page" : undefined}>{item.label}</ConsoleLink>)}</PageTabs>;
}

export function DocumentationNavigation({ active, onNavigate }: { active: Section; onNavigate: (path: string) => void }) {
  const items: Array<{ label: string; section: Section }> = [
    { label: "Sources", section: "sources" },
    { label: "All files", section: "documents" },
    { label: "Collections", section: "collections" },
    { label: "API contracts", section: "contracts" },
  ];
  return <PageTabs label="Documentation areas">{items.map((item) => <ConsoleLink key={item.section} path={sectionPath(item.section)} onNavigate={onNavigate} className={`page-tab ${active === item.section ? "active" : ""}`} ariaCurrent={active === item.section ? "page" : undefined}>{item.label}</ConsoleLink>)}</PageTabs>;
}
