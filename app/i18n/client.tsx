"use client";

import { createInstance, type i18n } from "i18next";
import { I18nextProvider, initReactI18next } from "react-i18next";
import { useEffect, useState, type ReactNode } from "react";

import { i18nOptions } from "./options";
import { directionForLocale, localeCookieName, localeStorageKey, normalizeLocale, type SupportedLocale } from "./settings";

function createClientInstance(locale: SupportedLocale): i18n {
  const instance = createInstance();
  void instance.use(initReactI18next).init(i18nOptions(locale));
  return instance;
}

export function persistLocale(locale: SupportedLocale) {
  document.cookie = `${localeCookieName}=${encodeURIComponent(locale)}; Max-Age=31536000; Path=/; SameSite=Lax`;
  localStorage.setItem(localeStorageKey, locale);
  document.documentElement.lang = locale;
  document.documentElement.dir = directionForLocale(locale);
}

export function I18nProvider({ locale, children }: { locale: SupportedLocale; children: ReactNode }) {
  const [instance] = useState(() => createClientInstance(locale));

  useEffect(() => {
    const storedLocale = normalizeLocale(localStorage.getItem(localeStorageKey));
    if (storedLocale && storedLocale !== instance.resolvedLanguage) {
      void instance.changeLanguage(storedLocale);
      persistLocale(storedLocale);
      return;
    }
    persistLocale(locale);
  }, [instance, locale]);

  return <I18nextProvider i18n={instance}>{children}</I18nextProvider>;
}
