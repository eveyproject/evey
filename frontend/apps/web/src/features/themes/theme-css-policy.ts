// evey.theme.css() injects into the PARENT document at runtime and never meets
// the Go sanitizer, which only runs on theme.css at install. Instead of
// re-implementing it here, accept one shape: custom property declarations. A
// `--x: value` only acts where the already-sanitized stylesheet references it.

const MAX_DECLARATIONS = 200;
const MAX_VALUE_LENGTH = 500;
const CUSTOM_PROPERTY_NAME = /^--[A-Za-z0-9_-]{1,100}$/;

// Mirrors the Go reCustomPropExternalURL pass. Backslashes go too: CSS escapes
// are how the server-side regexes get bypassed.
// No `;` here: splitDeclarations already cut on the top-level ones, so any that
// remains is inside a quoted font name or a parenthesised value.
const FORBIDDEN_IN_VALUE = /url\s*\(|image-set|expression\s*\(|@import|javascript\s*:|data\s*:|https?\s*:|\/\/|\/\*|[{}<>\\]/i;

export interface ThemeCSSPolicyResult {
  css: string;
  accepted: number;
  rejected: string[];
}

// splitDeclarations cuts on top-level `;` only, so a semicolon inside a quoted
// font name or a parenthesised value does not fragment a declaration.
function splitDeclarations(raw: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let quote: string | null = null;
  let start = 0;

  for (let i = 0; i < raw.length; i++) {
    const c = raw[i];
    if (quote) {
      if (c === quote) quote = null;
      continue;
    }
    if (c === '"' || c === "'") {
      quote = c;
    } else if (c === "(" || c === "[") {
      depth++;
    } else if (c === ")" || c === "]") {
      if (depth > 0) depth--;
    } else if (c === ";" && depth === 0) {
      out.push(raw.slice(start, i));
      start = i + 1;
    }
  }
  out.push(raw.slice(start));
  return out;
}

export function applyThemeCSSPolicy(raw: string): ThemeCSSPolicyResult {
  const accepted: string[] = [];
  const rejected: string[] = [];

  for (const decl of splitDeclarations(raw)) {
    const trimmed = decl.trim();
    if (!trimmed) continue;
    if (accepted.length >= MAX_DECLARATIONS) {
      rejected.push(trimmed.slice(0, 60));
      continue;
    }

    const colon = trimmed.indexOf(":");
    if (colon < 0) {
      rejected.push(trimmed.slice(0, 60));
      continue;
    }

    const name = trimmed.slice(0, colon).trim();
    const value = trimmed.slice(colon + 1).trim();

    if (!CUSTOM_PROPERTY_NAME.test(name)) {
      rejected.push(trimmed.slice(0, 60));
      continue;
    }
    if (!value || value.length > MAX_VALUE_LENGTH || FORBIDDEN_IN_VALUE.test(value)) {
      rejected.push(trimmed.slice(0, 60));
      continue;
    }

    accepted.push(`${name}:${value}`);
  }

  return {
    css: accepted.length ? `:root{${accepted.join(";")}}` : "",
    accepted: accepted.length,
    rejected,
  };
}
