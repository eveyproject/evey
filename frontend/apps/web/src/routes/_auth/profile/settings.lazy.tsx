import { Palette, Bell, Shield, Smartphone, QrCode, Pencil, X, Users, Plus, Inbox, ChevronRight, PlayCircle, Puzzle } from "lucide-react";
import type React from "react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useState, useRef, useEffect } from "react";
import { useUserServiceGetCurrentUser, useUserServiceUpdateUser, useUserServiceListProfiles, useMediaRequestServiceGetRequestPolicy } from "@evey/api";
import { useParentalServiceGetParentalSettings, parentalServiceUpdateParentalSettings, getParentalServiceGetParentalSettingsQueryKey, apiErrorMessage } from "@evey/api";
import { getUserServiceGetCurrentUserQueryKey, getUserServiceListProfilesQueryKey } from "@evey/api";
import { eveyFetchRaw } from "@evey/api";
import { useMutation } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { useTranslation } from "react-i18next";
import { useActiveProfile } from "@/hooks/useActiveProfile";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { ThemeSettings } from "@/features/user/ThemeSettings";
import { LanguageSettings } from "@/features/user/LanguageSettings";
import { RegionSettings } from "@/features/user/RegionSettings";
import { AutoplayNextSettings } from "@/features/user/AutoplayNextSettings";
import { PlaybackSettings } from "@/features/user/PlaybackSettings";
import { NotificationSettings } from "@/features/user/NotificationSettings";
import { DeviceSettings } from "@/features/user/DeviceSettings";
import { QuickConnectSettings } from "@/features/user/QuickConnectSettings";
import { ProfileManagerList } from "@/features/profile/ProfileManagerList";
import { CreateProfileDialog } from "@/features/profile/CreateProfileDialog";
import { PluginSlot } from "@/features/plugins/PluginSlot";
import { usePluginSlotEntries } from "@/features/plugins/usePluginSlot";
import { ICON } from "@/constants/iconSizes";

export const Route = createLazyFileRoute("/_auth/profile/settings")({
  component: SettingsPage,
});

// USER_SETTINGS_SLOT is the only slot a plugin may use to add its own
// per-profile settings UI (the server rejects every other settings.* slot).
// Each plugin mounting it becomes one entry under the "Plugins" menu.
const USER_SETTINGS_SLOT = "settings.user.appearance";

type Tab = "appearance" | "playback" | "profiles" | "notifications" | "parental" | "devices" | "quick-connect" | "plugins";

const TABS: { key: Tab; labelKey: string; icon: React.ReactNode }[] = [
  { key: "appearance", labelKey: "auth.settings.tabs.appearance", icon: <Palette size={ICON.md} strokeWidth={1.5} /> },
  { key: "playback", labelKey: "auth.settings.tabs.playback", icon: <PlayCircle size={ICON.md} strokeWidth={1.5} /> },
  { key: "profiles", labelKey: "auth.settings.tabs.profiles", icon: <Users size={ICON.md} strokeWidth={1.5} /> },
  { key: "notifications", labelKey: "auth.settings.tabs.notifications", icon: <Bell size={ICON.md} strokeWidth={1.5} /> },
  { key: "parental", labelKey: "auth.settings.tabs.parental", icon: <Shield size={ICON.md} strokeWidth={1.5} /> },
  { key: "devices", labelKey: "auth.settings.tabs.devices", icon: <Smartphone size={ICON.md} strokeWidth={1.5} /> },
  { key: "quick-connect", labelKey: "auth.settings.tabs.quickConnect", icon: <QrCode size={ICON.md} strokeWidth={1.5} /> },
];

// One menu entry. The label wraps instead of being cut: a vertical menu has
// the room for it, and the tab strip this replaced forced the reader to
// scroll sideways to reach "Contrôle parental" / "Schnellverbindung".
function NavItem({
  active, onClick, icon, label, testId, nested = false,
}: {
  active: boolean;
  onClick: () => void;
  icon?: React.ReactNode;
  label: string;
  testId?: string;
  nested?: boolean;
}) {
  return (
    <button
      type="button"
      data-testid={testId}
      onClick={onClick}
      aria-current={active ? "page" : undefined}
      className={`flex items-center gap-2 min-w-0 text-left rounded-[var(--radius-md)] px-3 py-2 transition-colors-fast ${
        nested ? "text-xs" : "text-sm"
      } ${
        active
          ? "bg-[var(--surface-raised)] text-[var(--accent)] font-medium"
          : "text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface-raised)]"
      }`}
    >
      {icon && <span className="shrink-0">{icon}</span>}
      <span className="min-w-0 break-words">{label}</span>
    </button>
  );
}

