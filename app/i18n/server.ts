import { createInstance } from "i18next";

import { i18nOptions } from "./options";
import type { SupportedLocale } from "./settings";

export async function getServerTranslator(locale: SupportedLocale) {
  const instance = createInstance();
  await instance.init(i18nOptions(locale));
  return instance.getFixedT(locale);
}
