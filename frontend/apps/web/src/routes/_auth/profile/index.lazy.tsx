import { Settings, Clock, Bell, Monitor, ChevronRight } from "lucide-react";
import type React from "react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import { useUserServiceGetCurrentUser, useUserServiceUpdateUser } from "@evey/api";
import type { V1User } from "@evey/api";
import { toast } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { useQueryClient } from "@tanstack/react-query";
import { getUserServiceGetCurrentUserQueryKey } from "@evey/api";

export const Route = createLazyFileRoute("/_auth/profile/")({
  component: ProfilePage,
});

function formatDate(iso: string | undefined): string {
  if (!iso) return "";
  return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" });
}

const NAV_ITEM_KEYS: { to: "/profile/settings" | "/profile/history" | "/profile/notifications"; labelKey: string; descKey: string; icon: React.ReactNode }[] = [
  {
    to: "/profile/settings",
    labelKey: "auth.profile.nav.settings",
    descKey: "auth.profile.nav.settingsDesc",
    icon: <Settings size={20} strokeWidth={1.5} />,
  },
  {
    to: "/profile/history",
    labelKey: "auth.profile.nav.watchHistory",
    descKey: "auth.profile.nav.watchHistoryDesc",
    icon: <Clock size={20} strokeWidth={1.5} />,
  },
  {
    to: "/profile/notifications",
    labelKey: "auth.profile.nav.notifications",
    descKey: "auth.profile.nav.notificationsDesc",
    icon: <Bell size={20} strokeWidth={1.5} />,
  },
];

function ProfilePage() {
  const { t } = useTranslation();
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
        toast.success(t("auth.profile.profileUpdated"));
      },
      onError: (err: Error) => toast.error(err.message || t("auth.profile.failedToUpdate")),
    },
  });

  const handleSave = () => {
    if (!user?.id) return;
    updateUser.mutate({ id: user.id, data: { username, email } });
  };

  if (isLoading) {
    return (
      <div className="px-6 py-8 text-sm text-[var(--text-muted)]">{t("common.loading")}</div>
    );
  }

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">{t("auth.profile.title")}</h1>

      <Card>
        <div className="p-6 flex flex-col gap-5">
          <div className="flex items-center gap-4">
            <div className="w-16 h-16 rounded-full bg-[var(--accent)] flex items-center justify-center text-white text-2xl font-bold shrink-0">
              {user?.username?.charAt(0).toUpperCase() ?? "U"}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-lg font-semibold text-[var(--text)] truncate">{user?.username ?? "---"}</p>
              <p className="text-sm text-[var(--text-muted)] truncate">{user?.email ?? "---"}</p>
              <div className="flex items-center gap-2 mt-1">
                {user?.isAdmin && (
                  <span className="text-xs bg-[var(--accent)] text-white px-2 py-0.5 rounded-[var(--radius-sm)]">
                    {t("auth.profile.admin")}
                  </span>
                )}
                {user?.createdAt && (
                  <span className="text-xs text-[var(--text-muted)]">
                    {t("auth.profile.memberSince", { date: formatDate(user.createdAt) })}
                  </span>
                )}
              </div>
            </div>
            {!editing && (
              <Button variant="secondary" onClick={() => user && startEdit(user)} className="shrink-0">
                {t("auth.profile.edit")}
              </Button>
            )}
          </div>

          {editing && (
            <div className="flex flex-col gap-3 pt-4 border-t border-[var(--border)]">
              <Input
                label={t("auth.profile.username")}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
              <Input
                label={t("auth.profile.email")}
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
              <div className="flex gap-2">
                <Button variant="primary" loading={updateUser.isPending} onClick={handleSave}>
                  {t("auth.profile.save")}
                </Button>
                <Button variant="ghost" onClick={() => setEditing(false)}>
                  {t("auth.profile.cancel")}
                </Button>
              </div>
            </div>
          )}
        </div>
      </Card>

      <nav className="flex flex-col gap-2">
        {NAV_ITEM_KEYS.map((item) => (
          <Link
            key={item.to}
            to={item.to}
            className="flex items-center gap-3 p-4 bg-[var(--surface-raised)] rounded-[var(--radius-md)] text-[var(--text)] hover:bg-[var(--border)] transition-colors group"
          >
            <span className="text-[var(--text-muted)] group-hover:text-[var(--accent)] transition-colors shrink-0">
              {item.icon}
            </span>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium">{t(item.labelKey)}</p>
              <p className="text-xs text-[var(--text-muted)]">{t(item.descKey)}</p>
            </div>
            <ChevronRight size={16} className="text-[var(--text-muted)] shrink-0" />
          </Link>
        ))}
        {user?.isAdmin && (
          <Link
            to="/admin"
            className="flex items-center gap-3 p-4 bg-[var(--surface-raised)] rounded-[var(--radius-md)] hover:bg-[var(--border)] transition-colors group"
          >
            <Monitor size={20} strokeWidth={1.5} className="text-[var(--accent)] shrink-0" />
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-[var(--accent)]">{t("auth.profile.nav.adminPanel")}</p>
              <p className="text-xs text-[var(--text-muted)]">{t("auth.profile.nav.adminPanelDesc")}</p>
            </div>
            <ChevronRight size={16} className="text-[var(--text-muted)] shrink-0" />
          </Link>
        )}
      </nav>
    </div>
  );
}
