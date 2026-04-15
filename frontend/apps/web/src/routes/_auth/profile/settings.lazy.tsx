import { Palette, Bell, Shield, Smartphone, QrCode, Pencil, X } from "lucide-react";
import type React from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useState, useEffect, useRef } from "react";
import { useUserServiceGetCurrentUser, useUserServiceUpdateUser, useUserServiceListProfiles, useUserServiceUpdateProfile } from "@evey/api";
import { getUserServiceGetCurrentUserQueryKey, getUserServiceListProfilesQueryKey } from "@evey/api";
import { eveyFetchRaw } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Select";
import { ThemeSettings } from "@/features/user/ThemeSettings";
import { LanguageSettings } from "@/features/user/LanguageSettings";
import { NotificationSettings } from "@/features/user/NotificationSettings";
import { DeviceSettings } from "@/features/user/DeviceSettings";
import { QuickConnectSettings } from "@/features/user/QuickConnectSettings";

export const Route = createLazyFileRoute("/_auth/profile/settings")({
  component: SettingsPage,
});

type Tab = "appearance" | "notifications" | "parental" | "devices" | "quick-connect";

const TABS: { key: Tab; labelKey: string; icon: React.ReactNode }[] = [
  { key: "appearance", labelKey: "auth.settings.tabs.appearance", icon: <Palette size={16} strokeWidth={1.5} /> },
  { key: "notifications", labelKey: "auth.settings.tabs.notifications", icon: <Bell size={16} strokeWidth={1.5} /> },
  { key: "parental", labelKey: "auth.settings.tabs.parental", icon: <Shield size={16} strokeWidth={1.5} /> },
  { key: "devices", labelKey: "auth.settings.tabs.devices", icon: <Smartphone size={16} strokeWidth={1.5} /> },
  { key: "quick-connect", labelKey: "auth.settings.tabs.quickConnect", icon: <QrCode size={16} strokeWidth={1.5} /> },
];

