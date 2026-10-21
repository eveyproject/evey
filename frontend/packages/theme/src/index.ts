export * from "./tokens";
export * from "./themeVars";

// Web only: apply theme via CSS custom properties
export function applyThemeToDOM(name: string, customVars?: Record<string, string>) {
  const link = document.getElementById("theme-link") as HTMLLinkElement | null;
  if (customVars) {
    const root = document.documentElement;
    Object.entries(customVars).forEach(([k, v]) => root.style.setProperty(k, v));
  } else if (link) {
    // href and media are written only when they change: rewriting either
    // makes the browser load the sheet again, and a layout read while it is
    // pending forces a layout on an unstyled theme (Firefox: "Layout was
    // forced before the page was fully loaded"). index.html already links the
    // default theme, with no media attribute, which means all media.
    const href = `/themes/${name}.css`;
    if (link.getAttribute("href") !== href) link.href = href;
    if (link.media !== "" && link.media !== "all") link.media = "all";
  }
}
