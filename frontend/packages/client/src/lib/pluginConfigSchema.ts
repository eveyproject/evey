// A plugin's config schema (its manifest YAML) and the checks the admin's
// form runs before a Save. The web card and the phone screen kept two copies
// of these; they now read the same one.

import type { TFunction } from "i18next";

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

export function validateConfigField(key: string, value: string, schema: ConfigFieldSchema, t: TFunction): string | undefined {
  if (schema.required && value.trim() === "") return t("admin.plugins.validation.required", { field: key });
  if ((schema.type === "integer" || schema.type === "number") && value !== "") {
    const n = Number(value);
    if (isNaN(n)) return t("admin.plugins.validation.notNumber", { field: key });
    if (schema.type === "integer" && !Number.isInteger(n)) return t("admin.plugins.validation.notInteger", { field: key });
    if (schema.min !== undefined && n < schema.min) return t("admin.plugins.validation.min", { field: key, min: schema.min });
    if (schema.max !== undefined && n > schema.max) return t("admin.plugins.validation.max", { field: key, max: schema.max });
  }
  return undefined;
}
