"use client";

import { useEffect, useState } from "react";

import { APIError, api } from "../../lib/api";
import type {
  APIAIProviderConnection,
  APIAIProviderUsage,
  APIAIWorkflowPrompt,
  APIAIWorkloadProfile,
  APIIntegrationAnalysis,
  APIProduct,
  APIRecipe,
  APIRecipeReference,
} from "../../lib/api";
import {
  type AIWorkload,
  aiModelDefaults,
  aiProviderLabel,
  aiProviderOrigin,
  aiWorkloads,
  analysisMatchesIntegration,
  recipeAnalysisIsFreshlyRunning,
} from "./shared";

type LoadProblemReporter = (area: string, error: unknown) => void;

export function useAIWorkspaceState({
  product,
  fixturePreview,
  onLoadProblem,
  showToast,
}: {
  product: APIProduct;
  fixturePreview: boolean;
  onLoadProblem: LoadProblemReporter;
  showToast: (message: string) => void;
}) {
  const productID = product.id;
  const [aiConnections, setAIConnections] = useState<APIAIProviderConnection[]>([]);
  const [aiProfiles, setAIProfiles] = useState<APIAIWorkloadProfile[]>([]);
  const [aiPrompts, setAIPrompts] = useState<APIAIWorkflowPrompt[]>([]);
  const [analyses, setAnalyses] = useState<APIIntegrationAnalysis[]>([]);
  const [recipes, setRecipes] = useState<APIRecipe[]>([]);
  const [aiProviderUsage, setAIProviderUsage] = useState<APIAIProviderUsage[]>([]);
  const [recipeBusy, setRecipeBusy] = useState(false);
  const [workloadOpen, setWorkloadOpen] = useState(false);
  const [workloadBusy, setWorkloadBusy] = useState(false);
  const [workloadRole, setWorkloadRole] = useState<AIWorkload>("analysis");
  const [workloadConnectionID, setWorkloadConnectionID] = useState("");
  const [providerOpen, setProviderOpen] = useState(false);
  const [providerPickerOpen, setProviderPickerOpen] = useState(false);
  const [providerBusy, setProviderBusy] = useState(false);
  const [providerEnabled, setProviderEnabled] = useState(true);
  const [providerIsBackup, setProviderIsBackup] = useState(false);
  const [providerBackupAnalysisModel, setProviderBackupAnalysisModel] = useState(aiModelDefaults.openai.analysis);
  const [providerKind, setProviderKind] = useState<APIAIProviderConnection["provider"]>("openai");
  const [providerEndpoint, setProviderEndpoint] = useState(aiProviderOrigin("openai"));
  const [workloadModel, setWorkloadModel] = useState(aiModelDefaults.openai.analysis);
  const [providerCredential, setProviderCredential] = useState("");
  const [workloadInputTokens, setWorkloadInputTokens] = useState("8192");
  const [workloadOutputTokens, setWorkloadOutputTokens] = useState("1024");
  const [workloadDailyBudget, setWorkloadDailyBudget] = useState("1000000");
  const [workloadEnabled, setWorkloadEnabled] = useState(false);
  const [promptOpen, setPromptOpen] = useState(false);
  const [promptBusy, setPromptBusy] = useState(false);
  const [promptKey, setPromptKey] = useState<APIAIWorkflowPrompt["key"] | null>(null);
  const [promptInstructions, setPromptInstructions] = useState("");

  useEffect(() => {
    if (fixturePreview) return;
    let cancelled = false;

    Promise.all([api.aiConnections(), api.aiProfiles(productID)])
      .then(([connections, profiles]) => {
        if (!cancelled) {
          setAIConnections(connections);
          setAIProfiles(profiles);
        }
      })
      .catch((error) => onLoadProblem("AI configuration", error));

    api.aiPrompts(productID)
      .then((prompts) => {
        if (!cancelled) setAIPrompts(prompts);
      })
      .catch((error) => onLoadProblem("AI workflow prompts", error));

    Promise.all([api.analyses(productID), api.recipes(productID), api.aiUsage(productID)])
      .then(([analysisValues, recipeValues, usageValues]) => {
        if (!cancelled) {
          setAnalyses(analysisValues);
          setRecipes(recipeValues);
          setAIProviderUsage(usageValues.providers);
        }
      })
      .catch((error) => onLoadProblem("AI content", error));

    return () => { cancelled = true; };
  }, [fixturePreview, onLoadProblem, productID]);

	  function openAIConnection(provider: APIAIProviderConnection["provider"]) {
	    const connection = aiConnections.find((item) => item.provider === provider);
	    setProviderKind(provider);
	    setProviderEndpoint(connection?.endpoint ?? aiProviderOrigin(provider));
	    setProviderCredential("");
	    setProviderEnabled(connection?.enabled ?? true);
	    setProviderIsBackup(connection?.is_backup ?? false);
	    setProviderBackupAnalysisModel(connection?.backup_models.analysis ?? aiModelDefaults[provider].analysis);
	    setProviderPickerOpen(false);
	    setProviderOpen(true);
	  }

	  function openAIWorkload(role: AIWorkload) {
	    const profile = aiProfiles.find((item) => item.workload === role);
	    const connection = aiConnections.find((item) => item.id === profile?.provider_connection_id) ?? aiConnections.find((item) => item.enabled && !item.is_backup);
	    setWorkloadRole(role);
	    setWorkloadConnectionID(connection?.id ?? "");
	    setWorkloadModel(profile?.model ?? (connection ? aiModelDefaults[connection.provider][role] : ""));
	    setWorkloadInputTokens(String(profile?.max_input_tokens ?? 128000));
	    setWorkloadOutputTokens(String(profile?.max_output_tokens ?? 8192));
	    setWorkloadDailyBudget(String(profile?.daily_token_budget ?? 0));
	    setWorkloadEnabled(profile?.enabled ?? false);
	    setWorkloadOpen(true);
	  }

	  function changeAIWorkloadConnection(connectionID: string) {
	    setWorkloadConnectionID(connectionID);
	    const connection = aiConnections.find((item) => item.id === connectionID);
	    if (connection) setWorkloadModel(aiModelDefaults[connection.provider][workloadRole]);
	  }

	  async function saveAIConnection() {
	    setProviderBusy(true);
	    try {
	      const current = aiConnections.find((item) => item.provider === providerKind);
	      const value = await api.saveAIConnection({ organisation_id: product.organisation_id, provider: providerKind, endpoint: providerEndpoint, credential: providerCredential, enabled: providerEnabled, is_backup: providerIsBackup, backup_models: providerIsBackup ? { analysis: providerBackupAnalysisModel } : {}, revision: current?.revision ?? 0 });
	      setAIConnections((items) => [...items.filter((item) => item.id !== value.id && item.provider !== value.provider), value]);
	      setProviderCredential("");
	      setProviderOpen(false);
	      showToast(`${aiProviderLabel(value.provider)} connected.`);
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not connect AI provider.");
	    } finally {
	      setProviderBusy(false);
	    }
	  }

	  async function testAIConnection(connection: APIAIProviderConnection) {
	    try {
	      const value = await api.testAIConnection(connection.id);
	      setAIConnections((items) => items.map((item) => item.id === value.id ? value : item));
	      showToast(`${aiProviderLabel(value.provider)} connection works.`);
	    } catch (error) {
	      const updated = await api.aiConnections().catch(() => aiConnections);
	      setAIConnections(updated);
	      showToast(error instanceof APIError ? error.message : "Connection test failed.");
	    }
	  }

	  async function saveAIWorkload() {
	    setWorkloadBusy(true);
	    try {
	      const current = aiProfiles.find((item) => item.workload === workloadRole);
	      const value = await api.saveAIProfile(product.id, workloadRole, { organisation_id: product.organisation_id, provider_connection_id: workloadConnectionID, model: workloadModel, max_input_tokens: Number(workloadInputTokens), max_output_tokens: Number(workloadOutputTokens), daily_token_budget: Number(workloadDailyBudget), enabled: workloadEnabled, revision: current?.revision ?? 0 });
	      setAIProfiles((items) => [...items.filter((item) => item.workload !== value.workload), value].sort((a, b) => a.workload.localeCompare(b.workload)));
	      setWorkloadOpen(false);
	      showToast(`${aiWorkloads.find((workload) => workload.role === value.workload)?.name ?? value.workload} workload saved.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not save AI model.");
    } finally {
      setWorkloadBusy(false);
    }
	  }

	  async function saveAIWorkloadSelection(role: AIWorkload, connectionID: string, modelID: string) {
	    setWorkloadBusy(true);
	    try {
	      const current = aiProfiles.find((item) => item.workload === role);
	      const value = await api.saveAIProfile(product.id, role, { organisation_id: product.organisation_id, provider_connection_id: connectionID, model: modelID, max_input_tokens: current?.max_input_tokens ?? 128000, max_output_tokens: current?.max_output_tokens ?? 8192, daily_token_budget: current?.daily_token_budget ?? 0, enabled: true, revision: current?.revision ?? 0 });
	      setAIProfiles((items) => [...items.filter((item) => item.workload !== value.workload), value].sort((a, b) => a.workload.localeCompare(b.workload)));
	      showToast(`${aiWorkloads.find((workload) => workload.role === value.workload)?.name ?? value.workload} workload saved.`);
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not save AI model.");
	    } finally {
	      setWorkloadBusy(false);
	    }
	  }

  function openAIPrompt(prompt: APIAIWorkflowPrompt) {
    setPromptKey(prompt.key);
    setPromptInstructions(prompt.instructions);
    setPromptOpen(true);
  }

  function replaceAIPrompt(value: APIAIWorkflowPrompt) {
    setAIPrompts((items) => items.some((item) => item.key === value.key)
      ? items.map((item) => item.key === value.key ? value : item)
      : [...items, value]);
  }

  async function handleAIPromptMutationError(error: unknown, fallback: string) {
    if (error instanceof APIError && error.status === 409 && promptKey) {
      try {
        const latest = await api.aiPrompts(product.id);
        setAIPrompts(latest);
        const current = latest.find((item) => item.key === promptKey);
        if (current) setPromptInstructions(current.instructions);
        showToast("This workflow changed elsewhere. The latest instructions are loaded; review them before saving again.");
        return;
      } catch {
        showToast("This workflow changed elsewhere. Reload AI configuration before trying again.");
        return;
      }
    }
    showToast(error instanceof APIError ? error.message : fallback);
  }

  async function saveAIPromptOverride() {
    const current = aiPrompts.find((item) => item.key === promptKey);
    if (!current || !promptInstructions.trim() || promptInstructions.trim() === current.instructions.trim()) return;
    setPromptBusy(true);
    try {
      const value = await api.saveAIPrompt(product.id, current.key, promptInstructions, current.revision);
      replaceAIPrompt(value);
      setPromptOpen(false);
      showToast(`${value.label} instructions saved as ${value.effective_version}.`);
    } catch (error) {
      await handleAIPromptMutationError(error, "Could not save workflow instructions.");
    } finally {
      setPromptBusy(false);
    }
  }

  async function resetAIPromptOverride() {
    const current = aiPrompts.find((item) => item.key === promptKey);
    if (!current || current.source === "default") return;
    if (!window.confirm(`Restore ${current.label} to its built-in default? The current custom instructions cannot be restored from the console.`)) return;
    setPromptBusy(true);
    try {
      const value = await api.resetAIPrompt(product.id, current.key, current.revision);
      replaceAIPrompt(value);
      setPromptInstructions(value.instructions);
      setPromptOpen(false);
      showToast(`${value.label} restored to ${value.default_version}.`);
    } catch (error) {
      await handleAIPromptMutationError(error, "Could not restore the default workflow instructions.");
    } finally {
      setPromptBusy(false);
    }
  }

  async function createRecipe(prompt: string, integrationID: string): Promise<APIRecipe | null> {
    setRecipeBusy(true);
    try {
      const value = await api.createRecipe(product.id, prompt, integrationID);
      setRecipes((items) => [value, ...items.filter((item) => item.id !== value.id)]);
      api.analyses(product.id).then(setAnalyses).catch(() => {});
      showToast("Recipe draft created from current product evidence.");
      return value;
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not create this recipe.");
      return null;
    } finally {
      setRecipeBusy(false);
    }
  }

  async function generateRecipesFromEvidence(integrationID: string) {
    setRecipeBusy(true);
    try {
      const scopedAnalyses = analyses.filter((candidate) => analysisMatchesIntegration(candidate, integrationID));
      const latestAnalysis = [...scopedAnalyses].sort((left, right) => right.created_at.localeCompare(left.created_at))[0];
      if (recipeAnalysisIsFreshlyRunning(latestAnalysis)) {
        showToast("Evidence analysis is still running. Try again when it is ready for review.");
        return;
      }
      const analysis = await api.analyseIntegration(product.id, integrationID);
      setAnalyses((items) => [analysis, ...items.filter((item) => item.id !== analysis.id)]);
      const unansweredBlocker = analysis.unknowns.find((unknown) => unknown.blocking);
      if (analysis.state !== "review" || unansweredBlocker) {
        showToast(unansweredBlocker
          ? `Resolve “${unansweredBlocker.question}” by attaching or configuring the required evidence, then rerun analysis.`
          : analysis.state === "failed"
            ? "Evidence analysis failed. Review the API evidence and try again."
            : "Evidence analysis is still running. Try again when it is ready for review.");
        return;
      }
      const generated = await api.generateRecipes(product.id, analysis.id, integrationID);
      setRecipes((items) => [...generated, ...items.filter((item) => !generated.some((candidate) => candidate.id === item.id))]);
      showToast(`${generated.length} grounded recipe${generated.length === 1 ? "" : "s"} generated for review.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Recipes could not be generated from the current evidence.");
    } finally {
      setRecipeBusy(false);
    }
  }

	  async function generateIntegrationSetupGuide(integrationID: string) {
		const analysis = await api.analyseIntegration(product.id, integrationID);
		setAnalyses((items) => [analysis, ...items.filter((item) => item.id !== analysis.id)]);
		return analysis;
	  }

	  async function reworkRecipe(recipe: APIRecipe, instruction: string) {
	    setRecipeBusy(true);
	    try {
	      const value = await api.reworkRecipe(product.id, recipe.id, instruction);
	      setRecipes((items) => items.map((item) => item.id === value.id ? value : item));
	      showToast("A new recipe revision is ready for review.");
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not rework this recipe.");
	    } finally {
	      setRecipeBusy(false);
	    }
	  }

	  async function editRecipe(recipe: APIRecipe, markdown: string, references: APIRecipeReference[], visibility: APIRecipe["visibility"]) {
	    setRecipeBusy(true);
	    try {
	      const value = await api.updateRecipe(product.id, recipe.id, markdown, references, visibility);
	      setRecipes((items) => items.map((item) => item.id === value.id ? value : item));
	      showToast("Human-authored recipe revision saved for review.");
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not save this recipe revision.");
	    } finally {
	      setRecipeBusy(false);
	    }
	  }

	  async function approveRecipe(recipe: APIRecipe) {
	    setRecipeBusy(true);
	    try {
	      const value = await api.approveRecipe(product.id, recipe.id);
	      setRecipes((items) => items.map((item) => item.id === value.id ? value : item));
	      showToast("Current recipe revision approved.");
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not approve this recipe.");
	    } finally {
	      setRecipeBusy(false);
	    }
	  }

	  async function publishRecipe(recipe: APIRecipe) {
	    setRecipeBusy(true);
	    try {
	      const value = await api.publishRecipe(product.id, recipe.id);
	      setRecipes((items) => items.map((item) => item.id === value.id ? value : item));
	      showToast("Recipe published to MCP resources.");
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not publish this recipe.");
	    } finally {
	      setRecipeBusy(false);
	    }
	  }

	  async function runSystemDoctor() {
    try {
      const value = await api.systemDoctor();
      const passing = value.checks.filter((check) => check.status === "ok").length;
      showToast(value.status === "ok" ? `System Doctor passed all ${passing} checks.` : `System Doctor found ${value.checks.length - passing} check(s) requiring attention.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "System Doctor could not run.");
    }
  }


  return {
    aiConnections, setAIConnections,
    aiProfiles, setAIProfiles,
    aiPrompts, setAIPrompts,
    analyses, setAnalyses,
    recipes, setRecipes,
    aiProviderUsage, setAIProviderUsage,
    recipeBusy, setRecipeBusy,
    workloadOpen, setWorkloadOpen,
    workloadBusy, setWorkloadBusy,
    workloadRole, setWorkloadRole,
    workloadConnectionID, setWorkloadConnectionID,
    providerOpen, setProviderOpen,
    providerPickerOpen, setProviderPickerOpen,
    providerBusy, setProviderBusy,
    providerEnabled, setProviderEnabled,
    providerIsBackup, setProviderIsBackup,
    providerBackupAnalysisModel, setProviderBackupAnalysisModel,
    providerKind, setProviderKind,
    providerEndpoint, setProviderEndpoint,
    workloadModel, setWorkloadModel,
    providerCredential, setProviderCredential,
    workloadInputTokens, setWorkloadInputTokens,
    workloadOutputTokens, setWorkloadOutputTokens,
    workloadDailyBudget, setWorkloadDailyBudget,
    workloadEnabled, setWorkloadEnabled,
    promptOpen, setPromptOpen,
    promptBusy, setPromptBusy,
    promptKey, setPromptKey,
    promptInstructions, setPromptInstructions,
    openAIConnection,
    openAIWorkload,
    changeAIWorkloadConnection,
    saveAIConnection,
    testAIConnection,
    saveAIWorkload,
    saveAIWorkloadSelection,
    openAIPrompt,
    saveAIPromptOverride,
    resetAIPromptOverride,
    createRecipe,
    generateRecipesFromEvidence,
    generateIntegrationSetupGuide,
    reworkRecipe,
    editRecipe,
    approveRecipe,
    publishRecipe,
    runSystemDoctor,
  };
}
