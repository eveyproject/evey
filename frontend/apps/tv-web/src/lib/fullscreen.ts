/**
 * A television client lives full screen: served under /tv in the TV's own
 * browser, the page otherwise sits under the browser's address bar, film
 * included. The browser only grants full screen from a gesture, so it is asked
 * on every key or click while the page is not full screen - the first press
 * of the remote puts it there, and back there after the viewer left it.
 * Only the page the server serves under /tv asks: the packaged application
 * (file:) is already full screen, and the dev server is left alone.
 */

interface FullscreenDoc {
  fullscreenEnabled?: boolean;
  fullscreenElement: Element | null;
  documentElement: { requestFullscreen?: () => Promise<void> };
}

export function wantsFullscreen(pathname: string, doc: FullscreenDoc): boolean {
  if (!/^\/tv(\/|$)/.test(pathname)) return false;
  if (doc.fullscreenEnabled === false || !doc.documentElement.requestFullscreen) return false;
  return !doc.fullscreenElement;
}

export function keepFullscreen(win: Window = window): void {
  const ask = () => {
    const doc = win.document;
    if (!wantsFullscreen(win.location.pathname, doc)) return;
    // Refused (no gesture recognised, a browser that says no): the next press asks again.
    doc.documentElement.requestFullscreen().catch(() => {});
  };
  win.addEventListener("keydown", ask, true);
  win.addEventListener("click", ask, true);
}
