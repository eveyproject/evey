export * from "./tokens";

// Web only: apply theme via CSS custom properties
export function applyThemeToDOM(name: string, customVars?: Record<string, string>) {
  const link = document.getElementById("theme-link") as HTMLLinkElement | null;
  if (customVars) {
    const root = document.documentElement;
    Object.entries(customVars).forEach(([k, v]) => root.style.setProperty(k, v));
  } else if (link) {
    link.href = `/themes/${name}.css`;
  }
}
