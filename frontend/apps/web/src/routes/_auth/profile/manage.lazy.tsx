import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useDemoLock } from "@/hooks/useDemoMode";
import { Plus } from "lucide-react";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { ProfileManagerList } from "@/features/profile/ProfileManagerList";
import { CreateProfileDialog } from "@/features/profile/CreateProfileDialog";

export const Route = createLazyFileRoute("/_auth/profile/manage")({
  component: ProfileManagePage,
});

function ProfileManagePage() {
  const { t } = useTranslation();
  // Profiles are the shared account's on the demo: creating one is locked.
  const { lock } = useDemoLock();
  const [creating, setCreating] = useState(false);
  const { data: user } = useUserServiceGetCurrentUser();
  const userId = user?.id ?? "";
  const { data: profilesResp, isLoading } = useUserServiceListProfiles(userId, {
    query: { enabled: !!userId },
  });
  const profiles = profilesResp?.profiles ?? [];
  const hasKids = profiles.some((p) => p.isKids === true);

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <div className="flex items-center justify-between gap-4 flex-wrap">
        <div>
          <h1 className="text-2xl font-bold text-[var(--text)]">
            {t("auth.profile.manage.title", "Manage Profiles")}
          </h1>
          <p className="text-sm text-[var(--text-muted)] mt-1">
            {t(
              "auth.profile.manage.description",
              "Add, edit, or remove profiles. The default profile cannot be deleted.",
            )}
          </p>
        </div>
        <Button {...lock} variant="primary" onClick={() => setCreating(true)}>
          <Plus size={16} aria-hidden="true" />
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
          hasKidsProfile={hasKids}
        />
      )}
    </div>
  );
}
