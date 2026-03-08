import { useState, useEffect } from "react";
import {
  useNotificationServiceGetNotificationSettings,
  useNotificationServiceUpdateNotificationSettings,
} from "@evey/api";
import { Button } from "@/components/ui/Button";

type NotificationLevel =
  | "NOTIFICATION_LEVEL_UNSPECIFIED"
  | "NOTIFICATION_LEVEL_DISABLED"
  | "NOTIFICATION_LEVEL_ALL"
  | "NOTIFICATION_LEVEL_RELEVANT"
  | "NOTIFICATION_LEVEL_HIGHLY_RELEVANT";

const LEVELS: NotificationLevel[] = [
  "NOTIFICATION_LEVEL_DISABLED",
  "NOTIFICATION_LEVEL_HIGHLY_RELEVANT",
  "NOTIFICATION_LEVEL_RELEVANT",
  "NOTIFICATION_LEVEL_ALL",
];

const LEVEL_LABELS: Record<NotificationLevel, string> = {
  NOTIFICATION_LEVEL_UNSPECIFIED: "Default",
  NOTIFICATION_LEVEL_DISABLED: "Disabled",
  NOTIFICATION_LEVEL_ALL: "All",
  NOTIFICATION_LEVEL_RELEVANT: "Relevant",
  NOTIFICATION_LEVEL_HIGHLY_RELEVANT: "Important only",
};

interface LevelSelectProps {
  label: string;
  value: NotificationLevel | undefined;
  onChange: (v: NotificationLevel) => void;
}

function LevelSelect({ label, value, onChange }: LevelSelectProps) {
  return (
    <div className="flex items-center justify-between py-3 border-b border-[var(--border)] last:border-0">
      <span className="text-sm text-[var(--text)]">{label}</span>
      <select
        value={value ?? "NOTIFICATION_LEVEL_UNSPECIFIED"}
        onChange={(e) => onChange(e.target.value as NotificationLevel)}
        className="text-sm bg-[var(--surface-raised)] border border-[var(--border)] rounded-[var(--radius-sm)] px-2 py-1 text-[var(--text)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
      >
        {LEVELS.map((l) => (
          <option key={l} value={l}>{LEVEL_LABELS[l]}</option>
        ))}
      </select>
    </div>
  );
}

export function NotificationSettings() {
  const { data: settings, isLoading } = useNotificationServiceGetNotificationSettings();

  const [movieLevel, setMovieLevel] = useState<NotificationLevel>("NOTIFICATION_LEVEL_UNSPECIFIED");
  const [seriesLevel, setSeriesLevel] = useState<NotificationLevel>("NOTIFICATION_LEVEL_UNSPECIFIED");
  const [musicLevel, setMusicLevel] = useState<NotificationLevel>("NOTIFICATION_LEVEL_UNSPECIFIED");

  useEffect(() => {
    if (settings) {
      if (settings.movieLevel) setMovieLevel(settings.movieLevel);
      if (settings.seriesLevel) setSeriesLevel(settings.seriesLevel);
      if (settings.musicLevel) setMusicLevel(settings.musicLevel);
    }
  }, [settings]);

  const update = useNotificationServiceUpdateNotificationSettings({
    mutation: { onSuccess: () => {} },
  });

  const handleSave = () => {
    update.mutate({
      data: { movieLevel, seriesLevel, musicLevel },
    });
  };

  if (isLoading) {
    return <p className="text-sm text-[var(--text-muted)]">Loading...</p>;
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="bg-[var(--surface-raised)] rounded-[var(--radius-md)] px-4">
        <LevelSelect label="Movies" value={movieLevel} onChange={setMovieLevel} />
        <LevelSelect label="Series" value={seriesLevel} onChange={setSeriesLevel} />
        <LevelSelect label="Music" value={musicLevel} onChange={setMusicLevel} />
      </div>
      <div>
        <Button
          variant="primary"
          loading={update.isPending}
          onClick={handleSave}
        >
          Save
        </Button>
      </div>
    </div>
  );
}
