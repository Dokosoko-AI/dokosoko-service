import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { APIError, api, type APISystemConfiguration } from "../../lib/api";
import { Badge } from "../core/control";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "../core";
import { PageHeader as PageHeading, PanelHeader } from "../core/layout";
import { SettingsTabs } from "./catalog-settings-views";

type ConfigurationSource = APISystemConfiguration["items"][number]["source"];

function sourceColor(source: ConfigurationSource): "blue" | "violet" | "zinc" {
  if (source === "environment") return "violet";
  if (source === "configuration_file") return "blue";
  return "zinc";
}

export function ConfigurationSettingsView({ available, onNavigate }: { available: boolean; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const [configuration, setConfiguration] = useState<APISystemConfiguration | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!available) return;
    let active = true;
    void api.systemConfiguration()
      .then((value) => { if (active) setConfiguration(value); })
      .catch((reason: unknown) => {
        if (!active) return;
        setError(reason instanceof APIError ? reason.message : t("configurationSettings.loadError"));
      });
    return () => { active = false; };
  }, [available, t]);

  function sourceLabel(source: ConfigurationSource) {
    if (source === "environment") return t("configurationSettings.sourceEnvironment");
    if (source === "configuration_file") return t("configurationSettings.sourceFile");
    return t("configurationSettings.sourceBuiltIn");
  }

  return <>
    <PageHeading eyebrow={t("navigation.settings")} title={t("routes.configuration")} />
    <SettingsTabs active="configuration" onNavigate={onNavigate} />
    <section className="panel">
      <PanelHeader title={t("configurationSettings.effectiveTitle")} description={t("configurationSettings.description")} />
      {!available && <div className="empty-row">{t("configurationSettings.unavailable")}</div>}
      {available && !configuration && !error && <div className="empty-row">{t("configurationSettings.loading")}</div>}
      {error && <div className="empty-row">{error}</div>}
      {configuration && <>
        <div className="notice"><span><strong>{configuration.configuration_file ? t("configurationSettings.fileLoaded") : t("configurationSettings.noFile")}</strong> {configuration.configuration_file ?? t("configurationSettings.environmentAndDefaults")}</span></div>
        <div className="notice"><span>{t("configurationSettings.restartNotice")}</span></div>
        <Table label={t("configurationSettings.effectiveTitle")} dense>
          <TableHead><TableRow><TableHeader>{t("configurationSettings.key")}</TableHeader><TableHeader>{t("configurationSettings.effectiveValue")}</TableHeader><TableHeader>{t("configurationSettings.source")}</TableHeader></TableRow></TableHead>
          <TableBody>{configuration.items.map((item) => <TableRow key={item.key}>
            <TableCell><code>{item.key}</code></TableCell>
            <TableCell>{item.sensitive ? (item.configured ? t("configurationSettings.configuredRedacted") : t("configurationSettings.notConfigured")) : item.configured ? <code>{item.value}</code> : t("configurationSettings.notConfigured")}</TableCell>
            <TableCell><Badge color={sourceColor(item.source)}>{sourceLabel(item.source)}</Badge></TableCell>
          </TableRow>)}</TableBody>
        </Table>
      </>}
    </section>
  </>;
}
