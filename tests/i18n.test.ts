import assert from "node:assert/strict";
import test from "node:test";

import { createInstance } from "i18next";

import { i18nOptions } from "../app/i18n/options";
import { de } from "../app/i18n/locales/de";
import { en } from "../app/i18n/locales/en";
import { es } from "../app/i18n/locales/es";
import { fr } from "../app/i18n/locales/fr";
import { ja } from "../app/i18n/locales/ja";
import { ptBR } from "../app/i18n/locales/pt-BR";
import { uk } from "../app/i18n/locales/uk";
import {
  defaultLocale,
  directionForLocale,
  localeFromAcceptLanguage,
  normalizeLocale,
  supportedLocales,
  type SupportedLocale,
} from "../app/i18n/settings";

type FlatResource = Record<string, string>;

function flattenResource(value: unknown, prefix = "", result: FlatResource = {}): FlatResource {
  assert.ok(value && typeof value === "object");
  for (const [key, child] of Object.entries(value)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (typeof child === "string") result[path] = child;
    else flattenResource(child, path, result);
  }
  return result;
}

function placeholders(value: string) {
  return [...value.matchAll(/\{\{[^{}]+\}\}/g)].map((match) => match[0]).sort();
}

const localeResources = { en, es, fr, de, ja, uk, "pt-BR": ptBR } as const;

async function translator(locale: SupportedLocale) {
  const instance = createInstance();
  await instance.init(i18nOptions(locale));
  return instance;
}

test("supported locales normalize aliases and negotiate Accept-Language by quality", () => {
  assert.deepEqual(supportedLocales, ["en", "es", "fr", "de", "ja", "uk", "pt-BR"]);
  assert.equal(defaultLocale, "en");
  assert.equal(normalizeLocale("pt_BR"), "pt-BR");
  assert.equal(normalizeLocale("pt-PT"), "pt-BR");
  assert.equal(normalizeLocale("fr-CA"), "fr");
  assert.equal(normalizeLocale("JA-jp"), "ja");
  assert.equal(normalizeLocale("ua"), "uk");
  assert.equal(normalizeLocale("not-a-locale"), null);
  assert.equal(localeFromAcceptLanguage("de;q=0.7, fr-CA;q=0.9, en;q=0.8"), "fr");
  assert.equal(localeFromAcceptLanguage("es;q=0, uk;q=0.6"), "uk");
  assert.equal(localeFromAcceptLanguage("zh-Hant, en;q=0.5"), "en");
  assert.equal(localeFromAcceptLanguage(null), "en");
  for (const locale of supportedLocales) assert.equal(directionForLocale(locale), "ltr");
});

test("every locale contains every English key with identical interpolation placeholders", () => {
  const source = flattenResource(en);
  for (const [locale, resource] of Object.entries(localeResources)) {
    const target = flattenResource(resource);
    const missing = Object.keys(source).filter((key) => !(key in target));
    assert.deepEqual(missing, [], `${locale} is missing translation keys`);

    for (const [key, sourceValue] of Object.entries(source)) {
      assert.deepEqual(
        placeholders(target[key]),
        placeholders(sourceValue),
        `${locale}:${key} changed its interpolation contract`,
      );
    }

    const extra = Object.keys(target).filter((key) => !(key in source));
    if (locale === "uk") {
      assert.ok(extra.length > 0, "Ukrainian must define few and many plural forms");
      assert.ok(extra.every((key) => /_(few|many)$/.test(key)), `Unexpected Ukrainian-only keys: ${extra.join(", ")}`);
      assert.ok(extra.every((key) => `${key.replace(/_(few|many)$/, "")}_one` in source));
    } else {
      assert.deepEqual(extra, [], `${locale} has unexpected translation keys`);
    }

    assert.doesNotMatch(
      Object.values(target).join("\n"),
      /ZXQ|<x\d|<х\d|КСНУМ|Nox\d/i,
      `${locale} contains a leaked translation placeholder marker`,
    );
  }
});

test("representative UI keys resolve in every supported locale without key echoes", async () => {
  const expectedSettingsTitles: Record<SupportedLocale, string> = {
    en: "Settings",
    es: "Configuración",
    fr: "Paramètres",
    de: "Einstellungen",
    ja: "設定",
    uk: "Налаштування",
    "pt-BR": "Configurações",
  };

  for (const locale of supportedLocales) {
    const instance = await translator(locale);
    assert.equal(instance.resolvedLanguage, locale);
    assert.equal(instance.t("settings.title"), expectedSettingsTitles[locale]);
    assert.notEqual(instance.t("sdkImport.importPackage"), "sdkImport.importPackage");
    assert.notEqual(instance.t("sdkImport.exactReleaseImported", { version: "1.2.3" }), "sdkImport.exactReleaseImported");
    assert.notEqual(instance.t("navigation.settings"), "navigation.settings");
    assert.notEqual(instance.t("tools.confirmMutationLiveTest", { method: "POST", name: "create", revision: 3 }), "tools.confirmMutationLiveTest");
  }
});

test("Ukrainian uses one, few, many, and other plural categories", async () => {
  const instance = await translator("uk");
  assert.equal(instance.t("agentAccess.accounts", { count: 1 }), "1 рахунок");
  assert.equal(instance.t("agentAccess.accounts", { count: 2 }), "2 акаунти");
  assert.equal(instance.t("agentAccess.accounts", { count: 5 }), "5 акаунтів");
  assert.notEqual(instance.t("agentAccess.accounts", { count: 1.5 }), "agentAccess.accounts");
});

test("number and date formatting follow the active locale", async () => {
  const english = await translator("en");
  const german = await translator("de");
  const value = 12_345.6;
  assert.equal(english.t("format.number", { value }), new Intl.NumberFormat("en").format(value));
  assert.equal(german.t("format.number", { value }), new Intl.NumberFormat("de").format(value));
  assert.notEqual(english.t("format.number", { value }), german.t("format.number", { value }));

  const date = new Date("2026-08-27T08:30:00.000Z");
  assert.notEqual(english.t("format.date", { value: date }), german.t("format.date", { value: date }));
  assert.doesNotMatch(english.t("format.date", { value: date }), /format\.date|Invalid/);
});
