import type { APITool } from "../../lib/api";

export function toolIsCommon(tool: Pick<APITool, "scope">) {
  return tool.scope !== "api";
}

export function toolIsOwnedByIntegration(tool: Pick<APITool, "scope" | "owner_integration_id">, integrationID: string) {
  return tool.scope === "api" && tool.owner_integration_id === integrationID;
}

export function toolCanAttachToIntegration(tool: Pick<APITool, "scope" | "owner_integration_id">, integrationID: string) {
  return toolIsCommon(tool) || toolIsOwnedByIntegration(tool, integrationID);
}

export function partitionIntegrationTools<T extends Pick<APITool, "id" | "scope" | "owner_integration_id">>(
  tools: readonly T[],
  boundToolIDs: ReadonlySet<string>,
  integrationID: string,
) {
  return {
    apiOwned: tools.filter((tool) => toolIsOwnedByIntegration(tool, integrationID)),
    attachedCommon: tools.filter((tool) => boundToolIDs.has(tool.id) && toolIsCommon(tool)),
    foreignAPI: tools.filter((tool) => boundToolIDs.has(tool.id) && tool.scope === "api" && tool.owner_integration_id !== integrationID),
  };
}
