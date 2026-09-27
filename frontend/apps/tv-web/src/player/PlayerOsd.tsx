import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { formatTime } from "@evey/api";
import type { Chapter } from "@evey/client";
import { Icon } from "../components/icons";
import { TrickplayTile } from "./TrickplayTile";

export const BAR_BUTTONS = ["rewind", "play", "forward", "speed", "settings"] as const;
export type BarFocus = (typeof BAR_BUTTONS)[number] | "timeline";

// The bar over the picture: the title and its badges at the top, the buttons,
// the trickplay tile and the timeline at the bottom. The remote drives it
// through the screen's key handler; `barFocus` only says which one is lit.
export function PlayerOsd({
  mediaId,
  title,
  roomId,
  syncConnected,
  quality,
  shown,
  duration,
  preview,
  pendingDelta,
  endsAt,
  paused,
  speed,
  skipBack,
  skipFwd,
  chapters,
  barFocus,
  onPress,
}: {
  mediaId: string;
  title: string;
  roomId?: string;
  syncConnected: boolean;
  quality: number;
  // The preview position while a seek is pending, else the playhead.
  shown: number;
  duration: number;
  preview: number | null;
  pendingDelta: number;
  endsAt: string | null;
  paused: boolean;
  speed: number;
  skipBack: number;
  skipFwd: number;
  chapters: Chapter[] | undefined;
  barFocus: BarFocus | null;
  onPress: (key: BarFocus) => void;
}) {
  const { t } = useTranslation();
  const pct = duration > 0 ? Math.min(100, (shown / duration) * 100) : 0;

  const control = (key: BarFocus, label: string, content: ReactNode, size: "small" | "normal" | "primary" = "normal") => (
    <button
      type="button"
      tabIndex={-1}
      aria-label={label}
      className={`ctl ${size} ${barFocus === key ? "focused" : ""}`}
      onClick={() => onPress(key)}
    >
      {content}
    </button>
  );

  return (
    <div className="osd-layer">
      <div className="osd-topbar">
        <span className="osd-title">{title}</span>
        <span className="osd-badges">
          {roomId ? (
            <span className={`osd-room ${syncConnected ? "on" : ""}`}>
              {syncConnected ? t("media.syncplay.connected", "Connected") : t("media.syncplay.join", "Join a room")}
            </span>
          ) : null}
          {quality > 0 ? <span className="osd-badge">{quality}p</span> : null}
        </span>
      </div>
      <div className="osd-panel">
        <div className="osd-trickplay" style={{ left: `calc(4rem + 7rem + 1.6rem + (100% - 8rem - 14rem - 3.2rem) * ${pct / 100})` }}>
          <TrickplayTile mediaId={mediaId} previewTimeSec={preview} warm />
        </div>
        <div className="osd-controls">
          <div className="osd-group">
            {control("rewind", t("player.controls.rewind", { seconds: skipBack }), `-${skipBack}s`)}
            {control("play", paused ? t("player.controls.play") : t("player.controls.pause"), <Icon name={paused ? "play" : "pause"} size={40} />, "primary")}
            {control("forward", t("player.controls.forward", { seconds: skipFwd }), `+${skipFwd}s`)}
          </div>
          <div className="osd-group">
            {endsAt ? <span className="osd-ends">{t("player.controls.endsAt", { time: endsAt, defaultValue: `Ends at ${endsAt}` })}</span> : null}
            {control("speed", t("player.settings.speed", "Speed"), `${speed}x`, "small")}
            {control("settings", t("player.controls.more", "More"), <Icon name="settings" size={26} />, "small")}
          </div>
        </div>
        <div className="osd-timeline">
          <span className="osd-time">{formatTime(shown)}</span>
          {pendingDelta !== 0 ? <span className="osd-delta">{pendingDelta > 0 ? "+" : "-"}{formatTime(Math.abs(pendingDelta))}</span> : null}
          <div className={`osd-track-hit ${barFocus === "timeline" ? "focused" : ""}`}>
            <div className="osd-track">
              <div className="osd-fill" style={{ width: `${pct}%` }} />
              {duration > 0 && (chapters ?? []).map((c, i) => {
                const p = (c.startTimeMs / 1000 / duration) * 100;
                return p > 0 && p < 100 ? <i key={i} className="osd-notch" style={{ left: `${p}%` }} /> : null;
              })}
            </div>
            {barFocus === "timeline" ? <span className="osd-knob" style={{ left: `${pct}%` }} /> : null}
          </div>
          <span className="osd-time">{formatTime(duration)}</span>
        </div>
      </div>
    </div>
  );
}
