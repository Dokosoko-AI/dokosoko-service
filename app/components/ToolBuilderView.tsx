"use client";


import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  ArrowLeft,
  Bot,
  Check,
  CheckCircle2,
  KeyRound,
  Plus,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  TriangleAlert,
  Wrench,
  XCircle,
} from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import {
  api,
  boundedToolBuilderChatHistory,
  toolBuilderFollowUpDraft,
  TOOL_BUILDER_CHAT_LIMITS,
} from "../lib/api";
import type {
  APIGrantDefinition,
  APIIntegration,
  APIProduct,
  APIRuntimeSetup,
  APITool,
  APIToolAuthorizationPolicy,
  APIToolBuilderAnalysis,
  APIToolBuilderChatMessage,
  APIToolBuilderDraft,
  APIToolBuilderFinding,
  APIToolBuilderImportCandidate,
  APIToolBuilderImportKind,
  APIToolBuilderProposal,
  APIToolBuilderValidation,
  APIToolHTTPMethod,
  APIToolRequestMapping,
  APIToolRisk,
  APIToolUpstreamAuth,
  APIToolUpstreamAuthType,
} from "../lib/api";
import { integrationPath } from "../lib/console-routes";
import {
  apiToolHTTPPath,
  apiToolHTTPPathProblem,
  apiToolPersistenceContext,
  lockAPIToolBuilderDraft,
  runtimeLockForConnection,
} from "../lib/tool-builder-runtime-context";
import {
  toolCredentialBinding,
  toolCredentialBindingMatches,
  versionedResponseIsCurrent,
} from "../lib/tool-builder-safety";
import { Badge, Button } from "./core/control";
import { PageHeader, PanelHeader, SegmentedControl } from "./core/layout";
import {
  AUTH_TYPES,
  CREDENTIAL_AUTH_TYPES,
  HTTP_METHODS,
  IDENTIFIER_PATTERN,
  IMPORT_KINDS,
  RISKS,
  type ActiveProposal,
  type ProposalDecision,
  type ReviewChange,
  type ToolDraftForm,
  containsCredentialMaterial,
  draftForAssistance,
  draftToForm,
  endpointOrigin,
  errorMessage,
  formatReviewValue,
  localValidation,
  reviewChanges,
  sanitizeDraft,
  stableValue,
  toolDraftFromTool,
  utf8ByteLength,
} from "./tool-builder/model";

export type ToolBuilderMode = "ai" | "import" | "manual";

export type ToolBuilderViewProps = {
  product: APIProduct;
  grants: APIGrantDefinition[];
  tool?: APITool | null;
  initialProposal?: APIToolBuilderProposal | null;
  aiAvailable: boolean;
  initialMode?: ToolBuilderMode;
  apiContext?: { integration: APIIntegration; setup: APIRuntimeSetup };
  onNavigate: (path: string) => void;
  onMessage: (message: string) => void;
  onDirtyChange?: (dirty: boolean) => void;
  onSaved?: (tool: APITool) => void | Promise<void>;
};

function BuilderLink({ path, onNavigate, className, children }: { path: string; onNavigate: (path: string) => void; className?: string; children: ReactNode }) {
  return <a href={path} className={className} onClick={(event) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onNavigate(path);
  }}>{children}</a>;
}

function authTypeLabel(type: APIToolUpstreamAuthType, t: TFunction) {
  return type === "delegated_oauth" ? t("tools.delegatedOAuth")
    : type === "none" ? t("tools.noAuthentication")
      : type === "bearer" ? t("tools.bearerToken")
        : type === "authorization_scheme" ? t("tools.authorizationScheme")
          : type === "api_key_header" ? t("tools.apiKeyHeader")
            : type === "api_key_query" ? t("tools.apiKeyQueryParameter")
              : type === "basic" ? t("tools.httpBasic")
                : type === "oauth_client_credentials" ? t("tools.oauthClientCredentials")
                  : t("tools.customSecretHeader");
}

function authTypeDescription(type: APIToolUpstreamAuthType, t: TFunction) {
  return type === "delegated_oauth" ? t("tools.delegatedOAuthDescription")
    : type === "none" ? t("tools.noAuthenticationDescription")
      : type === "bearer" ? t("tools.bearerTokenDescription")
        : type === "authorization_scheme" ? t("tools.authorizationSchemeDescription")
          : type === "api_key_header" ? t("tools.apiKeyHeaderDescription")
            : type === "api_key_query" ? t("tools.apiKeyQueryParameterDescription")
              : type === "basic" ? t("tools.httpBasicDescription")
                : type === "oauth_client_credentials" ? t("tools.oauthClientCredentialsDescription")
                  : t("tools.customSecretHeaderDescription");
}

function importKindLabel(kind: APIToolBuilderImportKind, t: TFunction) {
  return kind === "openapi_document" ? t("toolBuilder.openAPIDocument")
    : kind === "postman" ? t("toolBuilder.postmanCollection")
      : t("toolBuilder.cURLCommand");
}

function reviewFieldLabel(field: ReviewChange["field"], apiScoped: boolean, t: TFunction) {
  if (apiScoped && field === "endpoint") return t("toolBuilder.relativePath");
  const keys = {
    namespace: "toolBuilder.namespace",
    name: "toolBuilder.toolName",
    description: "toolBuilder.purpose",
    http_method: "toolBuilder.httpMethod",
    endpoint: "toolBuilder.endpoint",
    timeout_ms: "toolBuilder.timeoutMs",
    input_schema: "toolBuilder.inputSchema",
    output_schema: "toolBuilder.outputSchema",
    upstream_auth: "toolBuilder.upstreamAuthentication",
    request_mapping: "toolBuilder.requestMapping",
    response_mapping: "toolBuilder.responseMapping",
    authorization_policy: "toolBuilder.authorizationPolicy",
    request_example: "toolBuilder.requestExample",
    response_example: "toolBuilder.responseExample",
  } as const;
  return t(keys[field]);
}

function findingMessage(finding: APIToolBuilderFinding, t: TFunction) {
  const findingValue = finding.message.match(/“([^”]*)”/)?.[1] ?? finding.message;
  const baseField = finding.field?.split(".")[0] ?? "";
  const field = baseField === "input_schema" ? t("toolBuilder.inputSchema")
    : baseField === "output_schema" ? t("toolBuilder.outputSchema")
      : baseField === "request_example" ? t("toolBuilder.requestExample")
        : baseField === "response_example" ? t("toolBuilder.responseExample")
          : baseField === "request_mapping" ? t("toolBuilder.requestMapping")
            : baseField === "upstream_auth" ? t("toolBuilder.upstreamAuthentication")
              : baseField === "credential" ? t("toolBuilder.credential")
                : t("toolBuilder.field");
  const messages: Partial<Record<string, string>> = {
    invalid_namespace: t("toolBuilder.findingInvalidIdentifier"),
    invalid_name: t("toolBuilder.findingInvalidIdentifier"),
    description_required: t("toolBuilder.findingDescriptionRequired"),
    description_too_long: t("toolBuilder.findingDescriptionTooLong"),
    invalid_timeout: t("toolBuilder.findingInvalidTimeout"),
    endpoint_required: t("toolBuilder.findingEndpointRequired"),
    https_required: t("toolBuilder.findingHTTPSRequired"),
    localhost_http_required: t("toolBuilder.findingLocalhostHTTPRequired"),
    default_https_port_required: t("toolBuilder.findingDefaultHTTPSPortRequired"),
    endpoint_credentials: t("toolBuilder.findingEndpointCredentials"),
    endpoint_query: t("toolBuilder.findingEndpointQuery"),
    endpoint_fragment: t("toolBuilder.findingEndpointFragment"),
    invalid_endpoint: t("toolBuilder.findingInvalidEndpoint"),
    object_required: t("toolBuilder.findingObjectRequired", { field }),
    invalid_json: t("toolBuilder.findingInvalidJSON", { field }),
    mapping_name_invalid: t("toolBuilder.findingMappingNameInvalid", { parameter: findingValue || t("toolBuilder.empty") }),
    path_parameter_missing: t("toolBuilder.findingPathParameterMissing", { parameter: findingValue }),
    get_body_mapping: t("toolBuilder.findingGetBodyMapping", { parameter: findingValue }),
    authorization_scheme_required: t("toolBuilder.findingAuthorizationSchemeRequired"),
    header_name_required: t("toolBuilder.findingHeaderNameRequired"),
    query_name_required: t("toolBuilder.findingQueryNameRequired"),
    username_required: t("toolBuilder.findingUsernameRequired"),
    client_id_required: t("toolBuilder.findingClientIDRequired"),
    token_url_required: t("toolBuilder.findingTokenURLRequired"),
    invalid_token_url: t("toolBuilder.findingInvalidTokenURL"),
    credential_required: t("toolBuilder.findingCredentialRequired"),
    unknown_grant: t("toolBuilder.findingUnknownGrant", { grant: findingValue }),
    deprecated_grant: t("toolBuilder.findingDeprecatedGrant", { grant: findingValue }),
    critical_confirmation: t("toolBuilder.findingCriticalConfirmation"),
    mutation_without_idempotency: t("toolBuilder.findingMutationWithoutIdempotency"),
    runtime_connection_required: t("toolBuilder.findingRuntimeConnectionRequired"),
    invalid_http_path: t("toolBuilder.findingInvalidHTTPPath"),
  };
  return messages[finding.code] ?? finding.message;
}

