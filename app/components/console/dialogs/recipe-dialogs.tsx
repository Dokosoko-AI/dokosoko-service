"use client";

import type { APIRecipe } from "../../../lib/api";
import { Select, Textarea } from "../../core";
import { Button, Dialog } from "../../core/control";
import { parseRecipeSpecEditor, recipeEditableSpec } from "./recipe-spec-editor";

export type RecipeDialogState =
  | { kind: "create"; integrationID: string; value: string }
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
  if (!state) return null;

  const creating = state.kind === "create";
  const editing = state.kind === "edit";
  const validation = editing ? parseRecipeSpecEditor(state.value, recipeEditableSpec(state.recipe)) : null;
  const validationError = validation && !validation.ok ? validation.error : "";
  const title = creating ? "Create product integration recipe" : editing ? `Edit ${state.recipe.title}` : `Rework ${state.recipe.title}`;
  const description = creating
    ? "Describe one concrete product capability for the coding agent to implement. The agent already has MCP access, so exclude connector setup."
    : editing
      ? "The server owns the product-integration instructions. You may keep or remove reviewed documentation references and change visibility."
      : "Describe the specific product-integration step, evidence gap, or verification detail that should change.";
  const label = creating ? "Implementation outcome" : editing ? "Reviewed reference IDs (JSON)" : "Rework instruction";
  const hint = creating
    ? "Use one tangible outcome, such as “Create and persist a sandbox payment.”"
    : editing
      ? "Use only IDs already reviewed for this revision. Steps and checks are regenerated from the exact product capability and cannot be edited here."
      : "Ask for one focused correction. Do not request MCP connection instructions.";
  const maxLength = creating ? 4000 : editing ? 5000 : 8000;
  const submitLabel = creating ? "Generate recipe" : editing ? "Save references" : "Generate revision";
  const describedBy = `recipe-dialog-hint${validationError ? " recipe-dialog-error" : ""}`;

  return <Dialog
    open
    onClose={(open) => { if (!open && !busy) onClose(); }}
    title={title}
    description={description}
    actions={<><Button outline disabled={busy} onClick={onClose}>Cancel</Button><Button disabled={busy || !state.value.trim() || Boolean(validationError)} onClick={onSubmit}>{busy ? "Working…" : submitLabel}</Button></>}
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
        <span>Visibility</span>
        <Select id="recipe-dialog-visibility" name="recipe-dialog-visibility" disabled={busy} value={state.visibility} onChange={(event) => onVisibilityChange(event.target.value as APIRecipe["visibility"])}>
          <option value="private">Private</option>
          <option value="public">Public</option>
        </Select>
        <small>Public recipes become available through public MCP after approval and publication.</small>
      </label>}
    </div>
  </Dialog>;
}
