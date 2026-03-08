import { useEffect, type ReactNode } from "react";
import { useThemeStore } from "@evey/store";
import { applyThemeToDOM } from "@evey/theme";

export function ThemeProvider({ children }: { children: ReactNode }) {
  const { theme, customVars } = useThemeStore();

  useEffect(() => {
    applyThemeToDOM(theme, customVars ?? undefined);
  }, [theme, customVars]);

  return <>{children}</>;
}
