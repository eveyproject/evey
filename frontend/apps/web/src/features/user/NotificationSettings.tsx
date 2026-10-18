import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import {
  useNotificationServiceGetNotificationSettings,
  useNotificationServiceUpdateNotificationSettings,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Select";
import {
  isPushSupported,
  isPushSubscribed,
  subscribeToPush,
  unsubscribeFromPush,
} from "@/lib/webpush";

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

const LEVEL_LABEL_KEYS: Record<NotificationLevel, string> = {
  NOTIFICATION_LEVEL_UNSPECIFIED: "auth.notificationSettings.levelDefault",
  NOTIFICATION_LEVEL_DISABLED: "auth.notificationSettings.levelDisabled",
  NOTIFICATION_LEVEL_ALL: "auth.notificationSettings.levelAll",
  NOTIFICATION_LEVEL_RELEVANT: "auth.notificationSettings.levelRelevant",
  NOTIFICATION_LEVEL_HIGHLY_RELEVANT: "auth.notificationSettings.levelImportant",
};

interface LevelSelectProps {
  label: string;
  value: NotificationLevel | undefined;
  onChange: (v: NotificationLevel) => void;
}

function LevelSelect({ label, value, onChange }: LevelSelectProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between py-3 border-b border-[var(--border)] last:border-0">
      <span className="text-sm text-[var(--text)]">{label}</span>
      <Select
        value={value ?? "NOTIFICATION_LEVEL_UNSPECIFIED"}
        onChange={(v) => onChange(v as NotificationLevel)}
        options={LEVELS.map((l) => ({ value: l, label: t(LEVEL_LABEL_KEYS[l]) }))}
        className="w-auto"
      />
    </div>
  );
}

function PushNotificationSection() {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(false);
  const [supported] = useState(() => isPushSupported());

  useEffect(() => {
    if (!supported) return;
    void (async () => {
      try {
        const subscribed = await isPushSubscribed();
        setEnabled(subscribed);
      } catch (err) { console.warn("push check failed", err); }
    })();
  }, [supported]);

  if (!supported) {
    // Browsers expose push only in a secure context (HTTPS or localhost): say
    // so instead of blaming the server.
    return (
      <p className="text-xs text-[var(--text-muted)]">
        {window.isSecureContext
          ? t("auth.notificationSettings.pushUnsupported")
          : t("auth.notificationSettings.pushNeedsHttps")}
      </p>
    );
  }

  const handleToggle = async () => {
    setLoading(true);
    try {
      if (!enabled) {
        const ok = await subscribeToPush();
        if (!ok) {
          toast.error(t("auth.notificationSettings.permissionDenied"));
          return;
        }
        setEnabled(true);
        toast.success(t("auth.notificationSettings.pushEnabled"));
      } else {
        await unsubscribeFromPush();
        setEnabled(false);
        toast.success(t("auth.notificationSettings.pushDisabled"));
      }
    } catch (err) {
      toast.error((err as Error).message || t("auth.notificationSettings.pushError"));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex items-center justify-between py-3">
      <div>
        <p className="text-sm text-[var(--text)]">{t("auth.notificationSettings.pushNotifications")}</p>
        <p className="text-xs text-[var(--text-muted)] mt-0.5">
          {t("auth.notificationSettings.pushDesc")}
        </p>
      </div>
      <Button
        variant={enabled ? "secondary" : "primary"}
        loading={loading}
        onClick={handleToggle}
      >
        {enabled ? t("common.disable") : t("common.enable")}
      </Button>
    </div>
  );
}

export function NotificationSettings() {
  const { t } = useTranslation();
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
      onSuccess: () => toast.success(t("auth.notificationSettings.saved")),
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("auth.notificationSettings.saveError"))),
    },
  });

  const handleSave = () => {
    update.mutate({
      data: { movieLevel, seriesLevel, musicLevel },
    });
  };

  if (isLoading) {
    return <p className="text-sm text-[var(--text-muted)]">{t("common.loading")}</p>;
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="bg-[var(--surface-raised)] rounded-[var(--radius-md)] px-4">
        <LevelSelect label={t("auth.notificationSettings.movies")} value={movieLevel} onChange={setMovieLevel} />
        <LevelSelect label={t("auth.notificationSettings.series")} value={seriesLevel} onChange={setSeriesLevel} />
        <LevelSelect label={t("auth.notificationSettings.music")} value={musicLevel} onChange={setMusicLevel} />
      </div>
      <div>
        <Button
          variant="primary"
          loading={update.isPending}
          onClick={handleSave}
        >
          {t("common.save")}
        </Button>
      </div>
      <div className="bg-[var(--surface-raised)] rounded-[var(--radius-md)] px-4">
        <PushNotificationSection />
      </div>
    </div>
  );
}
