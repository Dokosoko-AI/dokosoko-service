"use client";


import { useTranslation } from "react-i18next";
import type { APIRecipe } from "../../../lib/api";
import { Select, Textarea } from "../../core";
import { Button, Dialog } from "../../core/control";
import { parseRecipeSpecEditor, recipeEditableSpec } from "./recipe-spec-editor";

export type RecipeDialogState =
  | { kind: "create"; value: string }
  | { kind: "edit"; recipe: APIRecipe; value: string; visibility: APIRecipe["visibility"] }
  | { kind: "rework"; recipe: APIRecipe; value: string };

export function RecipeDialogs({ state, busy, onChange, onVisibilityChange, onClose, onSubmit }: {
  state: RecipeDialogState | null;
  busy: boolean;
  onChange: (value: string) => void;
  onVisibilityChange: (visibility: APIRecipe["visibility"]) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  const { t } = useTranslation();
  if (!state) return null;

  const creating = state.kind === "create";
  const editing = state.kind === "edit";
  const validation = editing ? parseRecipeSpecEditor(state.value, recipeEditableSpec(state.recipe)) : null;
  const validationError = validation && !validation.ok
    ? validation.code === "unavailable" ? t("recipeDialogs.specUnavailable")
      : validation.code === "invalid_json" ? t("recipeDialogs.invalidJSONArray")
        : validation.code === "not_array" ? t("recipeDialogs.referenceIDsMustBeJSONArray")
          : validation.code === "too_many" ? t("recipeDialogs.tooManyReferenceIDs")
            : validation.code === "invalid_ids" ? t("recipeDialogs.invalidReferenceIDs")
              : t("recipeDialogs.unreviewedReferenceIDs")
    : "";
  const title = creating ? t("recipeDialogs.createProductIntegrationRecipe") : editing ? t("recipeDialogs.editRecipe", { title: state.recipe.title }) : t("recipeDialogs.reworkRecipe", { title: state.recipe.title });
  const description = creating
    ? t("recipeDialogs.createDescription")
    : editing
      ? t("recipeDialogs.editDescription")
      : t("recipeDialogs.reworkDescription");
  const label = creating ? t("recipeDialogs.implementationOutcome") : editing ? t("recipeDialogs.reviewedReferenceIDs") : t("recipeDialogs.reworkInstruction");
  const hint = creating
    ? t("recipeDialogs.createHint")
    : editing
      ? t("recipeDialogs.editHint")
      : t("recipeDialogs.reworkHint");
  const maxLength = creating ? 4000 : editing ? 5000 : 8000;
  const submitLabel = creating ? t("recipeDialogs.generateRecipe") : editing ? t("recipeDialogs.saveReferences") : t("recipeDialogs.generateRevision");
  const describedBy = `recipe-dialog-hint${validationError ? " recipe-dialog-error" : ""}`;

  return <Dialog
    open
    onClose={(open) => { if (!open && !busy) onClose(); }}
    title={title}
    description={description}
    actions={<><Button outline disabled={busy} onClick={onClose}>{t("common.cancel")}</Button><Button disabled={busy || !state.value.trim() || Boolean(validationError)} onClick={onSubmit}>{busy ? t("recipeDialogs.working") : submitLabel}</Button></>}
  >
    <div className="recipe-dialog-form">
      <label className="auth-field" htmlFor="recipe-dialog-value">
        <span>{label}</span>
        <Textarea
          id="recipe-dialog-value"
          name="recipe-dialog-value"
          className={editing ? "recipe-spec-editor" : undefined}
          rows={editing ? 8 : 6}
          maxLength={maxLength}
          spellCheck={!editing}
          aria-invalid={Boolean(validationError)}
          aria-describedby={describedBy}
          value={state.value}
          onChange={(event) => onChange(event.target.value)}
        />
        <small id="recipe-dialog-hint">{hint}</small>
        {validationError && <div className="auth-problem" id="recipe-dialog-error" role="status" aria-live="polite" aria-atomic="true">{validationError}</div>}
      </label>
      {state.kind === "edit" && <label className="auth-field" htmlFor="recipe-dialog-visibility">
        <span>{t("recipeDialogs.visibility")}</span>
        <Select id="recipe-dialog-visibility" name="recipe-dialog-visibility" disabled={busy} value={state.visibility} onChange={(event) => onVisibilityChange(event.target.value as APIRecipe["visibility"])}>
          <option value="private">{t("recipeDialogs.private")}</option>
          <option value="public">{t("recipeDialogs.public")}</option>
        </Select>
        <small>{t("recipeDialogs.publicRecipesBecomeAvailableThroughPublicMCPAfterApproval")}</small>
      </label>}
    </div>
  </Dialog>;
}