function FindingList({ findings, onOpen }: { findings: APIToolBuilderFinding[]; onOpen: (field?: string) => void }) {
  const { t } = useTranslation();
  if (findings.length === 0) return <div className="tool-builder-ready"><CheckCircle2 /><span><strong>{t("toolBuilder.noFindings")}</strong><small>{t("toolBuilder.theCurrentFieldsPassAvailableChecks")}</small></span></div>;
  return <div className="tool-builder-findings">{findings.map((finding, index) => <button type="button" className={`tool-builder-finding ${finding.level}`} key={`${finding.code}:${finding.field ?? "general"}:${index}`} onClick={() => onOpen(finding.field)}><span>{finding.level === "error" ? <XCircle /> : finding.level === "warning" ? <TriangleAlert /> : <CheckCircle2 />}</span><span><strong>{t("toolBuilder.validationFinding")} <code>{finding.code}</code></strong><small>{findingMessage(finding, t)}</small></span></button>)}</div>;
}

export function ToolBuilderView({ product, grants, tool = null, initialProposal = null, aiAvailable, initialMode = "ai", apiContext, onNavigate, onMessage, onDirtyChange, onSaved }: ToolBuilderViewProps) {
  const { t } = useTranslation();
  const generatedID = useId().replaceAll(":", "");
  const apiScoped = Boolean(apiContext);
  const runtimeConnections = apiContext?.setup.endpoint_bindings ?? [];
  const initialRuntimeConnectionID = tool?.runtime_service_connection_id ?? runtimeConnections[0]?.id ?? "";
  const initialRuntimeConnection = runtimeConnections.find((connection) => connection.id === initialRuntimeConnectionID);
  const initialRuntimeLock = apiContext ? runtimeLockForConnection(apiContext.setup, initialRuntimeConnection) : null;
  const unscopedInitialCanonical = useMemo(() => {
    const draft = toolDraftFromTool(tool);
    if (!tool && apiContext) {
      const namespace = apiContext.integration.family_key.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");
      return { ...draft, namespace: IDENTIFIER_PATTERN.test(namespace) ? namespace : "api" };
    }
    return draft;
  }, [apiContext, tool]);
  const initialHTTPPath = tool?.http_path || apiToolHTTPPath(unscopedInitialCanonical.endpoint, "/");
  const initialCanonical = useMemo(() => initialRuntimeLock
    ? lockAPIToolBuilderDraft(unscopedInitialCanonical, initialRuntimeLock, initialHTTPPath)
    : unscopedInitialCanonical,
  [initialHTTPPath, initialRuntimeLock, unscopedInitialCanonical]);
  const initialForm = useMemo(() => draftToForm(initialCanonical), [initialCanonical]);
  const editing = Boolean(tool);
  const seededProposal = useMemo<ActiveProposal | null>(() => {
    if (!initialProposal) return null;
    const sanitized = sanitizeDraft(initialProposal.draft, initialCanonical);
    const proposalPath = apiScoped ? apiToolHTTPPath(sanitized.endpoint, initialHTTPPath) : "";
    const draft = initialRuntimeLock ? lockAPIToolBuilderDraft(sanitized, initialRuntimeLock, proposalPath) : sanitized;
    const changes = reviewChanges(initialCanonical, draft, initialProposal.changes ?? [], editing, apiScoped);
    return {
      source: "live-test",
      summary: initialProposal.summary || initialProposal.reply || t("toolBuilder.suggestedFromConsentedSanitizedLiveTestEvidence"),
      draft,
      changes,
      findings: initialProposal.findings ?? [],
    };
  }, [apiScoped, editing, initialCanonical, initialHTTPPath, initialProposal, initialRuntimeLock, t]);
  const [form, setForm] = useState<ToolDraftForm>(initialForm);
  const [runtimeConnectionID, setRuntimeConnectionID] = useState(initialRuntimeConnectionID);
  const [runtimeHTTPPath, setRuntimeHTTPPath] = useState(initialHTTPPath);
  const [credential, setCredential] = useState("");
  const [credentialBinding, setCredentialBinding] = useState("");
  const [mode, setMode] = useState<ToolBuilderMode>(initialMode);
  const [instruction, setInstruction] = useState("");
  const [chatHistory, setChatHistory] = useState<APIToolBuilderChatMessage[]>([]);
  const [importKind, setImportKind] = useState<APIToolBuilderImportKind>("openapi_document");
  const [importSource, setImportSource] = useState("");
  const [importCandidates, setImportCandidates] = useState<APIToolBuilderImportCandidate[]>([]);
  const [proposal, setProposal] = useState<ActiveProposal | null>(seededProposal);
  const [proposalDecisions, setProposalDecisions] = useState<Record<string, ProposalDecision>>({});
  const [proposalStale, setProposalStale] = useState(false);
  const [validation, setValidation] = useState<APIToolBuilderValidation | null>(null);
  const [analysis, setAnalysis] = useState<APIToolBuilderAnalysis | null>(null);
  const [busy, setBusy] = useState<"propose" | "import" | "validate" | "analyse" | "save" | null>(null);
  const [status, setStatus] = useState(seededProposal ? t("toolBuilder.liveTestProposalReady", { count: seededProposal.changes.length }) : "");
  const proposalHeadingRef = useRef<HTMLHeadingElement>(null);
  // A seeded proposal is a distinct in-memory draft state. Subsequent request
  // guards compare this monotonically increasing version before using results.
  const draftVersionRef = useRef(seededProposal ? 1 : 0);
  const importInputVersionRef = useRef(0);
  const editable = !tool || tool.state === "draft";
  const runtimeConnection = runtimeConnections.find((connection) => connection.id === runtimeConnectionID);
  const runtimeLock = apiContext ? runtimeLockForConnection(apiContext.setup, runtimeConnection) : null;
  const runtimePathProblem = apiScoped ? apiToolHTTPPathProblem(runtimeHTTPPath) : "";
  const contextualForm = useMemo(() => runtimeLock
    ? draftToForm(lockAPIToolBuilderDraft(draftForAssistance(form), runtimeLock, runtimeHTTPPath))
    : form,
  [form, runtimeHTTPPath, runtimeLock]);
  const storedCredentialReusable = apiScoped || !tool || !form.credential_present || !CREDENTIAL_AUTH_TYPES.has(form.upstream_auth.type) || (
    endpointOrigin(form.endpoint) === endpointOrigin(initialForm.endpoint)
    && stableValue(form.upstream_auth) === stableValue(initialForm.upstream_auth)
  );
  const enteredCredentialReusable = !credential || toolCredentialBindingMatches(
    credentialBinding,
    form.endpoint,
    form.upstream_auth,
  );
  const validationForm = useMemo(() => storedCredentialReusable ? contextualForm : { ...contextualForm, credential_present: false }, [contextualForm, storedCredentialReusable]);
  const validationCredential = enteredCredentialReusable ? credential : "";
  const local = useMemo(() => {
    const result = localValidation(validationForm, grants, validationCredential);
    if (!apiScoped) return result;
    const runtimeFindings: APIToolBuilderFinding[] = [
      ...(!runtimeLock ? [{ level: "error" as const, code: "runtime_connection_required", field: "runtime_service_connection_id", message: t("toolBuilder.findingRuntimeConnectionRequired") }] : []),
      ...(runtimePathProblem ? [{ level: "error" as const, code: "invalid_http_path", field: "endpoint", message: runtimePathProblem }] : []),
    ];
    return { draft: runtimeFindings.length > 0 ? null : result.draft, findings: [...result.findings, ...runtimeFindings] };
  }, [apiScoped, grants, runtimeLock, runtimePathProblem, t, validationCredential, validationForm]);
  const assistanceDraft = useMemo(() => draftForAssistance(validationForm), [validationForm]);
  const followUpDraft = useMemo(() => toolBuilderFollowUpDraft(
    assistanceDraft,
    proposal?.draft ?? null,
    proposalDecisions,
    proposalStale,
    editing,
  ), [assistanceDraft, editing, proposal, proposalDecisions, proposalStale]);
  const instructionBytes = useMemo(() => utf8ByteLength(instruction.trim()), [instruction]);
  const instructionProblem = useMemo(() => {
    if (containsCredentialMaterial(instruction)) return "Remove credential-like material and enter it only in the separate credential field.";
    if (instructionBytes > TOOL_BUILDER_CHAT_LIMITS.maxMessageBytes) return `Keep each message within ${TOOL_BUILDER_CHAT_LIMITS.maxMessageBytes} UTF-8 bytes.`;
    return "";
  }, [instruction, instructionBytes]);
  const dirty = useMemo(() => stableValue(form) !== stableValue(initialForm) || Boolean(credential) || runtimeConnectionID !== initialRuntimeConnectionID || runtimeHTTPPath !== initialHTTPPath, [credential, form, initialForm, initialHTTPPath, initialRuntimeConnectionID, runtimeConnectionID, runtimeHTTPPath]);
  const findings = useMemo(() => {
    const combined = [...local.findings, ...(validation?.findings ?? [])];
    const seen = new Set<string>();
    return combined.filter((finding) => {
      const key = `${finding.level}:${finding.code}:${finding.field ?? ""}:${finding.message}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  }, [local.findings, validation]);
  const errors = findings.filter((finding) => finding.level === "error");
  const warnings = findings.filter((finding) => finding.level === "warning");

  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  useEffect(() => {
    onDirtyChange?.(dirty);
    return () => onDirtyChange?.(false);
  }, [dirty, onDirtyChange]);

  function domID(field: string) {
    return `${generatedID}-tool-${field.replace(/[^a-z0-9_-]/gi, "-")}`;
  }

  function focusID(field: string) {
    const root = field === "credential_present" ? "credential" : field.split(".", 1)[0];
    return domID(root);
  }

  function describedBy(field: string, helpID?: string) {
    const hasFinding = findings.some((finding) => finding.field === field || finding.field?.startsWith(`${field}.`) || (field === "credential" && finding.field === "credential_present"));
    return [helpID, hasFinding ? `${domID(field)}-finding` : ""].filter(Boolean).join(" ") || undefined;
  }

  function fieldFindings(field: string) {
    return findings.filter((finding) => finding.field === field || finding.field?.startsWith(`${field}.`) || (field === "credential" && finding.field === "credential_present"));
  }

  function renderFieldFindings(field: string) {
    const values = fieldFindings(field);
    if (values.length === 0) return null;
    return <span className="tool-builder-inline-findings" id={`${domID(field)}-finding`}>{values.map((finding) => <small className={finding.level} key={`${finding.code}:${finding.message}`}>{findingMessage(finding, t)}</small>)}</span>;
  }

  function markDraftChanged(updater: (current: ToolDraftForm) => ToolDraftForm) {
    if (busy === "save") return;
    const nextForm = updater(form);
    draftVersionRef.current += 1;
    setForm(nextForm);
    if (credential && !toolCredentialBindingMatches(credentialBinding, nextForm.endpoint, nextForm.upstream_auth)) {
      setCredential("");
      setCredentialBinding("");
      setStatus(t("toolBuilder.theEnteredCredentialWasClearedBecauseItsDestinationOr"));
    }
    setValidation(null);
    setAnalysis(null);
    if (proposal) setProposalStale(true);
  }

  function acceptCurrentDraftResponse(version: number, label: string) {
    if (draftVersionRef.current === version) return true;
    setStatus(t("toolBuilder.resultDiscardedBecauseTheDraftOrCredentialChangedWhile", { label: String(label) }));
    return false;
  }

  function acceptImportResponse(draftVersion: number, inputVersion: number) {
    if (!acceptCurrentDraftResponse(draftVersion, "Import")) return false;
    if (versionedResponseIsCurrent(inputVersion, importInputVersionRef.current)) return true;
    setStatus(t("toolBuilder.importResultDiscardedBecauseTheSourceOrImportFormat"));
    return false;
  }

  function builderContext(draft: APIToolBuilderDraft) {
    const contextDraft = runtimeLock
      ? lockAPIToolBuilderDraft(draft, runtimeLock, apiToolHTTPPath(draft.endpoint, runtimeHTTPPath))
      : draft;
    return {
      draft: contextDraft,
      credential_will_be_supplied: apiScoped ? false : Boolean(validationCredential.trim()),
      ...(tool ? { base_tool_id: tool.id, base_revision: tool.revision } : {}),
    };
  }

  function openFinding(field?: string) {
    if (!field) return;
    const target = document.getElementById(focusID(field));
    target?.focus();
    target?.scrollIntoView({ behavior: "smooth", block: "center" });
  }

  function setActiveProposal(source: "ai" | "import", value: APIToolBuilderProposal | APIToolBuilderImportCandidate, reviewBase: APIToolBuilderDraft, candidateFallback = reviewBase) {
    const sanitized = sanitizeDraft(value.draft, candidateFallback);
    const proposalPath = apiScoped ? apiToolHTTPPath(sanitized.endpoint, runtimeHTTPPath) : "";
    const proposedDraft = runtimeLock ? lockAPIToolBuilderDraft(sanitized, runtimeLock, proposalPath) : sanitized;
    const lockedReviewBase = runtimeLock ? lockAPIToolBuilderDraft(reviewBase, runtimeLock, runtimeHTTPPath) : reviewBase;
    const changes = reviewChanges(lockedReviewBase, proposedDraft, value.changes ?? [], editing, apiScoped);
    if (source === "ai" && changes.length === 0) {
      // A newer assistant turn without field changes supersedes any older
      // proposal. Leaving the prior diff actionable would make the visible
      // conversation and the review state disagree.
      setProposal(null);
      setProposalDecisions({});
      setProposalStale(false);
      return 0;
    }
    setProposal({
      source,
      summary: value.summary || (source === "ai" && "reply" in value ? value.reply : "") || t("toolBuilder.proposedFieldChanges", { count: changes.length }),
      draft: proposedDraft,
      changes,
      findings: value.findings ?? [],
    });
    setProposalDecisions({});
    setProposalStale(false);
    setStatus(t("toolBuilder.proposalReadyWithChanges", { count: changes.length, source: source === "ai" ? t("toolBuilder.ai") : t("toolBuilder.import") }));
    requestAnimationFrame(() => proposalHeadingRef.current?.focus());
    return changes.length;
  }

  async function proposeDraft() {
    const userMessage = instruction.trim();
    if (!userMessage || !aiAvailable) return;
    if (instructionProblem) {
      const message = instructionProblem;
      setStatus(message);
      onMessage(message);
      return;
    }
    const history = boundedToolBuilderChatHistory(chatHistory);
    const requestVersion = draftVersionRef.current;
    setBusy("propose");
    setStatus(proposal && !proposalStale ? t("toolBuilder.sendingACredentialFreeFollowUpAboutThePending") : t("toolBuilder.sendingACredentialFreeMessageToTheAssistant"));
    try {
      const result = await api.proposeToolDraft(product.id, { ...builderContext(followUpDraft), instruction: userMessage, history });
      if (!acceptCurrentDraftResponse(requestVersion, t("toolBuilder.assistant"))) return;
      const assistantMessage = (result.reply || result.summary || t("toolBuilder.assistantNoFurtherDetails")).trim();
      setChatHistory(boundedToolBuilderChatHistory([...history, { role: "user", content: userMessage }, { role: "assistant", content: assistantMessage }]));
      setInstruction("");
      const changeCount = setActiveProposal("ai", result, assistanceDraft, followUpDraft);
      if (changeCount === 0) setStatus(t("toolBuilder.assistantRepliedWithoutProposingFieldChangesTheDraftRemains"));
    } catch (error) {
      if (!acceptCurrentDraftResponse(requestVersion, t("toolBuilder.assistant"))) return;
      const message = errorMessage(error, t("toolBuilder.requestCouldNotBeCompleted"));
      setStatus(t("toolBuilder.proposalFailed", { message: String(message) }));
      onMessage(message);
    } finally {
      setBusy(null);
    }
  }

  async function importDraft() {
    if (!importSource.trim()) return;
    const requestVersion = draftVersionRef.current;
    const importInputVersion = importInputVersionRef.current;
    const source = importSource.trim();
    const kind = importKind;
    setBusy("import");
    setStatus(t("toolBuilder.inspectingTheImportAsUntrustedInput"));
    try {
      // Import is a valid starting point for a brand-new tool, so use the
      // canonical assistance draft while the empty manual form has expected
      // validation errors. Candidates remain proposals and must still pass
      // complete validation before they can be saved.
      const result = await api.importToolDraft(product.id, { ...builderContext(assistanceDraft), source: { kind, value: source } });
      if (!acceptImportResponse(requestVersion, importInputVersion)) return;
      const importFindings = result.findings ?? [];
      const sourceFindings = importFindings.filter((finding) => finding.field === "source");
      const candidates = result.candidates.map((candidate) => ({
        ...candidate,
        findings: [...(candidate.findings ?? []), ...sourceFindings],
      }));
      // The raw source may have contained a credential that the server stripped.
      // Retain only normalized candidates and findings after a successful parse.
      setImportSource("");
      importInputVersionRef.current += 1;
      setImportCandidates(candidates);
      setValidation({ valid: !importFindings.some((finding) => finding.level === "error"), network_call_performed: false, findings: importFindings });
      if (candidates.length === 1) setActiveProposal("import", candidates[0], assistanceDraft);
      else setStatus(t("toolBuilder.importCandidatesAreReadyForReview", { length: String(candidates.length) }));
    } catch (error) {
      if (!acceptImportResponse(requestVersion, importInputVersion)) return;
      const message = errorMessage(error, t("toolBuilder.requestCouldNotBeCompleted"));
      setStatus(t("toolBuilder.importFailed", { message: String(message) }));
      onMessage(message);
    } finally {
      setBusy(null);
    }
  }

  async function analyseDraft() {
    if (!local.draft || !aiAvailable) return;
    const requestVersion = draftVersionRef.current;
    setBusy("analyse");
    setStatus(t("toolBuilder.analyzingTheCredentialFreeDraft"));
    try {
      const result = await api.analyseToolDraft(product.id, builderContext(local.draft));
      if (!acceptCurrentDraftResponse(requestVersion, t("toolBuilder.analysis"))) return;
      if (result.network_call_performed !== false) throw new Error(t("toolBuilder.analysisDidNotConfirmThatUpstreamExecutionWasDisabled"));
      setAnalysis(result);
      setStatus(t("toolBuilder.draftAnalysisComplete"));
    } catch (error) {
      if (!acceptCurrentDraftResponse(requestVersion, t("toolBuilder.analysis"))) return;
      const message = errorMessage(error, t("toolBuilder.requestCouldNotBeCompleted"));
      setStatus(t("toolBuilder.analysisFailed", { message: String(message) }));
      onMessage(message);
    } finally {
      setBusy(null);
    }
  }

  async function validateDraft() {
    if (!local.draft) {
      setStatus(t("toolBuilder.resolveTheLocalErrorsBeforeCheckingTheDraft"));
      openFinding(local.findings.find((finding) => finding.level === "error")?.field);
      return null;
    }
    const requestVersion = draftVersionRef.current;
    setBusy("validate");
    setStatus(t("toolBuilder.checkingTheDraftWithoutCallingTheUpstreamEndpoint"));
    try {
      const result = await api.validateToolDraft(product.id, builderContext(local.draft));
      if (!acceptCurrentDraftResponse(requestVersion, t("toolBuilder.validation"))) return null;
      if (result.network_call_performed !== false) throw new Error(t("toolBuilder.validationDidNotConfirmThatUpstreamExecutionWasDisabled"));
      setValidation(result);
      setStatus(result.valid ? t("toolBuilder.draftValidationPassedWithoutAnUpstreamCall") : t("toolBuilder.draftValidationFoundIssuesToResolve"));
      if (!result.valid) openFinding(result.findings.find((finding) => finding.level === "error")?.field);
      return result;
    } catch (error) {
      if (!acceptCurrentDraftResponse(requestVersion, t("toolBuilder.validation"))) return null;
      const message = errorMessage(error, t("toolBuilder.requestCouldNotBeCompleted"));
      setStatus(t("toolBuilder.validationFailed", { message: String(message) }));
      onMessage(message);
      return null;
    } finally {
      setBusy(null);
    }
  }

  async function saveDraft(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editable || !local.draft || busy) {
      openFinding(local.findings.find((finding) => finding.level === "error")?.field);
      return;
    }
    setBusy("save");
    setStatus(t("toolBuilder.validatingBeforeSaving"));
    try {
      const checked = await api.validateToolDraft(product.id, builderContext(local.draft));
      if (checked.network_call_performed !== false) throw new Error(t("toolBuilder.validationDidNotConfirmThatUpstreamExecutionWasDisabled"));
      setValidation(checked);
      if (!checked.valid || checked.findings.some((finding) => finding.level === "error")) {
        setStatus(t("toolBuilder.resolveTheValidationErrorsBeforeSaving"));
        openFinding(checked.findings.find((finding) => finding.level === "error")?.field);
        return;
      }
      const commonPersistence = {
        endpoint: local.draft.endpoint,
        upstream_auth: local.draft.upstream_auth,
        ...(CREDENTIAL_AUTH_TYPES.has(local.draft.upstream_auth.type) && enteredCredentialReusable && credential.trim() ? { credential: credential } : {}),
      };
      const runtimeContext = runtimeLock ? apiToolPersistenceContext(runtimeLock, runtimeHTTPPath) : null;
      const runtimePersistence = runtimeContext ? {
        runtime_service_connection_id: runtimeContext.runtime_service_connection_id,
        http_path: runtimeContext.http_path,
      } : null;
      const persisted = {
        description: local.draft.description,
        http_method: local.draft.http_method,
        timeout_ms: local.draft.timeout_ms,
        input_schema: local.draft.input_schema,
        output_schema: local.draft.output_schema,
        request_mapping: local.draft.request_mapping,
        response_mapping: local.draft.response_mapping,
        authorization_policy: local.draft.authorization_policy,
        request_example: local.draft.request_example ?? null,
        response_example: local.draft.response_example ?? null,
        ...(runtimePersistence ?? commonPersistence),
      };
      const saved = tool
        ? await api.updateTool(product.id, tool.id, { ...persisted, revision: tool.revision })
        : await api.createTool(product.id, { ...persisted, ...(runtimeContext ?? { scope: "common" as const }), organisation_id: product.organisation_id, namespace: local.draft.namespace, name: local.draft.name });
      setCredentialBinding("");
      setCredential("");
      setStatus(t("toolBuilder.draftSaved"));
      onMessage(t("toolBuilder.savedAsADraft", { namespace: String(saved.namespace), name: String(saved.name) }));
      await onSaved?.(saved);
      onDirtyChange?.(false);
      onNavigate(`/tool/${encodeURIComponent(saved.id)}`);
    } catch (error) {
      const message = errorMessage(error, t("toolBuilder.requestCouldNotBeCompleted"));
      setStatus(t("toolBuilder.saveFailed", { message: String(message) }));
      onMessage(message);
    } finally {
      setBusy(null);
    }
  }

  function applyProposalField(change: ReviewChange) {
    if (!proposal || proposalStale) return;
    if (busy === "save") return;
    const proposedForm = draftToForm(proposal.draft);
    draftVersionRef.current += 1;
    const nextForm: ToolDraftForm = (() => {
      switch (change.field) {
        case "endpoint":
          if (apiScoped) return form;
          return { ...form, endpoint: proposedForm.endpoint };
        case "input_schema": return { ...form, input_schema_text: proposedForm.input_schema_text };
        case "output_schema": return { ...form, output_schema_text: proposedForm.output_schema_text };
        case "request_example": return { ...form, request_example_text: proposedForm.request_example_text };
        case "response_example": return { ...form, response_example_text: proposedForm.response_example_text };
        default: return { ...form, [change.field]: proposedForm[change.field] };
      }
    })();
    if (apiScoped && change.field === "endpoint") setRuntimeHTTPPath(apiToolHTTPPath(proposal.draft.endpoint, runtimeHTTPPath));
    const credentialCleared = Boolean(credential && !toolCredentialBindingMatches(credentialBinding, nextForm.endpoint, nextForm.upstream_auth));
    setForm(nextForm);
    if (credentialCleared) {
      setCredential("");
      setCredentialBinding("");
    }
    setValidation(null);
    setAnalysis(null);
    setProposalDecisions((current) => ({ ...current, [change.field]: "accepted" }));
    setStatus(credentialCleared
      ? t("toolBuilder.acceptedAndCredentialCleared", { label: reviewFieldLabel(change.field, apiScoped, t) })
      : t("toolBuilder.acceptedTheDraftHasNotBeenSaved", { label: reviewFieldLabel(change.field, apiScoped, t), value2: "" }));
  }

  function rejectProposalField(change: ReviewChange) {
    if (busy === "save") return;
    setProposalDecisions((current) => ({ ...current, [change.field]: "rejected" }));
    setStatus(t("toolBuilder.keptUnchanged", { label: reviewFieldLabel(change.field, apiScoped, t) }));
  }

  function updateAuth(next: Partial<APIToolUpstreamAuth>) {
    markDraftChanged((current) => ({ ...current, upstream_auth: { ...current.upstream_auth, ...next } }));
  }

  function updatePolicy(next: Partial<APIToolAuthorizationPolicy>) {
    markDraftChanged((current) => {
      const policy = { ...current.authorization_policy, ...next };
      if (policy.risk === "critical") policy.confirmation_required = true;
      return { ...current, authorization_policy: policy };
    });
  }

  function toggleGrant(key: string) {
    const selected = form.authorization_policy.required_grants.includes(key);
    updatePolicy({ required_grants: selected ? form.authorization_policy.required_grants.filter((item) => item !== key) : [...form.authorization_policy.required_grants, key] });
  }

  function addRequestMapping() {
    let index = Object.keys(form.request_mapping.parameter_locations).length + 1;
    let name = `parameter_${index}`;
    while (name in form.request_mapping.parameter_locations) name = `parameter_${++index}`;
    markDraftChanged((current) => ({ ...current, request_mapping: { parameter_locations: { ...current.request_mapping.parameter_locations, [name]: "body" } } }));
  }

  function renameRequestMapping(previous: string, next: string) {
    markDraftChanged((current) => {
      const values = { ...current.request_mapping.parameter_locations };
      const location = values[previous];
      delete values[previous];
      values[next] = location;
      return { ...current, request_mapping: { parameter_locations: values } };
    });
  }

  function setRequestMappingLocation(parameter: string, location: APIToolRequestMapping["parameter_locations"][string]) {
    markDraftChanged((current) => ({ ...current, request_mapping: { parameter_locations: { ...current.request_mapping.parameter_locations, [parameter]: location } } }));
  }

  function removeRequestMapping(parameter: string) {
    markDraftChanged((current) => {
      const values = { ...current.request_mapping.parameter_locations };
      delete values[parameter];
      return { ...current, request_mapping: { parameter_locations: values } };
    });
  }

  function changeRuntimeConnection(nextConnectionID: string) {
    if (editing || busy === "save") return;
    draftVersionRef.current += 1;
    setRuntimeConnectionID(nextConnectionID);
    setValidation(null);
    setAnalysis(null);
    if (proposal) setProposalStale(true);
  }

  function changeRuntimeHTTPPath(nextPath: string) {
    if (busy === "save") return;
    draftVersionRef.current += 1;
    setRuntimeHTTPPath(nextPath);
    setValidation(null);
    setAnalysis(null);
    if (proposal) setProposalStale(true);
  }

  const activeAuth = contextualForm.upstream_auth.type;
  const credentialLabel = form.upstream_auth.type === "basic" ? t("tools.password") : form.upstream_auth.type === "oauth_client_credentials" ? t("tools.clientSecret") : form.upstream_auth.type === "bearer" ? t("tools.bearerToken") : t("tools.secretValue");
  const saveDisabled = Boolean(busy) || !editable || errors.length > 0;
  const formLocked = !editable || busy === "save";

  return <form className="tool-builder" aria-busy={Boolean(busy)} onSubmit={saveDraft} noValidate>
    <div className="tool-builder-breadcrumb"><BuilderLink path={apiContext ? integrationPath(apiContext.integration.id, "tools") : "/tools"} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{apiContext ? t("toolBuilder.backToTools", { display_name: String(apiContext.integration.display_name) }) : t("toolBuilder.backToTools2")}</BuilderLink><code>{apiContext ? `/integration/${apiContext.integration.id}/tools/${editing ? tool?.id : "new"}` : editing ? `/tool/${tool?.id}` : "/tools/new"}</code></div>
    <PageHeader
      eyebrow={apiContext ? t("toolBuilder.apiTool", { display_name: String(apiContext.integration.display_name) }) : editing ? t("toolBuilder.editCommonTool") : t("toolBuilder.newCommonTool")}
      title={editing ? t("toolBuilder.copy", { namespace: String(tool?.namespace), name: String(tool?.name) }) : apiContext ? t("toolBuilder.buildAnAPITool") : t("toolBuilder.buildACustomTool")}
      description={apiContext ? t("toolBuilder.turnOneOperationFromThisAPIIntoAnAgent") : t("toolBuilder.defineOneReusableAgentFacingHTTPCapabilityAIAnd")}
      action={<span className="heading-actions tool-builder-heading-actions"><Badge color={editable ? "amber" : "zinc"}>{tool?.state ?? t("toolBuilder.unsaved")}</Badge><Button color="indigo" type="submit" disabled={saveDisabled}>{busy === "save" ? t("common.saving") : t("toolBuilder.saveDraft")}</Button></span>}
    />

    {!editable && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>{t("toolBuilder.thisRevisionIsNotEditable")}</strong><small>{t("toolBuilder.publishedAndRetiredToolsAreImmutableCreateANew")}</small></span></div>}

    {apiContext && <div className="notice"><ShieldCheck /><span><strong>{t("toolBuilder.apiOwnedExecutionBoundary")}</strong> {t("toolBuilder.thisToolStaysWith")} {apiContext.integration.display_name}{t("toolBuilder.endpointHostsAuthenticationAndCredentialsAreInheritedFromAccess")}</span></div>}

    {tool?.endpoint_requires_review && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>{t("toolBuilder.theStoredEndpointContainedUnsafeURLMetadata")}</strong><small>{t("toolBuilder.itsQueryUserInformationOrFragmentWasRedactedEnter")}</small></span></div>}

    <div className="tool-builder-mode-row">
      <span><strong>{t("toolBuilder.startFrom")}</strong><small>{t("toolBuilder.everyModeEditsTheSameUnsavedDraft")}</small></span>
      <SegmentedControl label={t("toolBuilder.toolBuilderMode")} items={[{ id: "ai", label: t("common.aiAssist") }, { id: "import", label: t("common.import") }, { id: "manual", label: t("common.manual") }]} value={mode} onChange={setMode} />
    </div>

    <p className="sr-only" role="status" aria-live="polite" aria-atomic="true">{status}</p>

    <div className="tool-builder-layout">
      <div className="tool-builder-main">
        {mode === "ai" && <section className="panel tool-builder-source-panel" aria-labelledby={`${generatedID}-ai-title`}>
          <PanelHeader title={<span id={`${generatedID}-ai-title`}><Sparkles />{t("toolBuilder.aiAssistedDraft")}</span>} description={t("toolBuilder.askQuestionsAndRefineTheCapabilityOverSeveralTurns")} action={<Badge color="violet">{t("toolBuilder.proposalOnly")}</Badge>} />
          {aiAvailable ? <div className="tool-builder-chat-shell">
            <ol className="tool-builder-chat-transcript" role="log" aria-label={t("toolBuilder.conversationWithTheToolBuilderAssistant")} aria-live="polite" aria-relevant="additions">
              {chatHistory.map((message, index) => <li className={`tool-builder-chat-message ${message.role}`} key={`${index}:${message.role}:${message.content}`}><span className="tool-builder-chat-avatar" aria-hidden="true">{message.role === "assistant" ? <Bot /> : <Sparkles />}</span><span><strong>{message.role === "assistant" ? t("toolBuilder.assistant") : t("toolBuilder.you")}</strong><p>{message.content}</p></span></li>)}
            </ol>
            {chatHistory.length === 0 && <div className="tool-builder-chat-empty"><Bot /><span><strong>{t("toolBuilder.startWithAnOutcomeOrAQuestion")}</strong><small>{t("toolBuilder.theAssistantCanAskForMissingDetailsOrPropose")}</small></span></div>}
            <div className="tool-builder-chat-composer">
              <label className="auth-field" htmlFor={`${generatedID}-ai-instruction`}><span>{t("toolBuilder.messageTheAssistant")}</span><textarea id={`${generatedID}-ai-instruction`} value={instruction} maxLength={TOOL_BUILDER_CHAT_LIMITS.maxMessageBytes} aria-invalid={Boolean(instructionProblem)} aria-describedby={[`${generatedID}-ai-instruction-help`, instructionProblem ? `${generatedID}-ai-instruction-error` : ""].filter(Boolean).join(" ")} onChange={(event) => setInstruction(event.target.value)} placeholder={t("toolBuilder.forExampleHelpMeDesignAReadOnlyReadiness")} /><small id={`${generatedID}-ai-instruction-help`}>{instructionBytes}/{TOOL_BUILDER_CHAT_LIMITS.maxMessageBytes} {t("toolBuilder.utfN8BytesTheLastSixExchangesAreSent")}</small>{instructionProblem && <small className="error" id={`${generatedID}-ai-instruction-error`} role="alert">{instructionProblem}</small>}</label>
              <div className="tool-builder-source-actions"><Button type="button" color="indigo" disabled={Boolean(busy) || !instruction.trim() || Boolean(instructionProblem)} onClick={proposeDraft}><Sparkles data-slot="icon" />{busy === "propose" ? t("toolBuilder.responding") : t("toolBuilder.sendMessage")}</Button><Button type="button" outline disabled={Boolean(busy) || !local.draft} onClick={analyseDraft}><Bot data-slot="icon" />{busy === "analyse" ? t("toolBuilder.analyzing") : t("toolBuilder.analyzeCurrentDraft")}</Button></div>
              <p className="tool-builder-chat-boundary"><ShieldCheck />{apiScoped ? t("toolBuilder.yourMessageAndANonSecretPreviewComposedFrom") : t("toolBuilder.yourMessageAndCurrentNonSecretDraftIncludingThe")} {t("toolBuilder.useSyntheticExamplesAndKeepSecretsOutOfEvery")}</p>
            </div>
          </div> : <div className="tool-builder-source-empty"><Bot /><span><strong>{t("toolBuilder.aiAssistanceIsUnavailable")}</strong><small>{t("toolBuilder.configureAnAnalysisProviderOrContinueInManualOr")}</small></span><BuilderLink path="/settings/ai" onNavigate={onNavigate} className="core-button core-button-outline">{t("toolBuilder.configureAI")}</BuilderLink></div>}
          {analysis && <div className="tool-builder-analysis"><span><Bot /></span><span><strong>{t("toolBuilder.analysis")}</strong><p>{analysis.reply || analysis.summary || t("toolBuilder.analysisCompleted")}</p></span></div>}
        </section>}

        {mode === "import" && <section className="panel tool-builder-source-panel" aria-labelledby={`${generatedID}-import-title`}>
          <PanelHeader title={<span id={`${generatedID}-import-title`}><TerminalSquare />{t("toolBuilder.importARequest")}</span>} description={t("toolBuilder.inspectAPastedOpenAPIDocumentPostmanCollectionOrCURL")} action={<Badge color="amber">{t("toolBuilder.untrusted")}</Badge>} />
          <div className="tool-builder-source-body">
            <div className="two-fields"><label className="auth-field" htmlFor={`${generatedID}-import-kind`}><span>{t("toolBuilder.importFormat")}</span><select id={`${generatedID}-import-kind`} value={importKind} disabled={busy === "import"} onChange={(event) => { importInputVersionRef.current += 1; setImportKind(event.target.value as APIToolBuilderImportKind); setImportCandidates([]); if (proposal?.source === "import") setProposalStale(true); }}>{IMPORT_KINDS.map((kind) => <option key={kind} value={kind}>{importKindLabel(kind, t)}</option>)}</select></label><span className="tool-builder-import-hint"><ShieldCheck /><small>{t("toolBuilder.pasteReviewableTextOnlyURLFetchingAndAutomaticPublishing")} {apiScoped ? t("toolBuilder.thisAPITool") : t("toolBuilder.theSeparateSecretField")}.</small></span></div>
            <label className="auth-field" htmlFor={`${generatedID}-import-source`}><span>{importKind === "curl" ? t("toolBuilder.curlCommand") : importKind === "postman" ? t("toolBuilder.postmanCollectionJSON") : t("toolBuilder.openapiJSONOrYAML")}</span><textarea id={`${generatedID}-import-source`} className="code-input" value={importSource} disabled={busy === "import"} onChange={(event) => { importInputVersionRef.current += 1; setImportSource(event.target.value); setImportCandidates([]); if (proposal?.source === "import") setProposalStale(true); }} spellCheck={false} placeholder={importKind === "curl" ? t("toolBuilder.curlXPOSTHttpsApiVendorExampleV1Projects") : importKind === "postman" ? t("toolBuilder.pasteAnExportedPostmanCollectionV2N1Document") : t("toolBuilder.openapiN3N1N0")} /></label>
            <div className="tool-builder-source-actions"><Button type="button" color="indigo" disabled={Boolean(busy) || !importSource.trim()} onClick={importDraft}><TerminalSquare data-slot="icon" />{busy === "import" ? t("toolBuilder.inspecting") : t("toolBuilder.reviewImport")}</Button></div>
          </div>
          {importCandidates.length > 1 && <div className="tool-builder-candidates" aria-label={t("toolBuilder.importCandidates")}><h3>{t("toolBuilder.chooseAnOperationToReview")}</h3>{importCandidates.map((candidate, index) => <button type="button" key={`${candidate.draft.namespace}.${candidate.draft.name}:${index}`} onClick={() => setActiveProposal("import", candidate, assistanceDraft)}><span><strong>{candidate.draft.namespace}.{candidate.draft.name}</strong><small>{candidate.summary || candidate.draft.description}</small></span><Badge color={candidate.valid === false ? "red" : "blue"}>{candidate.valid === false ? t("toolBuilder.needsWork") : t("toolBuilder.review")}</Badge></button>)}</div>}
        </section>}

        {mode === "manual" && <section className="panel tool-builder-source-panel tool-builder-manual-intro" aria-labelledby={`${generatedID}-manual-title`}><span className="settings-icon"><Wrench /></span><span><h2 id={`${generatedID}-manual-title`}>{t("toolBuilder.manualSetup")}</h2><p>{t("toolBuilder.completeTheSharedContractBelowChangesRemainLocalUntil")}</p></span><Badge color="blue">{t("toolBuilder.youControlEveryField")}</Badge></section>}

        {proposal && <section className={`panel tool-builder-proposal ${proposalStale ? "stale" : ""}`} aria-labelledby={`${generatedID}-proposal-title`}>
          <div className="tool-builder-proposal-heading"><span><small>{proposal.source === "live-test" ? t("toolBuilder.liveTestAIProposal") : proposal.source === "ai" ? t("toolBuilder.aiProposal") : t("toolBuilder.importProposal")}</small><h2 id={`${generatedID}-proposal-title`} ref={proposalHeadingRef} tabIndex={-1}>{t("toolBuilder.reviewBeforeApplying")}</h2><p>{proposal.summary}</p></span><Badge color={proposalStale ? "red" : "violet"}>{proposalStale ? t("toolBuilder.stale") : t("toolBuilder.changes", { length: String(proposal.changes.length) })}</Badge></div>
          {proposalStale && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>{t("toolBuilder.theDraftChangedAfterThisProposal")}</strong><small>{t("toolBuilder.dismissItAndGenerateOrImportAFreshProposal")}</small></span></div>}
          <div className="tool-builder-change-list">{proposal.changes.map((change) => {
            const decision = proposalDecisions[change.field];
            return <article className={`tool-builder-change ${decision ?? "pending"}`} key={change.field}>
              <header><span><strong>{reviewFieldLabel(change.field, apiScoped, t)}</strong>{change.securitySensitive && <Badge color="amber"><ShieldCheck />{t("toolBuilder.securitySensitive")}</Badge>}</span>{decision && <Badge color={decision === "accepted" ? "green" : "zinc"}>{decision === "accepted" ? t("toolBuilder.accepted") : t("toolBuilder.rejected")}</Badge>}</header>
              {change.rationale && <p>{change.rationale}</p>}
              <div className="tool-builder-diff"><div><small>{t("toolBuilder.before")}</small><pre>{formatReviewValue(change.before, t("toolBuilder.notSet"))}</pre></div><div><small>{t("toolBuilder.proposed")}</small><pre>{formatReviewValue(change.after, t("toolBuilder.notSet"))}</pre></div></div>
              <div className="tool-builder-change-actions"><Button type="button" outline disabled={proposalStale || Boolean(decision) || busy === "save"} onClick={() => rejectProposalField(change)}><XCircle data-slot="icon" />{t("toolBuilder.keepCurrent")}</Button><Button type="button" color="indigo" disabled={proposalStale || Boolean(decision) || busy === "save"} onClick={() => applyProposalField(change)}><Check data-slot="icon" />{t("toolBuilder.acceptChange")}</Button></div>
            </article>;
          })}{proposal.changes.length === 0 && <div className="tool-builder-ready"><CheckCircle2 /><span><strong>{t("toolBuilder.noFieldChanges")}</strong><small>{t("toolBuilder.theProposalMatchesTheCurrentDraft")}</small></span></div>}</div>
          {proposal.findings.length > 0 && <div className="tool-builder-proposal-findings"><h3>{t("toolBuilder.proposalFindings")}</h3><FindingList findings={proposal.findings} onOpen={openFinding} /></div>}
          <div className="tool-builder-proposal-footer"><small>{t("toolBuilder.acceptedValuesUpdateOnlyThisUnsavedFormCredentialsAre")}</small><Button type="button" outline onClick={() => { setProposal(null); setProposalDecisions({}); setProposalStale(false); setStatus(t("toolBuilder.proposalDismissedAnyAcceptedChangesRemainInTheUnsaved")); }}>{t("toolBuilder.dismissProposal")}</Button></div>
        </section>}

        <fieldset className="panel tool-builder-fieldset" disabled={formLocked}>
          <legend>{t("toolBuilder.identityAndPurpose")}</legend><p>{t("toolBuilder.chooseAStableAgentFacingIdentityIdentityCannotChange")}</p>
          <div className="tool-builder-fields"><div className="two-fields"><label className="auth-field" htmlFor={domID("namespace")}><span>{t("toolBuilder.namespace")}</span><input id={domID("namespace")} data-tool-field="namespace" maxLength={64} readOnly={editing} value={form.namespace} aria-invalid={fieldFindings("namespace").some((finding) => finding.level === "error")} aria-describedby={describedBy("namespace", `${domID("namespace")}-help`)} onChange={(event) => markDraftChanged((current) => ({ ...current, namespace: event.target.value.toLowerCase() }))} placeholder="platform" /><small id={`${domID("namespace")}-help`}>{t("toolBuilder.lowerCaseContractGroupSuchAs")} <code>billing</code>.</small>{renderFieldFindings("namespace")}</label><label className="auth-field" htmlFor={domID("name")}><span>{t("toolBuilder.toolName")}</span><input id={domID("name")} data-tool-field="name" maxLength={64} readOnly={editing} value={form.name} aria-invalid={fieldFindings("name").some((finding) => finding.level === "error")} aria-describedby={describedBy("name", `${domID("name")}-help`)} onChange={(event) => markDraftChanged((current) => ({ ...current, name: event.target.value.toLowerCase() }))} placeholder="check_readiness" /><small id={`${domID("name")}-help`}>{t("toolBuilder.lowerCaseActionNameStartingWithALetter")}</small>{renderFieldFindings("name")}</label></div>
            <label className="auth-field" htmlFor={domID("description")}><span>{t("toolBuilder.purpose")}</span><textarea id={domID("description")} data-tool-field="description" maxLength={500} value={form.description} aria-invalid={fieldFindings("description").some((finding) => finding.level === "error")} aria-describedby={describedBy("description", `${domID("description")}-help`)} onChange={(event) => markDraftChanged((current) => ({ ...current, description: event.target.value }))} placeholder={t("toolBuilder.describeOneActionWhenAnAgentShouldUseIt")} /><small id={`${domID("description")}-help`}>{t("toolBuilder.descriptionCharacterCount", { count: form.description.length, max: 500 })}</small>{renderFieldFindings("description")}</label></div>
        </fieldset>

        <fieldset className="panel tool-builder-fieldset" disabled={formLocked}>
          <legend>{t("toolBuilder.contract")}</legend><p>{t("toolBuilder.defineTheJSONAcceptedAndReturnedByTheTool")}</p>
          <div className="tool-builder-fields tool-builder-schema-grid"><label className="auth-field" htmlFor={domID("input_schema")}><span>{t("toolBuilder.inputJSONSchema")}</span><textarea id={domID("input_schema")} data-tool-field="input_schema" className="code-input" spellCheck={false} value={form.input_schema_text} aria-invalid={fieldFindings("input_schema").some((finding) => finding.level === "error")} aria-describedby={describedBy("input_schema")} onChange={(event) => markDraftChanged((current) => ({ ...current, input_schema_text: event.target.value }))} />{renderFieldFindings("input_schema")}</label><label className="auth-field" htmlFor={domID("output_schema")}><span>{t("toolBuilder.outputJSONSchema")}</span><textarea id={domID("output_schema")} data-tool-field="output_schema" className="code-input" spellCheck={false} value={form.output_schema_text} aria-invalid={fieldFindings("output_schema").some((finding) => finding.level === "error")} aria-describedby={describedBy("output_schema")} onChange={(event) => markDraftChanged((current) => ({ ...current, output_schema_text: event.target.value }))} />{renderFieldFindings("output_schema")}</label></div>
        </fieldset>

        <fieldset className="panel tool-builder-fieldset" disabled={formLocked}>
          <legend>{apiScoped ? t("toolBuilder.apiOperation") : t("toolBuilder.executionAndAuthentication")}</legend><p>{apiScoped ? t("toolBuilder.chooseTheSavedAPIServiceConnectionAndTheRelative") : t("toolBuilder.fixTheDestinationRequestShapeAndUpstreamCredentialStrategy")}</p>
          <div className="tool-builder-fields"><div className="tool-builder-execution-grid"><label className="auth-field" htmlFor={domID("http_method")}><span>{t("toolBuilder.method")}</span><select id={domID("http_method")} data-tool-field="http_method" value={form.http_method} onChange={(event) => {
              const method = event.target.value as APIToolHTTPMethod;
              markDraftChanged((current) => ({ ...current, http_method: method, authorization_policy: { ...current.authorization_policy, risk: method === "GET" ? "low" : method === "DELETE" ? "critical" : current.authorization_policy.risk === "low" || current.authorization_policy.risk === "critical" ? "medium" : current.authorization_policy.risk, confirmation_required: method === "DELETE" ? true : current.authorization_policy.confirmation_required } }));
            }}>{HTTP_METHODS.map((method) => <option key={method}>{method}</option>)}</select></label>{apiScoped ? <><label className="auth-field" htmlFor={domID("runtime_service_connection_id")}><span>{t("toolBuilder.serviceConnection")}</span><select id={domID("runtime_service_connection_id")} data-tool-field="runtime_service_connection_id" value={runtimeConnectionID} disabled={editing || runtimeConnections.length === 0} aria-invalid={!runtimeLock} aria-describedby={`${domID("runtime_service_connection_id")}-help`} onChange={(event) => changeRuntimeConnection(event.target.value)}><option value="">{t("toolBuilder.chooseAConfiguredConnection")}</option>{runtimeConnections.map((connection) => <option key={connection.id} value={connection.id}>{connection.name}</option>)}</select><small id={`${domID("runtime_service_connection_id")}-help`}>{editing ? t("toolBuilder.theServiceConnectionIsImmutableAfterTheFirstSave") : runtimeConnections.length > 0 ? t("toolBuilder.savedEndpointAndAuthenticationRevisionsAreSelectedByEnvironment") : t("toolBuilder.configureThisAPISEndpointAndAuthenticationInAccess")}</small>{renderFieldFindings("runtime_service_connection_id")}</label><label className="auth-field" htmlFor={domID("endpoint")}><span>{t("toolBuilder.relativePath")}</span><input id={domID("endpoint")} data-tool-field="endpoint" value={runtimeHTTPPath} aria-invalid={Boolean(runtimePathProblem) || fieldFindings("endpoint").some((finding) => finding.level === "error")} aria-describedby={describedBy("endpoint", `${domID("endpoint")}-help`)} onChange={(event) => changeRuntimeHTTPPath(event.target.value)} placeholder="/v1/voices/{voice_id}" /><small id={`${domID("endpoint")}-help`}>{t("toolBuilder.startsWith")} <code>/</code>{t("toolBuilder.theServiceHostAndAuthenticationComeFromAccess")}</small>{renderFieldFindings("endpoint")}</label></> : <label className="auth-field" htmlFor={domID("endpoint")}><span>{t("toolBuilder.fixedEndpoint")}</span><input id={domID("endpoint")} data-tool-field="endpoint" type="url" value={form.endpoint} aria-invalid={fieldFindings("endpoint").some((finding) => finding.level === "error")} aria-describedby={describedBy("endpoint", `${domID("endpoint")}-help`)} onChange={(event) => markDraftChanged((current) => ({ ...current, endpoint: event.target.value }))} placeholder="https://api.vendor.example/v1/readiness" /><small id={`${domID("endpoint")}-help`}>{t("toolBuilder.httpsIsRequiredExceptForLocalhostDevelopment")}</small>{renderFieldFindings("endpoint")}</label>}<label className="auth-field" htmlFor={domID("timeout_ms")}><span>{t("toolBuilder.timeoutMs")}</span><input id={domID("timeout_ms")} data-tool-field="timeout_ms" type="number" min={100} max={60000} step={100} value={form.timeout_ms} aria-invalid={fieldFindings("timeout_ms").some((finding) => finding.level === "error")} aria-describedby={describedBy("timeout_ms")} onChange={(event) => markDraftChanged((current) => ({ ...current, timeout_ms: Number(event.target.value) }))} />{renderFieldFindings("timeout_ms")}</label></div>

          {apiScoped ? runtimeLock ? <div className="runtime-current-summary"><span className="settings-icon"><ShieldCheck /></span><span><strong>{runtimeConnection?.name ?? t("toolBuilder.apiServiceConnection")}</strong><small>{runtimeLock.baseURL}</small></span><span><small>{t("toolBuilder.authentication")}</small><strong>{authTypeLabel(activeAuth, t)}</strong></span><span><small>{t("toolBuilder.credential")}</small><strong>{runtimeLock.credentialPresent ? "Managed by Authorization" : t("toolBuilder.notRequired")}</strong></span></div> : <div className="capability-unavailable"><TriangleAlert /><span><strong>Connect Authorization first</strong><small>{t("toolBuilder.anAPIToolCannotStoreItsOwnEndpointOr")}</small></span><BuilderLink path={integrationPath(apiContext!.integration.id, "authorization")} onNavigate={onNavigate} className="entity-back-link">Open Authorization</BuilderLink></div> : <>
            <div className="tool-builder-subsection"><div><h3>{t("toolBuilder.upstreamAuthentication")}</h3><p>{authTypeDescription(activeAuth, t)}</p></div><label className="auth-field" htmlFor={domID("upstream_auth")}><span>{t("toolBuilder.authenticationType")}</span><select id={domID("upstream_auth")} data-tool-field="upstream_auth" value={form.upstream_auth.type} onChange={(event) => markDraftChanged((current) => ({ ...current, upstream_auth: { type: event.target.value as APIToolUpstreamAuthType } }))}>{AUTH_TYPES.map((option) => <option value={option} key={option}>{authTypeLabel(option, t)}</option>)}</select>{renderFieldFindings("upstream_auth")}</label></div>

            {form.upstream_auth.type === "authorization_scheme" && <label className="auth-field" htmlFor={`${domID("upstream_auth")}-scheme`}><span>{t("toolBuilder.authorizationScheme")}</span><input id={`${domID("upstream_auth")}-scheme`} data-tool-field="upstream_auth.scheme" value={form.upstream_auth.scheme ?? ""} onChange={(event) => updateAuth({ scheme: event.target.value })} placeholder={t("toolBuilder.ssws")} /><small>{t("toolBuilder.theFixedRequestHeaderBecomes")} <code>Authorization: {form.upstream_auth.scheme?.trim() || "Scheme"} &lt;encrypted credential&gt;</code>.</small>{renderFieldFindings("upstream_auth.scheme")}</label>}
            {["api_key_header", "custom_header"].includes(form.upstream_auth.type) && <div className="two-fields"><label className="auth-field" htmlFor={`${domID("upstream_auth")}-header`}><span>{t("toolBuilder.headerName")}</span><input id={`${domID("upstream_auth")}-header`} value={form.upstream_auth.header_name ?? ""} onChange={(event) => updateAuth({ header_name: event.target.value })} placeholder={form.upstream_auth.type === "api_key_header" ? t("toolBuilder.xAPIKey") : t("toolBuilder.xVendorToken")} /></label><label className="auth-field" htmlFor={`${domID("upstream_auth")}-prefix`}><span>{t("toolBuilder.valuePrefixOptional")}</span><input id={`${domID("upstream_auth")}-prefix`} value={form.upstream_auth.prefix ?? ""} onChange={(event) => updateAuth({ prefix: event.target.value })} placeholder={t("toolBuilder.token")} /><small>{t("toolBuilder.dokosokoInsertsOneSpaceBetweenANonEmptyPrefix")}</small></label></div>}
            {form.upstream_auth.type === "api_key_query" && <label className="auth-field" htmlFor={`${domID("upstream_auth")}-query`}><span>{t("toolBuilder.queryParameterName")}</span><input id={`${domID("upstream_auth")}-query`} value={form.upstream_auth.query_name ?? ""} onChange={(event) => updateAuth({ query_name: event.target.value })} placeholder="api_key" /></label>}
            {form.upstream_auth.type === "basic" && <label className="auth-field" htmlFor={`${domID("upstream_auth")}-username`}><span>{t("toolBuilder.username")}</span><input id={`${domID("upstream_auth")}-username`} value={form.upstream_auth.username ?? ""} onChange={(event) => updateAuth({ username: event.target.value })} autoComplete="off" /></label>}
            {form.upstream_auth.type === "oauth_client_credentials" && <><div className="two-fields"><label className="auth-field" htmlFor={`${domID("upstream_auth")}-client`}><span>{t("toolBuilder.clientID")}</span><input id={`${domID("upstream_auth")}-client`} value={form.upstream_auth.client_id ?? ""} onChange={(event) => updateAuth({ client_id: event.target.value })} autoComplete="off" /></label><label className="auth-field" htmlFor={`${domID("upstream_auth")}-token-url`}><span>{t("toolBuilder.tokenURL")}</span><input id={`${domID("upstream_auth")}-token-url`} type="url" value={form.upstream_auth.token_url ?? ""} onChange={(event) => updateAuth({ token_url: event.target.value })} placeholder="https://identity.vendor.example/oauth/token" /></label></div><div className="two-fields"><label className="auth-field" htmlFor={`${domID("upstream_auth")}-auth-method`}><span>{t("toolBuilder.tokenEndpointAuthentication")}</span><select id={`${domID("upstream_auth")}-auth-method`} value={form.upstream_auth.token_endpoint_auth_method ?? "client_secret_basic"} onChange={(event) => updateAuth({ token_endpoint_auth_method: event.target.value as "client_secret_basic" | "client_secret_post" })}><option value="client_secret_basic">{t("toolBuilder.clientSecretBasic")}</option><option value="client_secret_post">{t("toolBuilder.clientSecretPOST")}</option></select></label><label className="auth-field" htmlFor={`${domID("upstream_auth")}-resource`}><span>{t("toolBuilder.resourceOptional")}</span><input id={`${domID("upstream_auth")}-resource`} value={form.upstream_auth.resource ?? ""} onChange={(event) => updateAuth({ resource: event.target.value })} placeholder="https://api.vendor.example" /></label></div><div className="two-fields"><label className="auth-field" htmlFor={`${domID("upstream_auth")}-scopes`}><span>{t("toolBuilder.scopes")}</span><input id={`${domID("upstream_auth")}-scopes`} value={(form.upstream_auth.scopes ?? []).join(" ")} onChange={(event) => updateAuth({ scopes: event.target.value.split(/[\s,]+/).filter(Boolean) })} placeholder={t("toolBuilder.projectsReadProjectsWrite")} /></label><label className="auth-field" htmlFor={`${domID("upstream_auth")}-audience`}><span>{t("toolBuilder.audienceOptionalProviderSpecific")}</span><input id={`${domID("upstream_auth")}-audience`} value={form.upstream_auth.audience ?? ""} onChange={(event) => updateAuth({ audience: event.target.value })} /></label></div></>}

            {CREDENTIAL_AUTH_TYPES.has(form.upstream_auth.type) && <div className="tool-builder-credential"><label className="auth-field" htmlFor={domID("credential")}><span>{credentialLabel}</span><input id={domID("credential")} data-tool-field="credential" type="password" autoComplete="new-password" value={credential} aria-invalid={fieldFindings("credential").some((finding) => finding.level === "error")} aria-describedby={describedBy("credential", `${domID("credential")}-help`)} onChange={(event) => { if (busy === "save") return; const nextCredential = event.target.value; draftVersionRef.current += 1; setCredentialBinding(nextCredential ? toolCredentialBinding(form.endpoint, form.upstream_auth) : ""); setCredential(nextCredential); setValidation(null); setAnalysis(null); if (proposal) setProposalStale(true); }} placeholder={form.credential_present && storedCredentialReusable ? t("toolBuilder.leaveBlankToKeepTheStoredCredential") : t("toolBuilder.requiredBeforeSaving")} /><small id={`${domID("credential")}-help`}>{form.credential_present && !storedCredentialReusable ? t("toolBuilder.reEnterTheCredentialBecauseItsDestinationOrAuthentication") : form.credential_present ? t("toolBuilder.aCredentialIsAlreadyStoredEnterAValueOnly") : t("toolBuilder.encryptedOnlyOnFinalSave")}</small>{renderFieldFindings("credential")}</label><div className="tool-builder-secret-boundary"><KeyRound /><span><strong>{t("toolBuilder.localSecretBoundary")}</strong><small>{t("toolBuilder.thisValueIsExcludedFromAIImportAnalysisAnd")}</small></span></div></div>}</>}

            <div className="tool-builder-subsection"><div><h3>{t("toolBuilder.requestMapping")}</h3><p>{t("toolBuilder.mapInputPropertiesToFixedRequestLocationsWithNo")} {form.http_method === "GET" ? t("toolBuilder.getInputIsSentAsQueryParameters") : t("toolBuilder.inputIsSentAsOneJSONRequestBody")}.</p></div><Button type="button" outline onClick={addRequestMapping}><Plus data-slot="icon" />{t("toolBuilder.addMapping")}</Button></div>
            <div id={domID("request_mapping")} data-tool-field="request_mapping" tabIndex={-1} className="tool-builder-mapping-list">{Object.entries(form.request_mapping.parameter_locations).map(([parameter, location], index) => <div className="tool-builder-mapping-row" key={`${parameter}:${index}`}><label className="auth-field"><span className="sr-only">{t("toolBuilder.parameterName")}</span><input value={parameter} aria-label={t("toolBuilder.mappingParameterName", { value1: String(index + 1) })} onChange={(event) => renameRequestMapping(parameter, event.target.value)} /></label><label className="auth-field"><span className="sr-only">{t("toolBuilder.requestLocation")}</span><select aria-label={t("toolBuilder.mappingRequestLocation", { value1: String(parameter || index + 1) })} value={location} onChange={(event) => setRequestMappingLocation(parameter, event.target.value as APIToolRequestMapping["parameter_locations"][string])}><option value="path">{t("toolBuilder.path")}</option><option value="query">{t("toolBuilder.query")}</option><option value="header">{t("toolBuilder.header")}</option><option value="body">{t("toolBuilder.body")}</option></select></label><Button type="button" outline aria-label={t("toolBuilder.removeMappingFor", { value1: String(parameter || t("toolBuilder.rowNumber", { number: index + 1 })) })} onClick={() => removeRequestMapping(parameter)}><XCircle data-slot="icon" />{t("toolBuilder.remove")}</Button></div>)}{Object.keys(form.request_mapping.parameter_locations).length === 0 && <p className="tool-builder-empty-copy">{t("toolBuilder.noExplicitMappings")} {form.http_method === "GET" ? t("toolBuilder.eachInputPropertyBecomesAQueryParameter") : t("toolBuilder.theInputObjectBecomesTheJSONRequestBody")}</p>}{renderFieldFindings("request_mapping")}</div>
            <label className="auth-field" htmlFor={domID("response_mapping")}><span>{t("toolBuilder.responseResultPathOptional")}</span><input id={domID("response_mapping")} data-tool-field="response_mapping" value={form.response_mapping.result_path ?? ""} onChange={(event) => markDraftChanged((current) => ({ ...current, response_mapping: event.target.value ? { result_path: event.target.value } : {} }))} placeholder="data.result" /><small>{t("toolBuilder.dotSeparatedPathToTheValueReturnedToThe")}</small></label>
          </div>
        </fieldset>

        <fieldset className="panel tool-builder-fieldset" disabled={formLocked}>
          <legend>{t("toolBuilder.authorizationPolicy")}</legend><p>{t("toolBuilder.requireRegisteredDeploymentGrantsAndExplicitHumanSafeguardsBefore")}</p>
          <div className="tool-builder-fields"><div id={domID("authorization_policy")} data-tool-field="authorization_policy" tabIndex={-1} className="tool-builder-grants" role="group" aria-labelledby={`${domID("authorization_policy")}-label`}><span id={`${domID("authorization_policy")}-label`}><strong>{t("toolBuilder.requiredGrants")}</strong><small>{t("toolBuilder.selectEveryGrantACallerMustHold")}</small></span>{grants.length > 0 ? <div>{grants.map((grant) => <label className="compact-check" key={grant.id}><input type="checkbox" aria-label={t("toolBuilder.requireGrant", { name: grant.display_name })} checked={form.authorization_policy.required_grants.includes(grant.key)} onChange={() => toggleGrant(grant.key)} /><span><strong>{grant.display_name}</strong><small><code>{grant.key}</code> · {grant.state}</small></span></label>)}</div> : <p className="tool-builder-empty-copy">{t("toolBuilder.noGrantsAreRegisteredThisToolWillHaveNo")}</p>}{renderFieldFindings("authorization_policy.required_grants")}</div>
            <div className="two-fields"><label className="auth-field" htmlFor={`${domID("authorization_policy")}-risk`}><span>{t("toolBuilder.risk")}</span><select id={`${domID("authorization_policy")}-risk`} value={form.authorization_policy.risk} onChange={(event) => updatePolicy({ risk: event.target.value as APIToolRisk })}>{RISKS.map((risk) => <option value={risk} key={risk}>{risk === "low" ? t("enumLabels.low") : risk === "medium" ? t("enumLabels.medium") : risk === "high" ? t("enumLabels.high") : t("enumLabels.critical")}</option>)}</select></label><label className="compact-check"><input type="checkbox" disabled={form.authorization_policy.risk === "critical"} checked={form.authorization_policy.confirmation_required || form.authorization_policy.risk === "critical"} onChange={(event) => updatePolicy({ confirmation_required: event.target.checked })} /><span>{t("toolBuilder.requireExplicitConfirmation")}</span></label></div>
            <label className="compact-check" id={`${domID("authorization_policy")}-idempotency`}><input type="checkbox" checked={form.authorization_policy.idempotency_required} onChange={(event) => updatePolicy({ idempotency_required: event.target.checked })} /><span>{t("toolBuilder.requireIdempotencyMetadataForMutationCalls")}</span></label>{renderFieldFindings("authorization_policy.idempotency_required")}</div>
        </fieldset>

        <fieldset className="panel tool-builder-fieldset" disabled={formLocked}>
          <legend>{t("toolBuilder.examples")}</legend><p>{t("toolBuilder.optionalExamplesMakeReviewEasierAndAreCheckedWithout")}</p>
          <div className="tool-builder-fields tool-builder-schema-grid"><label className="auth-field" htmlFor={domID("request_example")}><span>{t("toolBuilder.requestExample")}</span><textarea id={domID("request_example")} data-tool-field="request_example" className="code-input" spellCheck={false} value={form.request_example_text} aria-invalid={fieldFindings("request_example").some((finding) => finding.level === "error")} aria-describedby={describedBy("request_example")} onChange={(event) => markDraftChanged((current) => ({ ...current, request_example_text: event.target.value }))} placeholder={t("toolBuilder.projectIdProjectN123")} />{renderFieldFindings("request_example")}</label><label className="auth-field" htmlFor={domID("response_example")}><span>{t("toolBuilder.responseExample")}</span><textarea id={domID("response_example")} data-tool-field="response_example" className="code-input" spellCheck={false} value={form.response_example_text} aria-invalid={fieldFindings("response_example").some((finding) => finding.level === "error")} aria-describedby={describedBy("response_example")} onChange={(event) => markDraftChanged((current) => ({ ...current, response_example_text: event.target.value }))} placeholder={t("toolBuilder.readyTrue")} />{renderFieldFindings("response_example")}</label></div>
        </fieldset>
      </div>

      <aside className="tool-builder-rail" aria-label={t("toolBuilder.draftReadiness")}>
        <section className="panel tool-builder-rail-panel"><PanelHeader title={t("toolBuilder.readiness")} description={t("toolBuilder.localFindingsUpdateImmediatelyServerChecksNeverExecuteThe")} action={<Badge color={errors.length ? "red" : warnings.length ? "amber" : "green"}>{errors.length ? t("toolBuilder.errors", { length: String(errors.length) }) : warnings.length ? t("toolBuilder.warnings", { length: String(warnings.length) }) : t("toolBuilder.ready")}</Badge>} /><div className="tool-builder-rail-body"><FindingList findings={findings} onOpen={openFinding} /><Button type="button" outline className="full" disabled={Boolean(busy) || !local.draft} onClick={validateDraft}><ShieldCheck data-slot="icon" />{busy === "validate" ? t("toolBuilder.checking") : t("toolBuilder.checkDraft")}</Button></div></section>
        <section className="panel tool-builder-rail-panel tool-builder-safety"><PanelHeader title={t("toolBuilder.safetyBoundary")} /><div className="tool-builder-rail-body"><span><KeyRound /><small>{apiScoped ? t("toolBuilder.credentialManagedOnceInAPIAccess") : t("toolBuilder.credentialSentOnlyOnSaveDraft")}</small></span><span><Bot /><small>{t("toolBuilder.aiReceivesNonSecretFieldsOnly")}</small></span><span><TerminalSquare /><small>{t("toolBuilder.validationPerformsNoUpstreamCall")}</small></span><span><ShieldCheck /><small>{t("toolBuilder.publishingRemainsASeparateReview")}</small></span></div></section>
        <section className="panel tool-builder-rail-panel"><PanelHeader title={t("toolBuilder.draftState")} /><dl className="tool-builder-draft-state"><div><dt>{t("toolBuilder.identity")}</dt><dd><code>{form.namespace || "namespace"}.{form.name || "tool"}</code></dd></div><div><dt>{t("toolBuilder.scope")}</dt><dd>{apiContext ? apiContext.integration.display_name : t("toolBuilder.common")}</dd></div><div><dt>{t("toolBuilder.method")}</dt><dd>{form.http_method}</dd></div><div><dt>{t("toolBuilder.authentication")}</dt><dd>{apiScoped ? t("toolBuilder.inheritedFromAPI") : authTypeLabel(activeAuth, t)}</dd></div><div><dt>{t("toolBuilder.credential")}</dt><dd>{apiScoped ? runtimeLock?.credentialPresent ? t("toolBuilder.managedInAccess") : t("toolBuilder.notRequired") : credential ? t("toolBuilder.replacementEntered") : form.credential_present && storedCredentialReusable ? t("toolBuilder.stored") : CREDENTIAL_AUTH_TYPES.has(form.upstream_auth.type) ? t("toolBuilder.missing") : t("toolBuilder.notRequired")}</dd></div></dl></section>
      </aside>
    </div>
  </form>;
}
