import type { InitOptions } from "i18next";

import { resources } from "./resources";
import { defaultLocale, supportedLocales, type SupportedLocale } from "./settings";

export function i18nOptions(locale: SupportedLocale): InitOptions {
  return {
    resources,
    lng: locale,
    fallbackLng: defaultLocale,
    supportedLngs: [...supportedLocales],
    load: "currentOnly",
    defaultNS: "translation",
    ns: ["translation"],
    interpolation: { escapeValue: false },
    returnNull: false,
    initAsync: false,
  };
}
