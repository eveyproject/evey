import { useEffect, useCallback, useState } from "react";
import { useNavigate } from "@tanstack/react-router";

function isInputFocused(): boolean {
  const el = document.activeElement;
  if (!el) return false;
  const tag = el.tagName.toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select" || (el as HTMLElement).isContentEditable;
}

export function useKeyboardShortcuts(enabled = true) {
  const navigate = useNavigate();
  const [showHelp, setShowHelp] = useState(false);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      // Ctrl+K handled in TopBar already; add "/" to focus search
      if (e.key === "/" && !isInputFocused()) {
        e.preventDefault();
        void navigate({ to: "/search" });
        setTimeout(() => {
          const input = document.querySelector<HTMLInputElement>('input[type="search"]');
          input?.focus();
        }, 50);
        return;
      }

      if (e.key === "Escape") {
        // blur any focused input
        if (isInputFocused()) {
          (document.activeElement as HTMLElement)?.blur();
          return;
        }
        // close help if open
        if (showHelp) {
          setShowHelp(false);
          return;
        }
      }

      if (isInputFocused()) return;

      if (e.key === "h") {
        void navigate({ to: "/home" });
        return;
      }

      if (e.key === "?") {
        e.preventDefault();
        setShowHelp((v) => !v);
      }
    },
    [navigate, showHelp],
  );

  useEffect(() => {
    if (!enabled) return;
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [handleKeyDown, enabled]);

  return { showHelp, setShowHelp };
}
