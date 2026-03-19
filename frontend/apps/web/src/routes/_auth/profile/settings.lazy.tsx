import { createLazyFileRoute } from "@tanstack/react-router";
import { useState, useEffect, useRef } from "react";
import { useUserServiceGetCurrentUser, useUserServiceUpdateUser, useUserServiceListProfiles, useUserServiceUpdateProfile } from "@evey/api";
import { getUserServiceGetCurrentUserQueryKey, getUserServiceListProfilesQueryKey } from "@evey/api";
import { eveyFetch } from "@evey/api";
import { useQueryClient, useMutation } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Select";
import { ThemeSettings } from "@/features/user/ThemeSettings";
import { LanguageSettings } from "@/features/user/LanguageSettings";
import { NotificationSettings } from "@/features/user/NotificationSettings";
import { APIKeySettings } from "@/features/user/APIKeySettings";
import { DeviceSettings } from "@/features/user/DeviceSettings";

export const Route = createLazyFileRoute("/_auth/profile/settings")({
  component: SettingsPage,
});

type Tab = "appearance" | "playback" | "notifications" | "account" | "parental" | "api-keys" | "devices";

const TABS: { key: Tab; labelKey: string; icon: string }[] = [
  {
    key: "appearance",
    labelKey: "auth.settings.tabs.appearance",
    icon: "M4.098 19.902a3.75 3.75 0 005.304 0l6.401-6.402M6.75 21A3.75 3.75 0 013 17.25V4.125C3 3.504 3.504 3 4.125 3h5.25c.621 0 1.125.504 1.125 1.125v4.072M6.75 21a3.75 3.75 0 003.75-3.75V8.197M6.75 21h13.125c.621 0 1.125-.504 1.125-1.125v-5.25c0-.621-.504-1.125-1.125-1.125h-4.072M10.5 8.197l2.88-2.88c.438-.439 1.15-.439 1.59 0l3.712 3.713c.44.44.44 1.152 0 1.59l-2.879 2.88M6.75 17.25h.008v.008H6.75v-.008z",
  },
  {
    key: "playback",
    labelKey: "auth.settings.tabs.playback",
    icon: "M5.25 5.653c0-.856.917-1.398 1.667-.986l11.54 6.347a1.125 1.125 0 010 1.972l-11.54 6.347a1.125 1.125 0 01-1.667-.986V5.653z",
  },
  {
    key: "notifications",
    labelKey: "auth.settings.tabs.notifications",
    icon: "M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0",
  },
  {
    key: "account",
    labelKey: "auth.settings.tabs.account",
    icon: "M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z",
  },
  {
    key: "parental",
    labelKey: "auth.settings.tabs.parental",
    icon: "M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z",
  },
  {
    key: "devices",
    labelKey: "auth.settings.tabs.devices",
    icon: "M10.5 1.5H8.25A2.25 2.25 0 006 3.75v16.5a2.25 2.25 0 002.25 2.25h7.5A2.25 2.25 0 0018 20.25V3.75a2.25 2.25 0 00-2.25-2.25H13.5m-3 0V3h3V1.5m-3 0h3m-3 8.25h3",
  },
  {
    key: "api-keys",
    labelKey: "auth.settings.tabs.apiKeys",
    icon: "M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z",
  },
];

function SettingsPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>("appearance");

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">{t("auth.settings.title")}</h1>

      <div className="flex gap-1 border-b border-[var(--border)]">
        {TABS.map((tb) => (
          <button
            key={tb.key}
            onClick={() => setTab(tb.key)}
            className={`flex items-center gap-2 px-4 py-2 text-sm transition-colors border-b-2 -mb-px ${
              tab === tb.key
                ? "border-[var(--accent)] text-[var(--accent)]"
                : "border-transparent text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" viewBox="0 0 24 24">
              <path d={tb.icon} />
            </svg>
            {t(tb.labelKey)}
          </button>
        ))}
      </div>

      <Card>
        <div className="p-6">
          {tab === "appearance" && (
            <>
              <ThemeSettings />
              <div className="border-t border-[var(--border)] mt-6 pt-6">
                <LanguageSettings />
              </div>
            </>
          )}
          {tab === "playback" && <PlaybackSection />}
          {tab === "notifications" && <NotificationSettings />}
          {tab === "account" && <AccountSection />}
          {tab === "parental" && <ParentalSection />}
          {tab === "devices" && <DeviceSettings />}
          {tab === "api-keys" && <APIKeySettings />}
        </div>
      </Card>
    </div>
  );
}

const BITRATE_OPTIONS = [
  { value: "0", label: "Unlimited" },
  { value: "2000", label: "2 Mbps" },
  { value: "4000", label: "4 Mbps" },
  { value: "8000", label: "8 Mbps" },
  { value: "10000", label: "10 Mbps" },
  { value: "15000", label: "15 Mbps" },
  { value: "20000", label: "20 Mbps" },
  { value: "40000", label: "40 Mbps" },
];

function PlaybackSection() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", {
    query: { enabled: !!user?.id },
  });

  const profiles = profilesResp?.profiles;
  const defaultProfile = profiles?.find((p) => p.isDefault) ?? profiles?.[0];

  const [bitrate, setBitrate] = useState("0");

  useEffect(() => {
    if (defaultProfile?.preferences?.max_bitrate_kbps) {
      setBitrate(defaultProfile.preferences.max_bitrate_kbps);
    }
  }, [defaultProfile]);

  const updateProfile = useMutation({
    mutationFn: (bitrateKbps: string) =>
      eveyFetch({
        url: `/api/v1/profiles/${defaultProfile?.id}`,
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        data: {
          preferences: {
            ...defaultProfile?.preferences,
            max_bitrate_kbps: bitrateKbps,
          },
        },
      }),
    onSuccess: () => {
      if (user?.id) {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
      }
      toast.success(t("auth.settings.playback.bitrateUpdated"));
    },
    onError: (err: Error) => toast.error(err.message || t("auth.settings.playback.bitrateFailed")),
  });

  const handleChange = (value: string) => {
    setBitrate(value);
    updateProfile.mutate(value);
  };

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.playback.bitrateTitle")}</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">
          {t("auth.settings.playback.bitrateDesc")}
        </p>
      </div>
      <Select
        label={t("auth.settings.playback.maxBitrate")}
        options={BITRATE_OPTIONS}
        value={bitrate}
        onChange={handleChange}
        disabled={!defaultProfile}
      />
    </div>
  );
}

function AccountSection() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();

  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");

  const updateUser = useUserServiceUpdateUser({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceGetCurrentUserQueryKey() });
        setPassword("");
        setConfirm("");
        setError("");
        toast.success(t("auth.settings.account.passwordUpdated"));
      },
      onError: (err: Error) => toast.error(err.message || t("auth.settings.account.passwordFailed")),
    },
  });

  const handleSubmit = () => {
    if (!user?.id) return;
    if (password !== confirm) {
      setError(t("auth.settings.account.passwordMismatch"));
      return;
    }
    if (password.length < 8) {
      setError(t("auth.settings.account.passwordMin"));
      return;
    }
    setError("");
    updateUser.mutate({ id: user.id, data: { password } });
  };

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.account.changePassword")}</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">{t("auth.settings.account.changePasswordDesc")}</p>
      </div>
      <div className="flex flex-col gap-3">
        <Input
          label={t("auth.settings.account.newPassword")}
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          error={error}
        />
        <Input
          label={t("auth.settings.account.confirmPassword")}
          type="password"
          autoComplete="new-password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
        />
        <div>
          <Button
            variant="primary"
            loading={updateUser.isPending}
            onClick={handleSubmit}
          >
            {t("auth.settings.account.updatePassword")}
          </Button>
        </div>
      </div>
    </div>
  );
}

const RATING_OPTIONS = [
  { value: "", label: "No restriction" },
  { value: "G", label: "G — General Audiences" },
  { value: "PG", label: "PG — Parental Guidance" },
  { value: "PG-13", label: "PG-13 — Parents Strongly Cautioned" },
  { value: "R", label: "R — Restricted" },
  { value: "NC-17", label: "NC-17 — Adults Only" },
];

function PinInput({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <Input
      label={label}
      type="password"
      inputMode="numeric"
      maxLength={4}
      pattern="\d{4}"
      placeholder="••••"
      value={value}
      onChange={(e) => {
        const v = e.target.value.replace(/\D/g, "").slice(0, 4);
        onChange(v);
      }}
      autoComplete="off"
    />
  );
}

function ParentalSection() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", {
    query: { enabled: !!user?.id },
  });

  const profiles = profilesResp?.profiles;
  const defaultProfile = profiles?.find((p) => p.isDefault) ?? profiles?.[0];

  const enabled = defaultProfile?.preferences?.parental_enabled === "true";
  const [maxRating, setMaxRating] = useState(defaultProfile?.maxRating ?? "");
  const [pin, setPin] = useState("");
  const [confirmPin, setConfirmPin] = useState("");
  const [pinError, setPinError] = useState("");
  const pendingOpRef = useRef<"pin" | "other">("other");

  useEffect(() => {
    setMaxRating(defaultProfile?.maxRating ?? "");
  }, [defaultProfile]);

  const updateProfile = useUserServiceUpdateProfile({
    mutation: {
      onSuccess: () => {
        if (user?.id) {
          void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
        }
        if (pendingOpRef.current === "pin") {
          toast.success(t("auth.settings.parental.pin.saved"));
          setPin("");
          setConfirmPin("");
        } else {
          toast.success(t("auth.settings.parental.updated"));
        }
        pendingOpRef.current = "other";
      },
      onError: (err: Error) => toast.error(err.message || t("auth.settings.parental.failedToUpdate")),
    },
  });

  function handleToggleEnabled() {
    if (!defaultProfile?.id) return;
    updateProfile.mutate({
      id: defaultProfile.id,
      data: {
        preferences: {
          ...defaultProfile.preferences,
          parental_enabled: enabled ? "false" : "true",
        },
        maxRating: enabled ? undefined : (maxRating || undefined),
      },
    });
  }

  function handleSaveRating() {
    if (!defaultProfile?.id) return;
    updateProfile.mutate({
      id: defaultProfile.id,
      data: { maxRating: maxRating || undefined },
    });
  }

  function handleSavePin() {
    setPinError("");
    if (pin.length !== 4) {
      setPinError(t("auth.settings.parental.pin.mustBe4"));
      return;
    }
    if (pin !== confirmPin) {
      setPinError(t("auth.settings.parental.pin.mismatch"));
      return;
    }
    if (!defaultProfile?.id) return;
    pendingOpRef.current = "pin";
    updateProfile.mutate({
      id: defaultProfile.id,
      data: {
        preferences: {
          ...defaultProfile.preferences,
          parental_pin: pin,
        },
      },
    });
  }

  if (!defaultProfile) {
    return (
      <p className="text-sm text-[var(--text-muted)]">{t("auth.settings.parental.noProfile")}</p>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.parental.title")}</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">
          {t("auth.settings.parental.description")}
        </p>
      </div>

      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium text-[var(--text)]">{t("auth.settings.parental.enable")}</p>
          <p className="text-xs text-[var(--text-muted)]">{t("auth.settings.parental.enableDesc")}</p>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={enabled}
          onClick={handleToggleEnabled}
          disabled={updateProfile.isPending}
          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 disabled:opacity-50 ${
            enabled ? "bg-[var(--accent)]" : "bg-[var(--border)]"
          }`}
        >
          <span
            className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
              enabled ? "translate-x-6" : "translate-x-1"
            }`}
          />
        </button>
      </div>

      <div className="flex flex-col gap-3">
        <Select
          label={t("auth.settings.parental.maxRating")}
          options={RATING_OPTIONS}
          value={maxRating}
          onChange={setMaxRating}
          disabled={!enabled}
        />
        <div>
          <Button
            variant="primary"
            loading={updateProfile.isPending}
            disabled={!enabled || maxRating === (defaultProfile.maxRating ?? "")}
            onClick={handleSaveRating}
          >
            {t("auth.settings.parental.saveRating")}
          </Button>
        </div>
      </div>

      <div className="border-t border-[var(--border)] pt-4 flex flex-col gap-3">
        <div>
          <h4 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.parental.pin.title")}</h4>
          <p className="text-xs text-[var(--text-muted)] mt-1">{t("auth.settings.parental.pin.description")}</p>
        </div>
        <PinInput label={t("auth.settings.parental.pin.newPin")} value={pin} onChange={setPin} />
        <PinInput label={t("auth.settings.parental.pin.confirmPin")} value={confirmPin} onChange={setConfirmPin} />
        {pinError && <p className="text-xs" style={{ color: "var(--color-danger)" }}>{pinError}</p>}
        <div>
          <Button
            variant="primary"
            loading={updateProfile.isPending}
            disabled={!enabled || pin.length < 4}
            onClick={handleSavePin}
          >
            {t("auth.settings.parental.pin.savePin")}
          </Button>
        </div>
      </div>
    </div>
  );
}
