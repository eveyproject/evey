import { useThemeStore } from "@evey/store";
import { themes, type ThemeName } from "@evey/theme";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { pluginThemeName } from "./theme-utils";

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
      const data = await eveyFetch<{ themes: PluginThemeInfo[] }>({
        url: "/api/v1/plugins/themes",
        method: "GET",
      });
      return data.themes ?? [];
    },
    staleTime: 60_000,
  });
}

export function ThemeSelector() {
  const { theme, setTheme } = useThemeStore();
  const { data: pluginThemes } = usePluginThemes();

  return (
    <div className="flex flex-col gap-2">
      <label className="text-sm font-medium text-[var(--text-muted)]">Theme</label>
      <div className="flex gap-2 flex-wrap">
        {(Object.keys(themes) as ThemeName[]).map((name) => (
          <button
            key={name}
            onClick={() => setTheme(name)}
            className={`px-3 py-1.5 rounded-[var(--radius-md)] text-sm transition-colors-fast ${
              theme === name
                ? "bg-[var(--accent)] text-[var(--filter-active-text)]"
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
              onClick={() => setTheme(themeKey, pt.id)}
              className={`px-3 py-1.5 rounded-[var(--radius-md)] text-sm transition-colors-fast ${
                theme === themeKey
                  ? "bg-[var(--accent)] text-[var(--filter-active-text)]"
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
