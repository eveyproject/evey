import { useEffect, useMemo, useRef, useState } from "react";
import Hls from "hls.js";
import { createPlayerEventReporter } from "@evey/api";
import { buildTranscodeHlsUrl, liftForcedMuteOnNextGesture, pickPinnedLevel, prefersNativeHLS, startPlayback } from "./hlsPlayerUtils";
import { playbackSnapshot } from "../playbackSnapshot";


// Mirrors the server's shed contract (HTTP 503 on a refused playback start):
// - too_many_streams: the box can't prepare one more stream because of OTHER
//   concurrent playbacks.
// - file_not_ready: unrelated to load - this specific file's seek index is
//   still being built and will serve fine once that finishes.
// Never surface the server's `message` field (English, technical) to users -
// the client picks its own non-technical copy from this code.
export type ShedReason = "too_many_streams" | "file_not_ready" | null;

interface ShedErrorData {
  response?: { code?: number; text?: string };
  networkDetails?: unknown;
}

// Reads the shed reason off a fatal NETWORK_ERROR hls.js reports for a 503.
//
// hls.js's default XHR loader does NOT forward the response BODY to its
// onError callback on a failed request - it only passes
// `{ code: xhr.status, text: xhr.statusText }` (see XhrLoader.readystatechange
// in hls.js), so `data.response.text` here is the HTTP reason phrase (e.g.
// "Service Unavailable"), not our JSON body. The raw XHR is still reachable
// via `data.networkDetails` though, and manifest/level loads use
// `responseType: "text"`, so `.responseText` remains readable after the
// error - that's where the real body lives. `response.text` is checked first
// in case a custom loader (or a future hls.js version) starts forwarding the
// body directly under that field.
function extractShedReason(data: ShedErrorData): ShedReason {
  // A genuine load-shed carries the explicit X-Evey-Error header (the server's
  // shed contract in writeShed). Unlike the JSON body, that header is readable
  // even for a FRAGMENT error, where hls.js's default loader never forwards the
  // body - so a real file_not_ready shed on a segment is still recognised.
  const xhr = data.networkDetails as { getResponseHeader?: (name: string) => string | null } | undefined;
  const header = xhr?.getResponseHeader?.("X-Evey-Error");
  if (header === "too_many_streams" || header === "file_not_ready") return header;

  const raw = data.response?.text || (data.networkDetails as { responseText?: string } | undefined)?.responseText;
  if (raw) {
    try {
      const parsed = JSON.parse(raw) as { code?: string };
      if (parsed.code === "too_many_streams" || parsed.code === "file_not_ready") return parsed.code;
    } catch {
      // Not our JSON shape (e.g. a proxy's plain-text 503 page) - no explicit
      // shed signal.
    }
  }
  // No X-Evey-Error header and no known body code: this 503 carries no shed
  // signal. It is NOT necessarily a shed - a segment handler returns a plain
  // 503 ("not yet ready, retry") while a deep seek's encoder spins up. Return
  // null and let the caller decide (retry a fragment, default-shed a stream
  // start).
  return null;
}

interface UseTranscodePlaybackArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  hlsRef: React.MutableRefObject<InstanceType<typeof Hls> | null>;
  positionRef: React.MutableRefObject<number>;
  transcodeOffsetRef: React.MutableRefObject<number>;
  wasPausedRef: React.MutableRefObject<boolean>;
  streamTokenRef: React.MutableRefObject<string>;
  onStreamTokenRefreshRef: React.MutableRefObject<() => Promise<string>>;
  transcodeMode: boolean;
  mediaId: string;
  currentTrackIndex: number;
  /** Track picked by the CLIENT-side hls.js switch (hls.audioTrack), which
   * deliberately does not go through currentTrackIndex so the instance is not
   * destroyed. Natural rebuilds (token refresh, subtitles, offset) read it back
   * for the URL and re-select the rendition after MANIFEST_PARSED. */
  audioTrackOverrideRef?: React.MutableRefObject<number | null>;
  /** Height of the wanted rung (0 = Auto -> the top rung). Read INSIDE
   * MANIFEST_PARSED to pin the level before hls.js loads its first fragment. A
   * ref, not a prop: the value must be readable without a re-render, and the
   * sync effect in HLSPlayer still handles changes made during playback. */
  pinnedQualityRef?: React.MutableRefObject<number>;
  selectedBurnInTrack: number | null;
  startPositionMs: number;
  onEnded?: () => void;
  setIsBuffering: (v: boolean) => void;
  setFatalError: (v: boolean) => void;
  setShedReason: (v: ShedReason) => void;
  /** Applied audio resync (ms; positive = audio later). Baked into the
   * stream URL - a change re-creates the HLS session with a shifted encode. */
  audioOffsetMs?: number;
  /** Gapless multi-episode session ("lecture enchaînée", per-series toggle).
   * Baked into the stream URL (?gapless=1) - the server falls back to the
   * normal per-episode master when it can't honour it. */
  gapless?: boolean;
  /** wasm-audio path: request a VIDEO-ONLY master (?wasmAudio=1) whose AC-3/
   * E-AC-3 audio is decoded client-side (WasmAudioEngine). Toggling it off (the
   * fallback) rebuilds hls.js against the normal muxed/AAC master. */
  wasmAudio?: boolean;
}

