"use client";


import { useTranslation } from "react-i18next";
import { RefreshCw, TriangleAlert } from "lucide-react";
import { useEffect, useState } from "react";
import type {
  APIGrantDefinition,
  APIIntegration,
  APIProduct,
  APIRuntimeSetup,
  APITool,
  APIToolBuilderProposal,
} from "../../lib/api";
import { api } from "../../lib/api";
import { integrationPath } from "../../lib/console-routes";
import { ToolBuilderView } from "../ToolBuilderView";
import { Button } from "../core/control";

export function IntegrationToolBuilderRoute({
  integration,
  product,
  grants,
  tool = null,
  initialProposal = null,
  aiAvailable,
  onNavigate,
  onMessage,
  onDirtyChange,
  onSaved,
}: {
  integration: APIIntegration;
  product: APIProduct;
  grants: APIGrantDefinition[];
  tool?: APITool | null;
  initialProposal?: APIToolBuilderProposal | null;
  aiAvailable: boolean;
  onNavigate: (path: string) => void;
  onMessage: (message: string) => void;
  onDirtyChange?: (dirty: boolean) => void;
  onSaved?: (tool: APITool) => void | Promise<void>;
}) {
  const { t } = useTranslation();
  const [setup, setSetup] = useState<APIRuntimeSetup | null>(null);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    void api.integrationRuntimeSetup(integration.id).then((value) => {
      if (!cancelled) setSetup(value);
    }).catch((loadError: unknown) => {
      if (!cancelled) setError(loadError instanceof Error ? loadError.message : t("integrationToolBuilder.apiServiceAccessCouldNotBeLoaded"));
    });
    return () => { cancelled = true; };
  }, [attempt, integration.id, t]);

  if (!setup && !error) return <section className="panel entity-missing" aria-live="polite"><span className="entity-missing-icon"><RefreshCw /></span><div><h1>{t("integrationToolBuilder.loadingAPIToolContext")}</h1><p>{t("integrationToolBuilder.loadingTheAPIOwnedServiceConnectionAndMaskedAuthentication")}</p></div></section>;
  if (!setup) return <section className="panel entity-missing" role="alert"><span className="entity-missing-icon"><TriangleAlert /></span><div><h1>{t("integrationToolBuilder.apiToolContextUnavailable")}</h1><p>{error}</p></div><span className="heading-actions"><Button outline onClick={() => { setError(""); setAttempt((value) => value + 1); }}>{t("common.retry")}</Button><Button onClick={() => onNavigate(integrationPath(integration.id, "access"))}>{t("integrationToolBuilder.openAccess")}</Button></span></section>;

  return <ToolBuilderView
    key={`${integration.id}:${tool?.id ?? "new"}:${tool?.revision ?? 0}:${setup.service_connections.map((connection) => connection.revision).join("-")}`}
    product={product}
    grants={grants}
    tool={tool}
    initialProposal={initialProposal}
    aiAvailable={aiAvailable}
    apiContext={{ integration, setup }}
    onNavigate={onNavigate}
    onMessage={onMessage}
    onDirtyChange={onDirtyChange}
    onSaved={onSaved}
  />;
}
