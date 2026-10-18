import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  errText,
  eveyFetchRaw,
  getAdminMediaServiceGetMediaRepairHistoryQueryKey,
  getAdminMediaServiceListMediaRepairsQueryKey,
  SEEK_INDEX_SCAN_QUERY_BASE,
  useAdminMediaServiceApplyAudioShift,
  useLibraryServiceGetMediaItem,
} from "@evey/api";
import type { V1SeekIndexItem } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Spinner } from "@/components/ui/Spinner";
import { InfoTooltip } from "@/components/ui/InfoTooltip";

// Watching a few seconds of the film to judge a desync, and applying what the
// ear found.
//
// This panel exists because nothing the server measures answers the question.
// Comparing a file's two audio tracks says whether they agree with EACH OTHER,
// never which one follows the picture - a whole evening of measurements got
// that backwards on two real films, and the answer came in ten seconds from
// someone watching a clip. So: watch, adjust, watch again, apply.
//
// The clip is rendered server-side, because these files are 4K HEVC in
// Matroska with AC-3 or DTS and no browser plays a frame of that.
//
// Three things this had wrong and now does not:
//
//   - the clip was handed to <video src="/api/..."> as a raw URL. That route is
//     behind the admin bearer, a <video> attaches no Authorization header, and
//     every render came back 401 - which the browser reports as "no video with
//     a supported format or MIME type was found". It is fetched here instead,
//     with the app's own credentials, and shown from a blob; a refusal now
//     arrives as the server's own sentence.
//   - the offsets to try were a fixed ladder ending at 12s, on a page that
//     routinely reports twenty. The candidate is a free value now, bounded only
//     by what the server accepts, and the measurements this file carries are
//     offered as one-click starting points.
//   - the moment was a number field stepping by 30s. Judging sync means going
//     to the scene you remember, so it is a scrubber over the whole film.
const SHIFT_MAX_SEC = 60; // mirrors maintenance.SyncPreviewMaxShiftSec
const CLIP_LENGTHS_SEC = [10, 20, 45]; // 45 = maintenance.SyncPreviewMaxSec
const SPEEDS = [0.25, 0.5, 1];
// Nudges for the moment (seconds) and for the candidate offset (seconds). The
// offset ones go down to a twentieth of a second because that is where lip sync
// is actually decided, once the neighbourhood is found.
const AT_NUDGES = [-60, -10, -1, 1, 10, 60];
const SHIFT_NUDGES = [-1, -0.1, -0.05, 0.05, 0.1, 1];
const FALLBACK_FPS = 25;

function clamp(v: number, lo: number, hi: number): number {
  return v < lo ? lo : v > hi ? hi : v;
}

