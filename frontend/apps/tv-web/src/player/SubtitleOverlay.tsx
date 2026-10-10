import { memo } from "react";
import { useTranslation } from "react-i18next";
import { findActiveCueIndex, findActivePGSCue } from "@evey/api";
import {
  DEFAULT_SUBTITLE_STYLE,
  subtitleBgRgba,
  subtitleCueFontSizePx,
  useSubtitleTrack,
  type SubtitleStyle,
} from "@evey/client";

interface Props {
  mediaId: string;
  // Subtitle-relative ordinal the server addresses (/subtitle/{track}), or
  // null for none. Text and PGS tracks are both fetched and drawn here; a
  // bitmap the player cannot draw is burnt in server side instead.
  subtitleIndex: number | null;
  currentTimeSec: number;
  /** The appearance the viewer chose, as on the Android television. */
  style?: SubtitleStyle;
}

function SubtitleOverlayImpl({ mediaId, subtitleIndex, currentTimeSec, style }: Props) {
  // The fetch and the text/bitmap sniff live in @evey/client, shared with the
  // Android television; only the painting below is the DOM's.
  const { cues, pgs, preparing } = useSubtitleTrack(mediaId, subtitleIndex, currentTimeSec);
  const { t } = useTranslation();

  // The server is still producing the track (202 + Retry-After): said in the
  // subtitle's place rather than a menu entry that shows nothing for minutes.
  if (preparing && !pgs && cues.length === 0) {
    return <div className="subs-text">{t("player.subtitlesPreparing")}</div>;
  }

  if (pgs) {
    const { track, uris } = pgs;
    const idx = findActivePGSCue(track.cues, currentTimeSec * 1000);
    if (idx === -1 || track.screenW <= 0 || track.screenH <= 0) return null;
    const cue = track.cues[idx];
    // Positioned in percent of the source frame: the picture is letterboxed
    // by object-fit, so the overlay covers the video box, not the screen.
    return (
      <div className="subs-pgs">
        <img
          src={uris[idx]}
          alt=""
          style={{
            left: `${(cue.x / track.screenW) * 100}%`,
            top: `${(cue.y / track.screenH) * 100}%`,
            width: `${(cue.w / track.screenW) * 100}%`,
            height: `${(cue.h / track.screenH) * 100}%`,
          }}
        />
      </div>
    );
  }

  const active = findActiveCueIndex(cues, currentTimeSec);
  if (active === -1) return null;
  const chosen = style ?? DEFAULT_SUBTITLE_STYLE;
  return (
    <div
      className="subs-text"
      style={{
        color: chosen.color,
        fontSize: `${subtitleCueFontSizePx(window.innerHeight, chosen.fontSize)}px`,
        background: subtitleBgRgba(chosen.bgColor, chosen.bgOpacity),
        textShadow: chosen.shadow ? "0 0.144rem 0.384rem #000" : undefined,
        ...(chosen.position === "top" ? { top: "8%", bottom: "auto" } : {}),
      }}
    >
      {cues[active].text.split("\n").map((line, i) => <span key={i}>{line}</span>)}
    </div>
  );
}

export const SubtitleOverlay = memo(SubtitleOverlayImpl);
