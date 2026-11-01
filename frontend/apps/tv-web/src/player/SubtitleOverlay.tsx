import { memo } from "react";
import { useTranslation } from "react-i18next";
import {
  DEFAULT_SUBTITLE_STYLE,
  subtitleBgRgba,
  subtitleCueFontSizePx,
  useSubtitleCue,
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

// The fetch and the active-cue lookup (text or bitmap) live in @evey/client's
// useSubtitleCue, shared with the two Android players; only the painting below
// is the DOM's.
function SubtitleOverlayImpl({ mediaId, subtitleIndex, currentTimeSec, style }: Props) {
  const { preparing, text, pgs } = useSubtitleCue(mediaId, subtitleIndex, { currentTimeSec });
  const { t } = useTranslation();

  // The server is still producing the track (202 + Retry-After): said in the
  // subtitle's place rather than a menu entry that shows nothing for minutes.
  if (preparing) {
    return <div className="subs-text">{t("player.subtitlesPreparing")}</div>;
  }

  if (pgs) {
    // Positioned in percent of the source frame: the picture is letterboxed
    // by object-fit, so the overlay covers the video box, not the screen.
    return (
      <div className="subs-pgs">
        <img
          src={pgs.uri}
          alt=""
          style={{
            left: `${(pgs.cue.x / pgs.screenW) * 100}%`,
            top: `${(pgs.cue.y / pgs.screenH) * 100}%`,
            width: `${(pgs.cue.w / pgs.screenW) * 100}%`,
            height: `${(pgs.cue.h / pgs.screenH) * 100}%`,
          }}
        />
      </div>
    );
  }

  if (text === null) return null;
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
      {text.split("\n").map((line, i) => <span key={i}>{line}</span>)}
    </div>
  );
}

export const SubtitleOverlay = memo(SubtitleOverlayImpl);
