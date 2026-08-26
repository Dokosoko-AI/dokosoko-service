"use client";

import { useState, type Dispatch, type SetStateAction } from "react";

import { api } from "../../lib/api";
import type { APIMCPCatalog, APIMCPConnection, APIProduct, APITool, Distribution } from "../../lib/api";
import type { ConsoleFixtures } from "../../dev/console-fixtures";

export function useMCPWorkspaceState({ fixtures, product, apiConnected, setTools, showToast }: {
  fixtures?: ConsoleFixtures;
  product: APIProduct;
  apiConnected: boolean;
  setTools: Dispatch<SetStateAction<APITool[]>>;
  showToast: (message: string) => void;
}) {
  const [mcpConnections, setMCPConnections] = useState<APIMCPConnection[]>(fixtures?.mcpConnections ?? []);
  const [mcpConnectionOpen, setMCPConnectionOpen] = useState(false);
  const [mcpImportOpen, setMCPImportOpen] = useState(false);
  const [mcpBusy, setMCPBusy] = useState(false);
  const [mcpCatalog, setMCPCatalog] = useState<APIMCPCatalog | null>(null);
  const [mcpSelectedTools, setMCPSelectedTools] = useState<string[]>([]);
  const [mcpImportFailures, setMCPImportFailures] = useState<Record<string, string>>({});
  const [mcpName, setMCPName] = useState("");
  const [mcpNamespace, setMCPNamespace] = useState("");
  const [mcpEndpoint, setMCPEndpoint] = useState("");
  const [mcpAccessToken, setMCPAccessToken] = useState("");
  const [mcpForwardUserIdentity, setMCPForwardUserIdentity] = useState(false);
  const [mcpGrants, setMCPGrants] = useState("");
  const [mcpConfirmationRequired, setMCPConfirmationRequired] = useState(true);
  const [publicMCPEnabled, setPublicMCPEnabled] = useState(false);
  const [distribution, setDistribution] = useState<Distribution | null>(null);

  function fixtureCatalog(connection: APIMCPConnection): APIMCPCatalog {
    const schema = { type: "object", additionalProperties: false, properties: { title: { type: "string" } }, required: ["title"] };
    return { connection, catalog_hash: "sha256:48f2a81d", ttl_ms: 30000, tools: [
      { name: "incidents.create", title: "Create incident", description: "Create a support incident for the signed-in developer.", input_schema: schema, output_schema: { type: "object", additionalProperties: false, properties: { incident_id: { type: "string" } }, required: ["incident_id"] }, annotations: { destructiveHint: false }, schema_hash: "sha256:8f44e6" },
      { name: "incidents.get", title: "Get incident", description: "Read one support incident.", input_schema: { type: "object", additionalProperties: false, properties: { incident_id: { type: "string" } }, required: ["incident_id"] }, schema_hash: "sha256:1183ce" },
      { name: "incidents.comment", title: "Comment on incident", description: "Add a comment as the signed-in developer.", input_schema: { type: "object", additionalProperties: false, properties: { incident_id: { type: "string" }, body: { type: "string" } }, required: ["incident_id", "body"] }, annotations: { destructiveHint: false }, schema_hash: "sha256:211a40" },
    ] };
  }

  async function inspectMCPConnection(connection: APIMCPConnection) {
    setMCPBusy(true);
    try {
      const catalog = apiConnected ? await api.inspectMCPConnection(product.id, connection.id) : fixtureCatalog(connection);
      setMCPCatalog(catalog);
      setMCPSelectedTools(catalog.tools.map((tool) => tool.name));
      setMCPImportFailures({});
      setMCPImportOpen(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : "Could not inspect the upstream MCP catalog.");
    } finally {
      setMCPBusy(false);
    }
  }

  async function createMCPConnection() {
    setMCPBusy(true);
    try {
      const input = {
        organisation_id: product.organisation_id,
        name: mcpName,
        namespace: mcpNamespace,
        endpoint: mcpEndpoint,
        access_token: mcpAccessToken,
        forward_user_identity: mcpForwardUserIdentity,
      };
      const connection = apiConnected ? await api.createMCPConnection(product.id, input) : { id: `mcp_${Date.now()}`, product_id: product.id, protocol_version: "2026-07-28" as const, auth_mode: "access_token" as const, state: "active" as const, revision: 1, config: {}, created_at: new Date().toISOString(), updated_at: new Date().toISOString(), ...input };
      setMCPConnections((items) => [...items, connection]);
      setMCPConnectionOpen(false);
      setMCPName(""); setMCPNamespace(""); setMCPEndpoint(""); setMCPAccessToken(""); setMCPForwardUserIdentity(false);
      const catalog = apiConnected ? await api.inspectMCPConnection(product.id, connection.id) : fixtureCatalog(connection);
      setMCPCatalog(catalog);
      setMCPSelectedTools(catalog.tools.map((tool) => tool.name));
      setMCPImportFailures({});
      setMCPImportOpen(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : "Could not create the Stateless MCPv2 connection.");
    } finally {
      setMCPBusy(false);
    }
  }

  async function importMCPTools() {
    if (!mcpCatalog || mcpSelectedTools.length === 0) return;
    setMCPBusy(true);
    try {
      const grants = mcpGrants.split(",").map((value) => value.trim()).filter(Boolean);
      if (apiConnected) {
        const result = await api.importMCPTools(product.id, mcpCatalog.connection.id, { tool_names: mcpSelectedTools, required_grants: grants, confirmation_required: mcpConfirmationRequired, timeout_ms: 10000 });
        const changed = [...result.created, ...result.updated, ...result.unchanged, ...result.drifted];
        setTools((items) => [...items.filter((item) => !changed.some((candidate) => candidate.id === item.id)), ...changed]);
        setMCPConnections((items) => items.map((item) => item.id === result.connection.id ? result.connection : item));
        const rejected = Object.entries(result.rejected);
        if (rejected.length > 0) {
          setMCPImportFailures(result.rejected);
          setMCPSelectedTools(rejected.map(([name]) => name));
          const reviewed = result.created.length + result.updated.length + result.unchanged.length + result.drifted.length;
          showToast(`${reviewed} tool${reviewed === 1 ? "" : "s"} reviewed; ${rejected.length} rejected. Review the reasons in this dialog.`);
          return;
        }
        setMCPImportFailures({});
        setMCPImportOpen(false);
        setMCPGrants("");
        const messages = [
          result.created.length > 0 ? `${result.created.length} draft${result.created.length === 1 ? "" : "s"} created` : "",
          result.updated.length > 0 ? `${result.updated.length} draft${result.updated.length === 1 ? "" : "s"} updated` : "",
          result.unchanged.length > 0 ? `${result.unchanged.length} already current` : "",
          result.drifted.length > 0 ? `${result.drifted.length} published tool${result.drifted.length === 1 ? "" : "s"} blocked by schema drift` : "",
        ].filter(Boolean);
        showToast(messages.length > 0 ? `${messages.join("; ")}.` : "No upstream tool changes were needed.");
      } else {
        const imported = mcpCatalog.tools.filter((item) => mcpSelectedTools.includes(item.name)).map((item, index): APITool => ({ id: `tool_mcp_${index}`, organisation_id: product.organisation_id, product_id: product.id, namespace: mcpCatalog.connection.namespace, name: item.name.replace(/[^A-Za-z0-9_]/g, "_"), description: item.description ?? item.title ?? item.name, input_schema: item.input_schema, output_schema: item.output_schema ?? {}, state: "draft", revision: 1, http_method: "MCP", authorization_policy: { required_grants: grants, confirmation_required: mcpConfirmationRequired }, timeout_ms: 10000, backend_kind: "mcp", mcp_connection_id: mcpCatalog.connection.id, upstream_tool_name: item.name, upstream_schema_hash: item.schema_hash }));
        setTools((items) => [...items, ...imported]);
        setMCPImportFailures({});
        setMCPImportOpen(false);
        setMCPGrants("");
        showToast(`${imported.length} upstream tool${imported.length === 1 ? "" : "s"} imported as reviewed drafts.`);
      }
    } catch (error) {
      showToast(error instanceof Error ? error.message : "Could not import the selected MCP tools.");
    } finally {
      setMCPBusy(false);
    }
  }

  return {
    mcpConnections,
    setMCPConnections,
    mcpConnectionOpen,
    setMCPConnectionOpen,
    mcpImportOpen,
    setMCPImportOpen,
    mcpBusy,
    setMCPBusy,
    mcpCatalog,
    setMCPCatalog,
    mcpSelectedTools,
    setMCPSelectedTools,
    mcpImportFailures,
    setMCPImportFailures,
    mcpName,
    setMCPName,
    mcpNamespace,
    setMCPNamespace,
    mcpEndpoint,
    setMCPEndpoint,
    mcpAccessToken,
    setMCPAccessToken,
    mcpForwardUserIdentity,
    setMCPForwardUserIdentity,
    mcpGrants,
    setMCPGrants,
    mcpConfirmationRequired,
    setMCPConfirmationRequired,
    publicMCPEnabled,
    setPublicMCPEnabled,
    distribution,
    setDistribution,
    inspectMCPConnection,
    createMCPConnection,
    importMCPTools,
  };
}
