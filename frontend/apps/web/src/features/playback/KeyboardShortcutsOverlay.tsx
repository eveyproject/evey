import { useEffect, useMemo } from "react";

interface KeyboardShortcutsOverlayProps {
  onClose: () => void;
  skipBackwardSeconds?: number;
  skipForwardSeconds?: number;
}

export function KeyboardShortcutsOverlay({ onClose, skipBackwardSeconds = 10, skipForwardSeconds = 10 }: KeyboardShortcutsOverlayProps) {
  const shortcuts: [string, string][] = useMemo(() => [
    ["Space / K", "Play / Pause"],
    ["F", "Toggle fullscreen"],
    ["M", "Toggle mute"],
    ["J / ←", `Seek back ${skipBackwardSeconds}s`],
    ["L / →", `Seek forward ${skipForwardSeconds}s`],
    ["↑", "Volume up"],
    ["↓", "Volume down"],
    ["0–9", "Seek to 0%–90%"],
    ["C", "Toggle captions"],
    ["< (,)", "Decrease speed"],
    ["> (.)", "Increase speed"],
    ["P", "Picture-in-Picture"],
    ["?", "Show shortcuts"],
  ], [skipBackwardSeconds, skipForwardSeconds]);
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
          Keyboard Shortcuts
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
