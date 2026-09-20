// hud.ts - the receiver's on-screen dressing, pure and testable.
//
// The stock cast-media-player flashes its control bar and goes dark: a paused TV
// showed nothing about WHAT is paused or how much remains, and a finished
// playback left a black screen for the whole shutdown grace. main.ts owns
// the DOM and the CAF events; the models are built here.

export interface Osd {
  title: string;
  // "12:34 / 45:00" - elapsed over total.
  clock: string;
  // Localized remaining-time line, "" when the duration is unknown.
  remaining: string;
  // 0-1, clamped: the width of the progress fill. 0 without a duration.
  progress: number;
  // Localized state line, "" while playing - the TV's own bar is gone by then
  // and only a pause needs naming.
  status: string;
  paused: boolean;
}

// formatClock renders seconds as m:ss or h:mm:ss, the web player's format.
export function formatClock(sec: number): string {
  if (!isFinite(sec) || sec < 0) return "0:00";
  const s = Math.floor(sec);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const rest = s % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(rest)}` : `${m}:${pad(rest)}`;
}

// osdModel dresses the on-screen bar: what plays, where it stands, what
// remains. A television has no input of its own, so this is read-only by
// construction - it mirrors what the sender (phone, browser, TV app) just
// commanded, the way every cast receiver's overlay does.
export function osdModel(title: string, posSec: number, durSec: number, lang: string, paused: boolean): Osd {
  const fr = lang.toLowerCase().startsWith("fr");
  const hasDuration = isFinite(durSec) && durSec > 0;
  const pos = isFinite(posSec) && posSec > 0 ? posSec : 0;
  const clock = hasDuration ? `${formatClock(pos)} / ${formatClock(durSec)}` : formatClock(pos);
  let remaining = "";
  if (hasDuration && durSec > pos) {
    const left = formatClock(durSec - pos);
    remaining = fr ? `${left} restantes` : `${left} left`;
  }
  const progress = hasDuration ? Math.max(0, Math.min(1, pos / durSec)) : 0;
  const status = paused ? (fr ? "En pause" : "Paused") : "";
  return { title, clock, remaining, progress, status, paused };
}

// endCardModel dresses the shutdown grace after a playback that has no next
// episode to chain (finale, movie): the screen says the story is over
// instead of going black while the receiver waits to close.
export function endCardModel(title: string, lang: string): { heading: string; title: string } {
  const fr = lang.toLowerCase().startsWith("fr");
  return { heading: fr ? "Lecture terminée" : "Playback finished", title };
}
