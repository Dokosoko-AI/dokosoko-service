"use client";

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
  const [setup, setSetup] = useState<APIRuntimeSetup | null>(null);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    void api.integrationRuntimeSetup(integration.id).then((value) => {
      if (!cancelled) setSetup(value);
    }).catch((loadError: unknown) => {
      if (!cancelled) setError(loadError instanceof Error ? loadError.message : "API service access could not be loaded.");
    });
    return () => { cancelled = true; };
  }, [attempt, integration.id]);

  if (!setup && !error) return <section className="panel entity-missing" aria-live="polite"><span className="entity-missing-icon"><RefreshCw /></span><div><h1>Loading API tool context</h1><p>Loading the API-owned service connection and masked authentication metadata…</p></div></section>;
  if (!setup) return <section className="panel entity-missing" role="alert"><span className="entity-missing-icon"><TriangleAlert /></span><div><h1>API tool context unavailable</h1><p>{error}</p></div><span className="heading-actions"><Button outline onClick={() => { setError(""); setAttempt((value) => value + 1); }}>Retry</Button><Button onClick={() => onNavigate(integrationPath(integration.id, "access"))}>Open Access</Button></span></section>;

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
