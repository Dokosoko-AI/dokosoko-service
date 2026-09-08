import { parseConsolePath, settingsPath } from "./console-routes";

export function aiSetupReturnPath(search: string) {
  const value = new URLSearchParams(search).get("returnTo");
  if (!value || value.length > 2048 || !value.startsWith("/") || value.startsWith("//") || (value.includes("\\") || Array.from(value).some((character) => character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127))) return "";
  const url = new URL(value, "https://console.invalid");
  if (url.origin !== "https://console.invalid" || parseConsolePath(url.pathname).kind === "not-found" || url.pathname === settingsPath("ai")) return "";
  return url.pathname + url.search;
}
export function aiSettingsForSetup(path: string) {
  const returnTo = aiSetupReturnPath(new URLSearchParams({ returnTo: path }).toString());
  return settingsPath("ai") + (returnTo ? `?${new URLSearchParams({ returnTo })}` : "");
}
