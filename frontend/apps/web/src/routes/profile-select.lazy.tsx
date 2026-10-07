import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createLazyFileRoute, useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  userServiceRefreshToken,
} from "@evey/api";
import type { V1Profile } from "@evey/api";
import { useAuthStore, toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { PinPromptDialog } from "@/features/profile/PinPromptDialog";

const LAST_PROFILE_KEY = "evey:last_profile_id";

function profileAvatarUrl(p: V1Profile): string | null {
  return p.avatarPath && p.id ? `/api/v1/profiles/${p.id}/avatar` : null;
}

function profileInitial(p: V1Profile): string {
  return (p.name ?? "?").charAt(0).toUpperCase();
}

export const Route = createLazyFileRoute("/profile-select")({
  component: ProfileSelectPage,
});

function ProfileSelectPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { data: currentUser, isLoading: userLoading } = useUserServiceGetCurrentUser();
  const { data: profilesResp, isLoading: profilesLoading } = useUserServiceListProfiles(
    currentUser?.id ?? "",
    { query: { enabled: !!currentUser?.id } },
  );
  const profiles = useMemo(() => profilesResp?.profiles ?? [], [profilesResp]);
  const activeProfileId = useAuthStore((s) => s.profileId);
  const [busy, setBusy] = useState<string | null>(null);
  const [pinTarget, setPinTarget] = useState<V1Profile | null>(null);

  const hasKidsProfile = profiles.some((p) => p.isKids === true);
  const loading = userLoading || profilesLoading;
  const autoRedirectedRef = useRef(false);

  const currentProfile = profiles.find((p) => p.id === activeProfileId);
  const currentIsKids = currentProfile?.isKids === true;

  const switchProfile = useCallback(async (target: V1Profile, pinToken?: string): Promise<"ok" | "pin" | "fail"> => {
    if (!target.id) return "fail";
    try {
      const resp = await userServiceRefreshToken({ profileId: target.id, pinToken });
      if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      try { localStorage.setItem(LAST_PROFILE_KEY, target.id); } catch { /* ignore */ }
      // Clear, not invalidate: the previous profile's rows must not be shown
      // to the next one (a kids profile after an adult one) while they refetch.
      qc.clear();
      return "ok";
    } catch (err) {
      // 403 = the server wants a parental PIN proof (gated source or
      // PIN-carrying target). The refresh token is not consumed, so a retry
      // with a pinToken works.
      if (!pinToken && (err as { httpStatus?: number }).httpStatus === 403) return "pin";
      return "fail";
    }
  }, [qc]);

  const handleSelect = useCallback(async (p: V1Profile) => {
    if (!p.id || busy) return;
    const targetIsKids = p.isKids === true;
    const needsPin = currentIsKids && !targetIsKids;
    if (needsPin) {
      setPinTarget(p);
      return;
    }
    setBusy(p.id);
    const res = await switchProfile(p);
    setBusy(null);
    if (res === "ok") void navigate({ to: "/home" });
    else if (res === "pin") setPinTarget(p);
    else toast.error(t("common.error"));
  }, [busy, currentIsKids, switchProfile, navigate, t]);

  useEffect(() => {
    if (loading || autoRedirectedRef.current) return;
    if (profiles.length === 0) {
      autoRedirectedRef.current = true;
      void navigate({ to: "/home" });
      return;
    }
    if (!hasKidsProfile) {
      autoRedirectedRef.current = true;
      let targetId: string | null = null;
      try { targetId = localStorage.getItem(LAST_PROFILE_KEY); } catch { /* ignore */ }
      const target = profiles.find((p) => p.id === targetId) ?? profiles.find((p) => p.isDefault) ?? profiles[0];
      if (target) {
        void (async () => {
          if (target.id && target.id !== activeProfileId) {
            await switchProfile(target);
          }
          void navigate({ to: "/home" });
        })();
      } else {
        void navigate({ to: "/home" });
      }
    }
  }, [loading, hasKidsProfile, profiles, activeProfileId, switchProfile, navigate]);

  if (loading || !hasKidsProfile) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-[var(--bg)]">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="min-h-screen flex flex-col items-center justify-center px-4 py-12 bg-[var(--bg)]">
      <h1 className="text-3xl font-bold text-[var(--text)] mb-2">
        {t("auth.profile.select.title", "Who's watching?")}
      </h1>
      <p className="text-[var(--text-muted)] mb-10">
        {t("auth.profile.select.description", "Select a profile to continue.")}
      </p>
      <div className="flex flex-wrap items-start justify-center gap-8 max-w-4xl">
        {profiles.map((p) => (
          <button
            key={p.id}
            onClick={() => void handleSelect(p)}
            disabled={busy !== null}
            data-testid={`profile-select-${p.id}`}
            className="flex flex-col items-center gap-3 group disabled:opacity-50 disabled:cursor-wait cursor-pointer"
          >
            <span
              className="rounded-[var(--radius-md)] bg-[var(--accent)] text-[var(--filter-active-text)] flex items-center justify-center overflow-hidden transition-transform-fast group-hover:scale-105 group-focus:ring-2 group-focus:ring-[var(--accent)]"
              style={{ width: 128, height: 128, fontSize: 48 }}
            >
              {profileAvatarUrl(p) ? (
                <img src={profileAvatarUrl(p)!} alt="" className="w-full h-full object-cover" />
              ) : (
                <span className="font-medium">{profileInitial(p)}</span>
              )}
            </span>
            <span className="text-[var(--text)] text-base">
              {p.name}
              {p.isKids && (
                <span className="ml-2 text-xs text-[var(--text-muted)]">
                  {t("auth.profile.select.kidsBadge", "Kids")}
                </span>
              )}
            </span>
            {busy === p.id && <Spinner size="sm" />}
          </button>
        ))}
      </div>
      <Button
        variant="ghost"
        className="mt-10"
        onClick={() => void navigate({ to: "/profile/settings" })}
        data-testid="profile-select-manage"
      >
        {t("auth.profile.select.manage", "Manage Profiles")}
      </Button>
      {pinTarget && (
        <PinPromptDialog
          target={pinTarget}
          fallback={currentProfile ?? undefined}
          onCancel={() => setPinTarget(null)}
          onVerified={async (target, pinToken) => {
            setPinTarget(null);
            setBusy(target.id ?? null);
            const res = await switchProfile(target, pinToken);
            setBusy(null);
            if (res === "ok") void navigate({ to: "/home" });
            else toast.error(t("common.error"));
          }}
        />
      )}
    </div>
  );
}
