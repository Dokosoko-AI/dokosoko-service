export const supportedLocales = ["en", "es", "fr", "de", "ja", "uk", "pt-BR"] as const;

export type SupportedLocale = (typeof supportedLocales)[number];

export const defaultLocale: SupportedLocale = "en";
export const localeCookieName = "dokosoko-language";
export const localeStorageKey = localeCookieName;

const localeAliases: Record<string, SupportedLocale> = {
  en: "en",
  es: "es",
  fr: "fr",
  de: "de",
  ja: "ja",
  jp: "ja",
  uk: "uk",
  ua: "uk",
  pt: "pt-BR",
  "pt-br": "pt-BR",
};

export function normalizeLocale(value: string | null | undefined): SupportedLocale | null {
  if (!value) return null;
  const normalized = value.trim().replaceAll("_", "-").toLowerCase();
  return localeAliases[normalized] ?? localeAliases[normalized.split("-")[0]] ?? null;
}

export function localeFromAcceptLanguage(value: string | null | undefined): SupportedLocale {
  if (!value) return defaultLocale;

  const candidates = value
    .split(",")
    .map((entry, index) => {
      const [language, ...parameters] = entry.trim().split(";");
      const qualityParameter = parameters.find((parameter) => parameter.trim().toLowerCase().startsWith("q="));
      const quality = qualityParameter ? Number.parseFloat(qualityParameter.trim().slice(2)) : 1;
      return { language, quality: Number.isFinite(quality) ? quality : 0, index };
    })
    .sort((left, right) => right.quality - left.quality || left.index - right.index);

  for (const candidate of candidates) {
    if (candidate.quality <= 0) continue;
    const locale = normalizeLocale(candidate.language);
    if (locale) return locale;
  }

  return defaultLocale;
}

const localeDirections: Record<SupportedLocale, "ltr" | "rtl"> = {
  en: "ltr", es: "ltr", fr: "ltr", de: "ltr", ja: "ltr", uk: "ltr", "pt-BR": "ltr",
};

export function directionForLocale(locale: SupportedLocale): "ltr" | "rtl" {
  return localeDirections[locale];
}
