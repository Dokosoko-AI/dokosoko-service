"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { APIRecipe } from "../../../lib/api";
import { Textarea } from "../../core";
import { Button, Dialog } from "../../core/control";
import { AIReadinessPanel } from "../ai-readiness-panel";
import { RecipeReferencesDialog } from "./recipe-references-dialog";

export type RecipeDialogState =
  | { kind: "create"; value: string }
  | { kind: "edit"; recipe: APIRecipe }
  | { kind: "rework"; recipe: APIRecipe; value: string };

export function RecipeDialogs({ state, busy, onChange, onClose, onSubmit, onSaveReferences, onReload }: {
  state: RecipeDialogState | null;
  busy: boolean;
  onChange: (value: string) => void;
  onClose: () => void;
  onSubmit: () => void;
  onSaveReferences: (recipe: APIRecipe, referenceIDs: string[], visibility: APIRecipe["visibility"]) => Promise<APIRecipe>;
  onReload: (recipe: APIRecipe) => void;
}) {
  const { t } = useTranslation();
  const [canProcess, setCanProcess] = useState(false);
  if (!state) return null;
  if (state.kind === "edit") return <RecipeReferencesDialog key={`${state.recipe.id}:${state.recipe.current_revision_id}`} initialRecipe={state.recipe} busy={busy} onClose={onClose} onSave={onSaveReferences} onReload={onReload} />;

  const creating = state.kind === "create";
  return <Dialog open onClose={(open) => { if (!open && !busy) onClose(); }}
    title={creating ? t("recipeDialogs.createProductIntegrationRecipe") : t("recipeDialogs.reworkRecipe", { title: state.recipe.title })}
    description={creating ? t("recipeDialogs.createDescription") : t("recipeDialogs.reworkDescription")}
    actions={<><Button outline disabled={busy} onClick={onClose}>{t("common.cancel")}</Button><Button disabled={busy || !canProcess || !state.value.trim()} onClick={onSubmit}>{t(busy ? "recipeDialogs.working" : creating ? "recipeDialogs.generateRecipe" : "recipeDialogs.generateRevision")}</Button></>}
  >
    <AIReadinessPanel key={state.kind} preserveForm onCanProcess={setCanProcess} />
    <div className="recipe-dialog-form">
      <label className="auth-field" htmlFor="recipe-dialog-value">
        <span>{creating ? t("recipeDialogs.implementationOutcome") : t("recipeDialogs.reworkInstruction")}</span>
        <Textarea id="recipe-dialog-value" name="recipe-dialog-value" rows={6} maxLength={4000} disabled={busy} aria-describedby="recipe-dialog-hint" value={state.value} onChange={(event) => onChange(event.target.value)} />
        <small id="recipe-dialog-hint">{creating ? t("recipeDialogs.createHint") : t("recipeDialogs.reworkHint")}</small>
      </label>
    </div>
  </Dialog>;
}
