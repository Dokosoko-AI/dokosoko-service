"use client";

import {
  Activity,
  BookOpen,
  LayoutDashboard,
  LogOut,
  Radio,
  Settings,
  Sparkles,
  Users,
  Wrench,
} from "lucide-react";

import type { APIUser } from "../../lib/api";
import { type Section, sectionPath } from "../../lib/console-routes";
import { ThemeToggle } from "../ThemeToggle";
import { ConsoleLink } from "./console-link";

export type NavigationGroup = "apis" | "identity" | "tools" | "recipes" | "agent-access" | "activity";

export const navigation: Array<{
  id: NavigationGroup;
  label: string;
  icon: typeof LayoutDashboard;
  defaultSection: Section;
  sections: Array<{ id: Section; label: string }>;
}> = [
  { id: "apis", label: "APIs", icon: Sparkles, defaultSection: "product", sections: [{ id: "product", label: "APIs" }] },
  { id: "identity", label: "Identity", icon: Users, defaultSection: "identity", sections: [{ id: "identity", label: "Customer sign-in" }] },
  { id: "tools", label: "Tools", icon: Wrench, defaultSection: "tools", sections: [{ id: "tools", label: "Catalog" }, { id: "connections", label: "Connections" }] },
  { id: "recipes", label: "Recipes", icon: BookOpen, defaultSection: "recipes", sections: [{ id: "recipes", label: "Recipes" }] },
  { id: "agent-access", label: "Agent access", icon: Radio, defaultSection: "distribution", sections: [{ id: "distribution", label: "Agent access" }, { id: "widgets", label: "Widgets" }] },
  { id: "activity", label: "Activity", icon: Activity, defaultSection: "runs", sections: [{ id: "runs", label: "Activity" }] },
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
            <ConsoleLink
              key={item.id}
              path={sectionPath(item.defaultSection)}
              onNavigate={onNavigate}
              className={`nav-item ${active ? "active" : ""}`}
              ariaCurrent={active ? "page" : undefined}
            >
              <Icon />
              <span>{item.label}</span>
            </ConsoleLink>
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
          value={section === "settings" ? "settings" : activeNavigationID ?? "apis"}
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
