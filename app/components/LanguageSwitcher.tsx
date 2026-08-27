"use client";

import * as Headless from "@headlessui/react";
import { Check, ChevronDown } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useEffect } from "react";

import { persistLocale } from "../i18n/client";
import { defaultLocale, normalizeLocale, supportedLocales, type SupportedLocale } from "../i18n/settings";

const languageKeys = {
  en: "languages.en",
  es: "languages.es",
  fr: "languages.fr",
  de: "languages.de",
  ja: "languages.ja",
  uk: "languages.uk",
  "pt-BR": "languages.ptBR",
} as const;

const languageCodes: Record<SupportedLocale, string> = {
  en: "EN",
  es: "ES",
  fr: "FR",
  de: "DE",
  ja: "JA",
  uk: "UK",
  "pt-BR": "PT",
};

const languageFlags: Record<SupportedLocale, string> = {
  en: "🇬🇧",
  es: "🇪🇸",
  fr: "🇫🇷",
  de: "🇩🇪",
  ja: "🇯🇵",
  uk: "🇺🇦",
  "pt-BR": "🇧🇷",
};

export function LanguageSwitcher({ mobile = false }: { mobile?: boolean }) {
  const { t, i18n } = useTranslation();
  const locale = normalizeLocale(i18n.resolvedLanguage) ?? defaultLocale;

  useEffect(() => {
    document.title = t("metadata.title");
  }, [i18n.resolvedLanguage, t]);

  async function selectLocale(nextLocale: SupportedLocale) {
    if (nextLocale === locale) return;
    await i18n.changeLanguage(nextLocale);
    persistLocale(nextLocale);
  }

  return (
    <Headless.Menu>
      <Headless.MenuButton
        as="button"
        className="language-toggle"
        aria-label={t("languages.select")}
        title={`${t("languages.label")}: ${t(languageKeys[locale])}`}
      >
        <span className="language-flag" aria-hidden="true">{languageFlags[locale]}</span>
        <span className="language-name">{t(languageKeys[locale])}</span>
        <ChevronDown className="language-chevron" aria-hidden="true" />
      </Headless.MenuButton>
      <Headless.MenuItems transition anchor={mobile ? "bottom end" : "top start"} className="language-menu">
        {supportedLocales.map((candidate) => (
          <Headless.MenuItem
            as="button"
            type="button"
            key={candidate}
            className="language-menu-item"
            onClick={() => void selectLocale(candidate)}
            aria-current={candidate === locale ? "true" : undefined}
          >
            <span className="language-check" aria-hidden="true">{candidate === locale && <Check />}</span>
            <span className="language-flag" aria-hidden="true">{languageFlags[candidate]}</span>
            <span lang={candidate}>{t(languageKeys[candidate])}</span>
            <span className="language-menu-code" aria-hidden="true">{languageCodes[candidate]}</span>
          </Headless.MenuItem>
        ))}
      </Headless.MenuItems>
    </Headless.Menu>
  );
}
