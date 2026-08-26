import type { APIIntegration } from "../../../lib/api";
import type {
  APIContractBinding,
  APIDeveloperAssetPublication,
  APIDocumentationBinding,
  APISDKBinding,
  DeveloperAssetUsage,
} from "../../../lib/developer-assets-api";

export type DocumentationUsage = { integration: APIIntegration; binding: APIDocumentationBinding };
export type ContractUsage = { integration: APIIntegration; binding: APIContractBinding };
export type SDKUsage = {
  integration: APIIntegration;
  binding: APISDKBinding;
  publication?: APIDeveloperAssetPublication;
};

function integrationsByID(integrations: APIIntegration[]) {
  return new Map(integrations.map((integration) => [integration.id, integration]));
}

export function documentationUsages(usage: DeveloperAssetUsage, integrations: APIIntegration[], collectionID: string): DocumentationUsage[] {
  const byID = integrationsByID(integrations);
  return usage.documentation.flatMap((binding) => {
    const integration = byID.get(binding.api_id);
    return integration && binding.documentation_collection_id === collectionID && binding.lifecycle === "attached"
      ? [{ integration, binding }]
      : [];
  });
}

export function contractUsages(usage: DeveloperAssetUsage, integrations: APIIntegration[], contractID: string): ContractUsage[] {
  const byID = integrationsByID(integrations);
  return usage.contracts.flatMap((binding) => {
    const integration = byID.get(binding.api_id);
    return integration && binding.api_contract_id === contractID && binding.lifecycle === "attached"
      ? [{ integration, binding }]
      : [];
  });
}

export function sdkUsages(usage: DeveloperAssetUsage, integrations: APIIntegration[], packageID: string): SDKUsage[] {
  const byID = integrationsByID(integrations);
  const publications = [...usage.publications].sort((left, right) => Date.parse(right.published_at) - Date.parse(left.published_at));
  return usage.sdks.flatMap((binding) => {
    const integration = byID.get(binding.api_id);
    if (!integration || binding.sdk_package_id !== packageID || binding.state === "detached") return [];
    const publication = publications.find((value) => value.api_id === binding.api_id && value.sdks.some((asset) =>
      asset.binding_id === binding.id && asset.sdk_content_publication_id === binding.sdk_content_publication_id));
    return [{ integration, binding, publication }];
  });
}
