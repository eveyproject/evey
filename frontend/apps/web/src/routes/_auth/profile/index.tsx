import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useUserServiceGetCurrentUser, useUserServiceUpdateUser } from "@evey/api";
import type { V1User } from "@evey/api";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { useQueryClient } from "@tanstack/react-query";
import { getUserServiceGetCurrentUserQueryKey } from "@evey/api";

export const Route = createFileRoute("/_auth/profile/")({
  component: ProfilePage,
});

function ProfilePage() {
  const qc = useQueryClient();
  const { data: user, isLoading } = useUserServiceGetCurrentUser();

  const [editing, setEditing] = useState(false);
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");

  const startEdit = (u: V1User) => {
    setUsername(u.username ?? "");
    setEmail(u.email ?? "");
    setEditing(true);
  };

  const updateUser = useUserServiceUpdateUser({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceGetCurrentUserQueryKey() });
        setEditing(false);
      },
    },
  });

  const handleSave = () => {
    if (!user?.id) return;
    updateUser.mutate({ id: user.id, data: { username, email } });
  };

  if (isLoading) {
    return (
      <div className="px-6 py-8 text-sm text-[var(--text-muted)]">Loading...</div>
    );
  }

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">Profile</h1>

      <Card>
        <div className="p-6 flex flex-col gap-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <div className="w-14 h-14 rounded-full bg-[var(--accent)] flex items-center justify-center text-white text-xl font-bold">
                {user?.username?.charAt(0).toUpperCase() ?? "U"}
              </div>
              <div>
                <p className="font-semibold text-[var(--text)]">{user?.username ?? "—"}</p>
                <p className="text-sm text-[var(--text-muted)]">{user?.email ?? "—"}</p>
                {user?.isAdmin && (
                  <span className="text-xs bg-[var(--accent)] text-white px-2 py-0.5 rounded-[var(--radius-sm)] mt-1 inline-block">
                    Admin
                  </span>
                )}
              </div>
            </div>
            {!editing && (
              <Button variant="secondary" onClick={() => user && startEdit(user)}>
                Edit
              </Button>
            )}
          </div>

          {editing && (
            <div className="flex flex-col gap-3 pt-2 border-t border-[var(--border)]">
              <Input
                label="Username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
              <Input
                label="Email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
              <div className="flex gap-2">
                <Button variant="primary" loading={updateUser.isPending} onClick={handleSave}>
                  Save
                </Button>
                <Button variant="ghost" onClick={() => setEditing(false)}>
                  Cancel
                </Button>
              </div>
            </div>
          )}
        </div>
      </Card>

      <nav className="flex flex-col gap-2">
        <a
          href="/profile/settings"
          className="flex items-center justify-between p-4 bg-[var(--surface-raised)] rounded-[var(--radius-md)] text-sm text-[var(--text)] hover:bg-[var(--border)] transition-colors"
        >
          <span>Settings</span>
          <span className="text-[var(--text-muted)]">›</span>
        </a>
        <a
          href="/profile/history"
          className="flex items-center justify-between p-4 bg-[var(--surface-raised)] rounded-[var(--radius-md)] text-sm text-[var(--text)] hover:bg-[var(--border)] transition-colors"
        >
          <span>Watch History</span>
          <span className="text-[var(--text-muted)]">›</span>
        </a>
        <a
          href="/profile/notifications"
          className="flex items-center justify-between p-4 bg-[var(--surface-raised)] rounded-[var(--radius-md)] text-sm text-[var(--text)] hover:bg-[var(--border)] transition-colors"
        >
          <span>Notifications</span>
          <span className="text-[var(--text-muted)]">›</span>
        </a>
        {user?.isAdmin && (
          <a
            href="/admin"
            className="flex items-center justify-between p-4 bg-[var(--surface-raised)] rounded-[var(--radius-md)] text-sm text-[var(--accent)] hover:bg-[var(--border)] transition-colors"
          >
            <span>Admin Panel</span>
            <span className="text-[var(--text-muted)]">›</span>
          </a>
        )}
      </nav>
    </div>
  );
}
