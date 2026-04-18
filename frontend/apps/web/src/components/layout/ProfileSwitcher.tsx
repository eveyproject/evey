import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@evey/store";
import { userServiceRefreshToken } from "@evey/api";
import type { V1Profile } from "@evey/api";
import { Users, Check } from "lucide-react";
import { PinPromptDialog } from "@/features/profile/PinPromptDialog";

function profileAvatarUrl(p: V1Profile): string | null {
  return p.avatarPath && p.id ? `/api/v1/profiles/${p.id}/avatar` : null;
}

function profileInitial(p: V1Profile): string {
  return (p.name ?? "?").charAt(0).toUpperCase();
}

function ProfileAvatar({ profile, size = 32 }: { profile: V1Profile; size?: number }) {
  const url = profileAvatarUrl(profile);
  return (
    <span
      className="rounded-full bg-[var(--accent)] text-[var(--filter-active-text)] flex items-center justify-center overflow-hidden shrink-0"
      style={{ width: size, height: size, fontSize: size * 0.45 }}
    >
      {url ? (
        <img src={url} alt="" className="w-full h-full object-cover" />
      ) : (
        <span className="font-medium">{profileInitial(profile)}</span>
      )}
    </span>
  );
}

interface Props {
  profiles: V1Profile[];
  activeProfileId: string | null;
}

export function ProfileSwitcher({ profiles, activeProfileId }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [switching, setSwitching] = useState<string | null>(null);
  const [pinTarget, setPinTarget] = useState<V1Profile | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onMouseDown = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onMouseDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onMouseDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const active = profiles.find((p) => p.id === activeProfileId)
    ?? profiles.find((p) => p.isDefault)
    ?? profiles[0];

  const others = profiles.filter((p) => p.id && p.id !== active?.id);

  const doSwitch = useCallback(async (target: V1Profile) => {
    if (!target.id) return;
    setSwitching(target.id);
    try {
      useAuthStore.getState().setProfile(target.id);
      try {
        localStorage.setItem("evey:last_profile_id", target.id);
      } catch { /* ignore storage errors */ }
      const resp = await userServiceRefreshToken({});
      if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      await qc.invalidateQueries();
      setOpen(false);
    } catch (err) {
      console.warn("profile switch failed", err);
    } finally {
      setSwitching(null);
    }
  }, [qc]);

  const handleSwitch = useCallback(async (target: V1Profile) => {
    if (!target.id || switching) return;
    const targetIsKids = target.isKids === true;
    const currentIsKids = active?.isKids === true;
    if (currentIsKids && !targetIsKids) {
      setPinTarget(target);
      return;
    }
    await doSwitch(target);
  }, [active, doSwitch, switching]);

  const handleManage = useCallback(() => {
    setOpen(false);
    void navigate({ to: "/profile/manage" });
  }, [navigate]);

  if (!active) return null;

  return (
    <div ref={menuRef} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-8 h-8 rounded-full bg-[var(--accent)] flex items-center justify-center text-[var(--filter-active-text)] text-sm font-medium cursor-pointer hover:opacity-90 transition-opacity-fast overflow-hidden"
        aria-label={t("common.nav.profileSwitcher", "Switch profile")}
        aria-expanded={open}
        aria-haspopup="true"
        data-testid="profile-switcher-btn"
      >
        {profileAvatarUrl(active) ? (
          <img src={profileAvatarUrl(active)!} alt="" className="w-full h-full object-cover" />
        ) : (
          profileInitial(active)
        )}
      </button>
      {open && (
        <div
          className="absolute right-0 top-full mt-2 w-64 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] shadow-lg py-1 z-overlay animate-fade-in"
          role="menu"
          data-testid="profile-switcher-menu"
        >
          <div className="px-3 py-2 flex items-center gap-3">
            <ProfileAvatar profile={active} size={36} />
            <div className="flex flex-col min-w-0">
              <span className="text-sm text-[var(--text)] truncate">
                {active.name}
                <span className="text-[var(--text-muted)] ml-1">
                  ({t("common.you", "You")})
                </span>
              </span>
            </div>
            <Check size={16} className="ml-auto text-[var(--accent)] shrink-0" />
          </div>
          {others.length > 0 && (
            <>
              <div className="border-t border-[var(--border)] my-1" />
              {others.map((p) => (
                <button
                  key={p.id}
                  onClick={() => void handleSwitch(p)}
                  disabled={switching !== null}
                  role="menuitem"
                  data-testid={`profile-switch-${p.id}`}
                  className="w-full flex items-center gap-3 px-3 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface)] transition-colors-fast cursor-pointer disabled:opacity-50 disabled:cursor-wait"
                >
                  <ProfileAvatar profile={p} size={28} />
                  <span className="truncate">{p.name}</span>
                  {switching === p.id && (
                    <span className="ml-auto text-xs text-[var(--text-muted)]">
                      {t("common.loading", "Loading...")}
                    </span>
                  )}
                </button>
              ))}
            </>
          )}
          <div className="border-t border-[var(--border)] my-1" />
          <button
            onClick={handleManage}
            role="menuitem"
            data-testid="profile-manage-btn"
            className="w-full flex items-center gap-3 px-3 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface)] transition-colors-fast cursor-pointer"
          >
            <Users size={16} className="text-[var(--text-muted)] shrink-0" />
            {t("common.nav.manageProfiles", "Manage Profiles")}
          </button>
        </div>
      )}
      {pinTarget && (
        <PinPromptDialog
          target={pinTarget}
          onCancel={() => setPinTarget(null)}
          onVerified={async (target) => {
            setPinTarget(null);
            await doSwitch(target);
          }}
        />
      )}
    </div>
  );
}
