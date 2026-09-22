import { useEffect, type ReactNode } from "react";
import { init, setKeyMap, useFocusable, FocusContext, setFocus } from "@noriginmedia/norigin-spatial-navigation";
import { tvKeyOf, targetIsEditable } from "./keys";

// Spatial navigation: the library decides which focusable is "to the right"
// of the current one from geometry, so screens are built out of ordinary
// boxes and the remote walks them. Initialised once, before the first
// render. The key map feeds it the codes the television browsers send
// (see keys.ts) so webOS 6's keyCode-only events still move the focus.
let initialised = false;
export function initTvNavigation(): void {
  if (initialised) return;
  initialised = true;
  init({ debug: false, visualDebug: false, throttle: 0 });
  setKeyMap({
    left: [37],
    up: [38],
    right: [39],
    down: [40],
    enter: [13],
  });
}

export { useFocusable, FocusContext, setFocus };

/**
 * Screen-level "back" handling. Focus movement and "select" belong to the
 * spatial library; back (Escape, the TV's back button) and the media keys
 * are the app's, and a text field keeps its own keys except back.
 */
export function useTvKeys(handler: (key: "back" | "play" | "pause" | "playPause" | "rewind" | "forward" | "stop" | "search", e: KeyboardEvent) => void): void {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const key = tvKeyOf(e);
      if (!key) return;
      if (key === "left" || key === "right" || key === "up" || key === "down" || key === "select") return;
      if (targetIsEditable(e.target) && key !== "back") return;
      e.preventDefault();
      handler(key, e);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [handler]);
}

interface FocusRootProps {
  focusKey?: string;
  /** Focus the first child as soon as the screen mounts. */
  autoFocus?: boolean;
  className?: string;
  children: ReactNode;
}

/** A focus scope: what the remote can reach on this screen, or in this row. */
export function FocusRoot({ focusKey, autoFocus, className, children }: FocusRootProps) {
  const { ref, focusKey: key, focusSelf } = useFocusable({ focusKey, trackChildren: true, saveLastFocusedChild: true });
  useEffect(() => {
    if (autoFocus) focusSelf();
  }, [autoFocus, focusSelf]);
  return (
    <FocusContext.Provider value={key}>
      <div ref={ref} className={className}>
        {children}
      </div>
    </FocusContext.Provider>
  );
}
