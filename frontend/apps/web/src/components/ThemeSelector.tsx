import { useThemeStore } from "@evey/store";
import { themes, type ThemeName } from "@evey/theme";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { isPluginTheme, pluginThemeName, pluginNameFromTheme } from "./ThemeProvider";

const builtInLabels: Record<ThemeName, string> = {
  default: "Dark",
  amoled: "AMOLED",
  "light-default": "Light",
  "light-blue": "Light Blue",
};

interface PluginThemeInfo {
  id: string;
  name: string;
}

function usePluginThemes() {
  return useQuery<PluginThemeInfo[]>({
    queryKey: ["plugin-themes"],
    queryFn: async () => {
      const data = await eveyFetch<{ plugins: Array<{ id: string; name: string; capabilities: string[]; enabled: boolean }> }>({
        url: "/api/v1/admin/plugins",
        method: "GET",
      });
      return (data.plugins ?? [])
        .filter((p) => p.enabled && p.capabilities?.includes("ui.theme"))
        .map((p) => ({ id: p.id, name: p.name }));
    },
    staleTime: 60_000,
  });
}

// Store plugin ID → name mapping for the theme loader
const pluginThemeMap = new Map<string, string>();

export function getPluginIdForTheme(pluginName: string): string | undefined {
  return pluginThemeMap.get(pluginName);
}

export function ThemeSelector() {
  const { theme, setTheme } = useThemeStore();
  const { data: pluginThemes } = usePluginThemes();

  // Keep the ID mapping up to date
  pluginThemes?.forEach((pt) => pluginThemeMap.set(pt.name, pt.id));

  return (
    <div className="flex flex-col gap-2">
      <label className="text-sm font-medium" style={{ color: "var(--text-muted)" }}>Theme</label>
      <div className="flex gap-2 flex-wrap">
        {(Object.keys(themes) as ThemeName[]).map((name) => (
          <button
            key={name}
            onClick={() => setTheme(name)}
            className={`px-3 py-1.5 rounded-[var(--radius-md)] text-sm transition-colors ${
              theme === name
                ? "bg-[var(--accent)] text-white"
                : "bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            {builtInLabels[name]}
          </button>
        ))}
        {pluginThemes?.map((pt) => {
          const themeKey = pluginThemeName(pt.name);
          return (
            <button
              key={pt.id}
              onClick={() => setTheme(themeKey)}
              className={`px-3 py-1.5 rounded-[var(--radius-md)] text-sm transition-colors ${
                theme === themeKey
                  ? "bg-[var(--accent)] text-white"
                  : "bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text)]"
              }`}
            >
              {pt.name}
            </button>
          );
        })}
      </div>
    </div>
  );
}
