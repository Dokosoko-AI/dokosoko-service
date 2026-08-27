"use client";

import * as Headless from "@headlessui/react";
import { Check, Monitor, Moon, Sun } from "lucide-react";
import { useEffect, useRef, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";

type ThemePreference = "light" | "dark" | "system";
type ResolvedTheme = Exclude<ThemePreference, "system">;

const storageKey = "dokosoko-theme";
const systemThemeQuery = "(prefers-color-scheme: dark)";
const longPressDelay = 300;

const themeOptions = [
  { id: "light" as const, labelKey: "theme.light" as const, icon: Sun },
  { id: "dark" as const, labelKey: "theme.dark" as const, icon: Moon },
  { id: "system" as const, labelKey: "theme.system" as const, icon: Monitor },
];

function validThemePreference(value: string | null | undefined): value is ThemePreference {
  return value === "light" || value === "dark" || value === "system";
}

function currentPreference(): ThemePreference {
  const value = document.documentElement.dataset.themePreference;
  return validThemePreference(value) ? value : "system";
}

function serverPreference(): ThemePreference {
  return "system";
}

function resolvedTheme(preference: ThemePreference): ResolvedTheme {
  if (preference !== "system") return preference;
  return window.matchMedia(systemThemeQuery).matches ? "dark" : "light";
}

function applyTheme(preference: ThemePreference, persist: boolean) {
  const theme = resolvedTheme(preference);
  document.documentElement.dataset.themePreference = preference;
  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
  if (persist) localStorage.setItem(storageKey, preference);
}

function cycleTheme() {
  const currentIndex = themeOptions.findIndex((option) => option.id === currentPreference());
  const next = themeOptions[(currentIndex + 1) % themeOptions.length];
  applyTheme(next.id, true);
}

function subscribeToTheme(onChange: () => void) {
  const root = document.documentElement;
  const media = window.matchMedia(systemThemeQuery);
  const observer = new MutationObserver(onChange);
  const handleSystemChange = () => {
    if (currentPreference() === "system") applyTheme("system", false);
    onChange();
  };
  const handleStorage = (event: StorageEvent) => {
    if (event.key !== storageKey) return;
    applyTheme(validThemePreference(event.newValue) ? event.newValue : "system", false);
    onChange();
  };

  observer.observe(root, { attributes: true, attributeFilter: ["data-theme", "data-theme-preference"] });
  media.addEventListener("change", handleSystemChange);
  window.addEventListener("storage", handleStorage);
  return () => {
    observer.disconnect();
    media.removeEventListener("change", handleSystemChange);
    window.removeEventListener("storage", handleStorage);
  };
}

export function ThemeToggle({ mobile = false }: { mobile?: boolean }) {
  const { t } = useTranslation();
  const preference = useSyncExternalStore(subscribeToTheme, currentPreference, serverPreference);
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const longPressTimer = useRef<number | null>(null);
  const longPressTriggered = useRef(false);
  const openingLongPressMenu = useRef(false);
  const selected = themeOptions.find((option) => option.id === preference) ?? themeOptions[2];
  const SelectedIcon = selected.icon;
  const selectedLabel = t(selected.labelKey);

  function clearLongPressTimer() {
    if (longPressTimer.current === null) return;
    window.clearTimeout(longPressTimer.current);
    longPressTimer.current = null;
  }

  function startLongPress(event: React.PointerEvent<HTMLButtonElement>) {
    if (event.pointerType === "mouse" && event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.focus({ preventScroll: true });
    clearLongPressTimer();
    longPressTriggered.current = false;
    longPressTimer.current = window.setTimeout(() => {
      longPressTimer.current = null;
      longPressTriggered.current = true;
      openingLongPressMenu.current = true;
      buttonRef.current?.click();
      openingLongPressMenu.current = false;
    }, longPressDelay);
  }

  function finishLongPress() {
    clearLongPressTimer();
    if (longPressTriggered.current) {
      window.setTimeout(() => { longPressTriggered.current = false; }, 0);
    }
  }

  function handleClick(event: React.MouseEvent<HTMLButtonElement>) {
    if (openingLongPressMenu.current) return;
    event.preventDefault();
    event.stopPropagation();
    if (longPressTriggered.current) {
      longPressTriggered.current = false;
      return;
    }
    cycleTheme();
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLButtonElement>) {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    event.stopPropagation();
    cycleTheme();
  }

  useEffect(() => () => clearLongPressTimer(), []);

  return (
    <Headless.Menu>
      <Headless.MenuButton
        as="button"
        ref={buttonRef}
        type="button"
        className="theme-toggle"
        aria-label={t("theme.select")}
        title={t("theme.controlHint", { theme: selectedLabel })}
        onPointerDownCapture={startLongPress}
        onPointerUpCapture={finishLongPress}
        onPointerCancelCapture={finishLongPress}
        onClickCapture={handleClick}
        onKeyDownCapture={handleKeyDown}
        onContextMenu={(event) => event.preventDefault()}
      >
        <SelectedIcon aria-hidden="true" />
      </Headless.MenuButton>
      <Headless.MenuItems transition anchor={mobile ? "bottom end" : "top end"} className="theme-menu">
        {themeOptions.map((option) => {
          const Icon = option.icon;
          const active = option.id === preference;
          return (
            <Headless.MenuItem
              as="button"
              type="button"
              key={option.id}
              className="theme-menu-item"
              aria-current={active ? "true" : undefined}
              onClick={() => applyTheme(option.id, true)}
            >
              <span className="theme-check" aria-hidden="true">{active && <Check />}</span>
              <Icon aria-hidden="true" />
              <span>{t(option.labelKey)}</span>
            </Headless.MenuItem>
          );
        })}
      </Headless.MenuItems>
    </Headless.Menu>
  );
}
