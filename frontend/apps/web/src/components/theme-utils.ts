const PLUGIN_THEME_PREFIX = "plugin:";

export function isPluginTheme(name: string): boolean {
  return name.startsWith(PLUGIN_THEME_PREFIX);
}

export function pluginNameFromTheme(name: string): string {
  return name.slice(PLUGIN_THEME_PREFIX.length);
}

export function pluginThemeName(pluginName: string): string {
  return PLUGIN_THEME_PREFIX + pluginName;
}
