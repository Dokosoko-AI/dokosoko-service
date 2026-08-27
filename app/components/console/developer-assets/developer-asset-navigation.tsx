"use client";

import type { Section } from "../../../lib/console-routes";
import { sectionPath } from "../../../lib/console-routes";
import { PageTabs } from "../../core/layout";
import { ConsoleLink } from "../console-link";

export function DocumentationNavigation({ active, onNavigate }: { active: Section; onNavigate: (path: string) => void }) {
  const items: Array<{ label: string; section: Section }> = [
    { label: "Sources", section: "sources" },
    { label: "All files", section: "documents" },
    { label: "Collections", section: "collections" },
    { label: "API contracts", section: "contracts" },
    { label: "Query Lab", section: "query-lab" },
  ];
  return <PageTabs label="Docs areas">{items.map((item) => <ConsoleLink key={item.section} path={sectionPath(item.section)} onNavigate={onNavigate} className={`page-tab ${active === item.section ? "active" : ""}`} ariaCurrent={active === item.section ? "page" : undefined}>{item.label}</ConsoleLink>)}</PageTabs>;
}
