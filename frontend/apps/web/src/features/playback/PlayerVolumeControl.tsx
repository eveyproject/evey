import { VolumeX, Volume1, Volume2 } from "lucide-react";
import { useTranslation } from "react-i18next";

export interface PlayerVolumeControlProps {
  volume: number;
  muted: boolean;
  onToggleMute: () => void;
  onVolumeChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
}

export function PlayerVolumeControl({ volume, muted, onToggleMute, onVolumeChange }: PlayerVolumeControlProps) {
  const { t } = useTranslation();

  return (
    <div className="flex items-center gap-1">
      <button
        onClick={onToggleMute}
        aria-label={muted || volume === 0 ? t("player.controls.unmute") : t("player.controls.mute")}
        title={muted || volume === 0 ? `${t("player.controls.unmute")} (M)` : `${t("player.controls.mute")} (M)`}
        className="text-white hover:text-[var(--accent)] transition-colors-fast p-1"
      >
        {muted || volume === 0 ? (
          <VolumeX size={18} />
        ) : volume < 0.5 ? (
          <Volume1 size={18} />
        ) : (
          <Volume2 size={18} />
        )}
      </button>
      <input
        type="range"
        min={0}
        max={1}
        step={0.05}
        value={muted ? 0 : volume}
        onChange={onVolumeChange}
        className="w-20 accent-[var(--accent)] cursor-pointer hidden sm:block"
        aria-label={t("player.controls.volume")}
      />
    </div>
  );
}
