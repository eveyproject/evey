const THEME_LINK_PREFIX = "evey-plugin-theme-";

export function loadPluginTheme(pluginId: string): void {
  const id = THEME_LINK_PREFIX + pluginId;
  if (document.getElementById(id)) return;

  const link = document.createElement("link");
  link.id = id;
  link.rel = "stylesheet";
  link.href = `/api/v1/plugins/${pluginId}/theme.css`;
  document.head.appendChild(link);
}

export function unloadAllPluginThemes(): void {
  const links = document.querySelectorAll(`link[id^="${THEME_LINK_PREFIX}"]`);
  links.forEach((el) => el.remove());
}
