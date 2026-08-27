import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

class FakeElement {
  constructor(tagName = "element") {
    this.tagName = tagName;
    this.attributes = new Map();
    this.children = [];
    this.dataset = {};
    this.parentElement = null;
    this.textContent = "";
  }

  append(...children) {
    for (const child of children) {
      child.parentElement = this;
      this.children.push(child);
    }
  }

  replaceChildren(...children) {
    this.children = [];
    this.append(...children);
  }

  setAttribute(name, value) {
    this.attributes.set(name, String(value));
  }

  getAttribute(name) {
    return this.attributes.get(name) ?? null;
  }

  hasAttribute(name) {
    return this.attributes.has(name);
  }
}

test("localized MCP Web Component renders safe, chip-free labels from explicit and page locales", async () => {
  const source = await readFile(new URL("../internal/httpapi/agent_setup_button.js", import.meta.url), "utf8");
  const config = {
    deploymentName: `<img src=x onerror="alert(1)">`,
    setupURLs: {
      public: "https://dokosoko.example/agent-setup/public/prompt.md",
      private: "https://dokosoko.example/agent-setup/private/prompt.md",
    },
    assetBaseURL: "https://dokosoko.example/agent-client-icons",
  };

  const previous = {
    customElements: globalThis.customElements,
    document: globalThis.document,
    HTMLElement: globalThis.HTMLElement,
  };
  let Component;
  class FakeHTMLElement extends FakeElement {
    constructor() {
      super("dokosoko-mcp-button");
      this.isConnected = true;
    }

    attachShadow() {
      this.shadowRoot = new FakeElement("shadow-root");
      return this.shadowRoot;
    }
  }
  const documentElement = new FakeElement("html");
  documentElement.setAttribute("lang", "pt-BR");

  globalThis.HTMLElement = FakeHTMLElement;
  globalThis.document = {
    documentElement,
    createElement: (tagName) => new FakeElement(tagName),
  };
  globalThis.customElements = {
    get: () => undefined,
    define: (_name, value) => { Component = value; },
  };

  try {
    const runtime = source.replace("globalThis.__DOKOSOKO_AGENT_SETUP_CONFIG__", JSON.stringify(config));
    Function(runtime)();
    assert.ok(Component, "the script should define dokosoko-mcp-button");

    const automatic = new Component();
    automatic.setAttribute("kind", "private");
    automatic.setAttribute("lang", "auto");
    automatic.connectedCallback();
    const automaticLink = automatic.shadowRoot.children[1];
    assert.equal(automaticLink.href, config.setupURLs.private);
    assert.equal(automaticLink.lang, "pt-BR");
    assert.equal(automaticLink.children[0].textContent, `Conecte seu agente a ${config.deploymentName}`);
    assert.equal(automaticLink.children.length, 2, "the removed access chip must not be rendered");
    assert.equal(automaticLink.children[1].getAttribute("aria-hidden"), "true");
    assert.equal(automaticLink.children[1].children.length, 4);

    const japanese = new Component();
    japanese.setAttribute("lang", "ja-JP");
    japanese.connectedCallback();
    const japaneseLink = japanese.shadowRoot.children[1];
    assert.equal(japaneseLink.href, config.setupURLs.public);
    assert.equal(japaneseLink.lang, "ja");
    assert.equal(japaneseLink.children[0].textContent, `[パブリック] エージェントを${config.deploymentName}に接続`);
    assert.equal(japaneseLink.children[0].children.length, 0, "deployment names must remain text, never executable HTML");
  } finally {
    globalThis.customElements = previous.customElements;
    globalThis.document = previous.document;
    globalThis.HTMLElement = previous.HTMLElement;
  }
});