function ProfileCard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  // The avatar shown, and the one an upload REPLACES, belong to the profile in
  // front of the user. Targeting the household default meant anyone on a
  // non-default profile saw an initial instead of their picture, and their
  // upload landed on somebody else's profile. Same defect the mobile app had
  // until 03-09.
  const { profile: activeProfile } = useActiveProfile();

  const [editing, setEditing] = useState(false);
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPwd, setConfirmPwd] = useState("");
  const [avatarPreview, setAvatarPreview] = useState<string | null>(null);
  // Revoke the previous object URL when the preview changes or the page unmounts,
  // so repeated avatar uploads in one session don't leak blob URLs.
  useEffect(() => {
    return () => {
      if (avatarPreview) URL.revokeObjectURL(avatarPreview);
    };
  }, [avatarPreview]);
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
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("auth.profile.failedToUpdate"))),
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
    if (!file || !activeProfile?.id) return;
    const formData = new FormData();
    formData.append("avatar", file);
    try {
      const res = await eveyFetchRaw({ url: `/api/v1/profiles/${activeProfile.id}/avatar`, method: "POST", body: formData });
      if (!res.ok) throw new Error((await res.text()) || t("auth.profile.avatarFailed"));
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
    : activeProfile?.avatarPath && activeProfile?.id
      ? `/api/v1/profiles/${activeProfile.id}/avatar`
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
              <Pencil size={ICON.md} className="text-white"  aria-hidden="true" />
            </div>
          </button>
          <input ref={fileInputRef} type="file" accept="image/*" className="hidden" onChange={handleAvatarUpload} />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <p className="text-lg font-semibold text-[var(--text)] truncate">{user?.username ?? "..."}</p>
              {user?.isAdmin && (
                <span className="text-xs bg-[var(--accent)] text-[var(--filter-active-text)] px-2 py-0.5 rounded-[var(--radius-sm)]">{t("common.nav.admin")}</span>
              )}
            </div>
            <p className="text-sm text-[var(--text-muted)] truncate">{user?.email || "-"}</p>
            {memberSince && <p className="text-xs text-[var(--text-muted)] mt-0.5">{memberSince}</p>}
          </div>
          <button
            onClick={editing ? () => setEditing(false) : startEdit}
            className="p-2 rounded-[var(--radius-sm)] text-[var(--text-muted)] hover:text-[var(--accent)] hover:bg-[var(--surface-raised)] transition-colors-fast"
            aria-label={editing ? t("common.cancel") : t("auth.profile.editProfile")}
          >
            {editing ? <X size={ICON.lg}  aria-hidden="true" /> : <Pencil size={ICON.lg}  aria-hidden="true" />}
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
  const { data: requestPolicy } = useMediaRequestServiceGetRequestPolicy();
  // Plugins that ship their own per-profile settings UI. Nothing installed
  // means no "Plugins" entry at all rather than an empty section.
  const pluginEntries = usePluginSlotEntries(USER_SETTINGS_SLOT);
  const [openPluginId, setOpenPluginId] = useState<string | null>(null);
  // Resolve by id every render: a plugin uninstalled while its sub-menu is
  // open must fall back to the first one instead of rendering a dead frame.
  const activePlugin = pluginEntries.find((e) => e.pluginId === openPluginId) ?? pluginEntries[0];

  return (
    <div className="w-full max-w-4xl mx-auto px-4 sm:px-6 py-6 sm:py-8 flex flex-col gap-6 min-w-0">
      <h1 className="text-2xl font-bold text-[var(--text)]">{t("auth.settings.title")}</h1>

      <ProfileCard />

      {requestPolicy?.canRequest && (
        <Link to="/requests" className="block">
          <Card className="flex items-center gap-3 hover:bg-[var(--surface-raised)] transition-colors-fast">
            <Inbox size={ICON.lg} className="text-[var(--text-muted)]" />
            <span className="flex-1 text-sm font-medium text-[var(--text)]">{t("common.nav.requests")}</span>
            <ChevronRight size={ICON.md} className="text-[var(--text-muted)]" />
          </Card>
        </Link>
      )}

      {/* Menu + panel. The menu was a horizontal tab strip: six translated
          labels never fit, so it scrolled sideways even on a desktop, and the
          entries at the end were reachable only by dragging. A vertical menu
          (two columns on a phone, one beside the panel from sm up) removes
          the sideways scroll and gives every label the width to wrap. */}
      <div className="grid gap-4 sm:gap-6 sm:grid-cols-[14rem_minmax(0,1fr)] sm:items-start min-w-0">
        <nav aria-label={t("auth.settings.title")} className="grid grid-cols-2 sm:grid-cols-1 gap-1 min-w-0 sm:sticky sm:top-6">
          {TABS.map((tb) => (
            <NavItem
              key={tb.key}
              testId={`profile-settings-tab-${tb.key}`}
              active={tab === tb.key}
              onClick={() => setTab(tb.key)}
              icon={tb.icon}
              label={t(tb.labelKey)}
            />
          ))}
          {pluginEntries.length > 0 && (
            <>
              <NavItem
                testId="profile-settings-tab-plugins"
                active={tab === "plugins"}
                onClick={() => setTab("plugins")}
                icon={<Puzzle size={ICON.md} strokeWidth={1.5} />}
                label={t("auth.settings.tabs.plugins")}
              />
              {tab === "plugins" && (
                <div className="col-span-2 sm:col-span-1 flex flex-col gap-0.5 ml-2 pl-2 border-l border-[var(--border)] min-w-0">
                  {pluginEntries.map((entry) => (
                    <NavItem
                      key={entry.pluginId}
                      testId={`profile-settings-plugin-${entry.pluginId}`}
                      active={activePlugin?.pluginId === entry.pluginId}
                      onClick={() => setOpenPluginId(entry.pluginId)}
                      label={entry.name}
                      nested
                    />
                  ))}
                </div>
              )}
            </>
          )}
        </nav>

      <Card className="min-w-0">
        <div className="p-4 sm:p-6 min-w-0">
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
          {tab === "playback" && (
            <>
              <AutoplayNextSettings />
              <div className="border-t border-[var(--border)] mt-6 pt-6">
                <PlaybackSettings />
              </div>
            </>
          )}
          {tab === "profiles" && <ProfilesSection />}
          {tab === "notifications" && <NotificationSettings />}
          {tab === "parental" && <ParentalSection />}
          {tab === "devices" && <DeviceSettings />}
          {tab === "quick-connect" && <QuickConnectSettings />}
          {/* One plugin at a time, picked in the menu: a plugin's settings UI
              is a full iframe, and stacking every installed one in a single
              scroll was what made this section unreadable. */}
          {tab === "plugins" && activePlugin && (
            <PluginSlot name={USER_SETTINGS_SLOT} pluginId={activePlugin.pluginId} />
          )}
        </div>
      </Card>
      </div>
    </div>
  );
}

