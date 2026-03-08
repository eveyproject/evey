import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useUserServiceGetCurrentUser, useUserServiceUpdateUser } from "@evey/api";
import { getUserServiceGetCurrentUserQueryKey } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { ThemeSettings } from "@/features/user/ThemeSettings";
import { NotificationSettings } from "@/features/user/NotificationSettings";

export const Route = createFileRoute("/_auth/profile/settings")({
  component: SettingsPage,
});

type Tab = "appearance" | "notifications" | "account";

function SettingsPage() {
  const [tab, setTab] = useState<Tab>("appearance");

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">Settings</h1>

      <div className="flex gap-1 border-b border-[var(--border)]">
        {(["appearance", "notifications", "account"] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`px-4 py-2 text-sm capitalize transition-colors border-b-2 -mb-px ${
              tab === t
                ? "border-[var(--accent)] text-[var(--accent)]"
                : "border-transparent text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      <Card>
        <div className="p-6">
          {tab === "appearance" && <ThemeSettings />}
          {tab === "notifications" && <NotificationSettings />}
          {tab === "account" && <AccountSection />}
        </div>
      </Card>
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
      },
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
    <div className="flex flex-col gap-4">
      <h3 className="text-sm font-semibold text-[var(--text)]">Change Password</h3>
      <Input
        label="New password"
        type="password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        error={error}
      />
      <Input
        label="Confirm password"
        type="password"
        value={confirm}
        onChange={(e) => setConfirm(e.target.value)}
      />
      <Button
        variant="primary"
        loading={updateUser.isPending}
        onClick={handleSubmit}
      >
        Update password
      </Button>
    </div>
  );
}
