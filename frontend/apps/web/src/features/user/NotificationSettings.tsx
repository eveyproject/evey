import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import {
  useNotificationServiceGetNotificationSettings,
  useNotificationServiceUpdateNotificationSettings,
} from "@evey/api";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Select";
import {
  isFCMConfigured,
  requestPushToken,
  revokePushToken,
  getStoredPushToken,
} from "@/lib/fcm";

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

const VAPID_KEY = import.meta.env.VITE_FIREBASE_VAPID_KEY;

interface LevelSelectProps {
  label: string;
  value: NotificationLevel | undefined;
  onChange: (v: NotificationLevel) => void;
}

function LevelSelect({ label, value, onChange }: LevelSelectProps) {
  return (
    <div className="flex items-center justify-between py-3 border-b border-[var(--border)] last:border-0">
      <span className="text-sm text-[var(--text)]">{label}</span>
      <Select
        value={value ?? "NOTIFICATION_LEVEL_UNSPECIFIED"}
        onChange={(v) => onChange(v as NotificationLevel)}
        options={LEVELS.map((l) => ({ value: l, label: LEVEL_LABELS[l] }))}
        className="w-auto"
      />
    </div>
  );
}

function PushNotificationSection() {
  const { t } = useTranslation("auth");
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    setEnabled(!!getStoredPushToken());
  }, []);

  if (!isFCMConfigured()) {
    return (
      <p className="text-xs text-[var(--text-muted)]">
        {t("notificationSettings.pushNotConfigured")}
      </p>
    );
  }

  const handleToggle = async () => {
    setLoading(true);
    try {
      if (!enabled) {
        const token = await requestPushToken(VAPID_KEY ?? "");
        if (!token) {
          toast.error(t("notificationSettings.permissionDenied"));
          return;
        }
        await eveyFetch({
          url: "/api/v1/push/register",
          method: "POST",
          headers: { "Content-Type": "application/json" },
          data: { token, platform: "web" },
        });
        setEnabled(true);
        toast.success(t("notificationSettings.pushEnabled"));
      } else {
        const token = getStoredPushToken();
        await revokePushToken();
        if (token) {
          await eveyFetch({
            url: "/api/v1/push/unregister",
            method: "DELETE",
            headers: { "Content-Type": "application/json" },
            data: { token },
          });
        }
        setEnabled(false);
        toast.success(t("notificationSettings.pushDisabled"));
      }
    } catch (err) {
      toast.error((err as Error).message || t("notificationSettings.pushError"));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex items-center justify-between py-3">
      <div>
        <p className="text-sm text-[var(--text)]">{t("notificationSettings.pushNotifications")}</p>
        <p className="text-xs text-[var(--text-muted)] mt-0.5">
          {t("notificationSettings.pushDesc")}
        </p>
      </div>
      <Button
        variant={enabled ? "secondary" : "primary"}
        loading={loading}
        onClick={handleToggle}
      >
        {enabled ? t("disable", { ns: "common" }) : t("enable", { ns: "common" })}
      </Button>
    </div>
  );
}

export function NotificationSettings() {
  const { t } = useTranslation("auth");
  const { t: tc } = useTranslation("common");
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
    mutation: {
      onSuccess: () => toast.success(t("notificationSettings.saved")),
      onError: (err: Error) => toast.error(err.message || t("notificationSettings.saveError")),
    },
  });

  const handleSave = () => {
    update.mutate({
      data: { movieLevel, seriesLevel, musicLevel },
    });
  };

  if (isLoading) {
    return <p className="text-sm text-[var(--text-muted)]">{tc("loading")}</p>;
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="bg-[var(--surface-raised)] rounded-[var(--radius-md)] px-4">
        <LevelSelect label={t("notificationSettings.movies")} value={movieLevel} onChange={setMovieLevel} />
        <LevelSelect label={t("notificationSettings.series")} value={seriesLevel} onChange={setSeriesLevel} />
        <LevelSelect label={t("notificationSettings.music")} value={musicLevel} onChange={setMusicLevel} />
      </div>
      <div>
        <Button
          variant="primary"
          loading={update.isPending}
          onClick={handleSave}
        >
          {tc("save")}
        </Button>
      </div>
      <div className="bg-[var(--surface-raised)] rounded-[var(--radius-md)] px-4">
        <PushNotificationSection />
      </div>
    </div>
  );
}
