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
