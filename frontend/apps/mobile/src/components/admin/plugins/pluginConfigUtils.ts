// Pure helpers shared by the admin plugins screen (mirrors
// apps/web/src/features/admin/PluginCard.tsx and
// apps/web/src/routes/_auth/admin/plugins.lazy.tsx) - extracted here so the
// schema rendering / validation / escalation-parsing logic can be unit
// tested without mounting the screen.

export interface ConfigFieldSchema {
  type?: string;
  label?: string;
  description?: string;
  default?: unknown;
  required?: boolean;
  // YAML accepts either a flat list of strings or a list of {value,label}
  // objects (the documented standard form). Both shapes are normalised at
  // render time.
  options?: Array<string | { value: string; label?: string }>;
  min?: number;
  max?: number;
}

export function normaliseOptions(
  opts: ConfigFieldSchema["options"],
): Array<{ value: string; label: string }> {
  return (opts ?? []).map((o) =>
    typeof o === "string"
      ? { value: o, label: o }
      : { value: String(o.value), label: o.label ?? String(o.value) },
  );
}

export function validateConfigField(key: string, value: string, schema: ConfigFieldSchema): string | undefined {
  if (schema.required && value.trim() === "") return `${key} est requis`;
  if ((schema.type === "integer" || schema.type === "number") && value !== "") {
    const n = Number(value);
    if (isNaN(n)) return `${key} doit être un nombre`;
    if (schema.type === "integer" && !Number.isInteger(n)) return `${key} doit être un entier`;
    if (schema.min !== undefined && n < schema.min) return `${key} doit être >= ${schema.min}`;
    if (schema.max !== undefined && n > schema.max) return `${key} doit être <= ${schema.max}`;
  }
  return undefined;
}

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
  analysis?: { safe?: boolean; file_count?: number; issues?: string[]; urls?: string[] };
}

// installPreviewHasWarnings mirrors the web page: a subprocess permission or
// any analysis finding turns the confirm button into "Install anyway".
export function installPreviewHasWarnings(p: InstallPreview): boolean {
  return !!p.permissions?.includes("subprocess") || (p.analysis?.issues?.length ?? 0) > 0;
}
