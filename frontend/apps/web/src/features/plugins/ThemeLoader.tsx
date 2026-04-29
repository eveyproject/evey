// Loads all enabled themes and injects their CSS into isolated ShadowRoot containers.
// Each theme gets its own <div data-theme-id="..."> with a shadow root so that
// theme CSS rules are scoped and cannot leak into the main document.

import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";

interface ThemePlugin {
  id: string;
  kind: string;
  enabled: boolean;
  frontend?: Array<{ slot: string; src: string }>;
}

interface ListPluginsResponse {
  plugins?: ThemePlugin[];
}

export function ThemeLoader() {
  const isAuthenticated = useAuthStore((s) => !!s.accessToken);

  const { data } = useQuery<ListPluginsResponse>({
    queryKey: ["plugins", "list"],
    queryFn: () =>
      eveyFetch<ListPluginsResponse>({ url: "/api/v1/admin/plugins", method: "GET" }),
    staleTime: 60_000,
    enabled: isAuthenticated,
  });

  const themes = (data?.plugins ?? []).filter(
    (p) => p.kind === "theme" && p.enabled && !!p.id,
  );

  useEffect(() => {
    if (themes.length === 0) return;

    const hostElements: HTMLDivElement[] = [];

    for (const theme of themes) {
      const themeId = theme.id;
      const existingHost = document.querySelector(`[data-theme-id="${CSS.escape(themeId)}"]`);
      if (existingHost) continue;

      const host = document.createElement("div");
      host.setAttribute("data-theme-id", themeId);
      host.style.display = "none";
      document.body.appendChild(host);
      hostElements.push(host);

      const shadow = host.attachShadow({ mode: "closed" });

      fetch(`/api/v1/themes/${encodeURIComponent(themeId)}/theme.css`)
        .then((res) => {
          if (!res.ok) throw new Error(`HTTP ${res.status}`);
          return res.text();
        })
        .then((css) => {
          const style = document.createElement("style");
          style.textContent = css;
          shadow.appendChild(style);
        })
        .catch((err: unknown) => {
          console.warn(`[ThemeLoader] failed to load CSS for theme "${themeId}":`, err);
        });
    }

    return () => {
      for (const host of hostElements) {
        host.remove();
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [themes.map((t) => t.id).join(",")]);

  return null;
}
