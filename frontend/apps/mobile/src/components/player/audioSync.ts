// Audio resync (lip-sync) primitives shared by PlayerSettingsPanel and the
// player screen. Mirrors the web's OffsetStepper/buildTranscodeHlsUrl
// contract (see PlayerSettingsPanel.tsx:117-120 and hlsPlayerUtils.ts:70-104
// on the web app) so both clients dial the same range with the same step.

// OFFSET_STEP_MS is the resync granularity: fine enough to dial in lip sync,
// coarse enough that a few taps cover typical mux offsets.
export const OFFSET_STEP_MS = 100;
export const OFFSET_MAX_MS = 5000;

export function clampOffsetMs(ms: number): number {
  return Math.max(-OFFSET_MAX_MS, Math.min(OFFSET_MAX_MS, ms));
}

export function stepOffsetMs(current: number, dir: 1 | -1): number {
  return clampOffsetMs(current + dir * OFFSET_STEP_MS);
}

// formatOffsetMs renders a millisecond offset as a signed seconds label,
// matching the web's formatOffset (PlayerSettingsPanel.tsx:123-128).
export function formatOffsetMs(ms: number): string {
  if (ms === 0) return "0 ms";
  const sign = ms > 0 ? "+" : "-";
  const abs = Math.abs(ms);
  return abs >= 1000 ? `${sign}${(abs / 1000).toFixed(abs % 1000 === 0 ? 0 : 1)} s` : `${sign}${abs} ms`;
}

// withAudioOffsetParam bakes the resync value into the manifest URL the same
// way the web's buildTranscodeHlsUrl does (audioOffsetMs query param, server
// re-encodes the audio with a shifted input) - this is what lets the native
// player pick up the new sync on its next fetch of the source, no separate
// "restart session" call needed. Returns the url unchanged for a zero offset
// (matches the web: an explicit 0 is never sent) or an empty url.
export function withAudioOffsetParam(url: string, offsetMs: number): string {
  if (!url || offsetMs === 0) return url;
  const hashIdx = url.indexOf("#");
  const base = hashIdx >= 0 ? url.slice(0, hashIdx) : url;
  const hash = hashIdx >= 0 ? url.slice(hashIdx) : "";
  const sep = base.includes("?") ? "&" : "?";
  return `${base}${sep}audioOffsetMs=${Math.trunc(offsetMs)}${hash}`;
}
