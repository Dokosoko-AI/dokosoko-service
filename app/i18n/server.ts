import { createInstance } from "i18next";
import { cookies, headers } from "next/headers";

import { i18nOptions } from "./options";
import { localeCookieName, localeFromAcceptLanguage, normalizeLocale, type SupportedLocale } from "./settings";

export async function getRequestLocale(): Promise<SupportedLocale> {
  const cookieLocale = normalizeLocale((await cookies()).get(localeCookieName)?.value);
  if (cookieLocale) return cookieLocale;
  return localeFromAcceptLanguage((await headers()).get("accept-language"));
}

export async function getServerTranslator(locale: SupportedLocale) {
  const instance = createInstance();
  await instance.init(i18nOptions(locale));
  return instance.getFixedT(locale);
}