function ProfileCard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const defaultProfile = profilesResp?.profiles?.find((p) => p.isDefault) ?? profilesResp?.profiles?.[0];

  const [editing, setEditing] = useState(false);
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPwd, setConfirmPwd] = useState("");
  const [avatarPreview, setAvatarPreview] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const updateUser = useUserServiceUpdateUser({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceGetCurrentUserQueryKey() });
        setEditing(false);
        setPassword("");
        setConfirmPwd("");
        toast.success(t("auth.profile.profileUpdated"));
      },
      onError: (err: Error) => toast.error(err.message || t("auth.profile.failedToUpdate")),
    },
  });

  const startEdit = () => {
    setUsername(user?.username ?? "");
    setEmail(user?.email ?? "");
    setPassword("");
    setConfirmPwd("");
    setEditing(true);
  };

  const handleSave = () => {
    if (!user?.id) return;
    const data: Record<string, string> = {};
    if (username.trim() !== user.username) data.username = username.trim();
    if (email.trim() !== (user.email ?? "")) data.email = email.trim();
    if (password || confirmPwd) {
      if (password !== confirmPwd) { toast.error(t("auth.settings.account.passwordMismatch")); return; }
      if (password.length < 8) { toast.error(t("auth.settings.account.passwordMin")); return; }
      data.password = password;
    }
    if (Object.keys(data).length === 0) { setEditing(false); return; }
    updateUser.mutate({ id: user.id, data });
  };

  const handleAvatarClick = () => fileInputRef.current?.click();

  const handleAvatarUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file || !defaultProfile?.id) return;
    const formData = new FormData();
    formData.append("avatar", file);
    try {
      await eveyFetchRaw({ url: `/api/v1/profiles/${defaultProfile.id}/avatar`, method: "POST", body: formData });
      setAvatarPreview(URL.createObjectURL(file));
      if (user?.id) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
      toast.success(t("auth.profile.avatarUpdated"));
    } catch {
      toast.error(t("auth.profile.avatarFailed"));
    }
  };

  const initial = user?.username?.charAt(0).toUpperCase() ?? "U";
  const memberSince = user?.createdAt
    ? new Date(user.createdAt).toLocaleDateString(undefined, { year: "numeric", month: "long" })
    : null;

  const avatarUrl = avatarPreview
    ? avatarPreview
    : defaultProfile?.avatarPath && defaultProfile?.id
      ? `/api/v1/profiles/${defaultProfile.id}/avatar`
      : null;

  return (
    <Card>
      <div className="p-6 flex flex-col gap-4">
        <div className="flex items-center gap-4">
          <button
            onClick={handleAvatarClick}
            className="w-14 h-14 rounded-full bg-[var(--accent)] flex items-center justify-center text-[var(--filter-active-text)] text-xl font-bold shrink-0 relative group cursor-pointer overflow-hidden"
            aria-label={t("auth.profile.changeAvatar")}
          >
            {avatarUrl ? (
              <img src={avatarUrl} alt="" className="w-full h-full object-cover" />
            ) : (
              initial
            )}
            <div className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
              <Pencil size={16} className="text-white"  aria-hidden="true" />
            </div>
          </button>
          <input ref={fileInputRef} type="file" accept="image/*" className="hidden" onChange={handleAvatarUpload} />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <p className="text-lg font-semibold text-[var(--text)] truncate">{user?.username ?? "..."}</p>
              {user?.isAdmin && (
                <span className="text-xs bg-[var(--accent)] text-[var(--filter-active-text)] px-2 py-0.5 rounded-[var(--radius-sm)]">Admin</span>
              )}
            </div>
            <p className="text-sm text-[var(--text-muted)] truncate">{user?.email || "—"}</p>
            {memberSince && <p className="text-xs text-[var(--text-muted)] mt-0.5">{memberSince}</p>}
          </div>
          <button
            onClick={editing ? () => setEditing(false) : startEdit}
            className="p-2 rounded-[var(--radius-sm)] text-[var(--text-muted)] hover:text-[var(--accent)] hover:bg-[var(--surface-raised)] transition-colors-fast"
            aria-label={editing ? "Cancel" : "Edit profile"}
          >
            {editing ? <X size={18}  aria-hidden="true" /> : <Pencil size={18}  aria-hidden="true" />}
          </button>
        </div>

        {editing && (
          <div className="flex flex-col gap-3 pt-4 border-t border-[var(--border)]">
            <Input label={t("auth.profile.username")} value={username} onChange={(e) => setUsername(e.target.value)} />
            <Input label={t("auth.profile.email")} value={email} onChange={(e) => setEmail(e.target.value)} />
            <div className="border-t border-[var(--border)] pt-3 mt-1">
              <p className="text-xs text-[var(--text-muted)] mb-2">{t("auth.settings.account.changePasswordDesc")}</p>
              <div className="flex flex-col gap-3">
                <Input
                  label={t("auth.settings.account.newPassword")}
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
                <Input
                  label={t("auth.settings.account.confirmPassword")}
                  type="password"
                  autoComplete="new-password"
                  value={confirmPwd}
                  onChange={(e) => setConfirmPwd(e.target.value)}
                />
              </div>
            </div>
            <div className="flex gap-2 pt-1">
              <Button variant="primary" loading={updateUser.isPending} onClick={handleSave}>{t("auth.profile.save")}</Button>
              <Button variant="ghost" onClick={() => setEditing(false)}>{t("auth.profile.cancel")}</Button>
            </div>
          </div>
        )}
      </div>
    </Card>
  );
}

function SettingsPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>("appearance");

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">{t("auth.settings.title")}</h1>

      <ProfileCard />

      <div className="flex gap-1 border-b border-[var(--border)]">
        {TABS.map((tb) => (
          <button
            key={tb.key}
            onClick={() => setTab(tb.key)}
            className={`flex items-center gap-2 px-4 py-2 text-sm transition-colors-fast border-b-2 -mb-px ${
              tab === tb.key
                ? "border-[var(--accent)] text-[var(--accent)]"
                : "border-transparent text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            {tb.icon}
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
          {tab === "notifications" && <NotificationSettings />}
          {tab === "parental" && <ParentalSection />}
          {tab === "devices" && <DeviceSettings />}
          {tab === "quick-connect" && <QuickConnectSettings />}
        </div>
      </Card>
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
          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors-fast focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 disabled:opacity-50 ${
            enabled ? "bg-[var(--accent)]" : "bg-[var(--border)]"
          }`}
        >
          <span
            className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform-fast ${
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
