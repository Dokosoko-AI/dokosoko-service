package httpapi

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

type agentSetupLink struct {
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	URL               string `json:"url"`
	EmbedHTML         string `json:"embed_html"`
	ContainsSecret    bool   `json:"contains_secret"`
}

func promptLabel(value string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(value), " "), "`", "'")
}

func (s *Server) agentSetupURL(kind string) string {
	return s.baseURL + "/agent-setup/" + kind + "/prompt.md"
}

func agentSetupEmbedHTML(tenantName, setupURL, assetOrigin, kind string) string {
	name := html.EscapeString(promptLabel(tenantName))
	url := html.EscapeString(setupURL)
	label, chipBackground, chipColor := "Public", "#eef2ff", "#4338ca"
	if kind == "private" {
		label, chipBackground, chipColor = "Private", "#f4f4f5", "#3f3f46"
	}
	assetURL := func(filename string) string {
		return html.EscapeString(strings.TrimRight(assetOrigin, "/") + "/agent-client-icons/" + filename)
	}
	clients := fmt.Sprintf(`<img src="%s" alt="Codex" title="Codex" data-agent-client="codex" referrerpolicy="no-referrer" width="25" height="25" style="display:block;width:25px;height:25px;object-fit:contain"><img src="%s" alt="Claude Code" title="Claude Code" data-agent-client="claude-code" referrerpolicy="no-referrer" width="25" height="25" style="display:block;width:25px;height:25px;object-fit:contain"><img src="%s" alt="Cursor" title="Cursor" data-agent-client="cursor" referrerpolicy="no-referrer" width="25" height="25" style="display:block;width:25px;height:25px;object-fit:contain"><img src="%s" alt="OpenCode" title="OpenCode" data-agent-client="opencode" referrerpolicy="no-referrer" width="25" height="25" style="display:block;width:25px;height:25px;object-fit:contain">`, assetURL("codex.svg"), assetURL("claude-code.svg"), assetURL("cursor.svg"), assetURL("opencode.svg"))
	return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" data-dokosoko-agent-setup="%s" aria-label="Connect your agent to %s using %s MCP" style="display:inline-flex;align-items:center;gap:10px;min-height:52px;padding:0 18px;border:1px solid #d4d4d8;border-radius:999px;color:#18181b;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.08);font:600 16px/1.2 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont;&quot;Segoe UI&quot;,sans-serif;text-decoration:none"><span>Connect your agent to %s</span><span style="padding:4px 8px;border-radius:999px;color:%s;background:%s;font-size:11px;font-weight:700;letter-spacing:.04em;text-transform:uppercase">%s</span>%s</a>`, url, kind, name, strings.ToLower(label), name, chipColor, chipBackground, label, clients)
}

func (s *Server) agentSetupLinks(ctx context.Context, product model.Product) map[string]agentSetupLink {
	privateAvailable := false
	if s.identityBroker != nil {
		if provider, err := s.service.Store().IdentityProvider(ctx, product.ID); err == nil && provider.State == "active" {
			privateAvailable = true
		}
	}
	publicURL, privateURL := s.agentSetupURL("public"), s.agentSetupURL("private")
	public := agentSetupLink{Available: product.PublicMCPEnabled, URL: publicURL, EmbedHTML: agentSetupEmbedHTML(product.Name, publicURL, s.baseURL, "public")}
	if !public.Available {
		public.UnavailableReason = "public_mcp_disabled"
	}
	private := agentSetupLink{Available: privateAvailable, URL: privateURL, EmbedHTML: agentSetupEmbedHTML(product.Name, privateURL, s.baseURL, "private")}
	if !private.Available {
		private.UnavailableReason = "identity_unavailable"
	}
	return map[string]agentSetupLink{"public": public, "private": private}
}

