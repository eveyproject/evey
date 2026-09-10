import { useTranslation } from "react-i18next";
import { formatAudioTrackLabel, type TrackPick, type V1MediaStream } from "@evey/api";
import { Select } from "@/components/ui/Select";

interface MediaDetailTracksProps {
  streams: V1MediaStream[];
  pick: TrackPick;
  onChange: (pick: TrackPick) => void;
}

const AUTO = "auto";
const OFF = "off";

export function subtitleTrackLabel(s: V1MediaStream, fallback: string, forced: string): string {
  const name = s.title ?? s.language ?? fallback;
  return s.isForced ? `${name} - ${forced}` : name;
}

// The choice made here rides to the player as route params (see trackPick.ts).
// "Auto" is the player's own resolution, so an untouched page plays exactly
// what it always did; the selects only exist to say otherwise before the
// stream starts, instead of switching tracks mid-film.
export function MediaDetailTracks({ streams, pick, onChange }: MediaDetailTracksProps) {
  const { t } = useTranslation();
  const audio = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO" && s.index !== undefined);
  const subtitles = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE" && s.index !== undefined);
  if (audio.length < 2 && subtitles.length === 0) return null;

  const audioOptions = [
    { value: AUTO, label: t("media.streams.auto") },
    ...audio.map((s) => ({ value: String(s.index), label: formatAudioTrackLabel(s, t("media.track", { number: s.index })) })),
  ];
  const subtitleOptions = [
    { value: AUTO, label: t("media.streams.auto") },
    { value: OFF, label: t("media.streams.none") },
    ...subtitles.map((s) => ({
      value: String(s.index),
      label: subtitleTrackLabel(s, t("media.track", { number: s.index }), t("media.streams.forced")),
    })),
  ];

  return (
    <div className="flex flex-wrap gap-4 lg:col-span-2" data-testid="media-tracks">
      {audio.length > 1 && (
        <div className="min-w-[14rem]">
          <Select
            label={t("media.streams.audio")}
            data-testid="media-tracks-audio"
            options={audioOptions}
            value={pick.audio === undefined ? AUTO : String(pick.audio)}
            onChange={(v) => onChange({ ...pick, audio: v === AUTO ? undefined : Number(v) })}
          />
        </div>
      )}
      {subtitles.length > 0 && (
        <div className="min-w-[14rem]">
          <Select
            label={t("media.streams.subtitles")}
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
