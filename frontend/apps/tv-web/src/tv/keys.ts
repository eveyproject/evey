// The remote, reduced to what every television browser delivers: four
// directions, select and back. Everything the app does must be reachable
// with those six; the media keys are accelerators for the sets that send
// them (a browser often keeps them for itself).
export type TvKey = "left" | "right" | "up" | "down" | "select" | "back" | "play" | "pause" | "playPause" | "rewind" | "forward" | "stop" | "search";

// keyCode is what the older engines (webOS 6 = Chromium 79) report reliably
// for the remote; `key` covers the modern ones and a keyboard on a desktop.
const BY_CODE: Record<number, TvKey> = {
  37: "left",
  38: "up",
  39: "right",
  40: "down",
  13: "select",
  27: "back",
  8: "back",
  // LG webOS
  461: "back",
  415: "play",
  19: "pause",
  413: "stop",
  412: "rewind",
  417: "forward",
  // The remote's own search button, where the set has one.
  10225: "search",
  // Samsung Tizen
  10009: "back",
  10252: "playPause",
  10233: "playPause",
};

const BY_KEY: Record<string, TvKey> = {
  ArrowLeft: "left",
  ArrowRight: "right",
  ArrowUp: "up",
  ArrowDown: "down",
  Enter: "select",
  Escape: "back",
  Backspace: "back",
  GoBack: "back",
  BrowserBack: "back",
  MediaPlay: "play",
  MediaPause: "pause",
  MediaPlayPause: "playPause",
  MediaRewind: "rewind",
  MediaFastForward: "forward",
  MediaStop: "stop",
  " ": "playPause",
  BrowserSearch: "search",
  Search: "search",
};

export function tvKeyOf(e: Pick<KeyboardEvent, "key" | "keyCode">): TvKey | null {
  return BY_KEY[e.key] ?? BY_CODE[e.keyCode] ?? null;
}

// A text field keeps its own keys: arrows move the caret, Backspace deletes,
// Enter submits. Only "back" through Escape and the TV back button leaves it.
export function targetIsEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || target.isContentEditable;
}
