((config) => {
  "use strict";

  const elementName = "dokosoko-mcp-button";
  if (customElements.get(elementName)) return;

  const messages = {
    en: { "agentAccess.connectYourAgentToName": "Connect your agent to {{name}}", "agentAccess.public": "Public" },
    es: { "agentAccess.connectYourAgentToName": "Conecta tu agente a {{name}}", "agentAccess.public": "Público" },
    fr: { "agentAccess.connectYourAgentToName": "Connectez votre agent à {{name}}", "agentAccess.public": "Public" },
    de: { "agentAccess.connectYourAgentToName": "Agent mit {{name}} verbinden", "agentAccess.public": "öffentlich" },
    ja: { "agentAccess.connectYourAgentToName": "エージェントを{{name}}に接続", "agentAccess.public": "パブリック" },
    uk: { "agentAccess.connectYourAgentToName": "Підключити агента до {{name}}", "agentAccess.public": "ПУБЛІЧНІ" },
    "pt-BR": { "agentAccess.connectYourAgentToName": "Conecte seu agente a {{name}}", "agentAccess.public": "Público" },
  };
  const aliases = { en: "en", es: "es", fr: "fr", de: "de", ja: "ja", jp: "ja", uk: "uk", ua: "uk", pt: "pt-BR", "pt-br": "pt-BR" };
  const clients = [
    { id: "codex", name: "Codex", file: "codex.svg" },
    { id: "claude-code", name: "Claude Code", file: "claude-code.svg" },
    { id: "cursor", name: "Cursor", file: "cursor.svg" },
    { id: "opencode", name: "OpenCode", file: "opencode.svg" },
  ];
  const styles = `
    :host { display: inline-block; max-width: 100%; color-scheme: light; }
    a { box-sizing: border-box; display: inline-flex; align-items: center; gap: 10px; max-width: 100%; min-height: 52px; padding: 0 18px; border: 1px solid var(--dokosoko-mcp-button-border, #d4d4d8); border-radius: 999px; color: var(--dokosoko-mcp-button-color, #18181b); background: var(--dokosoko-mcp-button-background, #fff); box-shadow: 0 1px 2px rgba(0, 0, 0, .08); font: 600 16px/1.2 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; text-decoration: none; transition: border-color 120ms ease, box-shadow 120ms ease; }
    a:hover { border-color: var(--dokosoko-mcp-button-hover-border, #818cf8); }
    a:focus-visible { outline: 3px solid var(--dokosoko-mcp-button-focus, #818cf8); outline-offset: 3px; }
    .label { display: block; overflow: hidden; max-width: 260px; text-overflow: ellipsis; white-space: nowrap; }
    .clients { display: inline-flex; flex: 0 0 auto; align-items: center; gap: 5px; }
    img { display: block; width: 20px; height: 20px; object-fit: contain; }
    @media (prefers-reduced-motion: reduce) { a { transition: none; } }
  `;

  function normalizeLocale(value) {
    if (!value) return null;
    const normalized = String(value).trim().replaceAll("_", "-").toLowerCase();
    return aliases[normalized] || aliases[normalized.split("-")[0]] || null;
  }

  function t(locale, key, values = {}) {
    const template = messages[locale]?.[key] || messages.en[key] || key;
    return template.replace(/\{\{(\w+)\}\}/g, (_, name) => String(values[name] ?? ""));
  }

  class DokoSokoMCPButton extends HTMLElement {
    static get observedAttributes() {
      return ["kind", "lang"];
    }

    constructor() {
      super();
      this.attachShadow({ mode: "open" });
      this.localeObserver = null;
    }

    connectedCallback() {
      this.render();
      if (typeof MutationObserver === "function" && document.documentElement) {
        this.localeObserver = new MutationObserver(() => this.render());
        this.localeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ["lang"] });
      }
    }

    disconnectedCallback() {
      this.localeObserver?.disconnect();
      this.localeObserver = null;
    }

    attributeChangedCallback() {
      if (this.isConnected) this.render();
    }

    locale() {
      const explicit = this.getAttribute("lang");
      if (explicit && explicit.toLowerCase() !== "auto") return normalizeLocale(explicit) || "en";

      const candidates = [];
      let ancestor = this.parentElement;
      while (ancestor) {
        if (ancestor.hasAttribute("lang")) candidates.push(ancestor.getAttribute("lang"));
        ancestor = ancestor.parentElement;
      }
      candidates.push(document.documentElement?.getAttribute("lang"));
      candidates.push(...(navigator.languages || []), navigator.language);
      return candidates.map(normalizeLocale).find(Boolean) || "en";
    }

    render() {
      const kind = this.getAttribute("kind")?.toLowerCase() === "private" ? "private" : "public";
      const locale = this.locale();
      const connectLabel = t(locale, "agentAccess.connectYourAgentToName", { name: config.deploymentName });
      const labelText = kind === "public" ? `[${t(locale, "agentAccess.public")}] ${connectLabel}` : connectLabel;
      const style = document.createElement("style");
      style.textContent = styles;

      const link = document.createElement("a");
      link.href = config.setupURLs[kind];
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.lang = locale;
      link.setAttribute("aria-label", labelText);
      link.dataset.dokosokoAgentSetup = kind;
      link.dataset.dokosokoDeployment = config.deploymentName;

      const label = document.createElement("span");
      label.className = "label";
      label.textContent = labelText;
      link.append(label);

      const clientList = document.createElement("span");
      clientList.className = "clients";
      clientList.setAttribute("aria-hidden", "true");
      for (const client of clients) {
        const image = document.createElement("img");
        image.src = `${config.assetBaseURL}/${client.file}`;
        image.alt = "";
        image.title = client.name;
        image.width = 20;
        image.height = 20;
        image.dataset.agentClient = client.id;
        image.referrerPolicy = "no-referrer";
        clientList.append(image);
      }
      link.append(clientList);
      this.shadowRoot.replaceChildren(style, link);
    }
  }

  customElements.define(elementName, DokoSokoMCPButton);
})(globalThis.__DOKOSOKO_AGENT_SETUP_CONFIG__);
