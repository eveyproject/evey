import { useTranslation } from "react-i18next";
import { useNavigate } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceImpersonateUser, apiErrorMessage } from "@evey/api";
import type { V1Profile, V1User } from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { History, LogIn } from "lucide-react";
import { Avatar } from "@/components/ui/Avatar";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";

// The avatar route is the same one the profile switcher uses; a profile with no
// uploaded picture falls back to its initial.
function avatarUrl(p: V1Profile): string | null {
  return p.avatarPath && p.id ? `/api/v1/profiles/${p.id}/avatar` : null;
}

// UserProfilesPanel is the expanded half of a user row: who actually watches
// under this account. Impersonation lives HERE rather than on the row because
// "connect as this user" without a profile always lands on the default one -
// the panel is what lets an admin land on the profile they mean.
export function UserProfilesPanel({ user, profiles }: { user: V1User; profiles: V1Profile[] }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const doImpersonate = useAuthStore((s) => s.impersonate);

  const impersonate = useUserServiceImpersonateUser({
    mutation: {
      onSuccess: (data) => {
        if (!data.accessToken) return;
        // Same handover as the row-level button: stash the admin token, swap to
        // the target's, and clear the cache so no admin-session data (which
        // sees every library unfiltered) is served under the target's identity.
        doImpersonate(data.accessToken);
        queryClient.clear();
        navigate({ to: "/" });
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.users.failedImpersonate"))),
    },
  });

  if (profiles.length === 0) {
    return (
      <EmptyState title={t("admin.users.noProfiles")} />
    );
  }

  return (
    <ul className="flex flex-col gap-2">
      {profiles.map((p) => (
        <li
          key={p.id}
          className="flex flex-wrap items-center gap-3 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface)] px-3 py-2"
        >
          <Avatar initial={(p.name ?? "?").charAt(0).toUpperCase()} imageUrl={avatarUrl(p)} size={32} />
          <div className="flex min-w-[8rem] flex-col">
            <span className="font-medium text-[var(--text)]">{p.name}</span>
            <span className="text-xs text-[var(--text-muted)]">
              {p.createdAt ? new Date(p.createdAt).toLocaleDateString() : "-"}
            </span>
          </div>
          <div className="flex flex-wrap gap-1">
            {p.isDefault && <Badge>{t("admin.users.defaultProfile")}</Badge>}
            {p.isKids && <Badge variant="warning">{t("admin.users.kidsProfile")}</Badge>}
            {p.maxRating && <Badge variant="default">{p.maxRating}</Badge>}
          </div>
          <div className="ml-auto flex flex-wrap gap-2">
            {user.enabled !== false && (
              <Button
                variant="secondary"
                loading={impersonate.isPending}
                onClick={() => impersonate.mutate({ userId: user.id!, data: { profileId: p.id } })}
              >
                <LogIn size={14} className="mr-1 inline" />
                {t("admin.users.impersonateProfile", { name: p.name })}
              </Button>
            )}
            <Button
              variant="ghost"
              onClick={() => navigate({ to: "/admin/activity", search: { profileId: p.id } })}
            >
              <History size={14} className="mr-1 inline" />
              {t("admin.users.viewHistory")}
            </Button>
          </div>
        </li>
      ))}
    </ul>
  );
}