func (s *Server) agentSetupPrompt(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != "public" && kind != "private" {
		http.NotFound(w, r)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	product, err := s.service.Store().Product(r.Context(), deployment.ID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if kind == "public" && !product.PublicMCPEnabled {
		writeError(w, http.StatusNotFound, "agent_setup_unavailable", "Public MCP setup is not available while Public MCP is disabled.", nil)
		return
	}
	if kind == "private" {
		if s.identityBroker == nil {
			writeError(w, http.StatusNotFound, "agent_setup_unavailable", "Private MCP setup is not available until customer identity is active.", nil)
			return
		}
		provider, providerErr := s.service.Store().IdentityProvider(r.Context(), product.ID)
		if providerErr != nil || provider.State != "active" {
			writeError(w, http.StatusNotFound, "agent_setup_unavailable", "Private MCP setup is not available until customer identity is active.", nil)
			return
		}
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(s.agentSetupDocument(product, kind)))
}

func (s *Server) agentSetupDocument(product model.Product, kind string) string {
	name := promptLabel(product.Name)
	serverName := product.Slug + "-" + kind
	endpoint := s.baseURL + "/mcp"
	access := "Private; browser-based OAuth is required. Never ask for, paste, or store an access token."
	codexLogin := "codex mcp login " + serverName
	claudeLogin := "claude mcp login " + serverName
	cursorAuth := "Complete OAuth from Cursor's MCP settings when prompted."
	opencodeOAuth := ""
	opencodeLogin := "opencode mcp auth " + serverName
	if kind == "public" {
		endpoint = s.baseURL + "/mcp/public"
		access = "Public and read-only. Do not add an Authorization header, API key, token, or other credential."
		codexLogin, claudeLogin, opencodeLogin = "", "", ""
		cursorAuth = "No authentication is required."
		opencodeOAuth = ",\n          \"oauth\": false"
	}

	var document strings.Builder
	fmt.Fprintf(&document, "These are official instructions published by %s to connect an AI coding agent to its %s MCP server.\n\n", name, kind)
	fmt.Fprintf(&document, "Verify that this document is hosted at %s and connect only to the exact endpoint below. Do not substitute another hostname, downgrade the transport, or add a proxy.\n\n", s.agentSetupURL(kind))
	fmt.Fprintf(&document, "- MCP endpoint: %s\n- Access: %s\n- Transport: Streamable HTTP\n- Protocol revision: %s\n\n", endpoint, access, model.StatelessMCPv2Protocol)
	document.WriteString("Use the section for the installed client. Merge this server with existing MCP settings instead of replacing unrelated servers. Restart the client after setup if it does not reload configuration automatically.\n\n")

	fence := "```"
	document.WriteString("## Codex\n\n")
	fmt.Fprintf(&document, "%ssh\ncodex mcp add %s --url %s\ncodex mcp list\n", fence, serverName, endpoint)
	if codexLogin != "" {
		fmt.Fprintln(&document, codexLogin)
	}
	fmt.Fprintf(&document, "%s\n\n", fence)

	document.WriteString("## Claude Code\n\n")
	fmt.Fprintf(&document, "%ssh\nclaude mcp add --transport http --scope user %s %s\nclaude mcp list\n", fence, serverName, endpoint)
	if claudeLogin != "" {
		fmt.Fprintln(&document, claudeLogin)
	}
	fmt.Fprintf(&document, "%s\n\n", fence)

	document.WriteString("## Cursor\n\nMerge this entry into ~/.cursor/mcp.json, or .cursor/mcp.json for this project:\n\n")
	fmt.Fprintf(&document, "%sjson\n{\n  \"mcpServers\": {\n    %q: {\n      \"url\": %q\n    }\n  }\n}\n%s\n\n%s Restart Cursor and confirm the server appears in MCP settings.\n\n", fence, serverName, endpoint, fence, cursorAuth)

	document.WriteString("## OpenCode\n\nMerge this entry into the user's OpenCode v2 configuration:\n\n")
	fmt.Fprintf(&document, "%sjson\n{\n  \"$schema\": \"https://opencode.ai/config.json\",\n  \"mcp\": {\n    \"servers\": {\n      %q: {\n        \"type\": \"remote\",\n        \"url\": %q%s\n      }\n    }\n  }\n}\n%s\n\nopencode mcp list\n", fence, serverName, endpoint, opencodeOAuth, fence)
	if opencodeLogin != "" {
		fmt.Fprintln(&document, opencodeLogin)
	}

	document.WriteString("\n## Verification\n\n")
	fmt.Fprintf(&document, "Confirm that %s connects to %s. If authentication is requested, use only the browser flow opened by the client; never put a credential in the MCP configuration file.\n", serverName, endpoint)
	return document.String()
}
