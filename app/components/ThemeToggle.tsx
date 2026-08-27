"use client";

import { useSyncExternalStore } from "react";
import { Moon, Sun } from "lucide-react";
import { useTranslation } from "react-i18next";

type Theme = "light" | "dark";

const storageKey = "dokosoko-theme";

function currentTheme(): Theme {
  return document.documentElement.dataset.theme === "dark" ? "dark" : "light";
}

function serverTheme(): Theme {
  return "light";
}

function subscribeToTheme(onChange: () => void) {
  const observer = new MutationObserver(onChange);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  window.addEventListener("storage", onChange);
  return () => {
    observer.disconnect();
    window.removeEventListener("storage", onChange);
  };
}

export function ThemeToggle() {
  const { t } = useTranslation();
  const theme = useSyncExternalStore(subscribeToTheme, currentTheme, serverTheme);

  function toggleTheme() {
    const next: Theme = currentTheme() === "dark" ? "light" : "dark";
    document.documentElement.dataset.theme = next;
    document.documentElement.style.colorScheme = next;
    localStorage.setItem(storageKey, next);
  }

  const dark = theme === "dark";

  return (
    <button
      type="button"
      className="theme-toggle"
      role="switch"
      aria-checked={dark}
      aria-label={t("theme.darkMode")}
      title={dark ? t("theme.switchToLight") : t("theme.switchToDark")}
      onClick={toggleTheme}
    >
      {dark ? <Moon aria-hidden="true" /> : <Sun aria-hidden="true" />}
    </button>
  );
}
