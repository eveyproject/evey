import { useTranslation } from "react-i18next";
import { formatAudioTrackLabel, type TrackPick, type V1MediaStream, trackDisplayName } from "@evey/api";
import type { ReactNode } from "react";
import { Captions, Volume2 } from "lucide-react";
import { Select } from "@/components/ui/Select";
import { ICON } from "@/constants/iconSizes";

interface MediaDetailTracksProps {
  streams: V1MediaStream[];
  pick: TrackPick;
  onChange: (pick: TrackPick) => void;
}

const AUTO = "auto";
const OFF = "off";

function TrackLabel({ htmlFor, icon, text }: { htmlFor: string; icon: ReactNode; text: string }) {
  return (
    <label htmlFor={htmlFor} className="flex items-center gap-1.5 text-sm font-semibold text-[var(--text-secondary)]">
      <span className="text-[var(--accent)]" aria-hidden="true">{icon}</span>
      {text}
    </label>
  );
}

export function subtitleTrackLabel(s: V1MediaStream, fallback: string, forced: string, locale?: string): string {
  const name = trackDisplayName(s, fallback, locale);
  return s.isForced ? `${name} - ${forced}` : name;
}

// The choice made here rides to the player as route params (see trackPick.ts).
// "Auto" is the player's own resolution, so an untouched page plays exactly
// what it always did; the selects only exist to say otherwise before the
// stream starts, instead of switching tracks mid-film.
export function MediaDetailTracks({ streams, pick, onChange }: MediaDetailTracksProps) {
  const { t, i18n } = useTranslation();
  const audio = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO" && s.index !== undefined);
  const subtitles = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE" && s.index !== undefined);
  if (audio.length < 2 && subtitles.length === 0) return null;

  const audioOptions = [
    { value: AUTO, label: t("media.streams.auto") },
    ...audio.map((s) => ({ value: String(s.index), label: formatAudioTrackLabel(s, t("media.track", { number: s.index }), i18n.language) })),
  ];
  const subtitleOptions = [
    { value: AUTO, label: t("media.streams.auto") },
    { value: OFF, label: t("media.streams.none") },
    ...subtitles.map((s) => ({
      value: String(s.index),
      label: subtitleTrackLabel(s, t("media.track", { number: s.index }), t("media.streams.forced"), i18n.language),
    })),
  ];

  // Own label with an icon: the bare grey Select label read as a form field
  // under the action row, and users missed the picker entirely (2026-10-06).
  return (
    <div className="flex flex-wrap gap-4 lg:col-span-2" data-testid="media-tracks">
      {audio.length > 1 && (
        <div className="min-w-[14rem] flex flex-col gap-1">
          <TrackLabel htmlFor="media-tracks-audio" icon={<Volume2 size={ICON.sm} />} text={t("media.streams.audio")} />
          <Select
            id="media-tracks-audio"
            data-testid="media-tracks-audio"
            options={audioOptions}
            value={pick.audio === undefined ? AUTO : String(pick.audio)}
            onChange={(v) => onChange({ ...pick, audio: v === AUTO ? undefined : Number(v) })}
          />
        </div>
      )}
      {subtitles.length > 0 && (
        <div className="min-w-[14rem] flex flex-col gap-1">
          <TrackLabel htmlFor="media-tracks-subtitle" icon={<Captions size={ICON.sm} />} text={t("media.streams.subtitles")} />
          <Select
            id="media-tracks-subtitle"
            data-testid="media-tracks-subtitle"
            options={subtitleOptions}
            value={pick.subtitle === undefined ? AUTO : String(pick.subtitle)}
            onChange={(v) => onChange({ ...pick, subtitle: v === AUTO ? undefined : v === OFF ? OFF : Number(v) })}
          />
        </div>
      )}
    </div>
  );
}
