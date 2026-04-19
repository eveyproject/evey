import { Palette, Bell, Shield, Smartphone, QrCode, Pencil, X } from "lucide-react";
import type React from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useState, useEffect, useRef } from "react";
import { useUserServiceGetCurrentUser, useUserServiceUpdateUser, useUserServiceListProfiles } from "@evey/api";
import { useParentalServiceGetParentalSettings, parentalServiceUpdateParentalSettings, getParentalServiceGetParentalSettingsQueryKey } from "@evey/api";
import { getUserServiceGetCurrentUserQueryKey, getUserServiceListProfilesQueryKey } from "@evey/api";
import { eveyFetchRaw } from "@evey/api";
import { useMutation } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Select";
import { ThemeSettings } from "@/features/user/ThemeSettings";
import { LanguageSettings } from "@/features/user/LanguageSettings";
import { RegionSettings } from "@/features/user/RegionSettings";
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
              <div className="border-t border-[var(--border)] mt-6 pt-6">
                <RegionSettings />
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

const AGE_OPTIONS = [
  { value: "", label: "" }, // resolved at render-time via t()
  { value: "0", label: "0+" },
  { value: "6", label: "6+" },
  { value: "7", label: "7+" },
  { value: "10", label: "10+" },
  { value: "12", label: "12+" },
  { value: "13", label: "13+" },
  { value: "14", label: "14+" },
  { value: "15", label: "15+" },
  { value: "16", label: "16+" },
  { value: "17", label: "17+" },
  { value: "18", label: "18+" },
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
  const profileId = defaultProfile?.id ?? "";

  const { data: parental } = useParentalServiceGetParentalSettings(profileId, { query: { enabled: !!profileId } } as never);

  const enabled = parental?.enabled ?? false;
  const [maxAge, setMaxAge] = useState<string>("");
  const [pin, setPin] = useState("");
  const [confirmPin, setConfirmPin] = useState("");
  const [pinError, setPinError] = useState("");
  const [password, setPassword] = useState("");
  const pendingOpRef = useRef<"pin" | "other">("other");
  const hasPin = parental?.hasPin ?? false;

  useEffect(() => {
    setMaxAge(parental?.maxAge != null ? String(parental.maxAge) : "");
  }, [parental?.maxAge]);

  const updateParental = useMutation({
    mutationFn: (body: { enabled?: boolean; maxAge?: number; unsetMaxAge?: boolean; newPin?: string; currentPin?: string; currentPassword?: string }) =>
      parentalServiceUpdateParentalSettings(profileId, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: getParentalServiceGetParentalSettingsQueryKey(profileId) });
      if (pendingOpRef.current === "pin") {
        toast.success(t("auth.settings.parental.pin.saved"));
        setPin("");
        setConfirmPin("");
        setPassword("");
      } else {
        toast.success(t("auth.settings.parental.updated"));
      }
      pendingOpRef.current = "other";
    },
    onError: (err: Error) => toast.error(err.message || t("auth.settings.parental.failedToUpdate")),
  });

  function handleToggleEnabled() {
    if (!profileId) return;
    updateParental.mutate({ enabled: !enabled });
  }

  function handleSaveAge() {
    if (!profileId) return;
    if (maxAge === "") {
      updateParental.mutate({ unsetMaxAge: true });
    } else {
      updateParental.mutate({ maxAge: parseInt(maxAge, 10) });
    }
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
    // First-time set requires the account password; rotation uses no
    // extra field here (existing PIN is implied via sep. flow).
    if (!hasPin && password === "") {
      setPinError(t("auth.settings.parental.pin.passwordRequired"));
      return;
    }
    if (!profileId) return;
    pendingOpRef.current = "pin";
    const body: { newPin: string; currentPassword?: string } = { newPin: pin };
    if (!hasPin) body.currentPassword = password;
    updateParental.mutate(body);
  }

  if (!defaultProfile) {
    return (
      <p className="text-sm text-[var(--text-muted)]">{t("auth.settings.parental.noProfile")}</p>
    );
  }

  const ageOptions = AGE_OPTIONS.map((o) => ({
    value: o.value,
    label: o.value === "" ? t("auth.settings.parental.noRestriction") : o.label,
  }));

  const currentServerAge = parental?.maxAge != null ? String(parental.maxAge) : "";

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
          disabled={updateParental.isPending}
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
          label={t("auth.settings.parental.maxAge")}
          options={ageOptions}
          value={maxAge}
          onChange={setMaxAge}
          disabled={!enabled}
        />
        <div>
          <Button
            variant="primary"
            loading={updateParental.isPending}
            disabled={!enabled || maxAge === currentServerAge}
            onClick={handleSaveAge}
          >
            {t("auth.settings.parental.saveAge")}
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
        {!hasPin && (
          <Input
            type="password"
            label={t("auth.settings.parental.pin.passwordReauth")}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
        )}
        {pinError && <p className="text-xs" style={{ color: "var(--color-danger)" }}>{pinError}</p>}
        <div>
          <Button
            variant="primary"
            loading={updateParental.isPending}
            disabled={!enabled || pin.length < 4 || (!hasPin && password === "")}
            onClick={handleSavePin}
          >
            {t("auth.settings.parental.pin.savePin")}
          </Button>
        </div>
      </div>
    </div>
  );
}