// hhmmss renders a position the way an operator reads a film, not the way a
// query parameter carries it.
function hhmmss(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const r = s % 60;
  const mm = String(m).padStart(2, "0");
  const ss = String(r).padStart(2, "0");
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

function signed(v: number): string {
  return `${v > 0 ? "+" : ""}${Number(v.toFixed(2))}`;
}

// suggestedShifts turns what the scan measured about THIS file into candidate
// offsets. A fixed ladder cannot do this: the file that prompted the rework
// reported nineteen seconds and the list stopped at twelve. Both signs are
// offered because a measurement says how far apart two things are, never which
// one is late - that is exactly what the operator is here to decide.
export function suggestedShifts(item: Pick<V1SeekIndexItem, "audioTracksLagSec" | "audioDelaySec" | "audioShortfallSec">): number[] {
  const raw = [
    Number(item.audioTracksLagSec ?? 0),
    Number(item.audioDelaySec ?? 0),
    Number(item.audioShortfallSec ?? 0),
  ];
  const out: number[] = [];
  for (const v of raw) {
    const mag = Math.round(Math.abs(v) * 100) / 100;
    if (mag < 0.05 || mag > SHIFT_MAX_SEC) continue;
    for (const candidate of [-mag, mag]) {
      if (!out.includes(candidate)) out.push(candidate);
    }
  }
  return out.sort((a, b) => a - b);
}

export function SyncPreviewPanel({
  item,
  title,
  onClose,
}: {
  item: V1SeekIndexItem;
  title: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const mediaId = item.mediaId ?? "";
  const [atSec, setAtSec] = useState(600);
  const [lenSec, setLenSec] = useState(20);
  const [track, setTrack] = useState(0);
  const [shift, setShift] = useState(0);
  const [speed, setSpeed] = useState(1);
  const [clipURL, setClipURL] = useState<string | null>(null);
  // What is on screen right now, as opposed to what the controls are set to:
  // after nudging the offset the player still shows the PREVIOUS candidate, and
  // an operator who cannot see that judges the wrong clip.
  const [shown, setShown] = useState<{ at: number; shift: number; track: number } | null>(null);
  const [rendering, setRendering] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const urlRef = useRef<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const queryClient = useQueryClient();
  // The failure is said inline below, so the global net must not toast it too.
  // A shift is a repair: the file's history, the library journal and the
  // seek-index row all changed.
  const applyShift = useAdminMediaServiceApplyAudioShift({
    mutation: {
      meta: { silentError: true },
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: getAdminMediaServiceGetMediaRepairHistoryQueryKey(mediaId) });
        void queryClient.invalidateQueries({ queryKey: getAdminMediaServiceListMediaRepairsQueryKey() });
        void queryClient.invalidateQueries({ queryKey: [SEEK_INDEX_SCAN_QUERY_BASE] });
      },
    },
  });

  // The file's own description: how long it runs (so the moment is a scrubber
  // over the real film) and what its audio tracks are (so they are named rather
  // than numbered). Both are nice-to-have - the panel degrades to a plain
  // number field and "Track n" when the lookup fails.
  const { data: media } = useLibraryServiceGetMediaItem(mediaId, {
    query: { enabled: mediaId !== "", staleTime: 5 * 60_000, retry: false },
  });
  const durationSec = Math.floor(Number(media?.durationMs ?? 0) / 1000);
  const audioStreams = useMemo(
    () => (media?.streams ?? []).filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO"),
    [media],
  );
  const videoFps = useMemo(() => {
    const v = (media?.streams ?? []).find((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
    const fps = Number(v?.frameRate ?? 0);
    return fps > 0 ? fps : FALLBACK_FPS;
  }, [media]);
  const suggestions = useMemo(() => suggestedShifts(item), [item]);

  // A film shorter than the default 10-minute mark would open on a moment past
  // its end. Pull the cursor back to the middle once the duration is known.
  useEffect(() => {
    if (durationSec > 0 && atSec > durationSec) setAtSec(Math.floor(durationSec / 2));
  }, [durationSec, atSec]);

  const dropClip = useCallback(() => {
    if (urlRef.current) URL.revokeObjectURL(urlRef.current);
    urlRef.current = null;
    setClipURL(null);
    setShown(null);
  }, []);

  useEffect(() => () => {
    abortRef.current?.abort();
    if (urlRef.current) URL.revokeObjectURL(urlRef.current);
  }, []);

  const render = useCallback(async () => {
    if (!mediaId) return;
    abortRef.current?.abort();
    const ac = new AbortController();
    abortRef.current = ac;
    setRendering(true);
    setError(null);
    const params = new URLSearchParams({
      at: String(atSec),
      len: String(lenSec),
      track: String(track),
      shift: String(shift),
    });
    try {
      const res = await eveyFetchRaw({
        url: `/api/v1/admin/media/${encodeURIComponent(mediaId)}/sync-preview?${params.toString()}`,
        method: "GET",
        signal: ac.signal,
      });
      const blob = await res.blob();
      if (ac.signal.aborted) return;
      if (urlRef.current) URL.revokeObjectURL(urlRef.current);
      urlRef.current = URL.createObjectURL(blob);
      setClipURL(urlRef.current);
      setShown({ at: atSec, shift, track });
    } catch (e) {
      if (ac.signal.aborted) return;
      setError(errText(e as { message?: string }, t("admin.syncPreview.renderFailed")));
      dropClip();
    } finally {
      if (!ac.signal.aborted) setRendering(false);
    }
  }, [mediaId, atSec, lenSec, track, shift, t, dropClip]);

  // The rate is a property of the element, not of the source, and it is reset
  // every time a new clip loads - so it is (re)applied on both.
  useEffect(() => {
    const v = videoRef.current;
    if (v) v.playbackRate = speed;
  }, [speed, clipURL]);

  const stepFrame = (dir: 1 | -1) => {
    const v = videoRef.current;
    if (!v) return;
    v.pause();
    v.currentTime = clamp(v.currentTime + (dir / videoFps), 0, v.duration || lenSec);
  };

  const stale = shown !== null && (shown.at !== atSec || shown.shift !== shift || shown.track !== track);

  const trackLabel = (n: number) => {
    const s = audioStreams[n];
    if (!s) return t("admin.syncPreview.trackN", { n: n + 1 });
    const bits = [s.language, s.codec?.toUpperCase(), s.channels ? `${s.channels}ch` : "", s.title]
      .filter((b) => b)
      .join(" · ");
    return bits ? `${n + 1} - ${bits}` : t("admin.syncPreview.trackN", { n: n + 1 });
  };
  const trackCount = audioStreams.length > 0 ? audioStreams.length : 4;

  const nudgeBtn = (label: string, onClick: () => void, testId?: string) => (
    <button
      key={label}
      type="button"
      onClick={onClick}
      data-testid={testId}
      className="rounded border border-[var(--border)] bg-[var(--surface)] px-2 py-1 font-mono text-xs text-[var(--text)] hover:bg-[var(--surface-raised)]"
    >
      {label}
    </button>
  );

  return (
    <Modal open onClose={onClose} title={t("admin.syncPreview.title", { title })} size="3xl">
      <div className="flex flex-col gap-4" data-testid="sync-preview-panel">
        <p className="text-sm text-[var(--text-muted)]">
          {t("admin.syncPreview.intro")}
          <span className="ml-1">
            <InfoTooltip label={t("admin.syncPreview.moreInfo")}>
              <span className="flex flex-col gap-2 text-xs text-[var(--text-muted)]">
                <span className="block">{t("admin.syncPreview.detailWhy")}</span>
                <span className="block">{t("admin.syncPreview.detailHow")}</span>
              </span>
            </InfoTooltip>
          </span>
        </p>

        {/* The picture first: everything below it exists to change what plays
            here, and a control panel above the screen puts the answer last. */}
        <div className="relative w-full overflow-hidden rounded-md bg-black" style={{ aspectRatio: "16 / 9" }}>
          {clipURL ? (
            <video
              ref={videoRef}
              key={clipURL}
              src={clipURL}
              controls
              autoPlay
              className="h-full w-full"
              data-testid="sync-preview-video"
            />
          ) : (
            <p
              className="absolute inset-0 flex items-center justify-center px-6 text-center text-xs text-[var(--text-muted)]"
              data-testid="sync-preview-idle"
            >
              {t("admin.syncPreview.idle")}
            </p>
          )}
          {rendering && (
            <div
              className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-black/60 text-xs text-white"
              data-testid="sync-preview-rendering"
            >
              <Spinner />
              {t("admin.syncPreview.rendering")}
            </div>
          )}
        </div>

        {error && (
          <p className="text-sm text-[var(--color-danger)]" role="alert" data-testid="sync-preview-error">
            {error}
          </p>
        )}

        {/* Slowing the picture and the sound together is how a lip lands on a
            syllable: at quarter speed a tenth of a second is visible, at full
            speed it is not. The frame steps are the same idea taken to the end.
            Both act on the element, so they need a clip loaded. */}
        {clipURL && (
          <div className="flex flex-wrap items-center gap-3 text-xs text-[var(--text-muted)]">
            <span className="font-medium">{t("admin.syncPreview.speed")}</span>
            <div className="flex gap-1">
              {SPEEDS.map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setSpeed(s)}
                  aria-pressed={speed === s}
                  data-testid={`sync-preview-speed-${s}`}
                  className={`rounded border px-2 py-1 font-mono ${
                    speed === s
                      ? "border-[var(--accent)] bg-[var(--accent)] text-[var(--filter-active-text)]"
                      : "border-[var(--border)] bg-[var(--surface)] text-[var(--text)] hover:bg-[var(--surface-raised)]"
                  }`}
                >
                  {s}x
                </button>
              ))}
            </div>
            <span className="font-medium">{t("admin.syncPreview.frameStep")}</span>
            <div className="flex gap-1">
              {nudgeBtn("-1", () => stepFrame(-1), "sync-preview-frame-back")}
              {nudgeBtn("+1", () => stepFrame(1), "sync-preview-frame-fwd")}
            </div>
          </div>
        )}

        <div className="flex flex-col gap-4 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-2)] p-3">
          {/* Moment. A scrubber over the whole film, because the scene you
              remember is the one that answers the question - the number field
              this replaces stepped in half-minutes and never got there. */}
          <div className="flex flex-col gap-1.5">
            <div className="flex items-baseline justify-between gap-2 text-xs">
              <span className="font-medium text-[var(--text)]">{t("admin.syncPreview.at")}</span>
              <span className="font-mono text-[var(--text-muted)]" data-testid="sync-preview-at-label">
                {hhmmss(atSec)}
                {durationSec > 0 ? ` / ${hhmmss(durationSec)}` : ""}
              </span>
            </div>
            {durationSec > 0 ? (
              <input
                type="range"
                min={0}
                max={durationSec}
                step={1}
                value={Math.min(atSec, durationSec)}
                onChange={(e) => setAtSec(Number(e.target.value))}
                aria-label={t("admin.syncPreview.at")}
                className="w-full accent-[var(--accent)]"
                data-testid="sync-preview-at"
              />
            ) : (
              <input
                type="number"
                min={0}
                step={5}
                value={atSec}
                onChange={(e) => setAtSec(Math.max(0, Number(e.target.value)))}
                aria-label={t("admin.syncPreview.at")}
                className="w-28 rounded border border-[var(--border)] bg-[var(--surface)] px-2 py-1 text-sm text-[var(--text)]"
                data-testid="sync-preview-at"
              />
            )}
            <div className="flex flex-wrap gap-1">
              {AT_NUDGES.map((n) =>
                nudgeBtn(signed(n), () =>
                  setAtSec((v) => clamp(v + n, 0, durationSec > 0 ? durationSec : v + n)),
                ),
              )}
            </div>
          </div>

          {/* Candidate offset. Free within what the server accepts, with the
              file's own measurements as starting points. */}
          <div className="flex flex-col gap-1.5">
            <div className="flex items-baseline justify-between gap-2 text-xs">
              <span className="font-medium text-[var(--text)]">{t("admin.syncPreview.shift")}</span>
              <span className="flex items-center gap-1">
                <input
                  type="number"
                  min={-SHIFT_MAX_SEC}
                  max={SHIFT_MAX_SEC}
                  step={0.05}
                  value={shift}
                  onChange={(e) => setShift(clamp(Number(e.target.value), -SHIFT_MAX_SEC, SHIFT_MAX_SEC))}
                  aria-label={t("admin.syncPreview.shift")}
                  className="w-24 rounded border border-[var(--border)] bg-[var(--surface)] px-2 py-1 text-right font-mono text-sm text-[var(--text)]"
                  data-testid="sync-preview-shift"
                />
                <span className="text-xs text-[var(--text-muted)]">s</span>
              </span>
            </div>
            <input
              type="range"
              min={-SHIFT_MAX_SEC}
              max={SHIFT_MAX_SEC}
              step={0.05}
              value={shift}
              onChange={(e) => setShift(Number(e.target.value))}
              aria-label={t("admin.syncPreview.shiftSlider")}
              className="w-full accent-[var(--accent)]"
              data-testid="sync-preview-shift-slider"
            />
            <div className="flex flex-wrap gap-1">
              {SHIFT_NUDGES.map((n) =>
                nudgeBtn(signed(n), () =>
                  setShift((v) => clamp(Math.round((v + n) * 100) / 100, -SHIFT_MAX_SEC, SHIFT_MAX_SEC)),
                ),
              )}
              {nudgeBtn(t("admin.syncPreview.shiftReset"), () => setShift(0), "sync-preview-shift-reset")}
            </div>
            {suggestions.length > 0 && (
              <div className="flex flex-wrap items-center gap-1" data-testid="sync-preview-suggestions">
                <span className="text-xs text-[var(--text-muted)]">{t("admin.syncPreview.measured")}</span>
                {suggestions.map((s) =>
                  nudgeBtn(`${signed(s)} s`, () => setShift(s), `sync-preview-suggest-${s}`),
                )}
              </div>
            )}
          </div>

          <div className="flex flex-wrap items-end gap-3">
            <label className="text-xs text-[var(--text-muted)]">
              {t("admin.syncPreview.track")}
              <select
                value={track}
                onChange={(e) => setTrack(Number(e.target.value))}
                className="ml-1.5 rounded border border-[var(--border)] bg-[var(--surface)] px-1.5 py-1 text-[var(--text)]"
                data-testid="sync-preview-track"
              >
                {Array.from({ length: trackCount }, (_, n) => (
                  <option key={n} value={n}>
                    {trackLabel(n)}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-xs text-[var(--text-muted)]">
              {t("admin.syncPreview.clipLength")}
              <select
                value={lenSec}
                onChange={(e) => setLenSec(Number(e.target.value))}
                className="ml-1.5 rounded border border-[var(--border)] bg-[var(--surface)] px-1.5 py-1 text-[var(--text)]"
                data-testid="sync-preview-len"
              >
                {CLIP_LENGTHS_SEC.map((n) => (
                  <option key={n} value={n}>
                    {t("admin.syncPreview.seconds", { n })}
                  </option>
                ))}
              </select>
            </label>
            <Button
              variant="secondary"
              className="text-xs"
              loading={rendering}
              onClick={() => void render()}
              data-testid="sync-preview-render"
            >
              {t("admin.syncPreview.render")}
            </Button>
            {stale && !rendering && (
              <span className="text-xs text-[var(--color-warning)]" data-testid="sync-preview-stale">
                {t("admin.syncPreview.stale", { shift: signed(shown.shift), at: hhmmss(shown.at) })}
              </span>
            )}
          </div>
        </div>

        <div className="flex flex-col gap-1 border-t border-[var(--border)] pt-3">
          <div>
            <Button
              variant="primary"
              className="text-xs"
              disabled={shift === 0}
              loading={applyShift.isPending}
              onClick={() => applyShift.mutate({ mediaId, data: { track: String(track + 1), shiftSec: shift } })}
              data-testid="sync-preview-apply"
            >
              {t("admin.syncPreview.apply", { shift: signed(shift) })}
            </Button>
          </div>
          {/* An apply can be REFUSED, and the refusal has to be readable: a
              file whose timebase is not the 1 ms default only accepts shifts
              that are a whole number of its own ticks, so most values an
              operator picks here come back "not representable at the file's
              timecode scale". Without this the button simply stopped spinning
              and nothing happened - the same silent failure this panel was
              rebuilt to remove, on the other button. */}
          {applyShift.isError && (
            <p className="text-sm text-[var(--color-danger)]" role="alert" data-testid="sync-preview-apply-error">
              {errText(applyShift.error as { message?: string }, t("admin.syncPreview.applyFailed"))}
            </p>
          )}
          {applyShift.isSuccess && (
            <p className="text-sm text-[var(--color-success)]" data-testid="sync-preview-apply-done">
              {t("admin.syncPreview.applyDone")}
            </p>
          )}
          {/* Said out loud because it is the honest description of what this
              does: a person's ear is the evidence here, and the journal records
              it as such - which is better evidence than the file's own claim
              about itself. */}
          <p className="text-xs text-[var(--text-muted)]">{t("admin.syncPreview.applyHint")}</p>
        </div>
      </div>
    </Modal>
  );
}
