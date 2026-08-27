"use client";

import {
  BookOpen,
  GitBranch,
  LayoutDashboard,
  LogOut,
  Package,
  Radio,
  Settings,
  Users,
  Wrench,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import type { APIUser } from "../../lib/api";
import { type Section, sectionPath } from "../../lib/console-routes";
import { LanguageSwitcher } from "../LanguageSwitcher";
import { ThemeToggle } from "../ThemeToggle";
import { ConsoleLink } from "./console-link";

export type NavigationGroup = "apis" | "docs" | "sdk-packages" | "identity" | "tools" | "recipes" | "agent-access" | "outbox";

type NavigationLabelKey =
  | "navigation.apis"
  | "navigation.docs"
  | "navigation.sdksAndPackages"
  | "navigation.identity"
  | "navigation.tools"
  | "navigation.recipes"
  | "navigation.agentAccess"
  | "navigation.supportOutbox"
  | "navigation.sources"
  | "navigation.documents"
  | "navigation.apiContracts"
  | "navigation.queryLab"
  | "navigation.packages"
  | "navigation.customerSignIn"
  | "navigation.catalog"
  | "navigation.connections"
  | "navigation.mcpPreview";

export const navigation: Array<{
  id: NavigationGroup;
  labelKey: NavigationLabelKey;
  icon: typeof LayoutDashboard;
  defaultSection: Section;
  sections: Array<{ id: Section; labelKey: NavigationLabelKey; groupKey?: NavigationLabelKey }>;
  showSubsections?: boolean;
}> = [
  {
    id: "apis",
    labelKey: "navigation.apis",
    icon: GitBranch,
    defaultSection: "product",
    sections: [{ id: "product", labelKey: "navigation.apis" }],
  },
  {
    id: "docs",
    labelKey: "navigation.docs",
    icon: BookOpen,
    defaultSection: "sources",
    sections: [
      { id: "sources", labelKey: "navigation.sources" },
      { id: "documents", labelKey: "navigation.documents" },
      { id: "contracts", labelKey: "navigation.apiContracts" },
      { id: "query-lab", labelKey: "navigation.queryLab" },
    ],
    showSubsections: false,
  },
  {
    id: "sdk-packages",
    labelKey: "navigation.sdksAndPackages",
    icon: Package,
    defaultSection: "sdks",
    sections: [{ id: "sdks", labelKey: "navigation.packages" }],
  },
  { id: "identity", labelKey: "navigation.identity", icon: Users, defaultSection: "identity", sections: [{ id: "identity", labelKey: "navigation.customerSignIn" }] },
  { id: "tools", labelKey: "navigation.tools", icon: Wrench, defaultSection: "tools", sections: [{ id: "tools", labelKey: "navigation.catalog" }, { id: "connections", labelKey: "navigation.connections" }, { id: "mcp-preview", labelKey: "navigation.mcpPreview" }] },
  { id: "recipes", labelKey: "navigation.recipes", icon: BookOpen, defaultSection: "recipes", sections: [{ id: "recipes", labelKey: "navigation.recipes" }] },
  { id: "agent-access", labelKey: "navigation.agentAccess", icon: Radio, defaultSection: "distribution", sections: [{ id: "distribution", labelKey: "navigation.agentAccess" }] },
  { id: "outbox", labelKey: "navigation.supportOutbox", icon: LayoutDashboard, defaultSection: "reporting", sections: [{ id: "reporting", labelKey: "navigation.supportOutbox" }] },
];

export function ConsoleSidebar({
  section,
  activeNavigationID,
  currentUser,
  onLogout,
  onNavigate,
}: {
  section: Section;
  activeNavigationID?: NavigationGroup;
  currentUser?: APIUser | null;
  onLogout?: () => void | Promise<void>;
  onNavigate: (path: string) => void;
}) {
  const { t } = useTranslation();
  const displayName = currentUser?.display_name ?? t("account.defaultName");
  const initials = (currentUser?.display_name ?? t("account.defaultAdminName"))
    .split(/\s+/)
    .map((part) => part[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();

  return (
    <aside className="sidebar">
      <div className="brand">
        <span className="brand-mark" aria-hidden="true">D</span>
        <span className="brand-copy"><strong>DokoSoko</strong></span>
      </div>
      <nav aria-label={t("navigation.main")}>
        {navigation.map((item) => {
          const Icon = item.icon;
          const active = activeNavigationID === item.id;
          return (
            <div className="nav-group" key={item.id}>
              <ConsoleLink
                path={sectionPath(item.defaultSection)}
                onNavigate={onNavigate}
                className={`nav-item ${active ? "active" : ""}`}
                ariaCurrent={active && item.sections.some((candidate) => candidate.id === section && candidate.id === item.defaultSection) ? "page" : undefined}
              >
                <Icon />
                <span>{t(item.labelKey)}</span>
              </ConsoleLink>
              {active && item.showSubsections !== false && item.sections.length > 1 && <div className="nav-subsections" aria-label={t("navigation.sectionLabel", { name: t(item.labelKey) })}>
                {item.sections.map((candidate, index) => <span className="nav-subsection-entry" key={candidate.id}>
                  {candidate.groupKey && candidate.groupKey !== item.sections[index - 1]?.groupKey && <span className="nav-subsection-label">{t(candidate.groupKey)}</span>}
                  <ConsoleLink path={sectionPath(candidate.id)} onNavigate={onNavigate} className={`nav-subsection ${section === candidate.id ? "active" : ""}`} ariaCurrent={section === candidate.id ? "page" : undefined}>{t(candidate.labelKey)}</ConsoleLink>
                </span>)}
              </div>}
            </div>
          );
        })}
      </nav>
      <div className="sidebar-bottom">
        <div className="preference-controls">
          <LanguageSwitcher />
          <span className="preference-divider" aria-hidden="true" />
          <ThemeToggle />
        </div>
        <ConsoleLink
          path={sectionPath("settings")}
          onNavigate={onNavigate}
          className={`nav-item ${section === "settings" ? "active" : ""}`}
          ariaCurrent={section === "settings" ? "page" : undefined}
        >
          <Settings />
          <span>{t("navigation.settings")}</span>
        </ConsoleLink>
        <div className="account">
          <span className="avatar">{initials}</span>
          <span>
            <strong>{displayName}</strong>
            <small>{currentUser ? t("account.rootAdministrator") : t("account.platformAdmin")}</small>
          </span>
          {onLogout && (
            <button type="button" className="logout-button" aria-label={t("account.signOut")} title={t("account.signOut")} onClick={onLogout}>
              <LogOut />
            </button>
          )}
        </div>
      </div>
    </aside>
  );
}

export function ConsoleTopbar({
  productName,
  section,
  activeNavigationID,
  onGroupChange,
}: {
  productName: string;
  section: Section;
  activeNavigationID?: NavigationGroup;
  onGroupChange: (group: NavigationGroup | "settings") => void;
}) {
  const { t } = useTranslation();
  return (
    <header className="topbar">
      <div className="topbar-inner">
        <div className="product-switcher">
          <span className="product-logo">{productName.slice(0, 1).toUpperCase()}</span>
          <span><small>{t("navigation.deployment")}</small><strong>{productName}</strong></span>
        </div>
        <select
          className="mobile-navigation"
          aria-label={t("navigation.consoleSection")}
          value={section === "settings" ? "settings" : activeNavigationID ?? "apis"}
          onChange={(event) => onGroupChange(event.target.value as NavigationGroup | "settings")}
        >
          {navigation.map((item) => <option key={item.id} value={item.id}>{t(item.labelKey)}</option>)}
          <option value="settings">{t("navigation.settings")}</option>
        </select>
        <div className="topbar-actions">
          <div className="mobile-preference-controls"><LanguageSwitcher mobile /><span className="preference-divider" aria-hidden="true" /><ThemeToggle mobile /></div>
        </div>
      </div>
    </header>
  );
}
