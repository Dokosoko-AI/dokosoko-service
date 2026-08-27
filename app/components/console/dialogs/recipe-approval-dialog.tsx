"use client";


import { useTranslation } from "react-i18next";
import type { APIRecipe, APIRecipeRevision } from "../../../lib/api";
import { Badge, Button, Dialog } from "../../core/control";
import type { RecipeApprovalReview } from "./recipe-approval-review";

function findingColor(level: APIRecipeRevision["validation"][number]["level"]): "blue" | "amber" | "red" {
  if (level === "error") return "red";
  if (level === "warning") return "amber";
  return "blue";
}

function generatedByLabel(value: APIRecipeRevision["generated_by"]): string {
  if (value === "ai") return "AI";
  return value.charAt(0).toUpperCase() + value.slice(1);
}

export function RecipeApprovalDialog({ review, busy, onClose, onApprove }: {
  review: RecipeApprovalReview | null;
  busy: boolean;
  onClose: () => void;
  onApprove: (recipe: APIRecipe) => void | Promise<void>;
}) {
  const { t } = useTranslation();
  if (!review) return null;

  const { recipe, revision, currentRevisionID, integrations } = review;
  const bindings = revision.spec_version === 2
    ? [{ integration_id: revision.spec.integration_id, integration_revision_id: revision.integration_revision_id, integration_manifest_hash: revision.integration_manifest_hash }]
    : revision.api_bindings ?? [];
  const promptVersion = revision.prompt_version || (revision.generated_by === "ai" ? t("recipeApproval.notRecorded") : t("recipeApproval.notApplicable"));
  const promptHash = revision.prompt_hash || (revision.generated_by === "ai" ? t("recipeApproval.notRecorded") : t("recipeApproval.notApplicable"));

  async function approveReviewedRevision() {
    await onApprove(recipe);
    onClose();
  }

  return <Dialog
    open
    onClose={(open) => { if (!open && !busy) onClose(); }}
    title={t("recipeApproval.approveRevision", { title: String(recipe.title), revision: String(revision.revision) })}
    description={t("recipeApproval.reviewTheExactImmutableRevisionBelowApprovalAppliesOnly")}
    actions={<><Button outline disabled={busy} onClick={onClose}>{t("common.cancel")}</Button><Button disabled={busy} onClick={() => void approveReviewedRevision()}>{busy ? t("recipeApproval.approving") : t("recipeApproval.approveRevision2", { revision: String(revision.revision) })}</Button></>}
  >
    <div className="recipe-approval-review">
      <section aria-labelledby="recipe-approval-binding-heading">
        <h3 id="recipe-approval-binding-heading">{t("recipeApproval.revisionAndIntegrationBinding")}</h3>
        <dl className="entity-detail-grid recipe-approval-metadata">
          <div><dt>{t("recipeApproval.revisionNumber")}</dt><dd>{revision.revision}</dd></div>
          <div><dt>{t("recipeApproval.currentRevisionID")}</dt><dd><code>{currentRevisionID}</code></dd></div>
          <div><dt>{t("recipeApproval.integration")}</dt><dd>{integrations.map((integration) => integration.display_name).join(", ") || t("recipeApproval.unavailable")}</dd></div>
          <div><dt>{t("recipeApproval.integrationID")}</dt><dd>{bindings.map((binding) => <code key={binding.integration_id}>{binding.integration_id}</code>)}</dd></div>
          <div><dt>{t("recipeApproval.integrationRevisionID")}</dt><dd>{bindings.map((binding) => <code key={binding.integration_id}>{binding.integration_revision_id || t("recipeApproval.notRecorded")}</code>)}</dd></div>
          <div><dt>{t("recipeApproval.integrationManifestHash")}</dt><dd>{bindings.map((binding) => <code key={binding.integration_id}>{binding.integration_manifest_hash || t("recipeApproval.notRecorded")}</code>)}</dd></div>
        </dl>
      </section>

      <section aria-labelledby="recipe-approval-provenance-heading">
        <h3 id="recipe-approval-provenance-heading">{t("recipeApproval.generationProvenance")}</h3>
        <dl className="entity-detail-grid recipe-approval-metadata">
          <div><dt>{t("recipeApproval.generatedBy")}</dt><dd>{generatedByLabel(revision.generated_by)}</dd></div>
          <div><dt>{t("recipeApproval.model")}</dt><dd>{revision.model || t("recipeApproval.notApplicable")}</dd></div>
          <div><dt>{t("recipeApproval.promptVersion")}</dt><dd>{promptVersion}</dd></div>
          <div><dt>{t("recipeApproval.promptHash")}</dt><dd><code>{promptHash}</code></dd></div>
          <div><dt>{t("recipeApproval.createdBy")}</dt><dd>{revision.created_by}</dd></div>
          <div><dt>{t("recipeApproval.created")}</dt><dd>{t("format.dateTime", { value: new Date(revision.created_at) })}</dd></div>
        </dl>
      </section>

      <section aria-labelledby="recipe-approval-validation-heading">
        <h3 id="recipe-approval-validation-heading">{t("recipeApproval.validationFindings")}</h3>
        {revision.validation.length > 0 ? <ul className="recipe-approval-list">
          {revision.validation.map((finding, index) => <li key={`${finding.level}:${finding.code}:${index}`}>
            <Badge color={findingColor(finding.level)}>{finding.level}</Badge>
            <span><strong>{finding.code}</strong><small>{finding.message}</small></span>
          </li>)}
        </ul> : <p className="recipe-approval-empty">{t("recipeApproval.noValidationFindingsWereRecordedForThisRevision")}</p>}
      </section>

      <section aria-labelledby="recipe-approval-references-heading">
        <h3 id="recipe-approval-references-heading">{t("recipeApproval.references")}</h3>
        {revision.references && revision.references.length > 0 ? <ul className="recipe-approval-list recipe-approval-references">
          {revision.references.map((reference, index) => <li key={`${reference.kind}:${reference.resource_id ?? reference.url}:${index}`}>
            <Badge>{reference.kind}</Badge>
            <span>
              <strong><a href={reference.url} target="_blank" rel="noreferrer">{reference.label}</a></strong>
              <small>{reference.resource_id ? t("recipeApproval.copy", { resource_id: String(reference.resource_id) }) : ""}{reference.url}{reference.anchor ? t("recipeApproval.copy2", { anchor: String(reference.anchor) }) : ""}</small>
            </span>
          </li>)}
        </ul> : <p className="recipe-approval-empty">{t("recipeApproval.noReferencesWereSelectedForThisRevision")}</p>}
      </section>

      <section aria-labelledby="recipe-approval-markdown-heading">
        <h3 id="recipe-approval-markdown-heading">{t("recipeApproval.canonicalMarkdown")}</h3>
        <pre className="recipe-approval-markdown" role="region" aria-label={t("recipeApproval.canonicalMarkdownForRevision", { revision: String(revision.revision) })}><code>{revision.markdown || t("recipeApproval.noCanonicalMarkdown")}</code></pre>
      </section>
    </div>
  </Dialog>;
}
