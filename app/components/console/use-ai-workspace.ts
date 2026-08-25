"use client";

import { useEffect, useState } from "react";

import { APIError, api } from "../../lib/api";
import type {
  APIAIProviderConnection,
  APIAIProviderUsage,
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
  recipeMatchesIntegration,
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
  const [analyses, setAnalyses] = useState<APIIntegrationAnalysis[]>([]);
  const [recipes, setRecipes] = useState<APIRecipe[]>([]);
  const [aiProviderUsage, setAIProviderUsage] = useState<APIAIProviderUsage[]>([]);
  const [recipeBusy, setRecipeBusy] = useState(false);
  const [llmOpen, setLLMOpen] = useState(false);
  const [llmBusy, setLLMBusy] = useState(false);
  const [llmRole, setLLMRole] = useState<AIWorkload>("analysis");
  const [llmConnectionID, setLLMConnectionID] = useState("");
  const [providerOpen, setProviderOpen] = useState(false);
  const [providerPickerOpen, setProviderPickerOpen] = useState(false);
  const [providerBusy, setProviderBusy] = useState(false);
  const [providerEnabled, setProviderEnabled] = useState(true);
  const [providerIsBackup, setProviderIsBackup] = useState(false);
  const [providerBackupAnalysisModel, setProviderBackupAnalysisModel] = useState(aiModelDefaults.openai.analysis);
  const [providerBackupAssistantModel, setProviderBackupAssistantModel] = useState(aiModelDefaults.openai.assistant);
  const [llmProvider, setLLMProvider] = useState<APIAIProviderConnection["provider"]>("openai");
  const [llmEndpoint, setLLMEndpoint] = useState(aiProviderOrigin("openai"));
  const [llmModel, setLLMModel] = useState(aiModelDefaults.openai.analysis);
  const [llmCredential, setLLMCredential] = useState("");
  const [llmInputTokens, setLLMInputTokens] = useState("8192");
  const [llmOutputTokens, setLLMOutputTokens] = useState("1024");
  const [llmDailyBudget, setLLMDailyBudget] = useState("1000000");
  const [llmEnabled, setLLMEnabled] = useState(false);

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
	    setLLMProvider(provider);
	    setLLMEndpoint(connection?.endpoint ?? aiProviderOrigin(provider));
	    setLLMCredential("");
	    setProviderEnabled(connection?.enabled ?? true);
	    setProviderIsBackup(connection?.is_backup ?? false);
	    setProviderBackupAnalysisModel(connection?.backup_models.analysis ?? aiModelDefaults[provider].analysis);
	    setProviderBackupAssistantModel(connection?.backup_models.assistant ?? aiModelDefaults[provider].assistant);
	    setProviderPickerOpen(false);
	    setProviderOpen(true);
	  }

	  function openLLMProfile(role: AIWorkload) {
	    const profile = aiProfiles.find((item) => item.workload === role);
	    const connection = aiConnections.find((item) => item.id === profile?.provider_connection_id) ?? aiConnections.find((item) => item.enabled && !item.is_backup);
	    setLLMRole(role);
	    setLLMConnectionID(connection?.id ?? "");
	    setLLMModel(profile?.model ?? (connection ? aiModelDefaults[connection.provider][role] : ""));
	    setLLMInputTokens(String(profile?.max_input_tokens ?? 128000));
	    setLLMOutputTokens(String(profile?.max_output_tokens ?? (role === "assistant" ? 1024 : 8192)));
	    setLLMDailyBudget(String(profile?.daily_token_budget ?? 0));
	    setLLMEnabled(profile?.enabled ?? false);
	    setLLMOpen(true);
	  }

	  function changeLLMConnection(connectionID: string) {
	    setLLMConnectionID(connectionID);
	    const connection = aiConnections.find((item) => item.id === connectionID);
	    if (connection) setLLMModel(aiModelDefaults[connection.provider][llmRole]);
	  }

	  async function saveAIConnection() {
	    setProviderBusy(true);
	    try {
	      const current = aiConnections.find((item) => item.provider === llmProvider);
	      const value = await api.saveAIConnection({ organisation_id: product.organisation_id, provider: llmProvider, endpoint: llmEndpoint, credential: llmCredential, enabled: providerEnabled, is_backup: providerIsBackup, backup_models: providerIsBackup ? { analysis: providerBackupAnalysisModel, assistant: providerBackupAssistantModel } : {}, revision: current?.revision ?? 0 });
	      setAIConnections((items) => [...items.filter((item) => item.id !== value.id && item.provider !== value.provider), value]);
	      setLLMCredential("");
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

	  async function saveLLMProfile() {
	    setLLMBusy(true);
	    try {
	      const current = aiProfiles.find((item) => item.workload === llmRole);
	      const value = await api.saveAIProfile(product.id, llmRole, { organisation_id: product.organisation_id, provider_connection_id: llmConnectionID, model: llmModel, max_input_tokens: Number(llmInputTokens), max_output_tokens: Number(llmOutputTokens), daily_token_budget: Number(llmDailyBudget), enabled: llmEnabled, revision: current?.revision ?? 0 });
	      setAIProfiles((items) => [...items.filter((item) => item.workload !== value.workload), value].sort((a, b) => a.workload.localeCompare(b.workload)));
	      setLLMOpen(false);
	      showToast(`${aiWorkloads.find((workload) => workload.role === value.workload)?.name ?? value.workload} workload saved.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not save AI model.");
    } finally {
      setLLMBusy(false);
    }
	  }

	  async function saveAIWorkloadSelection(role: AIWorkload, connectionID: string, modelID: string) {
	    setLLMBusy(true);
	    try {
	      const current = aiProfiles.find((item) => item.workload === role);
	      const value = await api.saveAIProfile(product.id, role, { organisation_id: product.organisation_id, provider_connection_id: connectionID, model: modelID, max_input_tokens: current?.max_input_tokens ?? 128000, max_output_tokens: current?.max_output_tokens ?? (role === "assistant" ? 1024 : 8192), daily_token_budget: current?.daily_token_budget ?? 0, enabled: true, revision: current?.revision ?? 0 });
	      setAIProfiles((items) => [...items.filter((item) => item.workload !== value.workload), value].sort((a, b) => a.workload.localeCompare(b.workload)));
	      showToast(`${aiWorkloads.find((workload) => workload.role === value.workload)?.name ?? value.workload} workload saved.`);
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not save AI model.");
	    } finally {
	      setLLMBusy(false);
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

	  async function generateRecipesFromEvidence(integrationID?: string) {
	    setRecipeBusy(true);
	    try {
	      const scopedAnalyses = analyses.filter((candidate) => analysisMatchesIntegration(candidate, integrationID));
	      const scopedRecipes = recipes.filter((candidate) => recipeMatchesIntegration(candidate, integrationID));
	      let analysis = [...scopedAnalyses].sort((left, right) => right.created_at.localeCompare(left.created_at))[0];
	      const runningSince = analysis?.state === "running" ? Date.parse(analysis.created_at) : Number.NaN;
	      const staleRunning = analysis?.state === "running" && (!Number.isFinite(runningSince) || Date.now() - runningSince > 5 * 60 * 1000);
	      const evidenceChanged = scopedRecipes.some((recipe) => recipe.state === "outdated");
	      if (!analysis || analysis.state === "failed" || staleRunning || evidenceChanged) {
	        analysis = await api.analyseIntegration(product.id, integrationID);
	        setAnalyses((items) => [analysis, ...items.filter((item) => item.id !== analysis.id)]);
	      }
	      const unansweredBlocker = analysis.unknowns.find((unknown) => unknown.blocking && !unknown.answer?.trim());
	      if (analysis.state !== "review" || unansweredBlocker) {
	        showToast(unansweredBlocker ? `Answer “${unansweredBlocker.question}” before generating recipes.` : "Evidence analysis is still running. Try again when it is ready for review.");
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

	  async function generateIntegrationAgentGuide(integrationID: string) {
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
    analyses, setAnalyses,
    recipes, setRecipes,
    aiProviderUsage, setAIProviderUsage,
    recipeBusy, setRecipeBusy,
    llmOpen, setLLMOpen,
    llmBusy, setLLMBusy,
    llmRole, setLLMRole,
    llmConnectionID, setLLMConnectionID,
    providerOpen, setProviderOpen,
    providerPickerOpen, setProviderPickerOpen,
    providerBusy, setProviderBusy,
    providerEnabled, setProviderEnabled,
    providerIsBackup, setProviderIsBackup,
    providerBackupAnalysisModel, setProviderBackupAnalysisModel,
    providerBackupAssistantModel, setProviderBackupAssistantModel,
    llmProvider, setLLMProvider,
    llmEndpoint, setLLMEndpoint,
    llmModel, setLLMModel,
    llmCredential, setLLMCredential,
    llmInputTokens, setLLMInputTokens,
    llmOutputTokens, setLLMOutputTokens,
    llmDailyBudget, setLLMDailyBudget,
    llmEnabled, setLLMEnabled,
    openAIConnection,
    openLLMProfile,
    changeLLMConnection,
    saveAIConnection,
    testAIConnection,
    saveLLMProfile,
    saveAIWorkloadSelection,
    createRecipe,
    generateRecipesFromEvidence,
    generateIntegrationAgentGuide,
    reworkRecipe,
    editRecipe,
    approveRecipe,
    publishRecipe,
    runSystemDoctor,
  };
}


