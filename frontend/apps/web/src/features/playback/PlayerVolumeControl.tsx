import { VolumeX, Volume1, Volume2 } from "lucide-react";
import { useTranslation } from "react-i18next";

export interface PlayerVolumeControlProps {
  /** Continuous 0-2 level: 0-1 is native volume, 1-2 is amplification. */
  volume: number;
  muted: boolean;
  onToggleMute: () => void;
  onVolumeChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
}

const TRACK = "rgba(255,255,255,0.25)";
const ACCENT = "var(--accent)";
const BOOST = "var(--warning)"; // the >100% boost zone (semantic token, was hardcoded amber-500)

export function PlayerVolumeControl({ volume, muted, onToggleMute, onVolumeChange }: PlayerVolumeControlProps) {
  const { t } = useTranslation();

  const level = muted ? 0 : volume;
  const boosted = level > 1;
  const pct = Math.round(level * 100);
  const fill = (level / 2) * 100; // slider spans 0..2, so 100% sits at the middle

  // Two-tone fill: accent up to 100% (mid-track), amber from there into the
  // boost zone, neutral track for the remainder.
  const background = boosted
    ? `linear-gradient(to right, ${ACCENT} 0%, ${ACCENT} 50%, ${BOOST} 50%, ${BOOST} ${fill}%, ${TRACK} ${fill}%, ${TRACK} 100%)`
    : `linear-gradient(to right, ${ACCENT} 0%, ${ACCENT} ${fill}%, ${TRACK} ${fill}%, ${TRACK} 100%)`;

  return (
    <div className="flex items-center gap-1">
      <button
        onClick={onToggleMute}
        aria-label={muted || volume === 0 ? t("player.controls.unmute") : t("player.controls.mute")}
        title={muted || volume === 0 ? `${t("player.controls.unmute")} (M)` : `${t("player.controls.mute")} (M)`}
        className="text-white hover:text-[var(--accent)] transition-colors-fast p-1"
      >
        {muted || volume === 0 ? (
          <VolumeX size={18} aria-hidden="true" />
        ) : level < 0.5 ? (
          <Volume1 size={18} aria-hidden="true" />
        ) : (
          <Volume2 size={18} aria-hidden="true" />
        )}
      </button>
      <input
        type="range"
        min={0}
        max={2}
        step={0.05}
        value={level}
        onChange={onVolumeChange}
        style={{ background }}
        className={[
          "w-20 h-1.5 rounded-full appearance-none cursor-pointer hidden sm:block",
          "[&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-white [&::-webkit-slider-thumb]:cursor-pointer",
          "[&::-moz-range-thumb]:h-3 [&::-moz-range-thumb]:w-3 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:bg-white [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:cursor-pointer",
          "[&::-moz-range-track]:h-1.5 [&::-moz-range-track]:rounded-full [&::-moz-range-track]:bg-transparent",
        ].join(" ")}
        aria-label={t("player.controls.volume")}
        aria-valuetext={`${pct}%`}
      />
      {/* % readout - amber once past 100% to flag the amplification. */}
      <span
        className={[
          "hidden sm:block tabular-nums text-[11px] w-9 text-right select-none",
          boosted ? "text-amber-400 font-medium" : "text-white/60",
        ].join(" ")}
        aria-hidden="true"
      >
        {pct}%
      </span>
    </div>
  );
}
