export function useCssVar(name: string, fallback = "#888") {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}
