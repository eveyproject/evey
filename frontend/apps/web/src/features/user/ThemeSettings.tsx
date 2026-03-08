import { ThemeSelector } from "@/components/ThemeSelector";

export function ThemeSettings() {
  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-sm font-semibold text-[var(--text)]">Appearance</h3>
      <p className="text-xs text-[var(--text-muted)]">Choose a color theme for the interface.</p>
      <ThemeSelector />
    </div>
  );
}
