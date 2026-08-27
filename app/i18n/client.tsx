"use client";

import { createInstance, type i18n } from "i18next";
import { I18nextProvider, initReactI18next } from "react-i18next";
import { useEffect, useState, type ReactNode } from "react";

import { i18nOptions } from "./options";
import { directionForLocale, localeCookieName, localeFromAcceptLanguage, localeStorageKey, normalizeLocale, type SupportedLocale } from "./settings";

function createClientInstance(locale: SupportedLocale): i18n {
  const instance = createInstance();
  void instance.use(initReactI18next).init(i18nOptions(locale));
  return instance;
}

export function persistLocale(locale: SupportedLocale) {
  document.cookie = `${localeCookieName}=${encodeURIComponent(locale)}; Max-Age=31536000; Path=/; SameSite=Lax`;
  try {
    localStorage.setItem(localeStorageKey, locale);
  } catch {
    // Storage can be unavailable in hardened browser contexts. The cookie and
    // document attributes still preserve the active locale for this page.
  }
  document.documentElement.lang = locale;
  document.documentElement.dir = directionForLocale(locale);
}

function preferredClientLocale(): SupportedLocale {
  try {
    const storedLocale = normalizeLocale(localStorage.getItem(localeStorageKey));
    if (storedLocale) return storedLocale;
  } catch {
    // Continue with cookie and browser-language negotiation.
  }

  const cookie = document.cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(`${localeCookieName}=`));
  if (cookie) {
    try {
      const cookieLocale = normalizeLocale(decodeURIComponent(cookie.slice(localeCookieName.length + 1)));
      if (cookieLocale) return cookieLocale;
    } catch {
      // Ignore malformed client-owned cookies.
    }
  }

  return localeFromAcceptLanguage(navigator.languages?.join(",") || navigator.language);
}

export function I18nProvider({ locale, children }: { locale: SupportedLocale; children: ReactNode }) {
  const [instance] = useState(() => createClientInstance(locale));

  useEffect(() => {
    const preferredLocale = preferredClientLocale();
    if (preferredLocale !== instance.resolvedLanguage) {
      void instance.changeLanguage(preferredLocale);
    }
    persistLocale(preferredLocale);
  }, [instance, locale]);

  return <I18nextProvider i18n={instance}>{children}</I18nextProvider>;
}