export function useTranscodePlayback({
  videoRef,
  hlsRef,
  positionRef,
  transcodeOffsetRef,
  wasPausedRef,
  streamTokenRef,
  onStreamTokenRefreshRef,
  transcodeMode,
  mediaId,
  currentTrackIndex,
  audioTrackOverrideRef,
  pinnedQualityRef,
  selectedBurnInTrack,
  startPositionMs,
  onEnded,
  setIsBuffering,
  audioOffsetMs = 0,
  gapless = false,
  wasmAudio = false,
  setFatalError,
  setShedReason,
}: UseTranscodePlaybackArgs) {
  const [transcodeStartTime, setTranscodeStartTime] = useState(startPositionMs / 1000);
  // The playback-error beacon: what THIS player sees (decode errors, recovery
  // loops, element-level MediaError) becomes a server-side log line. One
  // reporter per media so its dedupe/budget spans the whole viewing, not one
  // hls.js instance (audio/quality switches rebuild the instance).
  const reportPlayerEvent = useMemo(() => createPlayerEventReporter(mediaId), [mediaId]);
  const [hlsLevels, setHlsLevels] = useState<number[]>([]);
  // The audio renditions the MASTER declares, read straight from hls.js. The
  // menu is normally built from the server's stream list, but a guest in a
  // "watch together" room cannot ask for it - GetStreamInfo needs a JWT they do
  // not have - and so had no way to pick a language at all. The manifest knows,
  // and it is the same manifest the guest is already playing.
  const [manifestAudioTracks, setManifestAudioTracks] = useState<{ name: string; lang: string }[]>([]);
  // Disarm handle for the forced-mute recovery: at most one armed at a time,
  // and removed on unmount so no document listener outlives the player.
  const disarmMuteLiftRef = useRef<(() => void) | null>(null);
  useEffect(() => () => { disarmMuteLiftRef.current?.(); disarmMuteLiftRef.current = null; }, []);
  // Height (px) of the rung hls.js is actually playing right now. Surfaced so the
  // settings panel can show "Auto (1080p)" instead of a redundant bare "Auto".
  const [autoLevelHeight, setAutoLevelHeight] = useState(0);

  useEffect(() => {
    if (!transcodeMode) return;
    const video = videoRef.current;
    if (!video) return;

    if (hlsRef.current) {
      hlsRef.current.destroy();
      hlsRef.current = null;
    }

    // VOD pipeline: the server precomputes the full manifest covering
    // the entire media (0 → totalDuration) and serves segments on
    // demand. There is no "transcode start offset" - the playhead is
    // an absolute position in the source. transcodeOffset must stay
    // at 0 so positionRef = currentTime matches the absolute time;
    // a non-zero offset (legacy sliding-window assumption) silently
    // produced UI/playhead drift on resume (UI showed 9:45 but
    // playback started at segment 0).
    //
    // Positioning comes from the PLAYLIST, not the fragment's internal
    // timestamp: hls.js places each segment at its absolute EXTINF offset in
    // the precalculated VOD manifest and applies a per-segment timestampOffset.
    // Each ffmpeg session resets the fragment's fMP4 tfdt to 0 (a fresh session
    // always does, with or without -copyts - verified against jellyfin-ffmpeg7),
    // and hls.js tolerates that because the EXTINF drives the timeline. So a
    // seek-spawned rafale at segment N lands at N's manifest position regardless
    // of its tfdt.
    transcodeOffsetRef.current = 0;
    const initialSeekSec = transcodeStartTime > 0 ? transcodeStartTime : 0;

    const hlsUrl = buildTranscodeHlsUrl(
      mediaId,
      streamTokenRef.current,
      audioTrackOverrideRef?.current ?? currentTrackIndex,
      selectedBurnInTrack,
      // The VOD manifest itself ignores startSec (the seek below is what moves
      // the playhead), but the server reads it off the master as a PRE-WARM
      // hint: the variant playlist then opens its encoder at this position
      // instead of at 0. A resume or an audio-track switch restarts mid-film,
      // and an encoder at 0 was one nobody read while the real position waited
      // for its own (prod, 2026-09-09: over a minute of spinner).
      initialSeekSec,
      audioOffsetMs,
      gapless,
      wasmAudio,
    );

    if (prefersNativeHLS()) {
      video.src = hlsUrl;
      // Native-HLS path (Safari macOS laptops + iOS). Preferred wherever the
      // <video> decodes HLS itself - lighter on CPU/battery than hls.js+MSE -
      // not only where MSE is missing. Segment auth rides the httpOnly evey_hls
      // cookie (issued off this master request's ?token=), and the token is the
      // long-lived native session token so a pause can't expire it mid-stream. The
      // stream is transcoded starting at `offset`, so video.currentTime is
      // local to the restarted segment - positionRef must hold the ABSOLUTE
      // position (offset + local) like the Hls.js path does, otherwise every
      // seek after an out-of-buffer restart on Safari lands offset seconds off.
      //
      // currentTime only sticks once metadata is loaded on native HLS, so the
      // resume/restart target is applied in loadedmetadata (mirroring the
      // hls.js MANIFEST_PARSED path). Without this the branch hard-seeked to 0,
      // so every resume from watch progress and every audio/subtitle restart on
      // Safari jumped back to the start of the media.
      function onLoadedMetadata() {
        if (initialSeekSec > 0) video!.currentTime = initialSeekSec;
        if (!wasPausedRef.current) {
          void startPlayback(video!, wasmAudio);
        }
      }
      function onTimeUpdate() { positionRef.current = transcodeOffsetRef.current + video!.currentTime; }
      function handleEnded() { onEnded?.(); }
      // Native HLS has no hls.js to observe: the element-level MediaError is
      // the ONLY error signal this path will ever get. Same beacon.
      function onNativeError() {
        const err = video!.error;
        if (!err) return;
        reportPlayerEvent({
          type: "video_element_error",
          details: err.message || "",
          fatal: true,
          code: err.code,
          posSec: transcodeOffsetRef.current + (video!.currentTime || 0),
        });
      }
      video.addEventListener("loadedmetadata", onLoadedMetadata);
      video.addEventListener("timeupdate", onTimeUpdate);
      video.addEventListener("ended", handleEnded);
      video.addEventListener("error", onNativeError);
      return () => {
        wasPausedRef.current = video.paused;
        video.removeEventListener("loadedmetadata", onLoadedMetadata);
        video.removeEventListener("timeupdate", onTimeUpdate);
        video.removeEventListener("ended", handleEnded);
        video.removeEventListener("error", onNativeError);
        video.src = "";
      };
    }

    const hls = new Hls({
      // -1 lets hls.js compute the start position from the manifest
      // (it'll seek to whatever we set via video.currentTime in
      // MANIFEST_PARSED). Forcing 0 here defeats the resume.
      startPosition: -1,
      maxBufferLength: 30,
      maxMaxBufferLength: 60,
      // Long-distance seeks spawn a fresh rafale on the server side;
      // ffmpeg can take 10-15 s to produce segment 0 of that rafale
      // on a 4K HEVC source. The hls.js default fragLoadingTimeOut
      // (20 s) is right at the edge and can fire when the rafale
      // races startup → "fragLoadTimeOut" surfaced as a fatal error
      // on legitimate seeks. Bump the per-fragment timeout to 60 s;
      // the orchestrator's own segmentWaitTimeout (25 s) keeps a
      // genuinely stuck request from hanging indefinitely.
      fragLoadingTimeOut: 60_000,
      fragLoadingMaxRetry: 4,
      // Emit CMCD (CTA-5004) on every segment / playlist request. The
      // server reads br, bl, mtp, bs, su, sid, ... and stores them in
      // playback_qoe_events for the /admin/qoe dashboard, plus uses
      // bs=1 to wake up a throttled ffmpeg immediately. Defaults: query
      // mode (?CMCD=...), auto-generated sessionId.
      cmcd: {},
      xhrSetup: (xhr: XMLHttpRequest, url: string) => {
        const tok = streamTokenRef.current;
        if (!tok) return;
        // The server bakes the stream token into segment URIs in the master/sub
        // playlists. The token has a 5-min sliding TTL that lapses during pause,
        // so on resume those baked-in URLs return 401. After a 401 retry refreshes
        // the token, every subsequent xhr must override the URL token - so we
        // always strip and re-inject, regardless of what the URL carries.
        try {
          const u = new URL(url, window.location.origin);
          u.searchParams.set("token", tok);
          xhr.open("GET", u.toString(), true);
        } catch {
          if (!url.includes("token=")) {
            const sep = url.includes("?") ? "&" : "?";
            xhr.open("GET", `${url}${sep}token=${tok}`, true);
          }
        }
      },
    });
    hlsRef.current = hls;

    hls.loadSource(hlsUrl);
    hls.attachMedia(video);

    // Track the rung ABR actually settled on, so "Auto" can show its resolution.
    hls.on(Hls.Events.LEVEL_SWITCHED, (_evt, data) => {
      const h = hls.levels?.[data.level]?.height;
      if (h) setAutoLevelHeight(h);
    });

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      // Pin the rung HERE, before any fragment load. The sync effect in
      // HLSPlayer is not enough: it depends on state set by setHlsLevels just
      // below, so it lands one re-render too late, and in that window hls.js
      // starts on free ABR at level 0 - a SECOND ffmpeg on the same hash, on a
      // host that only sustains one encode (see pickPinnedLevel).
      const pinned = pickPinnedLevel({
        levelHeights: hls.levels.map((l) => l.height),
        desiredHeight: pinnedQualityRef?.current ?? 0,
      });
      if (pinned >= 0 && hls.currentLevel !== pinned) {
        hls.currentLevel = pinned;
      }
      setHlsLevels(hls.levels.map((l) => l.height));
      // A master with no audio group has no audioTracks at all, and neither
      // does a stripped-down player in a test: absent is a legitimate answer.
      setManifestAudioTracks(
        (hls.audioTracks ?? []).map((t) => ({ name: t.name ?? t.lang ?? "", lang: t.lang ?? "" })),
      );
      // Re-select the track chosen by the client-side switch: this rebuild
      // (token refresh, subtitles, offset) did not go through
      // currentTrackIndex, and the master's DEFAULT is not enough to re-aim the
      // rendition when the URL carried the previous track.
      const override = audioTrackOverrideRef?.current;
      if (override != null && override >= 0 && override < hls.audioTracks.length && hls.audioTrack !== override) {
        hls.audioTrack = override;
      }
      // NB: hls.js ABR is intentionally NOT used on this transcode path - the
      // level-sync effect in HLSPlayer PINS a single rung (top for Auto, the chosen
      // one for manual) because free ABR loads several rungs at once on a transcode
      // box and thrashes. So no startLevel / abrEwmaDefaultEstimate tuning here.
      // VOD pipeline: the manifest covers the full media duration so we
      // seek the resume target via video.currentTime, which causes
      // hls.js to request the matching segment from the server.
      // SegmentOrchestrator (or the copy rung) spawns a rafale at that segment
      // with the matching -ss; hls.js accepts it because the playlist's EXTINF
      // places the segment - the fragment's tfdt resets to 0 per session and is
      // tolerated (positioning is by the manifest, not the fragment timestamp).
      if (initialSeekSec > 0) {
        video.currentTime = initialSeekSec;
      } else if (video.currentTime !== 0) {
        video.currentTime = 0;
      }
      // The video ELEMENT survives the rebuild, so its mute state here is the
      // viewer's intent from just before the switch, not a default.
      const wantedSound = !video.muted;
      // A rebuild while PAUSED must not resume playback - the native-HLS branch
      // already honoured wasPausedRef, this one did not, so switching track
      // while paused started the film again on its own.
      if (wasPausedRef.current) return;
      // Sound or nothing: a browser that blocks the autoplay leaves the video
      // paused and unmuted, and the controls' play button starts it on one click.
      void startPlayback(video, wasmAudio).then(() => {
        // The browser forced a muted start while the viewer was listening.
        // Without this recovery the mute sticks for the rest of the viewing
        // (the element is never recreated); the next gesture lifts it.
        if (wantedSound && video.muted) {
          disarmMuteLiftRef.current?.();
          disarmMuteLiftRef.current = liftForcedMuteOnNextGesture(video);
        }
      });
    });

    let retries = 0;
    let consecutive404 = 0;
    hls.on(Hls.Events.ERROR, (_, data) => {
      // Beacon every MEDIA error (fatal or not - the non-fatal ones are the
      // first symptom of a garbled stream) and every fatal error of any type.
      // The 2026-07-15 rainbow incident left zero server-side trace; this line
      // is what makes the next one diagnosable from the admin logs.
      if (data.fatal || data.type === Hls.ErrorTypes.MEDIA_ERROR) {
        const sn = data.frag?.sn;
        reportPlayerEvent({
          type: data.fatal ? "hls_fatal" : "hls_media_error",
          details: data.details,
          fatal: data.fatal,
          code: data.response?.code ?? 0,
          variant: data.frag?.level ?? 0,
          segment: typeof sn === "number" ? sn : 0,
          posSec: transcodeOffsetRef.current + (video.currentTime || 0),
          state: playbackSnapshot(video),
        });
      }
      if (!data.fatal) return;
      if (import.meta.env.DEV) console.warn("[player] HLS fatal error:", data.type, data.details, data.response?.code);
      if (data.type === Hls.ErrorTypes.NETWORK_ERROR && data.response?.code === 503) {
        const shed = extractShedReason(data);
        const isFragment = data.details === "fragLoadError" || data.details === "fragLoadTimeOut";
        if (shed) {
          // Explicit server load-shed (X-Evey-Error / body code): surface a
          // typed, non-technical reason and offer a manual retry, without
          // burning the generic retry budget below.
          setShedReason(shed);
          return;
        }
        if (!isFragment) {
          // A 503 on STREAM START (manifest/level) with no explicit code: keep
          // the "too many streams" default framing - its "try again shortly"
          // copy reads fine either way, and the box is genuinely refusing.
          setShedReason("too_many_streams");
          return;
        }
        // A 503 on a FRAGMENT with no shed signal is the segment handler's
        // transient "not yet ready, retry" - e.g. a deep seek whose audio
        // rebase encoder is still spinning up. Fall through to the generic
        // retry/backoff below instead of killing playback with a shed overlay.
      }
      retries++;
      if (retries > 20) {
        setFatalError(true);
        return;
      }
      if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
        if (data.response?.code === 401) {
          (async () => {
            try {
              const newToken = await onStreamTokenRefreshRef.current();
              streamTokenRef.current = newToken;
              // The token refresh is async; if the effect re-ran meanwhile
              // (audio/subtitle/quality switch destroyed this instance and
              // built a new one), don't drive the dead instance - the live
              // one in hlsRef is already loading.
              if (hls !== hlsRef.current) return;
              // For manifest/level failures, hls.js holds a stale URL with the
              // expired token. xhrSetup rewrites the token on every request,
              // but loadSource is needed to retry the manifest itself; for
              // fragment failures startLoad re-drives the loader.
              const isManifestLevel =
                data.details === "manifestLoadError" ||
                data.details === "manifestLoadTimeOut" ||
                data.details === "manifestParsingError" ||
                data.details === "levelLoadError" ||
                data.details === "levelLoadTimeOut";
              if (isManifestLevel) {
                const seekPos = transcodeOffsetRef.current + (video.currentTime || 0);
                const freshUrl = buildTranscodeHlsUrl(mediaId, newToken, audioTrackOverrideRef?.current ?? currentTrackIndex, selectedBurnInTrack, seekPos, audioOffsetMs, gapless, wasmAudio);
                hls.loadSource(freshUrl);
                hls.startLoad();
              } else {
                hls.startLoad();
              }
            } catch {
              // Only fail the LIVE instance - if the effect re-ran during the
              // async refresh (audio/subtitle/quality switch), this hls is dead
              // and the new one in hlsRef is already loading.
              if (hls === hlsRef.current) setFatalError(true);
            }
          })();
          return;
        }
        // 404: the session's segments are gone (cache eviction, restart).
        // 410: the server refused a STALE rendition URL - the ladder reshaped
        // under this player (capability flap, admission change) and our URLs
        // no longer name the renditions we think they do. Serving them anyway
        // is how the 2026-07-15 rainbow screen happened; the cure is the same
        // as the 404's: rebuild the player at the current position, which
        // refetches a coherent master with fresh sealed URLs.
        if (data.response?.code === 404 || data.response?.code === 410) {
          consecutive404++;
          if (consecutive404 >= 2) {
            hls.destroy();
            // The other two teardowns in this file null the ref alongside
            // destroy(); this one did not, leaving a destroyed instance
            // reachable through hlsRef. HLSPlayer reads hlsRef.current and then
            // hls.levels / hls.currentLevel, so the only thing hiding it today
            // is the fatal-error overlay sitting in front of those controls.
            hlsRef.current = null;
            video.removeAttribute("src");
            video.load();
            setFatalError(true);
            return;
          }
          const seekPos = transcodeOffsetRef.current + (video.currentTime || 0);
          if (seekPos > 0) {
            setTranscodeStartTime(seekPos);
          } else {
            // At position 0 the state setter is a no-op (same value, no
            // re-render): refetch the master in place instead, 401-style. The
            // fresh master carries freshly sealed URLs.
            const freshUrl = buildTranscodeHlsUrl(mediaId, streamTokenRef.current, audioTrackOverrideRef?.current ?? currentTrackIndex, selectedBurnInTrack, 0, audioOffsetMs, gapless, wasmAudio);
            hls.loadSource(freshUrl);
            hls.startLoad();
          }
          return;
        }
        const delay = Math.min(2000 * Math.pow(1.5, retries - 1), 15000);
        setTimeout(() => { if (hls === hlsRef.current) hls.startLoad(); }, delay);
      } else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
        hls.recoverMediaError();
      } else {
        setFatalError(true);
      }
    });
    hls.on(Hls.Events.FRAG_LOADED, () => { retries = 0; consecutive404 = 0; setShedReason(null); });

    function onTimeUpdate() { positionRef.current = transcodeOffsetRef.current + video!.currentTime; }
    function handleEnded() { onEnded?.(); }
    // A PAUSED player is not buffering. The browser fires `waiting` on pause too
    // (and on a seek while paused), and no `playing` follows to clear it - so the
    // "Buffering…" overlay sat in the middle of the screen for as long as you were
    // paused. Only ever spin for a stall during PLAYBACK.
    function handleWaiting() { if (!video!.paused) setIsBuffering(true); }
    function handlePlaying() { setIsBuffering(false); }
    function handlePause() { setIsBuffering(false); }
    // The element-level MediaError is the decoder's own verdict (code 3 =
    // MEDIA_ERR_DECODE - the "rainbow screen" family): report it even when
    // hls.js swallows or recovers the underlying event.
    function handleVideoError() {
      const err = video!.error;
      if (!err) return;
      reportPlayerEvent({
        type: "video_element_error",
        details: err.message || "",
        fatal: true,
        code: err.code,
        posSec: transcodeOffsetRef.current + (video!.currentTime || 0),
      });
    }
    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("ended", handleEnded);
    video.addEventListener("waiting", handleWaiting);
    video.addEventListener("playing", handlePlaying);
    video.addEventListener("pause", handlePause);
    video.addEventListener("error", handleVideoError);

    return () => {
      wasPausedRef.current = video.paused;
      video.removeEventListener("timeupdate", onTimeUpdate);
      video.removeEventListener("ended", handleEnded);
      video.removeEventListener("waiting", handleWaiting);
      video.removeEventListener("playing", handlePlaying);
      video.removeEventListener("pause", handlePause);
      video.removeEventListener("error", handleVideoError);
      hls.destroy();
      hlsRef.current = null;
      // Clear any in-flight buffering spinner when the instance is torn down
      // (audio/subtitle/quality switch rebuilds it); the fresh instance's
      // waiting/playing events re-drive the state. Avoids a stale spinner
      // surviving the transition.
      setIsBuffering(false);
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps -- streamTokenRef/onStreamTokenRefreshRef read imperatively; setFatalError/setShedReason/setIsBuffering/setTranscodeStartTime are stable setState
  }, [transcodeMode, mediaId, onEnded, currentTrackIndex, transcodeStartTime, selectedBurnInTrack, audioOffsetMs, gapless, wasmAudio]);

  return { transcodeStartTime, setTranscodeStartTime, hlsLevels, autoLevelHeight, manifestAudioTracks };
}
