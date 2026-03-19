import { useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";

interface KeyboardShortcutsOverlayProps {
  onClose: () => void;
  skipBackwardSeconds?: number;
  skipForwardSeconds?: number;
}

export function KeyboardShortcutsOverlay({ onClose, skipBackwardSeconds = 10, skipForwardSeconds = 10 }: KeyboardShortcutsOverlayProps) {
  const { t } = useTranslation();
  const shortcuts: [string, string][] = useMemo(() => [
    ["Space / K", t("player.shortcuts.playPause")],
    ["F", t("player.shortcuts.toggleFullscreen")],
    ["M", t("player.shortcuts.toggleMute")],
    ["J / \u2190", t("player.shortcuts.seekBack", { seconds: skipBackwardSeconds })],
    ["L / \u2192", t("player.shortcuts.seekForward", { seconds: skipForwardSeconds })],
    ["\u2191", t("player.shortcuts.volumeUp")],
    ["\u2193", t("player.shortcuts.volumeDown")],
    ["0\u20139", t("player.shortcuts.seekPercent")],
    ["C", t("player.shortcuts.toggleCaptions")],
    ["< (,)", t("player.shortcuts.decreaseSpeed")],
    ["> (.)", t("player.shortcuts.increaseSpeed")],
    ["P", t("player.shortcuts.pip")],
    ["?", t("player.shortcuts.showShortcuts")],
  ], [skipBackwardSeconds, skipForwardSeconds, t]);
  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape" || e.key === "?") {
        e.preventDefault();
        e.stopPropagation();
        onClose();
      }
    }
    document.addEventListener("keydown", handleKey, true);
    return () => document.removeEventListener("keydown", handleKey, true);
  }, [onClose]);

  return (
    <div
      className="absolute inset-0 z-50 flex items-center justify-center"
      style={{ backgroundColor: "rgba(0,0,0,0.75)" }}
      onClick={onClose}
    >
      <div
        className="w-full max-w-md rounded-[var(--radius-md)] p-6 shadow-2xl"
        style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold mb-4" style={{ color: "var(--text)" }}>
          {t("player.shortcuts.title")}
        </h2>
        <div className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2">
          {shortcuts.map(([key, action]) => (
            <div key={key} className="contents">
              <kbd
                className="px-2 py-0.5 rounded text-xs font-mono text-right"
                style={{
                  backgroundColor: "var(--surface-raised)",
                  color: "var(--text)",
                  border: "1px solid var(--border)",
                }}
              >
                {key}
              </kbd>
              <span className="text-sm" style={{ color: "var(--text-muted)" }}>
                {action}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
