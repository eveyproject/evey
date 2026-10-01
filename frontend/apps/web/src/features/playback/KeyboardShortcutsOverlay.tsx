import { useEffect, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useFocusTrap } from "@/hooks/useFocusTrap";

interface KeyboardShortcutsOverlayProps {
  onClose: () => void;
  skipBackwardSeconds?: number;
  skipForwardSeconds?: number;
}

export function KeyboardShortcutsOverlay({ onClose, skipBackwardSeconds = 30, skipForwardSeconds = 30 }: KeyboardShortcutsOverlayProps) {
  const { t } = useTranslation();
  const dialogRef = useRef<HTMLDivElement>(null);
  // Always active - component is only mounted when the overlay is open
  useFocusTrap(dialogRef, true);
  const shortcuts: [string, string][] = useMemo(() => [
    ["Space / K", t("player.shortcuts.playPause")],
    ["F", t("player.shortcuts.toggleFullscreen")],
    ["M", t("player.shortcuts.toggleMute")],
    ["J / \u2190", t("player.shortcuts.seekBack", { seconds: skipBackwardSeconds })],
    ["L / \u2192", t("player.shortcuts.seekForward", { seconds: skipForwardSeconds })],
    ["\u2191", t("player.shortcuts.volumeUp")],
    ["\u2193", t("player.shortcuts.volumeDown")],
    ["0-9", t("player.shortcuts.seekPercent")],
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
      className="absolute inset-0 z-overlay flex items-center justify-center bg-[var(--overlay-bg-strong)]"
      onClick={onClose}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label={t("player.shortcuts.title")}
        tabIndex={-1}
        className="w-full max-w-md rounded-[var(--radius-md)] p-6 shadow-2xl bg-[var(--surface)] border border-[var(--border)] outline-none"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold mb-4 text-[var(--text)]">
          {t("player.shortcuts.title")}
        </h2>
        <div className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2">
          {shortcuts.map(([key, action]) => (
            <div key={key} className="contents">
              <kbd
                className="px-2 py-0.5 rounded text-xs font-mono text-right bg-[var(--surface-raised)] text-[var(--text)] border border-[var(--border)]"
              >
                {key}
              </kbd>
              <span className="text-sm text-[var(--text-muted)]">
                {action}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
