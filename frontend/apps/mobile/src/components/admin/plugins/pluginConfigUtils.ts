import type { PluginSignatureInfo } from "./pluginSignature";
// Pure helpers of the admin plugins screen (mirrors
// apps/web/src/routes/_auth/admin/plugins.lazy.tsx) - extracted here so the
// escalation parsing can be unit tested without mounting the screen. The
// config schema and its validation live in @evey/client (pluginConfigSchema).

// parseBlockedIssues pulls the findings out of the install refusal produced by
// InstallerService (internal/app/plugin/install.go: "plugin blocked: critical
// security issues found: [a b c]").
//
// The server runs the same static analysis whatever the client, so mobile was
// never LESS safe than web here - it just printed the raw Go error in a toast,
// where web lists the findings. An admin deciding whether to trust a plugin
// should not have to read a stringified slice.
export function parseBlockedIssues(msg: string): string[] {
  const marker = "plugin blocked: ";
  const idx = msg.indexOf(marker);
  if (idx < 0) return [];
  let body = msg.slice(idx + marker.length).trim();
  // Drop the "critical security issues found:" preamble when it is there; other
  // refusals (forbidden theme.js pattern, analyzer failure) have none.
  const found = body.indexOf("issues found:");
  if (found >= 0) body = body.slice(found + "issues found:".length).trim();
  // The Go slice arrives as "[a b c]" - one finding per element, space
  // separated, which is the best the wire format offers.
  if (body.startsWith("[") && body.endsWith("]")) {
    return body
      .slice(1, -1)
      .split(/\s{1,}/)
      .map((x) => x.trim())
      .filter(Boolean);
  }
  return body ? [body] : [];
}

// parseEscalationEntries pulls "permission:network, capability:foo, ..." out
// of the gRPC error message produced by checkPermissionEscalation server-side
// (mirrors apps/web/src/routes/_auth/admin/plugins.lazy.tsx).
export function parseEscalationEntries(msg: string): string[] {
  const marker = "new entries requested: ";
  const idx = msg.indexOf(marker);
  if (idx < 0) return [];
  return msg
    .slice(idx + marker.length)
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

// InstallPreview is what POST /admin/plugins/preview returns for a source:
// the manifest facts plus the static analysis, BEFORE anything is installed.
export interface InstallPreview {
  name: string;
  version: string;
  description?: string;
  author?: string;
  language?: string;
  permissions?: string[];
  capabilities?: string[];
  source_hash?: string;
  // Present when a signature was provided with the package.
  signature?: PluginSignatureInfo;
  analysis?: { safe?: boolean; total_files_scanned?: number; issues?: string[]; urls?: string[] };
}

// installPreviewHasWarnings mirrors the web page: a subprocess permission or
// any analysis finding turns the confirm button into "Install anyway".
export function installPreviewHasWarnings(p: InstallPreview): boolean {
  return !!p.permissions?.includes("subprocess") || (p.analysis?.issues?.length ?? 0) > 0;
}
