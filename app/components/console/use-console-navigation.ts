"use client";

import { useTranslation } from "react-i18next";
import { startTransition, useCallback, useEffect, useRef, useState } from "react";

import {
  type ConsoleRoute,
  type Section,
  type SettingsTab,
  parseConsolePath,
  routeForSection,
  sectionPath,
} from "../../lib/console-routes";
import { type NavigationGroup, navigation } from "./workspace-navigation";

function browserRouteURL(path: string) {
  const preview =
    process.env.NODE_ENV === "development" &&
    new URLSearchParams(window.location.search).get("preview") === "fixtures"
      ? window.location.search
      : "";
  return `${path}${preview}`;
}

export function useConsoleNavigation({
  onLeaveToolBuilder,
}: {
  onLeaveToolBuilder: () => void;
}) {
  const { t } = useTranslation();
  const [consoleRoute, setConsoleRoute] = useState<ConsoleRoute>(() => routeForSection("product"));
  const consoleRouteRef = useRef(consoleRoute);
  const toolBuilderDirtyRef = useRef(false);

  const onToolBuilderDirtyChange = useCallback((dirty: boolean) => {
    toolBuilderDirtyRef.current = dirty;
  }, []);

  const confirmToolBuilderNavigation = useCallback((nextPath: string) => {
    const current = consoleRouteRef.current;
    if (!toolBuilderDirtyRef.current || current.kind !== "tool-builder" || current.path === nextPath) {
      return true;
    }
    return window.confirm(t("console.discardUnsavedToolChanges"));
  }, [t]);

  const navigateToPath = useCallback((path: string, replace = false) => {
    const next = parseConsolePath(path);
    const current = consoleRouteRef.current;
    if (!confirmToolBuilderNavigation(next.path)) return;
    if (current.path !== next.path) toolBuilderDirtyRef.current = false;
    if (next.kind !== "tool-builder") onLeaveToolBuilder();
    const method = replace ? "replaceState" : "pushState";
    if (window.location.pathname !== next.path || replace) {
      window.history[method](null, "", browserRouteURL(next.path));
    }
    window.scrollTo({ top: 0, behavior: "auto" });
    consoleRouteRef.current = next;
    startTransition(() => setConsoleRoute(next));
    requestAnimationFrame(() => document.getElementById("main-content")?.focus());
  }, [confirmToolBuilderNavigation, onLeaveToolBuilder]);

  const navigateToSection = useCallback((destination: Section) => {
    navigateToPath(sectionPath(destination));
  }, [navigateToPath]);

  const navigateToGroup = useCallback((group: NavigationGroup | "settings") => {
    if (group === "settings") {
      navigateToSection("settings");
      return;
    }
    const destination = navigation.find((item) => item.id === group);
    if (destination) navigateToSection(destination.defaultSection);
  }, [navigateToSection]);

  useEffect(() => {
    consoleRouteRef.current = consoleRoute;
  }, [consoleRoute]);

  useEffect(() => {
    const syncRoute = () => {
      const current = consoleRouteRef.current;
      const next = parseConsolePath(window.location.pathname);
      if (!confirmToolBuilderNavigation(next.path)) {
        window.history.pushState(null, "", browserRouteURL(current.path));
        return;
      }
      if (current.path !== next.path) toolBuilderDirtyRef.current = false;
      if (next.kind !== "not-found" && window.location.pathname !== next.path) {
        window.history.replaceState(null, "", `${next.path}${window.location.search}${window.location.hash}`);
      }
      if (next.kind !== "tool-builder") onLeaveToolBuilder();
      consoleRouteRef.current = next;
      startTransition(() => setConsoleRoute(next));
    };

    syncRoute();
    window.addEventListener("popstate", syncRoute);
    return () => window.removeEventListener("popstate", syncRoute);
  }, [confirmToolBuilderNavigation, onLeaveToolBuilder]);

  const section = consoleRoute.section;
  const settingsTab: SettingsTab =
    consoleRoute.kind === "section" && consoleRoute.section === "settings"
      ? consoleRoute.settingsTab ?? "overview"
      : "overview";

  return {
    consoleRoute,
    section,
    settingsTab,
    navigateToPath,
    navigateToSection,
    navigateToGroup,
    onToolBuilderDirtyChange,
  };
}
