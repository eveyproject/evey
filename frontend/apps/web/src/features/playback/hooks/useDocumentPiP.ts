import { useCallback, useEffect, useRef, useState } from "react";

// Document Picture-in-Picture brings the WHOLE player UI (controls, progress
// bar, subtitles, chapter skip, next episode prompt, ...) into a separate
// always-on-top window — not just the bare <video> element of the standard
// PiP API. Chrome/Edge 116+ only; we feature-detect and the caller falls
// back to the legacy PiP when documentPictureInPicture is undefined.
//
// Implementation pattern: we move the existing player container DOM node
// into the PiP window via appendChild. The hls.js MediaSource keeps working
// through the move because the <video> element identity is preserved.
// On close we restore the node into its original sibling position. The
// node-move approach (instead of React.createPortal) avoids refactoring
// the 26KB HLSPlayer.tsx, at the cost of "if you unmount the player while
// PiP is open, close the PiP first".

interface DocumentPiPGlobal {
  requestWindow(opts: {
    width: number;
    height: number;
    disallowReturnToOpener?: boolean;
    preferInitialWindowPlacement?: boolean;
  }): Promise<Window>;
  window: Window | null;
}

declare global {
  interface Window {
    documentPictureInPicture?: DocumentPiPGlobal;
  }
}

export interface UseDocumentPiPResult {
  /** True only when the browser actually supports Document PiP (Chrome 116+ et al). */
  available: boolean;
  /** True while the PiP window is open. */
  isOpen: boolean;
  /** Open or close, depending on current state. No-op when available=false. */
  toggle: () => Promise<void>;
}

/**
 * Hook that drives Document PiP for an element identified by a CSS selector
 * (defaults to `[data-player-container]`, the wrapper around <video> + the
 * controls overlay in HLSPlayer.tsx).
 */
export function useDocumentPiP(selector = "[data-player-container]"): UseDocumentPiPResult {
  const [available, setAvailable] = useState(false);
  const [isOpen, setIsOpen] = useState(false);

  // Anchor refs so we can restore the moved node into its original spot.
  // We capture the original parent + nextSibling at open time and put the
  // container back at close. Don't rely on these between hook re-renders —
  // they're only valid for the lifetime of one open/close pair.
  const originalParent = useRef<Node | null>(null);
  const originalNextSibling = useRef<Node | null>(null);

  useEffect(() => {
    setAvailable(typeof window !== "undefined" && !!window.documentPictureInPicture);
  }, []);

  const close = useCallback(() => {
    const pipWindow = window.documentPictureInPicture?.window;
    if (pipWindow) pipWindow.close();
  }, []);

  const open = useCallback(async () => {
    if (!window.documentPictureInPicture) return;
    const container = document.querySelector<HTMLElement>(selector);
    if (!container) {
      console.warn("[doc-pip] no element matches", selector);
      return;
    }

    const pipWindow = await window.documentPictureInPicture.requestWindow({
      width: 480,
      height: 270, // 16:9
      disallowReturnToOpener: false,
      preferInitialWindowPlacement: true,
    });

    copyStylesheets(document, pipWindow.document);

    // Capture the restoration anchor BEFORE moving — once moved, parentNode
    // becomes the PiP body.
    originalParent.current = container.parentNode;
    originalNextSibling.current = container.nextSibling;

    pipWindow.document.body.append(container);
    // The PiP window starts blank; give it Evey's dark theme so the
    // controls overlay stays readable.
    pipWindow.document.body.style.margin = "0";
    pipWindow.document.body.style.background = "#000";
    pipWindow.document.body.style.height = "100vh";
    pipWindow.document.body.style.overflow = "hidden";

    setIsOpen(true);

    pipWindow.addEventListener(
      "pagehide",
      () => {
        // Restore in the same DOM position so subsequent fullscreen, layout
        // observers, etc. keep working without confusion.
        const parent = originalParent.current;
        const next = originalNextSibling.current;
        if (parent) {
          if (next && next.parentNode === parent) {
            parent.insertBefore(container, next);
          } else {
            parent.appendChild(container);
          }
        }
        originalParent.current = null;
        originalNextSibling.current = null;
        setIsOpen(false);
      },
      { once: true },
    );
  }, [selector]);

  const toggle = useCallback(async () => {
    if (!available) return;
    if (isOpen) {
      close();
    } else {
      await open();
    }
  }, [available, isOpen, open, close]);

  // Defensive cleanup: if the React tree owning this hook unmounts while
  // the PiP is still open, the moved DOM node would become an orphan
  // (its React fiber is gone and nobody will restore it). Close the PiP
  // before unmount so the pagehide handler puts it back first.
  useEffect(() => {
    return () => {
      if (window.documentPictureInPicture?.window) {
        window.documentPictureInPicture.window.close();
      }
    };
  }, []);

  return { available, isOpen, toggle };
}

/**
 * copyStylesheets clones every <link rel="stylesheet"> and inline <style>
 * from the main document into the PiP window. Without this, Tailwind
 * classes don't apply and the moved controls render as raw HTML.
 *
 * Cross-origin stylesheets (those without cssRules access) are copied via
 * a <link> tag fallback. Same-origin sheets get their cssText copied as
 * an inline <style> for faster paint.
 */
function copyStylesheets(src: Document, dst: Document) {
  for (const sheet of Array.from(src.styleSheets)) {
    try {
      // Same-origin: re-emit cssText. Beats waiting on a fresh HTTP fetch.
      const cssText = Array.from(sheet.cssRules)
        .map((rule) => rule.cssText)
        .join("\n");
      const style = dst.createElement("style");
      style.textContent = cssText;
      dst.head.appendChild(style);
    } catch {
      // Cross-origin: clone the <link>.
      if (sheet.href) {
        const link = dst.createElement("link");
        link.rel = "stylesheet";
        link.href = sheet.href;
        dst.head.appendChild(link);
      }
    }
  }
}
