"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, APIError, type APIRecipe, type APIRecipeReferenceOptions } from "../../../lib/api";
import { Select } from "../../core";
import { Badge, Button, Dialog } from "../../core/control";
import { Checkbox, CheckboxField } from "../../core/checkbox";
import { Label } from "../../core/fieldset";
import { EvidenceContent } from "../../core/evidence-content";
import { AIReadinessPanel } from "../ai-readiness-panel";
import { recipeEditableSpec, recipeReferenceOptionsMatch, validRecipeReferenceSelection } from "./recipe-spec-editor";

export function RecipeReferencesDialog({ initialRecipe, busy, onClose, onSave, onReload }: {
  initialRecipe: APIRecipe; busy: boolean; onClose: () => void;
  onSave: (recipe: APIRecipe, referenceIDs: string[], visibility: APIRecipe["visibility"]) => Promise<APIRecipe>;
  onReload: (recipe: APIRecipe) => void;
}) {
  const { t } = useTranslation();
  const [recipe, setRecipe] = useState(initialRecipe);
  const [selected, setSelected] = useState<string[]>(recipeEditableSpec(initialRecipe)?.reference_ids ?? []);
  const [visibility, setVisibility] = useState(initialRecipe.visibility);
  const [options, setOptions] = useState<APIRecipeReferenceOptions | null>(null);
  const [problem, setProblem] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [canProcess, setCanProcess] = useState(false);
  const sequence = useRef(0);
  const editable = Boolean(recipeEditableSpec(recipe));
  const locked = busy || refreshing;

  useEffect(() => {
    if (!editable) return;
    let cancelled = false;
    const request = ++sequence.current;
    api.recipeReferenceOptions(recipe.product_id, recipe.id, recipe.revision, recipe.current_revision_id).then((value) => {
      if (cancelled || request !== sequence.current) return;
      if (!recipeReferenceOptionsMatch(recipe, value)) throw new Error("scope_mismatch");
      setOptions(value);
    }).catch((error) => {
      if (!cancelled && request === sequence.current) setProblem(error instanceof APIError ? error.message : t("recipeReferences.unavailable"));
    });
    return () => { cancelled = true; };
  }, [recipe, editable, attempt, t]);

  async function refresh() {
    ++sequence.current;
    setRefreshing(true); setOptions(null); setProblem("");
    try {
      const { recipe: value } = await api.recipe(recipe.product_id, recipe.id);
      if (value.product_id !== recipe.product_id || value.id !== recipe.id) throw new Error("scope_mismatch");
      setRecipe(value); setSelected(recipeEditableSpec(value)?.reference_ids ?? []); setVisibility(value.visibility);
      setAttempt((current) => current + 1); onReload(value);
    } catch (error) {
      setProblem(error instanceof APIError ? error.message : t("recipeReferences.unavailable"));
    } finally { setRefreshing(false); }
  }

  const valid = options && recipeReferenceOptionsMatch(recipe, options) && validRecipeReferenceSelection(selected, options);
  async function save() {
    if (locked || !valid || !canProcess) return;
    setProblem("");
    try {
      await onSave(recipe, selected, visibility);
      onClose();
    } catch (error) {
      setProblem(`${error instanceof APIError ? error.message + " " : ""}${t("recipeReferences.saveFailed")}`);
    }
  }

  return <Dialog open onClose={(open) => { if (!open && !locked) onClose(); }}
    title={t("recipeDialogs.editRecipe", { title: recipe.title })}
    description={t("recipeReferences.description")}
    actions={<><Button outline disabled={locked} onClick={onClose}>{t("common.cancel")}</Button><Button disabled={locked || !valid || !canProcess} onClick={() => void save()}>{t(busy ? "recipeDialogs.working" : "recipeDialogs.saveReferences")}</Button></>}
  >
    <div className="recipe-dialog-form">
      <AIReadinessPanel compact preserveForm onCanProcess={setCanProcess} />
      <p>{t("recipeReferences.revision", { revision: recipe.current_revision?.revision ?? recipe.revision })}</p>
      {problem && <p className="auth-problem" role="status" aria-live="polite">{problem}</p>}
      {!editable && <p role="status">{t("recipeDialogs.specUnavailable")}</p>}
      {editable && !options && !problem && <p role="status">{t("common.loading")}</p>}
      {options && <>
        <p>{t("recipeReferences.selection", { count: selected.length })}</p>
        {options.items.length === 0 && <p>{t("recipeReferences.empty")}</p>}
        {options.items.map(({ reference, evidence }) => {
          const id = reference.resource_id!;
          const checked = selected.includes(id);
          return <section key={id} className="recipe-reference-option">
            <CheckboxField>
              <Checkbox checked={checked} disabled={locked || (!checked && selected.length >= 8)} onChange={(value) => setSelected((current) => value ? [...current, id] : current.filter((value) => value !== id))} />
              <Label>{reference.label}</Label>
            </CheckboxField>
            <a href={reference.url} target="_blank" rel="noreferrer noopener">{reference.url}</a>
            {evidence.map((item, evidenceIndex) => <div key={`${item.kind}:${item.resource_id}:${evidenceIndex}`}>
              <p>{item.label} <Badge>{t(item.visibility === "public" ? "common.public" : "common.private")}</Badge></p>
              {item.excerpt ? <EvidenceContent text={item.excerpt} label={t("recipeReferences.excerpt", { name: item.label })} /> : <p>{t("recipeReferences.noExcerpt")}</p>}
              <details><summary>{t("recipeReferences.provenance")}</summary><dl><dt>{t("recipeReferences.resource")}</dt><dd>{item.resource_id}</dd>{item.version && <><dt>{t("recipeReferences.version")}</dt><dd>{item.version}</dd></>}<dt>{t("recipeReferences.fingerprint")}</dt><dd>{item.fingerprint}</dd></dl></details>
            </div>)}
          </section>;
        })}
        {!valid && <p className="auth-problem" role="status">{t("recipeReferences.changed")}</p>}
      </>}
      <label className="auth-field" htmlFor="recipe-dialog-visibility"><span>{t("recipeDialogs.visibility")}</span>
        <Select id="recipe-dialog-visibility" disabled={locked} value={visibility} onChange={(event) => setVisibility(event.target.value as APIRecipe["visibility"])}><option value="private">{t("recipeDialogs.private")}</option><option value="public">{t("recipeDialogs.public")}</option></Select>
        <small>{t("recipeDialogs.publicRecipesBecomeAvailableThroughPublicMCPAfterApproval")}</small>
      </label>
      <div><Button outline disabled={locked} onClick={() => void refresh()}>{t("recipeReferences.refresh")}</Button><p>{t("recipeReferences.refreshHint")}</p></div>
    </div>
  </Dialog>;
}
