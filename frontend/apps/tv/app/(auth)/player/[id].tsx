import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import { View, Text, Pressable, ActivityIndicator, StyleSheet, useWindowDimensions, useTVEventHandler } from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { useVideoPlayer, VideoView } from "expo-video";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceGetMediaItem,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceGetWatchProgress,
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaySession } from "../../../src/hooks/usePlaySession";
import { useAudioCapsBeacon } from "../../../src/hooks/useAudioCapsBeacon";
import { useProgressReporter } from "../../../src/hooks/useProgressReporter";
import { usePlaybackSettings } from "../../../src/hooks/usePlaybackSettings";
import { useNextEpisode } from "../../../src/hooks/useNextEpisode";
import { useChapters } from "../../../src/hooks/useChapters";
import { pickPreferredStreamIndex } from "../../../src/lib/langMatch";
import { useSleepTimer } from "../../../src/hooks/useSleepTimer";
import { seekDeltaForKey, clampSeekTarget, shouldReleaseSeekPreview } from "../../../src/hooks/playerSeek";
import { playerExit } from "../../../src/hooks/playerExit";
import { playerTitle } from "../../../src/lib/playerTitle";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { QualityPicker } from "../../../src/components/QualityPicker";
import { AudioTrackPicker, type AudioStream } from "../../../src/components/AudioTrackPicker";
import { SubtitlePicker, type SubtitleStream } from "../../../src/components/SubtitlePicker";
import { TrackOptionsMenu } from "../../../src/components/TrackOptionsMenu";
import { ChaptersPicker } from "../../../src/components/ChaptersPicker";
import { SleepTimerPicker } from "../../../src/components/SleepTimerPicker";
import { VolumePicker } from "../../../src/components/VolumePicker";
import { StreamInfoPanel } from "../../../src/components/StreamInfoPanel";
import { SkipIntroButton } from "../../../src/components/SkipIntroButton";
import { NextEpisodeOverlay } from "../../../src/components/NextEpisodeOverlay";
import { TrickplaySeekPreview } from "../../../src/components/TrickplaySeekPreview";
import { useParentalGate } from "../../../src/hooks/useParentalGate";
import { ParentalPINModal } from "../../../src/components/ParentalPINModal";
import { useSyncplay } from "../../../src/hooks/useSyncplay";
import { SyncplaySessionBadge } from "../../../src/components/SyncplaySessionBadge";

const IMAGE_SUBTITLE_CODECS = new Set([
  "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub",
]);

const CONTROLS_TIMEOUT = 4000;

function formatTime(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
  return `${m}:${String(sec).padStart(2, "0")}`;
}

