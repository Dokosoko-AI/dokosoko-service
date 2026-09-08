import type { APIIntegrationPublishStatus } from "./api";
import type { developerAssetsApi } from "./developer-assets-api";

export type PublicationRecord = Record<string, unknown>;
export function publicationRecord(value: unknown): PublicationRecord { return value && typeof value === "object" && !Array.isArray(value) ? value as PublicationRecord : {}; }
export function publicationRecords(value: unknown): PublicationRecord[] { return Array.isArray(value) ? value.map(publicationRecord) : []; }
export function publicationText(value: unknown): string { return typeof value === "string" ? value : typeof value === "number" ? String(value) : ""; }
function canonical(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonical);
  if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => [key, canonical(item)]));
  return value;
}
export type PublicationReviewKind = "documentation" | "contracts" | "sdks" | "tools" | "authorization" | "connections" | "global";
export type PublicationReviewRow = { key: string; kind: PublicationReviewKind; title: string; value: PublicationRecord; previous?: PublicationRecord; state: "added" | "changed" | "unchanged" | "removed" };
export type PublicationSelection = { revision?: number; version?: string; guidanceRevision?: number };
export type ResolvedPublicationReviewRow = PublicationReviewRow & { selection: PublicationSelection; previousSelection?: PublicationSelection };
function snapshotRows(snapshot: PublicationRecord): Omit<PublicationReviewRow, "state">[] {
  const assets = publicationRecord(snapshot.developer_assets);
  const rows: Omit<PublicationReviewRow, "state">[] = [];
  function add(kind: PublicationReviewKind, values: unknown, id: string, names: string[], source = "") {
    for (const value of publicationRecords(values)) {
      const identity = publicationText(value[id]);
      if (!identity) continue;
      const title = names.map((key) => publicationText(value[key])).find(Boolean) ?? "";
      rows.push({ key: `${kind}:${source}${identity}`, kind, title: kind === "tools" && value.namespace ? `${publicationText(value.namespace)}/${title}` : title, value });
    }
  }
  add("documentation", assets.documentation, "binding_id", ["documentation_collection_name"]);
  add("contracts", assets.contracts, "binding_id", ["api_contract_name"]);
  add("sdks", assets.sdks, "binding_id", ["sdk_package_display_name", "sdk_package_display_coordinate", "sdk_package_coordinate"]);
  for (const resource of publicationRecords(snapshot.resource_sets)) add(resource.kind === "api" ? "contracts" : "documentation", [resource], "set_id", ["name"], "legacy:");
  add("sdks", snapshot.sdks, "id", ["coordinate"], "legacy:");
  add("tools", snapshot.tools, "tool_id", ["name"]);
  add("authorization", snapshot.authorization_points, "id", ["name", "key"]);
  add("connections", snapshot.service_connections, "connection_id", ["name"]);
  if (publicationText(assets.global_documentation_publication_id)) rows.push({ key: "global:documentation", kind: "global", title: "", value: { publication_id: assets.global_documentation_publication_id, content_hash: assets.global_documentation_snapshot_hash } });
  return rows;
}
export function integrationPublicationRows(snapshot: PublicationRecord, previous: PublicationRecord = {}): PublicationReviewRow[] {
  const before = new Map(snapshotRows(previous).map((value) => [value.key, value]));
  const result: PublicationReviewRow[] = snapshotRows(snapshot).map((row) => {
    const prior = before.get(row.key); before.delete(row.key);
    return { ...row, previous: prior?.value, state: !prior ? "added" : JSON.stringify(canonical(prior.value)) === JSON.stringify(canonical(row.value)) ? "unchanged" : "changed" };
  });
  for (const row of before.values()) result.push({ ...row, previous: row.value, state: "removed" });
  return result;
}
export function reviewedPublicationInput(status: APIIntegrationPublishStatus, integrationID: string) {
  if (status.integration_id !== integrationID || !status.ready || !status.has_changes || !Number.isSafeInteger(status.candidate_revision) || status.candidate_revision < 1 || !status.current_manifest_hash.trim()) throw Error("publication_review_unavailable");
  return { integrationID, candidateRevision: status.candidate_revision, candidateManifestHash: status.current_manifest_hash };
}

type SelectionTransport = Pick<typeof developerAssetsApi, "documentationCollectionRevision" | "apiContractRevision" | "sdkRelease" | "sdkContentPublication" | "documentationPublication">;

// Names come from the reviewed snapshot. These reads resolve only immutable
// revision labels; mutable package/catalog heads never replace the selection.
export async function resolvePublicationReviewRows(rows: PublicationReviewRow[], deploymentID: string, transport: SelectionTransport): Promise<ResolvedPublicationReviewRow[]> {
  const cache = new Map<string, Promise<PublicationSelection>>();
  function selection(row: PublicationReviewRow, value: PublicationRecord): Promise<PublicationSelection> {
    const key = JSON.stringify([row.kind, canonical(value)]);
    const existing = cache.get(key);
    if (existing) return existing;
    const read = async (): Promise<PublicationSelection> => {
      const text = publicationText;
      function exact(record: { id: string; deployment_id: string }, id: string, scopeMatches = true) {
        if (record.id !== id || record.deployment_id !== deploymentID || !scopeMatches) throw Error("publication_selection_unavailable");
      }
      if (value.documentation_collection_id) {
        const id = text(value.documentation_collection_revision_id), collectionID = text(value.documentation_collection_id);
        const { revision } = await transport.documentationCollectionRevision(collectionID, id);
        exact(revision, id, revision.documentation_collection_id === collectionID);
        return { revision: revision.revision };
      }
      if (value.api_contract_id) {
        const id = text(value.api_contract_revision_id), contractID = text(value.api_contract_id);
        const revision = await transport.apiContractRevision(contractID, id);
        exact(revision, id, revision.api_contract_id === contractID);
        return { revision: revision.revision };
      }
      if (value.sdk_package_id) {
        const packageID = text(value.sdk_package_id), releaseID = text(value.sdk_release_id), publicationID = text(value.sdk_content_publication_id);
        const release = await transport.sdkRelease(packageID, releaseID);
        exact(release, releaseID, release.sdk_package_id === packageID);
        let guidanceRevision: number | undefined;
        if (publicationID) {
          const { publication } = await transport.sdkContentPublication(releaseID, publicationID);
          exact(publication, publicationID, publication.sdk_release_id === releaseID);
          guidanceRevision = publication.revision;
        }
        return { version: release.exact_version, guidanceRevision };
      }
      if (row.kind === "global") {
        const id = text(value.publication_id), publication = await transport.documentationPublication(id);
        exact(publication, id, publication.snapshot_hash === text(value.content_hash));
        return { revision: publication.revision };
      }
      const revision = [value.revision, value.tool_revision, value.connection_revision].find((item) => typeof item === "number") as number | undefined;
      return { revision, version: text(value.exact_version) || undefined };
    };
    const result = read(); cache.set(key, result); return result;
  }
  return Promise.all(rows.map(async (row) => ({ ...row, selection: await selection(row, row.value), previousSelection: row.previous ? await selection(row, row.previous) : undefined })));
}
