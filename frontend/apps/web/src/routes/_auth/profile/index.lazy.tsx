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

const NAV_ITEM_KEYS = [
  {
    to: "/profile/settings" as const,
    labelKey: "profile.nav.settings",
    descKey: "profile.nav.settingsDesc",
    icon: "M10.343 3.94c.09-.542.56-.94 1.11-.94h1.093c.55 0 1.02.398 1.11.94l.149.894c.07.424.384.764.78.93.398.164.855.142 1.205-.108l.737-.527a1.125 1.125 0 011.45.12l.773.774c.39.389.44 1.002.12 1.45l-.527.737c-.25.35-.272.806-.107 1.204.165.397.505.71.93.78l.893.15c.543.09.94.56.94 1.109v1.094c0 .55-.397 1.02-.94 1.11l-.893.149c-.425.07-.765.383-.93.78-.165.398-.143.854.107 1.204l.527.738c.32.447.269 1.06-.12 1.45l-.774.773a1.125 1.125 0 01-1.449.12l-.738-.527c-.35-.25-.806-.272-1.204-.107-.397.165-.71.505-.78.929l-.15.894c-.09.542-.56.94-1.11.94h-1.094c-.55 0-1.019-.398-1.11-.94l-.148-.894c-.071-.424-.384-.764-.781-.93-.398-.164-.854-.142-1.204.108l-.738.527c-.447.32-1.06.269-1.45-.12l-.773-.774a1.125 1.125 0 01-.12-1.45l.527-.737c.25-.35.273-.806.108-1.204-.165-.397-.506-.71-.93-.78l-.894-.15c-.542-.09-.94-.56-.94-1.109v-1.094c0-.55.398-1.02.94-1.11l.894-.149c.424-.07.765-.383.93-.78.165-.398.143-.854-.108-1.204l-.526-.738a1.125 1.125 0 01.12-1.45l.773-.773a1.125 1.125 0 011.45-.12l.737.527c.35.25.807.272 1.204.107.397-.165.71-.505.78-.929l.15-.894z M15 12a3 3 0 11-6 0 3 3 0 016 0z",
  },
  {
    to: "/profile/history" as const,
    labelKey: "profile.nav.watchHistory",
    descKey: "profile.nav.watchHistoryDesc",
    icon: "M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z",
  },
  {
    to: "/profile/notifications" as const,
    labelKey: "profile.nav.notifications",
    descKey: "profile.nav.notificationsDesc",
    icon: "M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0",
  },
];

function ProfilePage() {
  const { t } = useTranslation("auth");
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
        toast.success(t("profile.profileUpdated"));
      },
      onError: (err: Error) => toast.error(err.message || t("profile.failedToUpdate")),
    },
  });

  const handleSave = () => {
    if (!user?.id) return;
    updateUser.mutate({ id: user.id, data: { username, email } });
  };

  if (isLoading) {
    return (
      <div className="px-6 py-8 text-sm text-[var(--text-muted)]">{t("loading", { ns: "common" })}</div>
    );
  }

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">{t("profile.title")}</h1>

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
                    {t("profile.admin")}
                  </span>
                )}
                {user?.createdAt && (
                  <span className="text-xs text-[var(--text-muted)]">
                    {t("profile.memberSince", { date: formatDate(user.createdAt) })}
                  </span>
                )}
              </div>
            </div>
            {!editing && (
              <Button variant="secondary" onClick={() => user && startEdit(user)} className="shrink-0">
                {t("profile.edit")}
              </Button>
            )}
          </div>

          {editing && (
            <div className="flex flex-col gap-3 pt-4 border-t border-[var(--border)]">
              <Input
                label={t("profile.username")}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
              <Input
                label={t("profile.email")}
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
              <div className="flex gap-2">
                <Button variant="primary" loading={updateUser.isPending} onClick={handleSave}>
                  {t("profile.save")}
                </Button>
                <Button variant="ghost" onClick={() => setEditing(false)}>
                  {t("profile.cancel")}
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
            <svg
              className="w-5 h-5 text-[var(--text-muted)] group-hover:text-[var(--accent)] transition-colors shrink-0"
              fill="none"
              stroke="currentColor"
              strokeWidth={1.5}
              strokeLinecap="round"
              strokeLinejoin="round"
              viewBox="0 0 24 24"
            >
              <path d={item.icon} />
            </svg>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium">{t(item.labelKey)}</p>
              <p className="text-xs text-[var(--text-muted)]">{t(item.descKey)}</p>
            </div>
            <svg className="w-4 h-4 text-[var(--text-muted)] shrink-0" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </Link>
        ))}
        {user?.isAdmin && (
          <Link
            to="/admin"
            className="flex items-center gap-3 p-4 bg-[var(--surface-raised)] rounded-[var(--radius-md)] hover:bg-[var(--border)] transition-colors group"
          >
            <svg
              className="w-5 h-5 text-[var(--accent)] shrink-0"
              fill="none"
              stroke="currentColor"
              strokeWidth={1.5}
              strokeLinecap="round"
              strokeLinejoin="round"
              viewBox="0 0 24 24"
            >
              <path d="M9 17.25v1.007a3 3 0 01-.879 2.122L7.5 21h9l-.621-.621A3 3 0 0115 18.257V17.25m6-12V15a2.25 2.25 0 01-2.25 2.25H5.25A2.25 2.25 0 013 15V5.25m18 0A2.25 2.25 0 0018.75 3H5.25A2.25 2.25 0 003 5.25m18 0V12a2.25 2.25 0 01-2.25 2.25H5.25A2.25 2.25 0 013 12V5.25" />
            </svg>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-[var(--accent)]">{t("profile.nav.adminPanel")}</p>
              <p className="text-xs text-[var(--text-muted)]">{t("profile.nav.adminPanelDesc")}</p>
            </div>
            <svg className="w-4 h-4 text-[var(--text-muted)] shrink-0" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </Link>
        )}
      </nav>
    </div>
  );
}
