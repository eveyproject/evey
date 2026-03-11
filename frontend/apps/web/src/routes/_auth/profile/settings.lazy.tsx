import { createLazyFileRoute } from "@tanstack/react-router";
import { useState, useEffect } from "react";
import { useUserServiceGetCurrentUser, useUserServiceUpdateUser, useUserServiceListProfiles } from "@evey/api";
import { getUserServiceGetCurrentUserQueryKey, getUserServiceListProfilesQueryKey } from "@evey/api";
import { eveyFetch } from "@evey/api";
import { useQueryClient, useMutation } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Select";
import { ThemeSettings } from "@/features/user/ThemeSettings";
import { NotificationSettings } from "@/features/user/NotificationSettings";

export const Route = createLazyFileRoute("/_auth/profile/settings")({
  component: SettingsPage,
});

type Tab = "appearance" | "playback" | "notifications" | "account";

const TABS: { key: Tab; label: string; icon: string }[] = [
  {
    key: "appearance",
    label: "Appearance",
    icon: "M4.098 19.902a3.75 3.75 0 005.304 0l6.401-6.402M6.75 21A3.75 3.75 0 013 17.25V4.125C3 3.504 3.504 3 4.125 3h5.25c.621 0 1.125.504 1.125 1.125v4.072M6.75 21a3.75 3.75 0 003.75-3.75V8.197M6.75 21h13.125c.621 0 1.125-.504 1.125-1.125v-5.25c0-.621-.504-1.125-1.125-1.125h-4.072M10.5 8.197l2.88-2.88c.438-.439 1.15-.439 1.59 0l3.712 3.713c.44.44.44 1.152 0 1.59l-2.879 2.88M6.75 17.25h.008v.008H6.75v-.008z",
  },
  {
    key: "playback",
    label: "Playback",
    icon: "M5.25 5.653c0-.856.917-1.398 1.667-.986l11.54 6.347a1.125 1.125 0 010 1.972l-11.54 6.347a1.125 1.125 0 01-1.667-.986V5.653z",
  },
  {
    key: "notifications",
    label: "Notifications",
    icon: "M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0",
  },
  {
    key: "account",
    label: "Account",
    icon: "M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z",
  },
];

function SettingsPage() {
  const [tab, setTab] = useState<Tab>("appearance");

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">Settings</h1>

      <div className="flex gap-1 border-b border-[var(--border)]">
        {TABS.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={`flex items-center gap-2 px-4 py-2 text-sm transition-colors border-b-2 -mb-px ${
              tab === t.key
                ? "border-[var(--accent)] text-[var(--accent)]"
                : "border-transparent text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" viewBox="0 0 24 24">
              <path d={t.icon} />
            </svg>
            {t.label}
          </button>
        ))}
      </div>

      <Card>
        <div className="p-6">
          {tab === "appearance" && <ThemeSettings />}
          {tab === "playback" && <PlaybackSection />}
          {tab === "notifications" && <NotificationSettings />}
          {tab === "account" && <AccountSection />}
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
      toast.success("Bitrate limit updated");
    },
    onError: (err: Error) => toast.error(err.message || "Failed to update bitrate limit"),
  });

  const handleChange = (value: string) => {
    setBitrate(value);
    updateProfile.mutate(value);
  };

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">Streaming Bitrate Limit</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">
          Maximum bitrate for transcoded streams. Does not affect direct play.
        </p>
      </div>
      <Select
        label="Max bitrate"
        options={BITRATE_OPTIONS}
        value={bitrate}
        onChange={handleChange}
        disabled={!defaultProfile}
      />
    </div>
  );
}

function AccountSection() {
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
        toast.success("Password updated");
      },
      onError: (err: Error) => toast.error(err.message || "Failed to update password"),
    },
  });

  const handleSubmit = () => {
    if (!user?.id) return;
    if (password !== confirm) {
      setError("Passwords do not match");
      return;
    }
    if (password.length < 8) {
      setError("Password must be at least 8 characters");
      return;
    }
    setError("");
    updateUser.mutate({ id: user.id, data: { password } });
  };

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">Change Password</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">Use a strong password with at least 8 characters.</p>
      </div>
      <div className="flex flex-col gap-3">
        <Input
          label="New password"
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          error={error}
        />
        <Input
          label="Confirm password"
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
            Update password
          </Button>
        </div>
      </div>
    </div>
  );
}
