"use client";

import {
  BookOpen,
  LayoutDashboard,
  LibraryBig,
  LogOut,
  Radio,
  Settings,
  Users,
  Wrench,
} from "lucide-react";

import type { APIUser } from "../../lib/api";
import { type Section, sectionPath } from "../../lib/console-routes";
import { ThemeToggle } from "../ThemeToggle";
import { ConsoleLink } from "./console-link";

export type NavigationGroup = "catalog" | "identity" | "tools" | "recipes" | "agent-access" | "outbox";

export const navigation: Array<{
  id: NavigationGroup;
  label: string;
  icon: typeof LayoutDashboard;
  defaultSection: Section;
  sections: Array<{ id: Section; label: string; group?: string }>;
}> = [
  {
    id: "catalog",
    label: "Catalog",
    icon: LibraryBig,
    defaultSection: "product",
    sections: [
      { id: "product", label: "APIs" },
      { id: "sources", label: "Sources", group: "Documentation" },
      { id: "documents", label: "All files", group: "Documentation" },
      { id: "collections", label: "Collections", group: "Documentation" },
      { id: "contracts", label: "API contracts", group: "Documentation" },
      { id: "sdks", label: "SDKs" },
      { id: "query-lab", label: "Query Lab" },
    ],
  },
  { id: "identity", label: "Identity", icon: Users, defaultSection: "identity", sections: [{ id: "identity", label: "Customer sign-in" }] },
  { id: "tools", label: "Tools", icon: Wrench, defaultSection: "tools", sections: [{ id: "tools", label: "Catalog" }, { id: "connections", label: "Connections" }, { id: "mcp-preview", label: "MCP preview" }] },
  { id: "recipes", label: "Recipes", icon: BookOpen, defaultSection: "recipes", sections: [{ id: "recipes", label: "Recipes" }] },
  { id: "agent-access", label: "Agent access", icon: Radio, defaultSection: "distribution", sections: [{ id: "distribution", label: "Agent access" }] },
  { id: "outbox", label: "Support outbox", icon: LayoutDashboard, defaultSection: "reporting", sections: [{ id: "reporting", label: "Support outbox" }] },
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
  const displayName = currentUser?.display_name ?? "Yuriy";
  const initials = (currentUser?.display_name ?? "Yuriy Admin")
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
      <nav aria-label="Main navigation">
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
                <span>{item.label}</span>
              </ConsoleLink>
              {active && item.sections.length > 1 && <div className="nav-subsections" aria-label={`${item.label} sections`}>
                {item.sections.map((candidate, index) => <span className="nav-subsection-entry" key={candidate.id}>
                  {candidate.group && candidate.group !== item.sections[index - 1]?.group && <span className="nav-subsection-label">{candidate.group}</span>}
                  <ConsoleLink path={sectionPath(candidate.id)} onNavigate={onNavigate} className={`nav-subsection ${section === candidate.id ? "active" : ""}`} ariaCurrent={section === candidate.id ? "page" : undefined}>{candidate.label}</ConsoleLink>
                </span>)}
              </div>}
            </div>
          );
        })}
      </nav>
      <div className="sidebar-bottom">
        <ThemeToggle />
        <ConsoleLink
          path={sectionPath("settings")}
          onNavigate={onNavigate}
          className={`nav-item ${section === "settings" ? "active" : ""}`}
          ariaCurrent={section === "settings" ? "page" : undefined}
        >
          <Settings />
          <span>Settings</span>
        </ConsoleLink>
        <div className="account">
          <span className="avatar">{initials}</span>
          <span>
            <strong>{displayName}</strong>
            <small>{currentUser ? "Root administrator" : "Platform admin"}</small>
          </span>
          {onLogout && (
            <button type="button" className="logout-button" aria-label="Sign out" title="Sign out" onClick={onLogout}>
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
  return (
    <header className="topbar">
      <div className="topbar-inner">
        <div className="product-switcher">
          <span className="product-logo">{productName.slice(0, 1).toUpperCase()}</span>
          <span><small>Deployment</small><strong>{productName}</strong></span>
        </div>
        <select
          className="mobile-navigation"
          aria-label="Console section"
          value={section === "settings" ? "settings" : activeNavigationID ?? "catalog"}
          onChange={(event) => onGroupChange(event.target.value as NavigationGroup | "settings")}
        >
          {navigation.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
          <option value="settings">Settings</option>
        </select>
        <div className="topbar-actions">
          <div className="mobile-theme-toggle"><ThemeToggle /></div>
        </div>
      </div>
    </header>
  );
}