export default function PlayerScreen() {
  // `restart=1` means the viewer chose "from the beginning" on the media page.
  // Without it the player always resumes, which is the right default - this is
  // the only way to say otherwise.
  const { id, roomId, restart } = useLocalSearchParams<{ id: string; roomId?: string; restart?: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const mediaId = id ?? "";
  useAudioCapsBeacon(mediaId);
  const { width: screenWidth, height: screenHeight } = useWindowDimensions();

  const { data: media } = useLibraryServiceGetMediaItem(mediaId);

  // Two hops up so the top bar can name the SHOW, not just the episode: an
  // episode hangs off its season, and no payload carries the series title (see
  // the backlog note of 2026-08-03). Only fetched while an episode is playing,
  // and only once per playback - a bounded cost the rows could not afford.
  const isEpisodePlaying = media?.type === "MEDIA_ITEM_TYPE_EPISODE";
  const { data: season } = useLibraryServiceGetMediaItem(media?.parentId ?? "", {
    query: { enabled: isEpisodePlaying && !!media?.parentId },
  });
  const { data: series } = useLibraryServiceGetMediaItem(season?.parentId ?? "", {
    query: { enabled: isEpisodePlaying && !!season?.parentId },
  });

  // A series or a season has no file of its own: asking the server to stream
  // one can only 404, and the player would then blame the stream. It happened
  // in the field - the home hero hands the player whatever it is spotlighting,
  // and "Maison du dragon" is a series. Callers are supposed to resolve an
  // episode first; this is the backstop for the ones that do not, and for the
  // race where the media page has not loaded its episode list yet.
  const notPlayable =
    media?.type === "MEDIA_ITEM_TYPE_SERIES" || media?.type === "MEDIA_ITEM_TYPE_SEASON";
  useEffect(() => {
    if (notPlayable) router.replace(`/media/${mediaId}`);
  }, [notPlayable, mediaId]);
  const { streamUrl, sessionId, streamInfo, isLoading, isError, isAuthError, shedReason, stopSession, restartWithTracks, progress } =
    usePlaySession(mediaId);
  const profileId = useAuthStore((s) => s.profileId) ?? "";

  // Profile audio/subtitle language preference (audio_languages/subtitle_languages,
  // CSV of ISO 639-1 codes in profile.preferences - shared web/mobile/tv
  // convention, see settings/playback.tsx). Mirrors the web player's
  // HLSPlayer.tsx / mobile's player screen: applied to the initial track
  // resolution below, but never overrides an explicit in-session pick
  // (userPickedAudioRef / userPickedSubtitleRef).
  const { data: currentUserForPrefs } = useUserServiceGetCurrentUser();
  const { data: profilesRespForPrefs } = useUserServiceListProfiles(
    currentUserForPrefs?.id ?? "",
    { query: { enabled: !!currentUserForPrefs?.id } } as never,
  );
  const activeProfile = useMemo(
    () =>
      profilesRespForPrefs?.profiles?.find((p) => p.id === profileId) ??
      profilesRespForPrefs?.profiles?.find((p) => p.isDefault) ??
      profilesRespForPrefs?.profiles?.[0],
    [profilesRespForPrefs, profileId],
  );
  const profileAudioLangs = useMemo((): string[] => {
    const raw = activeProfile?.preferences?.["audio_languages"] ?? "";
    return raw.split(",").map((s) => s.trim()).filter(Boolean);
  }, [activeProfile]);
  const profileSubtitleLangs = useMemo((): string[] => {
    const raw = activeProfile?.preferences?.["subtitle_languages"] ?? "";
    return raw.split(",").map((s) => s.trim()).filter(Boolean);
  }, [activeProfile]);

  const [showControls, setShowControls] = useState(true);
  // True while the action bar owns the D-pad. Mirrors the existing
  // skipVisible / syncplayBadgeFocused pattern: exactly one thing interprets
  // a key press at a time.
  const [barFocused, setBarFocused] = useState(false);
  // The full-screen surface below every overlay. Some native view must hold
  // TV focus at all times or Android stops delivering key events entirely -
  // the screen deliberately has no other focusable while the bar is closed,
  // which left the whole player deaf (remote AND pointer) on real hardware.
  const [surfaceFocused, setSurfaceFocused] = useState(false);
  const surfaceRef = useRef<View>(null);

  const [showQualityPicker, setShowQualityPicker] = useState(false);
  const [showTrackMenu, setShowTrackMenu] = useState(false);
  const [showAudioPicker, setShowAudioPicker] = useState(false);
  const [showSubtitlePicker, setShowSubtitlePicker] = useState(false);
  const [showChaptersPicker, setShowChaptersPicker] = useState(false);
  const [showSleepTimerPicker, setShowSleepTimerPicker] = useState(false);
  const [showVolumePicker, setShowVolumePicker] = useState(false);
  const [showStreamInfoPanel, setShowStreamInfoPanel] = useState(false);
  const [skipVisible, setSkipVisible] = useState(false);
  const [selectedQuality, setSelectedQuality] = useState(0);
  const [selectedAudioIndex, setSelectedAudioIndex] = useState<number>(0);
  const [selectedSubtitleIndex, setSelectedSubtitleIndex] = useState<number | null>(null);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [playbackSpeed, setPlaybackSpeed] = useState(1);
  const [volumePct, setVolumePct] = useState(100);
  const [muted, setMuted] = useState(false);
  // Local target position shown the instant a seek key is pressed, released once
  // the real playhead (fed by the player's own timeUpdate listener) catches up -
  // see playerSeek.ts. Without this the progress bar only moved on the next
  // timeUpdate tick, which read as unresponsive during an accelerated hold.
  const [seekPreviewSec, setSeekPreviewSec] = useState<number | null>(null);
  // A fatal native failure (decoder error, 401 on a segment, broken manifest)
  // surfaces ONLY through the player's statusChange event - isError from
  // usePlaySession covers just the session-start network call. Ignoring it
  // left a black screen with live controls.
  const [nativeError, setNativeError] = useState<string | null>(null);
  // The native player is recreated per media (new streamUrl), but this state
  // wasn't reset with it - a fatal error on one item stayed glued to the screen
  // of the NEXT one (e.g. after a next-episode auto-advance), showing a dead
  // error screen over a stream that hadn't even started yet.
  useEffect(() => { setNativeError(null); }, [mediaId]);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Whether the NEXT player instance should start playing. Changing tracks or
  // quality recreates the native player, whose setup used to call play()
  // unconditionally - a paused screen sprang back to life on every track
  // change. Handlers snapshot the playing state right before restarting;
  // defaults to true so first entry (and next-episode remounts) autoplay.
  const resumeAfterRestartRef = useRef(true);

  const audioStreams: AudioStream[] = useMemo(
    () =>
      streamInfo?.mediaStreams
        ?.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO")
        .map((s, i) => ({
          index: s.index ?? 0,
          trackIndex: i,
          language: s.language,
          title: s.title,
          isDefault: s.isDefault,
          channels: s.channels,
        })) ?? [],
    [streamInfo],
  );

  const subtitleStreams: SubtitleStream[] = useMemo(
    () =>
      streamInfo?.mediaStreams
        ?.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE")
        .map((s, i) => ({
          index: s.index ?? 0,
          trackIndex: i,
          language: s.language,
          title: s.title,
          isDefault: s.isDefault,
          isForced: s.isForced,
          codec: s.codec,
          burnIn: IMAGE_SUBTITLE_CODECS.has(s.codec?.toLowerCase() ?? ""),
        })) ?? [],
    [streamInfo],
  );

  // Set initial selected audio index: profile preference first, then the
  // manifest's isDefault stream, then the first stream. Once the user has
  // manually picked a track, this must NOT run again: restartWithTracks
  // re-derives audioStreams (new reference), which would otherwise re-fire
  // this effect and reset the choice back to the isDefault track.
  const userPickedAudioRef = useRef(false);
  useEffect(() => {
    if (userPickedAudioRef.current) return;
    const preferred = pickPreferredStreamIndex(audioStreams, profileAudioLangs);
    if (preferred !== null) { setSelectedAudioIndex(preferred); return; }
    const def = audioStreams.find((s) => s.isDefault);
    if (def) setSelectedAudioIndex(def.index);
    else if (audioStreams.length > 0) setSelectedAudioIndex(audioStreams[0].index);
  }, [audioStreams, profileAudioLangs]);

  // Same guard for subtitles: applies the profile's subtitle_languages
  // preference once, and never once the user has picked a subtitle by hand
  // (see handleSubtitleSelect). No preference match leaves subtitles off -
  // matches prior TV behaviour (no isDefault-subtitle fallback).
  const userPickedSubtitleRef = useRef(false);
  useEffect(() => {
    if (userPickedSubtitleRef.current) return;
    const preferred = pickPreferredStreamIndex(subtitleStreams, profileSubtitleLangs);
    if (preferred !== null) setSelectedSubtitleIndex(preferred);
  }, [subtitleStreams, profileSubtitleLangs]);

  // A track pick made on the previous title means nothing on the next one
  // (auto-next-episode or a new navigation reuses this screen) - without this
  // reset, the profile language preference would never re-apply past the
  // first item played in a session.
  useEffect(() => {
    userPickedAudioRef.current = false;
    userPickedSubtitleRef.current = false;
  }, [mediaId]);

  const { data: chaptersData } = useChapters(mediaId);
  const chapters = chaptersData ?? [];

  // Leaving the screen calls stopSession(), which clears streamUrl. A null
  // source changes useVideoPlayer's dependency, and useReleasingSharedObject
  // then CREATES a fresh native player and releases the old one - in the middle
  // of the pop. That teardown dance is what throws
  // ERR_USING_RELEASED_SHARED_OBJECT from inside the screen.
  //
  // A bare VideoView screen whose source never goes null does not crash on exit
  // (measured on device 2026-08-03), so holding the last source is the fix:
  // identity stays stable, and unmount performs the single real release.
  const lastSourceRef = useRef<{ uri: string; contentType?: "hls" } | null>(null);
  const videoSource = useMemo(() => {
    if (!streamUrl) return lastSourceRef.current;
    const isHls = streamUrl.includes("m3u8");
    const next = {
      uri: streamUrl,
      ...(isHls ? { contentType: "hls" as const } : {}),
    };
    lastSourceRef.current = next;
    return next;
  }, [streamUrl]);

  const resumePositionSec = useMemo(() => {
    if (restart === "1") return 0;
    if (!progress?.positionMs || progress.played) return 0;
    const ms = Number(progress.positionMs);
    return ms > 0 ? ms / 1000 : 0;
  }, [progress, restart]);

  const player = useVideoPlayer(videoSource, (p) => {
    p.timeUpdateEventInterval = 0.5;
    if (resumePositionSec > 0) {
      p.currentTime = resumePositionSec;
    }
    if (resumeAfterRestartRef.current) p.play();
  });

  // expo-video hands JavaScript a proxy onto a NATIVE object. When the source
  // changes - a quality switch, an audio-track switch, leaving the screen -
  // that native object is released, and any later touch throws
  // "Cannot use shared object that was already released" (or, more cryptically,
  // "cannot be cast to VideoPlayer (received Integer)"). React's own listeners
  // are unsubscribed on cleanup, but callbacks that fire from OUTSIDE the
  // component lifecycle - a SyncPlay websocket message arriving mid-teardown -
  // have no such protection, and one of them took the whole player screen down
  // to a red box.
  //
  // `withPlayer` is the guard: it runs the access, and swallows exactly the
  // released-object failure. Anything else still surfaces.
  const withPlayer = useCallback(<T,>(fn: (p: NonNullable<typeof player>) => T): T | undefined => {
    if (!player) return undefined;
    try {
      return fn(player);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (/already released|cannot be cast to/i.test(msg)) return undefined;
      throw err;
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [player]);


  useEffect(() => {
    if (!player) return;
    const timeSub = player.addListener("timeUpdate", (ev) => {
      setCurrentTime(ev.currentTime);
      // A tick can land between the release and the unsubscribe.
      withPlayer((p) => setDuration(p.duration));
      setSeekPreviewSec((prev) => (prev !== null && shouldReleaseSeekPreview(ev.currentTime, prev) ? null : prev));
    });
    const statusSub = player.addListener("statusChange", (ev) => {
      withPlayer((p) => {
        setIsPlaying(p.playing);
        setDuration(p.duration);
      });
      if (ev.status === "error") {
        console.warn("[player] native playback error:", ev.error?.message);
        setNativeError(ev.error?.message ?? "playback error");
      }
    });
    const playSub = player.addListener("playingChange", () => {
      withPlayer((p) => setIsPlaying(p.playing));
    });
    return () => {
      timeSub.remove();
      statusSub.remove();
      playSub.remove();
    };
  }, [player, withPlayer]);

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  // SyncPlay ("watch together"): TV only ever JOINS a room, landing here via
  // a `?roomId=` param - see app/(auth)/syncplay/index.tsx and
  // src/hooks/useSyncplay.ts for the join flow and the host-authority rule.
  const [syncplayBadgeFocused, setSyncplayBadgeFocused] = useState(false);
  // Every one of these fires from a websocket, i.e. possibly while the player
  // is being torn down - hence withPlayer rather than a plain `player?.`.
  const syncplay = useSyncplay({
    getLocalPositionMs: () => withPlayer((p) => p.currentTime * 1000) ?? 0,
    onPlay: (posMs, shouldSeek) => {
      withPlayer((p) => {
        if (shouldSeek) p.currentTime = posMs / 1000;
        p.play();
      });
    },
    onPause: (posMs, shouldSeek) => {
      withPlayer((p) => {
        if (shouldSeek) p.currentTime = posMs / 1000;
        p.pause();
      });
    },
    onSeek: (posMs) => {
      withPlayer((p) => {
        p.currentTime = posMs / 1000;
      });
    },
  });
  useEffect(() => {
    if (roomId && !syncplay.state.roomId) syncplay.connect(roomId);
  }, [roomId, syncplay.state.roomId, syncplay.connect]);

  // MaxResumePct=90 (the standard watched ceiling): trigger MarkAsWatched once
  // playback crosses 90%. The server's ReportProgressBySession path
  // hardcodes Completed=false so without this fix the user has to
  // manually toggle "watched" after every episode.
  const { data: serverProgress } = usePlaybackServiceGetWatchProgress(
    mediaId,
    { profileId },
    { query: { enabled: !!mediaId && !!profileId } } as never,
  );
  const markAsWatchedMut = usePlaybackServiceMarkAsWatched();
  const autoMarkedRef = useRef(false);
  useEffect(() => { autoMarkedRef.current = false; }, [mediaId]);
  useEffect(() => {
    if (autoMarkedRef.current) return;
    if (serverProgress?.played) { autoMarkedRef.current = true; return; }
    if (!profileId || !mediaId) return;
    if (duration <= 0 || currentTime <= 0) return;
    if (currentTime / duration >= 0.9) {
      autoMarkedRef.current = true;
      markAsWatchedMut.mutate({ mediaItemId: mediaId, params: { profileId } });
    }
  }, [currentTime, duration, serverProgress?.played, profileId, mediaId, markAsWatchedMut]);

  // The action bar is a menu the viewer walks with the remote: hiding it from
  // under them mid-navigation (left/right do not reset this timer) made the
  // player feel unusable. While it holds focus the controls simply stay.
  const barFocusedRef = useRef(false);
  barFocusedRef.current = barFocused;
  const resetHideTimer = useCallback(() => {
    if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    setShowControls(true);
    if (barFocusedRef.current) return;
    hideTimerRef.current = setTimeout(() => setShowControls(false), CONTROLS_TIMEOUT);
  }, []);

  // Entering the bar cancels a pending hide; leaving it re-arms one.
  useEffect(() => {
    if (barFocused) {
      if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
      setShowControls(true);
    } else {
      resetHideTimer();
    }
  }, [barFocused, resetHideTimer]);

  useEffect(() => {
    resetHideTimer();
    return () => {
      if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    };
  }, [resetHideTimer]);

  const togglePlayPause = useCallback(() => {
    // Through withPlayer like every other native touch: a press can land after
    // the player was released (leaving the screen, switching track) and the
    // raw access throws "cannot be cast to VideoPlayer".
    withPlayer((p) => {
      if (p.playing) {
        p.pause();
        if (syncplay.state.connected) syncplay.sendPause(p.currentTime * 1000);
      } else {
        p.play();
        if (syncplay.state.connected) syncplay.sendPlay(p.currentTime * 1000);
      }
    });
    resetHideTimer();
  }, [withPlayer, resetHideTimer, syncplay]);

  const { data: playbackSettings } = usePlaybackSettings();
  const skipFwd = playbackSettings?.skipForwardSeconds ?? 30;
  const skipBwd = playbackSettings?.skipBackwardSeconds ?? 30;

  const seekBy = useCallback(
    (seconds: number) => {
      if (!player) return;
      // Native relative seek on the 0-based VOD timeline, transcode included.
      // This used to tear down and restart the session on transcode, which
      // replayed from 0 while the UI added an offset on top - every skip
      // reported roughly twice the real position and inflated the duration.
      player.seekBy(seconds);
      resetHideTimer();
    },
    [player, resetHideTimer],
  );

  // Remote-driven seek: left/right (plain skip) and longLeft/longRight
  // (accelerated, native TV "hold" events - see playerSeek.ts). Updates a local
  // preview position immediately so the progress bar moves on the SAME frame as
  // the keypress instead of waiting for the player's next timeUpdate tick.
  const seekByKey = useCallback(
    (type: string) => {
      const delta = seekDeltaForKey(type, skipBwd, skipFwd);
      if (delta === 0) return;
      const base = seekPreviewSec ?? currentTime;
      const target = clampSeekTarget(base, delta, duration);
      setSeekPreviewSec(target);
      seekBy(delta);
    },
    [skipBwd, skipFwd, seekPreviewSec, currentTime, duration, seekBy],
  );

  const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2] as const;
  const cycleSpeed = useCallback(() => {
    setPlaybackSpeed((prev) => {
      const idx = SPEEDS.indexOf(prev as (typeof SPEEDS)[number]);
      const next = SPEEDS[(idx + 1) % SPEEDS.length];
      withPlayer((p) => { p.playbackRate = next; });
      return next;
    });
    resetHideTimer();
  }, [withPlayer, resetHideTimer]);

  const handleQualitySelect = useCallback(
    (quality: number) => {
      setSelectedQuality(quality);
      resumeAfterRestartRef.current = player?.playing ?? true;
      restartWithTracks(selectedAudioIndex, selectedSubtitleIndex ?? undefined, quality || undefined);
    },
    [player, restartWithTracks, selectedAudioIndex, selectedSubtitleIndex],
  );

  const handleAudioSelect = useCallback(
    (index: number) => {
      userPickedAudioRef.current = true;
      setSelectedAudioIndex(index);
      resumeAfterRestartRef.current = player?.playing ?? true;
      restartWithTracks(index, selectedSubtitleIndex ?? undefined, selectedQuality || undefined);
    },
    [player, restartWithTracks, selectedSubtitleIndex, selectedQuality],
  );

  const handleSubtitleSelect = useCallback(
    (index: number | null) => {
      userPickedSubtitleRef.current = true;
      setSelectedSubtitleIndex(index);
      resumeAfterRestartRef.current = player?.playing ?? true;
      restartWithTracks(selectedAudioIndex, index ?? undefined, selectedQuality || undefined);
    },
    [player, restartWithTracks, selectedAudioIndex, selectedQuality],
  );

  const handleSkipIntro = useCallback(
    (toSec: number) => {
      withPlayer((p) => { p.currentTime = toSec; });
    },
    [withPlayer],
  );

  const handleToggleMute = useCallback(() => {
    setMuted((prev) => {
      const next = !prev;
      withPlayer((p) => { p.muted = next; });
      return next;
    });
    resetHideTimer();
  }, [withPlayer, resetHideTimer]);

  const handleVolumeSelect = useCallback(
    (pct: number) => {
      setVolumePct(pct);
      setMuted(false);
      withPlayer((p) => {
        p.volume = pct / 100;
        p.muted = false;
      });
      resetHideTimer();
    },
    [withPlayer, resetHideTimer],
  );

  // Same exit contract as the web/mobile player's back button (playerExit):
  // pop the player's own history entry when there is one - safe because episode
  // changes REPLACE - otherwise (deep link, device "loadAndPlay" command)
  // REPLACE toward the media page (an episode exits to its season) so no
  // player entry is ever left behind for a later back press to walk into.
  const handleBack = useCallback(() => {
    stopSession();
    const exit = playerExit(router.canGoBack(), mediaId, media);
    if (exit.action === "back") router.back();
    else router.replace(`/media/${exit.mediaId}` as never);
  }, [stopSession, mediaId, media]);

  const handleNavigateEpisode = useCallback(
    (episodeId: string) => {
      stopSession();
      router.replace(`/(auth)/player/${episodeId}` as never);
    },
    [stopSession],
  );

  // The next-episode auto-advance (and its "play now" button) jump straight
  // into another player instance, bypassing the media detail screen where
  // the parental PIN gate normally lives. useNextEpisode calls onNavigate
  // both on its own countdown and from playNow, so gating that single
  // callback covers both paths. onNavigate is wired through a ref so the
  // gate (which needs nextEp.nextEpisode's rating, only known once
  // Entry gate: the player is reachable WITHOUT going through the media detail
  // screen - the home hero, a deep link, and the remote "loadAndPlay" device
  // command all land straight here. Gating the screen itself (rather than each
  // caller) is the invariant that holds for any future entry point. The server
  // refuses the stream regardless; this turns a dead error into a PIN prompt.
  const { needsPin: entryNeedsPin, profileId: entryGateProfileId } = useParentalGate(media?.officialRating);
  const [entryUnlocked, setEntryUnlocked] = useState(false);
  const entryGated = !!media && !entryUnlocked && entryNeedsPin();

  useEffect(() => {
    if (entryGated) player?.pause();
  }, [entryGated, player]);

  const handleEntryPinVerified = useCallback(() => {
    setEntryUnlocked(true);
    player?.play();
  }, [player]);

  // useNextEpisode itself has resolved it) can be defined after the hook
  // call without creating a circular dependency.
  const nextEpNavigateRef = useRef<(episodeId: string) => void>(() => {});
  const forwardNextEpNavigate = useCallback((episodeId: string) => {
    nextEpNavigateRef.current(episodeId);
  }, []);

  const nextEp = useNextEpisode({
    media,
    currentTime,
    duration,
    onNavigate: forwardNextEpNavigate,
  });

  const { needsPin: nextEpNeedsPin, profileId: nextEpGateProfileId } = useParentalGate(
    nextEp.nextEpisode?.officialRating,
  );
  const [showNextEpPinModal, setShowNextEpPinModal] = useState(false);
  const [pendingNextEpisodeId, setPendingNextEpisodeId] = useState<string | null>(null);

  const gatedNavigateEpisode = useCallback(
    (episodeId: string) => {
      if (nextEpNeedsPin()) {
        setPendingNextEpisodeId(episodeId);
        setShowNextEpPinModal(true);
        return;
      }
      handleNavigateEpisode(episodeId);
    },
    [nextEpNeedsPin, handleNavigateEpisode],
  );
  nextEpNavigateRef.current = gatedNavigateEpisode;

  const handleNextEpPinVerified = useCallback(() => {
    setShowNextEpPinModal(false);
    if (pendingNextEpisodeId) {
      handleNavigateEpisode(pendingNextEpisodeId);
      setPendingNextEpisodeId(null);
    }
  }, [pendingNextEpisodeId, handleNavigateEpisode]);

  // Manual "next episode" control (item 2): same gate as the auto-advance,
  // triggered on demand instead of by the countdown.
  const handleManualNextEpisode = useCallback(() => {
    if (nextEp.nextEpisode?.id) gatedNavigateEpisode(nextEp.nextEpisode.id);
  }, [nextEp.nextEpisode, gatedNavigateEpisode]);

  // Manual "previous episode" control: never driven by the auto-advance
  // countdown, so it is gated independently on the PREVIOUS episode's own
  // rating rather than reusing the next-episode gate above.
  const { needsPin: prevEpNeedsPin, profileId: prevEpGateProfileId } = useParentalGate(
    nextEp.previousEpisode?.officialRating,
  );
  const [showPrevEpPinModal, setShowPrevEpPinModal] = useState(false);

  const handlePreviousEpisode = useCallback(() => {
    if (!nextEp.previousEpisode?.id) return;
    if (prevEpNeedsPin()) {
      setShowPrevEpPinModal(true);
      return;
    }
    handleNavigateEpisode(nextEp.previousEpisode.id);
  }, [nextEp.previousEpisode, prevEpNeedsPin, handleNavigateEpisode]);

  const handlePrevEpPinVerified = useCallback(() => {
    setShowPrevEpPinModal(false);
    if (nextEp.previousEpisode?.id) handleNavigateEpisode(nextEp.previousEpisode.id);
  }, [nextEp.previousEpisode, handleNavigateEpisode]);

  const sleepTimer = useSleepTimer({ onMinutesFire: () => player?.pause() });

  // "End of episode" sleep mode has no countdown of its own - it rides the same
  // "ended" signal the next-episode auto-advance uses. Cancelling the
  // auto-advance too stops it from starting the next item right after pausing.
  useEffect(() => {
    if (nextEp.ended && sleepTimer.state?.kind === "endOfEpisode") {
      player?.pause();
      sleepTimer.cancel();
      nextEp.cancel();
    }
  }, [nextEp.ended, sleepTimer.state?.kind, player, sleepTimer.cancel, nextEp.cancel]);

  const anyOverlayOpen =
    showQualityPicker || showTrackMenu || showAudioPicker || showSubtitlePicker ||
    showChaptersPicker || showSleepTimerPicker || showVolumePicker || showStreamInfoPanel ||
    nextEp.visible;

  // D-pad remote handler. The callback param is annotated explicitly: the
  // react-native-tvos alias resolves per-app, and the hoisted classic RN
  // types (shared workspace) leave HWEvent unresolvable in some installs -
  // a structural annotation keeps tsc deterministic everywhere.
  //
  // Full remote map (every plain direction plus its long-press variant is
  // claimed, so anything else lives behind the longSelect options menu):
  //   select/playPause  -> toggle play/pause
  //   longSelect        -> options menu (audio/subtitles/chapters/volume/sleep/info)
  //   left/right        -> skip back/forward
  //   longLeft/longRight-> accelerated seek (native TV "hold" events)
  //   up / longUp       -> cycle speed / previous episode
  //   down / longDown   -> quality picker / next episode
  //   menu/back         -> exit (playerExit contract)
  // The subscription must be STABLE, the logic must be FRESH.
  //
  // react-native's useTVEventHandler keys its effect on the handler's identity
  // (`useEffect(..., [handleEvent])`), so an inline arrow tears the native
  // listener down and re-adds it on every render - and this screen re-renders
  // about twice a second while playing (timeUpdateEventInterval = 0.5). A D-pad
  // press landing in that remove/re-add gap is silently dropped, which reads as
  // "the remote didn't respond". Same family as the syncplay churn fixed in
  // 3df3e8afb, but this one is the subscription itself.
  //
  // The body below is left EXACTLY as it was and simply re-assigned each render:
  // that keeps every value it closes over current without having to enumerate
  // them as dependencies, which is where a stale-closure bug would creep in on
  // a screen this stateful.
  const remoteHandlerRef = useRef<(evt: { eventType: string } | undefined) => void>(() => {});
  remoteHandlerRef.current = (evt: { eventType: string } | undefined) => {
    if (!evt) return;
    const type = evt.eventType;

    // While any overlay (picker/menu/next-episode card) is showing, its own
    // FocusableCard rows own the D-pad via the native TV focus engine - if we
    // kept interpreting the same key here (e.g. "select") it would double-fire
    // (native onPress AND togglePlayPause()).
    if (anyOverlayOpen) return;
    // The Skip button holds TV focus while visible; its own FocusableCard
    // native onPress handles OK, so don't ALSO toggle play/pause on "select"
    // (the double-fire the audit found). The dedicated playPause remote key
    // still pauses during an intro.
    if (skipVisible && type === "select") return;
    if (syncplayBadgeFocused && type === "select") return;
    // OK on the focused surface already fires its native onPress - handling
    // "select" here too would double-toggle play/pause.
    if (surfaceFocused && type === "select") return;
    // While the action bar holds focus its own buttons own the D-pad: left and
    // right walk the row, OK activates. Interpreting the same keys here would
    // seek behind the viewer's back while they are picking an audio track.
    if (barFocused) {
      if (type === "up" || type === "menu" || type === "back") {
        setBarFocused(false);
        resetHideTimer();
      }
      return;
    }

    if (type === "select" || type === "playPause") {
      togglePlayPause();
    } else if (type === "left" || type === "right" || type === "longLeft" || type === "longRight") {
      seekByKey(type);
    } else if (type === "down") {
      // The one way into the options, and the bar says so on screen. Every
      // other binding this screen used to carry (long-OK for the menu, up for
      // speed, long-up/long-down for episodes) was invisible: the owner could
      // not work out how to drive the player at all. They live in the bar now.
      if (showControls) setBarFocused(true);
      else resetHideTimer();
    } else if (type === "menu" || type === "back") {
      if (showControls) handleBack();
      else resetHideTimer();
    } else {
      // Any other key just wakes the controls rather than doing something the
      // viewer cannot predict.
      resetHideTimer();
    }
  };
  useTVEventHandler(useCallback((evt: { eventType: string } | undefined) => remoteHandlerRef.current?.(evt), []));

  // Imperative focus reclaim: whenever no overlay/bar should own the D-pad,
  // force native focus back onto the surface - if no native view is focused
  // Android stops delivering key events and the player goes deaf.
  useEffect(() => {
    if (barFocused || anyOverlayOpen) return;
    const t = setTimeout(
      () => (surfaceRef.current as unknown as { requestTVFocus?: () => void })?.requestTVFocus?.(),
      100,
    );
    return () => clearTimeout(t);
  }, [barFocused, anyOverlayOpen]);

  // Called from useProgressReporter's CLEANUP, which runs while the screen is
  // being popped and the native player may already be released. Reading it raw
  // threw ERR_USING_RELEASED_SHARED_OBJECT from an effect cleanup, and React
  // reported that against this component - the exit turned into an error
  // screen (device, 2026-08-03). Its neighbour below already learned this.
  const getPositionMs = useCallback(() => {
    return (withPlayer((p) => p.currentTime) ?? 0) * 1000;
  }, [withPlayer]);

  const getIsPaused = useCallback(() => {
    try {
      return !(player?.playing ?? false);
    } catch {
      return true;
    }
  }, [player]);

  const remoteCallbacks = useMemo(
    () => ({
      onRemotePause: () => {
        try {
          player?.pause();
        } catch (err) {
          console.warn("remote pause failed:", err);
        }
      },
      onRemotePlay: () => {
        try {
          player?.play();
        } catch (err) {
          console.warn("remote play failed:", err);
        }
      },
      onRemoteStop: () => {
        handleBack();
      },
    }),
    [player, handleBack],
  );

  useProgressReporter(sessionId, getPositionMs, getIsPaused, undefined, undefined, remoteCallbacks);

  // Server-side load shed (HTTP 503): distinct from isError below - nothing is
  // broken, the server just isn't ready to serve THIS request yet, for one of
  // two unrelated reasons (see usePlaySession's ShedReason doc). Calm clock
  // icon (not a red error) and copy that names neither "transcoding" nor any
  // other internal term.
  if (shedReason) {
    return (
      <View style={[styles.container, { backgroundColor: "#000" }]}>
        <Ionicons name="time-outline" size={48} color="rgba(255,255,255,0.7)" />
        <Text style={{ color: "#fff", fontSize: 24, fontWeight: "600", marginTop: 20, textAlign: "center", paddingHorizontal: 60 }}>
          {t(shedReason === "file_not_ready" ? "player.shed.fileNotReady.title" : "player.shed.tooManyStreams.title")}
        </Text>
        <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: 18, marginTop: 12, textAlign: "center", paddingHorizontal: 80 }}>
          {t(shedReason === "file_not_ready" ? "player.shed.fileNotReady.body" : "player.shed.tooManyStreams.body")}
        </Text>
        <Pressable
          onPress={() => {
            // Retrying after a shed means the user wants playback back - always autoplay.
            resumeAfterRestartRef.current = true;
            restartWithTracks(selectedAudioIndex, selectedSubtitleIndex ?? undefined, selectedQuality || undefined);
          }}
          style={{ marginTop: 28 }}
        >
          <Text style={{ color: theme.colors.accent, fontSize: 22, fontWeight: "600" }}>{t("player.retry")}</Text>
        </Pressable>
        <Pressable onPress={handleBack} style={{ marginTop: 20 }}>
          <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: 18 }}>{t("player.goBack", "Go back")}</Text>
        </Pressable>
      </View>
    );
  }

  // An expired session is not a broken stream. Retrying it can only fail
  // again, so the screen offers the one action that helps: sign in.
  if (isAuthError) {
    return (
      <View style={[styles.container, { backgroundColor: "#000" }]}>
        <Ionicons name="log-in-outline" size={48} color="rgba(255,255,255,0.7)" />
        <Text style={{ color: "#fff", fontSize: 24, fontWeight: "600", marginTop: 20, textAlign: "center", paddingHorizontal: 60 }}>
          {t("player.sessionExpired.title", "Your session has expired")}
        </Text>
        <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: 18, marginTop: 12, textAlign: "center", paddingHorizontal: 80 }}>
          {t("player.sessionExpired.body", "Sign in again to keep watching.")}
        </Text>
        <Pressable onPress={() => router.replace("/login")} style={{ marginTop: 28 }} hasTVPreferredFocus>
          <Text style={{ color: theme.colors.accent, fontSize: 22, fontWeight: "600" }}>{t("auth.login.signIn")}</Text>
        </Pressable>
        <Pressable onPress={handleBack} style={{ marginTop: 20 }}>
          <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: 18 }}>{t("player.goBack", "Go back")}</Text>
        </Pressable>
      </View>
    );
  }

  if (isError || nativeError) {
    return (
      <View style={[styles.container, { backgroundColor: "#000" }]}>
        <Text style={{ color: "#fff", fontSize: 24 }}>
          {t("player.failedToLoadStream", "Failed to load stream")}
        </Text>
        <Pressable
          onPress={() => {
            // A fatal native error otherwise stays glued to the screen even if
            // the retry succeeds - see the nativeError reset effect above for
            // the twin bug (a stale error surviving a media CHANGE).
            setNativeError(null);
            resumeAfterRestartRef.current = true;
            restartWithTracks(selectedAudioIndex, selectedSubtitleIndex ?? undefined, selectedQuality || undefined);
          }}
          style={{ marginTop: 24 }}
        >
          <Text style={{ color: theme.colors.accent, fontSize: 22, fontWeight: "600" }}>{t("player.retry")}</Text>
        </Pressable>
        <Pressable onPress={handleBack} style={{ marginTop: 20 }}>
          <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: 18 }}>{t("player.goBack", "Go back")}</Text>
        </Pressable>
      </View>
    );
  }

  if (isLoading || !streamUrl) {
    return (
      <View style={[styles.container, { backgroundColor: "#000" }]}>
        <ActivityIndicator size="large" color={theme.colors.accent} />
      </View>
    );
  }

  // The preview position (set the instant a seek key fires) takes over the
  // display until the real playhead catches up - see playerSeek.ts.
  const displayTime = seekPreviewSec ?? currentTime;
  const seekProgress = duration > 0 ? displayTime / duration : 0;

  return (
    <View style={[styles.container, { backgroundColor: "#000" }]}>
      <VideoView
        player={player}
        style={[styles.video, { width: screenWidth, height: screenHeight }]}
        nativeControls={false}
        contentFit="contain"
      />

      {/* Focus holder and pointer surface, kept under every overlay. */}
      <Pressable
        ref={surfaceRef}
        style={StyleSheet.absoluteFill}
        focusable={true}
        hasTVPreferredFocus={!barFocused && !anyOverlayOpen}
        onFocus={() => setSurfaceFocused(true)}
        onBlur={() => setSurfaceFocused(false)}
        onPress={() => {
          if (!showControls) {
            resetHideTimer();
            return;
          }
          togglePlayPause();
        }}
        accessible={true}
        accessibilityLabel={t("player.controls.surface", "Player surface")}
      />

      <QualityPicker
        visible={showQualityPicker}
        currentQuality={selectedQuality}
        onSelect={handleQualitySelect}
        onClose={() => setShowQualityPicker(false)}
      />

      <TrackOptionsMenu
        visible={showTrackMenu}
        onClose={() => setShowTrackMenu(false)}
        onOpenQuality={() => {
          setShowTrackMenu(false);
          setShowQualityPicker(true);
        }}
        qualityLabel={selectedQuality > 0 ? `${selectedQuality}p` : t("player.settings.auto")}
        onCycleSpeed={cycleSpeed}
        speedLabel={`${playbackSpeed}x`}
        onPreviousEpisode={nextEp.previousEpisode ? () => {
          setShowTrackMenu(false);
          handlePreviousEpisode();
        } : undefined}
        onNextEpisode={nextEp.nextEpisode ? () => {
          setShowTrackMenu(false);
          handleManualNextEpisode();
        } : undefined}
        hasChapters={chapters.length > 0}
        onOpenChapters={() => {
          setShowTrackMenu(false);
          setShowChaptersPicker(true);
        }}
        onOpenVolume={() => {
          setShowTrackMenu(false);
          setShowVolumePicker(true);
        }}
        onOpenSleepTimer={() => {
          setShowTrackMenu(false);
          setShowSleepTimerPicker(true);
        }}
        hasStreamInfo={!!streamInfo}
        onOpenStreamInfo={() => {
          setShowTrackMenu(false);
          setShowStreamInfoPanel(true);
        }}
      />

      <AudioTrackPicker
        visible={showAudioPicker}
        streams={audioStreams}
        currentIndex={selectedAudioIndex}
        onSelect={handleAudioSelect}
        onClose={() => setShowAudioPicker(false)}
      />

      <SubtitlePicker
        visible={showSubtitlePicker}
        streams={subtitleStreams}
        currentIndex={selectedSubtitleIndex}
        onSelect={handleSubtitleSelect}
        onClose={() => setShowSubtitlePicker(false)}
      />

      <ChaptersPicker
        visible={showChaptersPicker}
        onClose={() => setShowChaptersPicker(false)}
        chapters={chapters}
        currentTimeMs={currentTime * 1000}
        onSeek={(sec) => withPlayer((p) => { p.currentTime = sec; })}
      />

      <VolumePicker
        visible={showVolumePicker}
        onClose={() => setShowVolumePicker(false)}
        volumePct={volumePct}
        muted={muted}
        onSelect={handleVolumeSelect}
        onToggleMute={handleToggleMute}
      />

      <SleepTimerPicker
        visible={showSleepTimerPicker}
        onClose={() => setShowSleepTimerPicker(false)}
        state={sleepTimer.state}
        onPickMinutes={sleepTimer.setMinutes}
        onPickEndOfEpisode={sleepTimer.setEndOfEpisode}
        onCancel={sleepTimer.cancel}
      />

      <StreamInfoPanel
        visible={showStreamInfoPanel}
        onClose={() => setShowStreamInfoPanel(false)}
        streamInfo={streamInfo ?? null}
      />

      <SkipIntroButton mediaId={mediaId} currentTime={currentTime} onSkip={handleSkipIntro} onVisibleChange={setSkipVisible} />

      <NextEpisodeOverlay
        visible={nextEp.visible}
        ended={nextEp.ended}
        countdown={nextEp.countdown}
        nextEpisode={nextEp.nextEpisode}
        onPlayNow={nextEp.playNow}
        onCancel={nextEp.cancel}
      />

      {entryGateProfileId ? (
        <ParentalPINModal
          visible={entryGated}
          onClose={() => router.back()}
          onVerified={handleEntryPinVerified}
          profileId={entryGateProfileId}
        />
      ) : null}

      {nextEpGateProfileId ? (
        <ParentalPINModal
          visible={showNextEpPinModal}
          onClose={() => { setShowNextEpPinModal(false); setPendingNextEpisodeId(null); }}
          onVerified={handleNextEpPinVerified}
          profileId={nextEpGateProfileId}
        />
      ) : null}

      {prevEpGateProfileId ? (
        <ParentalPINModal
          visible={showPrevEpPinModal}
          onClose={() => setShowPrevEpPinModal(false)}
          onVerified={handlePrevEpPinVerified}
          profileId={prevEpGateProfileId}
        />
      ) : null}

      {showControls && (
        <View style={styles.overlay} pointerEvents="box-none">
          {/* Top bar */}
          <View style={styles.topBar}>
            <Text style={styles.mediaTitle} numberOfLines={1}>
              {playerTitle(media, series?.title ?? season?.title)}
            </Text>
            <View style={styles.badgeRow}>
              <SyncplaySessionBadge
                connected={syncplay.state.connected}
                onFocusChange={setSyncplayBadgeFocused}
                onPress={() => {
                  if (syncplay.state.connected) syncplay.leaveRoom();
                  else router.push("/(auth)/syncplay");
                }}
              />
              {selectedQuality > 0 && (
                <View style={styles.speedBadge}>
                  <Text style={styles.speedText}>{selectedQuality}p</Text>
                </View>
              )}
              <View style={styles.speedBadge}>
                <Text style={styles.speedText}>{playbackSpeed}x</Text>
              </View>
            </View>
          </View>

          {/* Centre: state and seek amounts only. Deliberately not buttons -
              OK already toggles play/pause and left/right already seek, so a
              focusable target here would only add a stop on the way down to
              the action bar. */}
          <View style={styles.centerControls}>
            <View style={styles.seekHint}>
              <Text style={styles.seekHintText}>-{skipBwd}s</Text>
            </View>
            <View style={styles.playIndicator}>
              <Text style={styles.playIcon}>{isPlaying ? "||" : "\u25B6"}</Text>
            </View>
            <View style={styles.seekHint}>
              <Text style={styles.seekHintText}>+{skipFwd}s</Text>
            </View>
          </View>

          {/* Trickplay thumbnail: the D-pad seek gesture has no live video
              preview to look at while it's happening (unlike a mouse
              scrubber), so this is the only way to find a scene at three
              metres without landing blind. Renders nothing on its own when
              there's no manifest yet, no generated sprites, or a failed
              fetch - the seek behaves exactly as it does today. */}
          <View style={styles.trickplayWrap} pointerEvents="none">
            <TrickplaySeekPreview mediaId={mediaId} previewTimeSec={seekPreviewSec} />
          </View>

          {/* Action bar: the only way to reach the options, and it says so.
              Focus enters it with the down key and leaves with up or back;
              while it holds focus the global remote handler stands down (see
              barFocused) so left/right walk the row instead of seeking. */}
          <View style={styles.actionBar} pointerEvents="box-none">
            {!barFocused ? (
              // focusable={false} on purpose: this must stay OUT of the D-pad
              // focus engine (down is what opens the bar, and a focus stop
              // here would swallow it), while still answering a click. That
              // click is the only way in for a pointer - an emulator driven
              // with a mouse, or a TV remote with a touchpad.
              <Pressable
                focusable={false}
                onPress={() => setBarFocused(true)}
                accessibilityLabel={t("player.controls.more", "More")}
                accessibilityRole="button"
              >
                <Text style={styles.actionHint}>{"\u25BC"} {t("player.controls.more", "More")}</Text>
              </Pressable>
            ) : (
              <>
                <FocusableCard
                  hasTVPreferredFocus
                  onPress={togglePlayPause}
                  accessibilityLabel={isPlaying ? t("player.controls.pause") : t("player.controls.play")}
                  style={styles.actionBtn}
                >
                  <Text style={styles.actionLabel}>
                    {isPlaying ? t("player.controls.pause") : t("player.controls.play")}
                  </Text>
                </FocusableCard>
                {audioStreams.length > 1 && (
                  <FocusableCard
                    onPress={() => setShowAudioPicker(true)}
                    accessibilityLabel={t("player.settings.audio")}
                    style={styles.actionBtn}
                  >
                    <Text style={styles.actionLabel}>{t("player.settings.audio")}</Text>
                  </FocusableCard>
                )}
                {subtitleStreams.length > 0 && (
                  <FocusableCard
                    onPress={() => setShowSubtitlePicker(true)}
                    accessibilityLabel={t("player.settings.subtitles")}
                    style={styles.actionBtn}
                  >
                    <Text style={styles.actionLabel}>{t("player.settings.subtitles")}</Text>
                  </FocusableCard>
                )}
                <FocusableCard
                  onPress={() => setShowTrackMenu(true)}
                  accessibilityLabel={t("player.controls.more", "More")}
                  style={styles.actionBtn}
                >
                  <Text style={styles.actionLabel}>{t("player.controls.more", "More")}</Text>
                </FocusableCard>
              </>
            )}
          </View>

          {/* Bottom bar */}
          <View style={styles.bottomBar}>
            <Text style={styles.timeText}>{formatTime(displayTime)}</Text>
            <View style={styles.seekBarTrack}>
              <View style={[styles.seekBarFill, { flex: seekProgress }]} />
              <View style={{ flex: 1 - seekProgress }} />
              {duration > 0 &&
                chapters.map((ch, i) => {
                  const pct = (ch.startTimeMs / 1000 / duration) * 100;
                  if (pct <= 0 || pct >= 100) return null;
                  return <View key={i} style={[styles.chapterNotch, { left: `${pct}%` }]} />;
                })}
            </View>
            <Text style={styles.timeText}>{formatTime(duration)}</Text>
          </View>
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  video: {},
  overlay: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: "rgba(0,0,0,0.4)",
    justifyContent: "space-between",
  },
  actionBar: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 16,
    paddingHorizontal: 40,
    marginBottom: 12,
    minHeight: 56,
  },
  actionHint: {
    color: "rgba(255,255,255,0.75)",
    fontSize: 18,
  },
  actionBtn: {
    paddingVertical: 12,
    paddingHorizontal: 28,
    borderRadius: 8,
    backgroundColor: "rgba(255,255,255,0.12)",
  },
  actionLabel: {
    color: "#fff",
    fontSize: 20,
    fontWeight: "600",
  },
  topBar: {
    paddingTop: 24,
    paddingHorizontal: 40,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  mediaTitle: {
    color: "#fff",
    fontSize: 24,
    fontWeight: "600",
    flex: 1,
  },
  badgeRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    marginLeft: 16,
  },
  speedBadge: {
    backgroundColor: "rgba(255,255,255,0.2)",
    paddingHorizontal: 12,
    paddingVertical: 4,
    borderRadius: 12,
  },
  speedText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "700",
  },
  centerControls: {
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
    gap: 80,
  },
  seekHint: {
    width: 64,
    height: 64,
    borderRadius: 32,
    backgroundColor: "rgba(255,255,255,0.15)",
    justifyContent: "center",
    alignItems: "center",
  },
  seekHintText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
  },
  playIndicator: {
    width: 80,
    height: 80,
    borderRadius: 40,
    backgroundColor: "rgba(255,255,255,0.25)",
    justifyContent: "center",
    alignItems: "center",
  },
  playIcon: {
    color: "#fff",
    fontSize: 28,
    fontWeight: "bold",
  },
  episodeHint: {
    width: 48,
    height: 48,
    borderRadius: 24,
    backgroundColor: "rgba(255,255,255,0.1)",
    justifyContent: "center",
    alignItems: "center",
  },
  episodeHintText: {
    color: "rgba(255,255,255,0.85)",
    fontSize: 20,
  },
  trickplayWrap: {
    alignItems: "center",
    marginBottom: 12,
  },
  bottomBar: {
    flexDirection: "row",
    alignItems: "center",
    paddingBottom: 32,
    paddingHorizontal: 40,
    gap: 16,
  },
  timeText: {
    color: "#fff",
    fontSize: 18,
    fontVariant: ["tabular-nums"],
    minWidth: 70,
    textAlign: "center",
  },
  seekBarTrack: {
    flex: 1,
    height: 6,
    borderRadius: 3,
    backgroundColor: "rgba(255,255,255,0.3)",
    flexDirection: "row",
    overflow: "hidden",
  },
  seekBarFill: {
    backgroundColor: "#fff",
  },
  chapterNotch: {
    position: "absolute",
    top: 0,
    bottom: 0,
    width: 2,
    backgroundColor: "rgba(0,0,0,0.5)",
  },
});
