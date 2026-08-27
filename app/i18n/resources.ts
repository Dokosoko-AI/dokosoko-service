import { de } from "./locales/de";
import { en } from "./locales/en";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { ja } from "./locales/ja";
import { ptBR } from "./locales/pt-BR";
import { uk } from "./locales/uk";

export const resources = {
  en: { translation: en },
  es: { translation: es },
  fr: { translation: fr },
  de: { translation: de },
  ja: { translation: ja },
  uk: { translation: uk },
  "pt-BR": { translation: ptBR },
} as const;
