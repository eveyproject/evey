import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { toast, useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { BRIDGE_SCRIPT } from "./widget-bridge";

interface PluginIframeProps {
  pluginId: string;
  src: string;
  context?: Record<string, unknown>;
}

function getThemeVars(): string {
  const computed = getComputedStyle(document.documentElement);
  const vars: string[] = [];
  for (const sheet of document.styleSheets) {
    try {
      for (const rule of sheet.cssRules) {
        if (rule instanceof CSSStyleRule && rule.selectorText === ":root") {
          for (let i = 0; i < rule.style.length; i++) {
            const prop = rule.style[i];
            if (prop.startsWith("--")) {
              vars.push(`${prop}: ${computed.getPropertyValue(prop).trim()};`);
            }
          }
        }
      }
    } catch {
      // cross-origin stylesheets — skip
    }
  }
  return vars.join("\n    ");
}

function detectThemeMode(): "dark" | "light" {
  const bg = getComputedStyle(document.documentElement).getPropertyValue("--bg").trim();
  if (!bg) return "dark";
  const temp = document.createElement("div");
  temp.style.color = bg;
  document.body.appendChild(temp);
  const rgb = getComputedStyle(temp).color;
  temp.remove();
  const match = rgb.match(/\d+/g);
  if (!match) return "dark";
  const luminance = (Number(match[0]) * 299 + Number(match[1]) * 587 + Number(match[2]) * 114) / 1000;
  return luminance > 128 ? "light" : "dark";
}

function buildSrcdoc(pluginId: string, token: string): string {
  const themeVars = getThemeVars();
  const scriptUrl = `/api/v1/plugins/${pluginId}/widget.js?token=${encodeURIComponent(token)}`;
  return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    *, *::before, *::after { box-sizing: border-box; }
    html, body { margin: 0; padding: 0; background: transparent; font-family: system-ui, -apple-system, sans-serif; }
    :root {
    ${themeVars}
    }
    body { color: var(--text, #f1f5f9); }
  </style>
  <script>${BRIDGE_SCRIPT}<\/script>
</head>
<body>
  <script src="${scriptUrl}"><\/script>
</body>
</html>`;
}

export function PluginIframe({ pluginId, src: _src, context }: PluginIframeProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [height, setHeight] = useState(100);
  const [error, setError] = useState(false);
  const navigate = useNavigate();
  const { t, i18n } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);

  const handleMessage = useCallback(
    (e: MessageEvent) => {
      const iframe = iframeRef.current;
      if (!iframe || e.source !== iframe.contentWindow) return;
      if (e.origin !== "null" && e.origin !== window.location.origin) return;

      const data = e.data as Record<string, unknown>;
      if (!data || typeof data.type !== "string") return;

      switch (data.type) {
        case "ready":
          iframe.contentWindow?.postMessage(
            {
              type: "init",
              context: context ?? {},
              locale: i18n.language,
              theme: detectThemeMode(),
            },
            "*",
          );
          break;
        case "navigate":
          if (typeof data.path === "string") {
            navigate({ to: data.path });
          }
          break;
        case "toast":
          if (typeof data.message === "string") {
            const level = data.level as "success" | "error" | "info";
            if (level === "success") toast.success(data.message);
            else if (level === "error") toast.error(data.message);
            else toast.info(data.message);
          }
          break;
        case "resize":
          if (typeof data.height === "number" && data.height > 0) {
            setHeight(data.height);
          }
          break;
      }
    },
    [context, i18n.language, navigate],
  );

  useEffect(() => {
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [handleMessage]);

  if (!accessToken) return null;

  if (error) {
    return (
      <div style={{ color: "var(--text-muted)", padding: "8px 0", fontSize: "0.875rem" }}>
        {t("admin.plugin.widgetLoadError")}
      </div>
    );
  }

  return (
    <iframe
      ref={iframeRef}
      sandbox="allow-scripts"
      srcDoc={buildSrcdoc(pluginId, accessToken)}
      onError={() => setError(true)}
      style={{
        border: "none",
        width: "100%",
        height: `${height}px`,
        overflow: "hidden",
        display: "block",
      }}
      title={`${t("admin.plugin.sandboxedWidget")} - ${pluginId}`}
    />
  );
}
