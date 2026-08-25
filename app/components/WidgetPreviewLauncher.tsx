"use client";

import { AlertCircle, MessageSquareText, RefreshCw, Send, Sparkles, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, FormEvent } from "react";
import { APIError, APIWidget, APIWidgetAgentSource, APIWidgetAgentTrace, APIWidgetConfiguration, api, streamWidgetMessage } from "../lib/api";
import { Button } from "./core/control";
import { MarkdownMessage } from "./MarkdownMessage";

type PreviewMessage = { id: string; role: "user" | "assistant"; text: string; sources?: APIWidgetAgentSource[]; trace?: APIWidgetAgentTrace };

function messageID() {
  return typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `preview-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function previewError(error: unknown) {
  if (error instanceof APIError) {
    switch (error.code) {
      case "widget_disabled": return "This widget is no longer active. Activate it before starting another preview.";
      case "widget_manifest_unavailable": return "This widget’s pinned API snapshot is unavailable. Review and reactivate the widget.";
      case "widget_assistant_unavailable": return "The Assistant workload is unavailable. Check its provider and model in Settings.";
      case "rate_limit_exceeded": return "The preview is sending too quickly. Wait a moment and try again.";
      default: return error.message;
    }
  }
  return error instanceof Error && error.name !== "AbortError" ? error.message : "The widget preview could not start.";
}

export function WidgetPreviewLauncher({ widgets, currentWidgetID, onOpenWidgets }: { widgets: APIWidget[]; currentWidgetID?: string; onOpenWidgets: () => void }) {
  const activeWidgets = useMemo(() => widgets.filter((widget) => widget.state === "active"), [widgets]);
  const [open, setOpen] = useState(false);
  const [chosenID, setChosenID] = useState("");
  const [configuration, setConfiguration] = useState<APIWidgetConfiguration | null>(null);
  const [messages, setMessages] = useState<PreviewMessage[]>([]);
  const [draft, setDraft] = useState("");
  const [status, setStatus] = useState<"idle" | "loading" | "ready" | "streaming" | "error">("idle");
  const [error, setError] = useState("");
  const sessionTokenRef = useRef("");
  const sessionWidgetRef = useRef("");
  const sessionExpiresRef = useRef(0);
  const generationRef = useRef(0);
  const streamAbortRef = useRef<AbortController | null>(null);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const launcherRef = useRef<HTMLButtonElement | null>(null);
  const transcriptRef = useRef<HTMLDivElement | null>(null);

  const selectedID = (activeWidgets.some((widget) => widget.id === chosenID) ? chosenID : "") || (currentWidgetID && activeWidgets.some((widget) => widget.id === currentWidgetID) ? currentWidgetID : "") || activeWidgets[0]?.id || "";

  useEffect(() => {
    if (!open) return;
    requestAnimationFrame(() => inputRef.current?.focus());
  }, [open, status]);

  useEffect(() => {
    if (!open) return;
    requestAnimationFrame(() => transcriptRef.current?.scrollTo({ top: transcriptRef.current.scrollHeight, behavior: status === "streaming" ? "auto" : "smooth" }));
  }, [messages, open, status]);

  useEffect(() => () => {
    generationRef.current += 1;
    streamAbortRef.current?.abort();
  }, []);

  async function establishSession(widgetID: string, resetTranscript: boolean, cancelActiveStream = true): Promise<string> {
    if (!widgetID) throw new APIError(409, "widget_unavailable", "Activate a widget before starting a preview.");
    const generation = ++generationRef.current;
    if (cancelActiveStream) streamAbortRef.current?.abort();
    setStatus("loading");
    setError("");
    if (resetTranscript) setMessages([]);
    try {
      // Resolve the same public configuration a customer receives before minting
      // the single-use admin bootstrap. A failed configuration read therefore
      // cannot leave an unused bearer waiting to expire.
      const nextConfiguration = await api.widgetConfiguration(widgetID);
      const bootstrap = await api.widgetPreviewBootstrap(widgetID);
      const session = await api.exchangeWidgetSession(bootstrap.bootstrapToken, window.location.origin);
      if (generation !== generationRef.current) return "";
      sessionTokenRef.current = session.sessionToken;
      sessionWidgetRef.current = widgetID;
      sessionExpiresRef.current = Date.parse(session.expiresAt);
      setConfiguration(nextConfiguration);
      setStatus("ready");
      if (resetTranscript) setMessages(nextConfiguration.appearance.greeting ? [{ id: messageID(), role: "assistant", text: nextConfiguration.appearance.greeting }] : []);
      return session.sessionToken;
    } catch (sessionError) {
      if (generation === generationRef.current) {
        sessionTokenRef.current = "";
        setStatus("error");
        setError(previewError(sessionError));
      }
      throw sessionError;
    }
  }

  useEffect(() => {
    if (!open || !selectedID) return;
    const currentSessionWorks = sessionWidgetRef.current === selectedID && sessionTokenRef.current && sessionExpiresRef.current > Date.now() + 15_000;
    if (!currentSessionWorks) void establishSession(selectedID, true).catch(() => undefined);
  }, [open, selectedID]);

  function close() {
    streamAbortRef.current?.abort();
    setStatus(sessionTokenRef.current ? "ready" : "idle");
    setOpen(false);
    requestAnimationFrame(() => launcherRef.current?.focus());
  }

  function chooseWidget(widgetID: string) {
    setChosenID(widgetID);
  }

  function updateAssistant(messageIDValue: string, text: string) {
    setMessages((items) => items.map((item) => item.id === messageIDValue ? { ...item, text: item.text + text } : item));
  }

  function addAssistantSource(messageIDValue: string, source: APIWidgetAgentSource) {
    setMessages((items) => items.map((item) => item.id === messageIDValue ? { ...item, sources: [...(item.sources ?? []), source] } : item));
  }

  function setAssistantTrace(messageIDValue: string, trace: APIWidgetAgentTrace) {
    setMessages((items) => items.map((item) => item.id === messageIDValue ? { ...item, trace } : item));
  }

  async function send(event: FormEvent) {
    event.preventDefault();
    const question = draft.trim();
    if (!question || question.length > 4000 || status === "streaming") return;
    const userMessage = { id: messageID(), role: "user" as const, text: question };
    const assistantMessage = { id: messageID(), role: "assistant" as const, text: "" };
    setDraft("");
    setError("");
    setMessages((items) => [...items, userMessage, assistantMessage]);
    setStatus("streaming");
    const abort = new AbortController();
    streamAbortRef.current = abort;
    try {
      let token = sessionTokenRef.current;
      if (!token || sessionWidgetRef.current !== selectedID || sessionExpiresRef.current <= Date.now() + 15_000) token = await establishSession(selectedID, false, false);
      setStatus("streaming");
      try {
        await streamWidgetMessage(token, question, (text) => updateAssistant(assistantMessage.id, text), abort.signal, (source) => addAssistantSource(assistantMessage.id, source), (trace) => setAssistantTrace(assistantMessage.id, trace));
      } catch (streamError) {
        if (!(streamError instanceof APIError) || streamError.status !== 401 || abort.signal.aborted) throw streamError;
        token = await establishSession(selectedID, false, false);
        setStatus("streaming");
        await streamWidgetMessage(token, question, (text) => updateAssistant(assistantMessage.id, text), abort.signal, (source) => addAssistantSource(assistantMessage.id, source), (trace) => setAssistantTrace(assistantMessage.id, trace));
      }
      if (!abort.signal.aborted) setStatus("ready");
    } catch (streamError) {
      if (abort.signal.aborted) return;
      setMessages((items) => items.filter((item) => item.id !== assistantMessage.id || item.text));
      setStatus("error");
      setError(previewError(streamError));
    }
  }

  const selectedWidget = activeWidgets.find((widget) => widget.id === selectedID);
  const appearance = configuration?.appearance ?? selectedWidget?.appearance;
  const accent = appearance?.accentColour ?? "#4f46e5";
  const theme = appearance?.theme ?? "auto";
  const canSend = Boolean(selectedWidget && draft.trim() && draft.trim().length <= 4000 && status !== "loading" && status !== "streaming");

  return <div className={`widget-preview-root ${open ? "open" : ""}`} style={{ "--widget-preview-accent": accent } as CSSProperties}>
    {open && <dialog open id="widget-preview-panel" className="widget-preview-panel" data-widget-theme={theme} aria-label="Widget preview" onKeyDown={(event) => { if (event.key === "Escape") close(); }}>
      <header className="widget-preview-header">
        <span className="widget-preview-mark"><Sparkles /></span>
        <span><strong>{selectedWidget?.name ?? "Widget preview"}</strong><small>Admin preview</small></span>
        <span className="widget-preview-header-actions"><button type="button" aria-label="Start a new widget preview session" title="New session" disabled={!selectedWidget || status === "loading" || status === "streaming"} onClick={() => void establishSession(selectedID, true).catch(() => undefined)}><RefreshCw /></button><button type="button" aria-label="Close widget preview" onClick={close}><X /></button></span>
      </header>

      {activeWidgets.length === 0 ? <div className="widget-preview-empty"><MessageSquareText /><div><strong>No active widget</strong><p>Activate a widget to test the same configuration your customers receive.</p></div><Button color="indigo" onClick={() => { close(); onOpenWidgets(); }}>Open widgets</Button></div> : <>
        {activeWidgets.length > 1 && <label className="widget-preview-selector"><span>Previewing</span><select aria-label="Widget to preview" value={selectedID} onChange={(event) => chooseWidget(event.target.value)}>{activeWidgets.map((widget) => <option key={widget.id} value={widget.id}>{widget.name}</option>)}</select></label>}
        <div ref={transcriptRef} className="widget-preview-transcript" role="log" aria-live="polite" aria-busy={status === "loading" || status === "streaming"}>
          {status === "loading" && messages.length === 0 && <div className="widget-preview-loading"><RefreshCw /><span>Starting a private preview…</span></div>}
          {status !== "loading" && messages.length === 0 && !error && <div className="widget-preview-welcome"><Sparkles /><span><strong>{configuration?.name ?? selectedWidget?.name}</strong><small>Ask about the APIs available through this widget.</small></span></div>}
          {messages.map((message) => <div key={message.id} className={`widget-preview-message ${message.role}`}>
            {message.role === "assistant" && message.text ? <MarkdownMessage>{message.text}</MarkdownMessage> : <span>{message.text || <i>Thinking…</i>}</span>}
            {message.role === "assistant" && (message.sources?.length || message.trace) ? <details className="widget-preview-grounding">
              <summary>Why this answer?</summary>
              {message.sources?.length ? <ul>{message.sources.map((source, index) => <li key={`${source.kind}-${source.title}-${index}`}><strong>{source.title}</strong><span>{source.kind}{source.revision ? ` · revision ${source.revision}` : ""}{source.integration ? ` · ${source.integration}` : ""}</span></li>)}</ul> : <p>No published guidance was selected.</p>}
              {message.trace && <small>Intent: {message.trace.intent} · {message.trace.recipeCount} recipe{message.trace.recipeCount === 1 ? "" : "s"} · {message.trace.documentationCount} document{message.trace.documentationCount === 1 ? "" : "s"} · prompt {message.trace.promptVersion}</small>}
            </details> : null}
          </div>)}
          {error && <div className="widget-preview-error" role="alert"><AlertCircle /><span>{error}</span><button type="button" onClick={() => void establishSession(selectedID, messages.length === 0).catch(() => undefined)}>Try again</button></div>}
        </div>
        <form className="widget-preview-composer" onSubmit={send}>
          <textarea ref={inputRef} aria-label="Ask the widget" placeholder="Ask a customer question…" rows={2} maxLength={4000} disabled={status === "loading"} value={draft} onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && !event.shiftKey) { event.preventDefault(); event.currentTarget.form?.requestSubmit(); } }} />
          <button type="submit" aria-label="Send message" disabled={!canSend}><Send /></button>
          <small>Admin preview uses the active product knowledge. Customer page context is added by the embedding backend.</small>
        </form>
      </>}
    </dialog>}
    <button ref={launcherRef} type="button" className="widget-preview-launcher" aria-label={open ? "Close widget preview" : "Open widget preview"} aria-expanded={open} aria-controls="widget-preview-panel" title="Test widget" onClick={() => open ? close() : setOpen(true)}><Sparkles /></button>
  </div>;
}
