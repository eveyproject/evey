// t() returns the raw fallback when i18next has no loaded resources yet
// (screens reachable before a server is connected). Interpolate manually so
// "{{count}} server(s) found" never reaches the screen.
export function interpolateFallback(s: string, vars: Record<string, string | number>): string {
  if (!s.includes("{{")) return s;
  return s.replace(/\{\{(\w+)\}\}/g, (match, key: string) =>
    Object.prototype.hasOwnProperty.call(vars, key) ? String(vars[key]) : match,
  );
}
