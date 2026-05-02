// Deprecated — CSS/JS theme lifecycle is now owned by ThemeLoader
// (frontend/apps/web/src/features/themes/ThemeLoader.tsx).
// These stubs exist to keep ThemeProvider.tsx compiling without modification.
// ThemeLoader is mounted in __root.tsx alongside ThemeProvider and handles
// <link> injection + iframe sandbox for theme.js.

/** @deprecated Use ThemeLoader (features/themes/ThemeLoader.tsx) */
export function loadPluginTheme(_pluginId: string): void {
  // no-op: ThemeLoader reacts to useThemeStore.pluginId changes
}

/** @deprecated Use ThemeLoader (features/themes/ThemeLoader.tsx) */
export function unloadAllPluginThemes(): void {
  // no-op: ThemeLoader cleans up on pluginId → null transition
}