function ProfilesSection() {
  const { t } = useTranslation();
  const [creating, setCreating] = useState(false);
  const { data: user } = useUserServiceGetCurrentUser();
  const userId = user?.id ?? "";
  const { data: profilesResp, isLoading } = useUserServiceListProfiles(userId, {
    query: { enabled: !!userId },
  });
  const profiles = profilesResp?.profiles ?? [];
  const hasKidsProfile = profiles.some((p) => p.isKids === true);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text)]">
            {t("auth.profile.manage.title", "Manage Profiles")}
          </h3>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            {t(
              "auth.profile.manage.description",
              "Add, edit, or remove profiles. The default profile cannot be deleted.",
            )}
          </p>
        </div>
        <Button
          variant="primary"
          onClick={() => setCreating(true)}
          data-testid="profile-settings-create-profile-btn"
        >
          <Plus size={ICON.md} aria-hidden="true" />
          {t("auth.profile.manage.createProfile", "Create Profile")}
        </Button>
      </div>

      {isLoading && (
        <div className="flex justify-center py-6">
          <Spinner />
        </div>
      )}

      {!isLoading && userId && (
        <ProfileManagerList userId={userId} profiles={profiles} />
      )}

      {userId && (
        <CreateProfileDialog
          open={creating}
          onClose={() => setCreating(false)}
          userId={userId}
          hasKidsProfile={hasKidsProfile}
        />
      )}
    </div>
  );
}

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
  // The server enforces the PIN and the age cap of the ACTIVE profile
  // (checkParental reads claims.ProfileID). Reading and writing the DEFAULT
  // profile's settings here showed a household member restrictions that were
  // never the ones applied to them, and edited a profile they were not on.
  const { profile: activeProfile } = useActiveProfile();
  const profileId = activeProfile?.id ?? "";

  const { data: parental } = useParentalServiceGetParentalSettings(profileId, { query: { enabled: !!profileId } });

  const [currentPin, setCurrentPin] = useState("");
  const [pin, setPin] = useState("");
  const [confirmPin, setConfirmPin] = useState("");
  const [pinError, setPinError] = useState("");
  const [password, setPassword] = useState("");
  const hasPin = parental?.hasPin ?? false;

  const updateParental = useMutation({
    mutationFn: (body: { enabled?: boolean; maxAge?: number; unsetMaxAge?: boolean; newPin?: string; currentPin?: string; currentPassword?: string }) =>
      parentalServiceUpdateParentalSettings(profileId, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: getParentalServiceGetParentalSettingsQueryKey(profileId) });
      toast.success(t("auth.settings.parental.pin.saved"));
      setCurrentPin("");
      setPin("");
      setConfirmPin("");
      setPassword("");
    },
    onError: (err: Error) => {
      toast.error(apiErrorMessage(err, t, t("auth.settings.parental.failedToUpdate")));
    },
  });

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
    // First-time set requires the account password (otherwise a stolen
    // JWT could lock the owner out). Rotation requires the current PIN.
    if (hasPin && currentPin.length !== 4) {
      setPinError(t("auth.settings.parental.pin.currentPinRequired", "Enter your current PIN"));
      return;
    }
    if (!hasPin && password === "") {
      setPinError(t("auth.settings.parental.pin.passwordRequired"));
      return;
    }
    if (!profileId) return;
    const body: { newPin: string; currentPin?: string; currentPassword?: string } = { newPin: pin };
    if (hasPin) {
      body.currentPin = currentPin;
    } else {
      body.currentPassword = password;
    }
    updateParental.mutate(body);
  }

  if (!activeProfile) {
    return (
      <p className="text-sm text-[var(--text-muted)]">{t("auth.settings.parental.noProfile")}</p>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.parental.title")}</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">
          {t("auth.settings.parental.pinScopeNote", "The PIN gates kids → adult profile switches. Per-profile age caps live on each profile (Profiles tab → Edit).")}
        </p>
      </div>

      <div className="flex flex-col gap-3">
        <div>
          <h4 className="text-sm font-semibold text-[var(--text)]">
            {hasPin
              ? t("auth.settings.parental.pin.changeTitle", "Change PIN")
              : t("auth.settings.parental.pin.title")}
          </h4>
          <p className="text-xs text-[var(--text-muted)] mt-1">{t("auth.settings.parental.pin.description")}</p>
        </div>
        {hasPin && (
          <PinInput
            label={t("auth.settings.parental.pin.currentPin", "Current PIN")}
            value={currentPin}
            onChange={setCurrentPin}
          />
        )}
        <PinInput
          label={t("auth.settings.parental.pin.newPin")}
          value={pin}
          onChange={setPin}
        />
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
            disabled={
              pin.length < 4 ||
              (hasPin && currentPin.length < 4) ||
              (!hasPin && password === "")
            }
            onClick={handleSavePin}
          >
            {hasPin
              ? t("auth.settings.parental.pin.updatePin", "Update PIN")
              : t("auth.settings.parental.pin.savePin")}
          </Button>
        </div>
      </div>
    </div>
  );
}
