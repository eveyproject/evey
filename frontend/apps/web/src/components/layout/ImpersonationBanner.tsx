import { useTranslation } from "react-i18next";
import { useNavigate } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@evey/store";
import { Button } from "@/components/ui/Button";

// Persistent banner shown while an admin is impersonating another user (the active
// token carries an `imp_by` claim). "Exit" restores the admin's own token - stashed
// by useAuthStore.impersonate - without a re-login.
export function ImpersonationBanner() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const impersonatedBy = useAuthStore((s) => s.impersonatedBy);
  const username = useAuthStore((s) => s.impersonatedUsername);
  const exitImpersonation = useAuthStore((s) => s.exitImpersonation);

  if (!impersonatedBy) return null;

  return (
    <div className="relative z-badge flex items-center justify-center gap-3 bg-[var(--badge-warning-bg,#713f12)] px-4 py-2 text-sm text-[var(--badge-warning-text,#fde047)]">
      <span>
        {username
          ? t("admin.users.impersonationBanner", { name: username })
          : t("admin.users.impersonationBannerGeneric")}
      </span>
      <Button
        variant="ghost"
        onClick={() => {
          // Drop the target's cached data before restoring the admin token so
          // the admin doesn't keep seeing the impersonated user's filtered views.
          exitImpersonation();
          queryClient.clear();
          void navigate({ to: "/admin/users" });
        }}
      >
        {t("admin.users.exitImpersonation")}
      </Button>
    </div>
  );
}
