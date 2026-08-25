"use client";

import { useState, type Dispatch, type SetStateAction } from "react";

import { APIError, api, type APIWidget, type APIWidgetInput } from "../../lib/api";
import { entityPath } from "../../lib/console-routes";

export function useWidgetWorkflow({ setWidgets, onNavigate, showToast }: {
  setWidgets: Dispatch<SetStateAction<APIWidget[]>>;
  onNavigate: (path: string) => void;
  showToast: (message: string) => void;
}) {
  const [widgetCreateOpen, setWidgetCreateOpen] = useState(false);
  const [widgetBusy, setWidgetBusy] = useState(false);
  const [widgetName, setWidgetName] = useState("Customer assistant");
  const [widgetOrigins, setWidgetOrigins] = useState("http://localhost:3000");
  const [widgetIntegrationIDs, setWidgetIntegrationIDs] = useState<string[]>([]);
  const [widgetCredential, setWidgetCredential] = useState<{ widgetID: string; secret: string } | null>(null);

  async function createWidget() {
    const allowedOrigins = widgetOrigins.split(/[\n,]/).map((value) => value.trim()).filter(Boolean);
    if (!widgetName.trim() || allowedOrigins.length === 0 || widgetIntegrationIDs.length === 0) {
      showToast("Add a name, an allowed origin, and at least one API.");
      return;
    }
    setWidgetBusy(true);
    try {
      const input: APIWidgetInput = { name: widgetName.trim(), allowed_origins: allowedOrigins, integration_ids: widgetIntegrationIDs, appearance: { theme: "auto", launcher_position: "right", greeting: "How can I help?" } };
      const created = await api.createWidget(input);
      setWidgets((items) => [...items, created.widget]);
      setWidgetCreateOpen(false);
      setWidgetCredential({ widgetID: created.widget.id, secret: created.secret });
      setWidgetName("Customer assistant");
      setWidgetOrigins("http://localhost:3000");
      setWidgetIntegrationIDs([]);
      onNavigate(entityPath("widget", created.widget.id));
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not create the widget.");
    } finally {
      setWidgetBusy(false);
    }
  }

  async function updateWidget(widget: APIWidget, input: APIWidgetInput): Promise<APIWidget | null> {
    setWidgetBusy(true);
    try {
      const updated = await api.updateWidget(widget.id, { ...input, revision: widget.revision });
      setWidgets((items) => items.map((item) => item.id === updated.id ? updated : item));
      showToast("Widget settings saved.");
      return updated;
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not update the widget.");
      return null;
    } finally {
      setWidgetBusy(false);
    }
  }

  async function setWidgetState(widget: APIWidget, state: "active" | "disabled"): Promise<APIWidget | null> {
    setWidgetBusy(true);
    try {
      const updated = state === "active" ? await api.activateWidget(widget.id, widget.revision) : await api.disableWidget(widget.id, widget.revision);
      setWidgets((items) => items.map((item) => item.id === updated.id ? updated : item));
      showToast(state === "active" ? "Widget is live." : "Widget disabled immediately.");
      return updated;
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not change widget state.");
      return null;
    } finally {
      setWidgetBusy(false);
    }
  }

  async function rotateWidgetSecret(widget: APIWidget) {
    setWidgetBusy(true);
    try {
      const created = await api.createWidgetSecret(widget.id);
      setWidgetCredential({ widgetID: widget.id, secret: created.secret });
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not create a new widget secret.");
    } finally {
      setWidgetBusy(false);
    }
  }

  return {
    widgetCreateOpen, setWidgetCreateOpen,
    widgetBusy,
    widgetName, setWidgetName,
    widgetOrigins, setWidgetOrigins,
    widgetIntegrationIDs, setWidgetIntegrationIDs,
    widgetCredential, setWidgetCredential,
    createWidget,
    updateWidget,
    setWidgetState,
    rotateWidgetSecret,
  };
}
