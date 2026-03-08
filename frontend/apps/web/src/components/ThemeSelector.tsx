import { useThemeStore } from "@evey/store";
import { themes, type ThemeName } from "@evey/theme";

export function ThemeSelector() {
  const { theme, setTheme } = useThemeStore();

  return (
    <div className="flex flex-col gap-2">
      <label className="text-sm font-medium text-[var(--text-muted)]">Theme</label>
      <div className="flex gap-2">
        {(Object.keys(themes) as ThemeName[]).map((name) => (
          <button
            key={name}
            onClick={() => setTheme(name)}
            className={`px-3 py-1.5 rounded-[var(--radius-md)] text-sm capitalize transition-colors ${
              theme === name
                ? "bg-[var(--accent)] text-white"
                : "bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            {name}
          </button>
        ))}
      </div>
    </div>
  );
}
