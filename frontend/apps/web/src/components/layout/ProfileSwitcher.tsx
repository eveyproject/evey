import { Avatar } from "@/components/ui/Avatar";
import { useCallback, useEffect, useRef, useState } from "react";
import type React from "react";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useAuthStore, toast } from "@evey/store";
import { userServiceRefreshToken, userServiceLogout, syncPlayServiceJoinByCode, normalizeRoomCode, ROOM_CODE_LENGTH } from "@evey/api";
import type { V1Profile } from "@evey/api";
import { Users, Check, Settings, Clock, BarChart3, QrCode, LayoutList, LogOut, ArrowLeftRight } from "lucide-react";
import { PinPromptDialog } from "@/features/profile/PinPromptDialog";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";

// Accept undefined: `active` can be undefined when the profiles list is empty
// (loading window, or a transient 0-profile state), and the avatar button calls
// these directly - without the guard that would crash every authenticated page.
function profileAvatarUrl(p: V1Profile | undefined): string | null {
  return p?.avatarPath && p.id ? `/api/v1/profiles/${p.id}/avatar` : null;
}

function profileInitial(p: V1Profile | undefined): string {
  return (p?.name ?? "?").charAt(0).toUpperCase();
}

function ProfileAvatar({ profile, size = 32 }: { profile: V1Profile; size?: number }) {
  return <Avatar initial={profileInitial(profile)} imageUrl={profileAvatarUrl(profile)} size={size} />;
}

interface Props {
  profiles: V1Profile[];
  activeProfileId: string | null;
}

export function ProfileSwitcher({ profiles, activeProfileId }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const logout = useAuthStore((s) => s.logout);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const [open, setOpen] = useState(false);
  const [switchOpen, setSwitchOpen] = useState(false);
  const [switching, setSwitching] = useState<string | null>(null);
  const [pinTarget, setPinTarget] = useState<V1Profile | null>(null);
  const [joinOpen, setJoinOpen] = useState(false);
  const [joinCode, setJoinCode] = useState("");
  const [joining, setJoining] = useState(false);
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

  const doSwitch = useCallback(async (target: V1Profile, pinToken?: string) => {
    if (!target.id) return;
    setSwitching(target.id);
    try {
      const resp = await userServiceRefreshToken({ profileId: target.id, pinToken });
      if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      try {
        localStorage.setItem("evey:last_profile_id", target.id);
      } catch { /* ignore storage errors */ }
      // Drop the whole cache, like logout: NONE of the previous profile's
      // cached data is valid for the new one. invalidateQueries (refetchType
      // "active") only refreshed the mounted page and left every other page's
      // data stale, so navigating right after a switch briefly showed the
      // PREVIOUS profile's content - including, when switching into a kids
      // profile, titles the parental filter should hide. clear() guarantees
      // the new profile never sees the old one's cached content.
      qc.clear();
      setOpen(false);
      setSwitchOpen(false);
    } catch (err) {
      // The server refuses the switch until a parental PIN proof is supplied
      // (gated SOURCE profile with a PIN, or PIN-carrying target): surface the
      // PIN prompt instead of a raw error. The refresh token is deliberately
      // not consumed server-side, so the retry with a pinToken works.
      if (!pinToken && (err as { httpStatus?: number }).httpStatus === 403) {
        setPinTarget(target);
        return;
      }
      console.warn("profile switch failed", err);
      toast.error(err instanceof Error ? err.message : t("auth.profile.switchFailed", "Could not switch profile"));
    } finally {
      setSwitching(null);
    }
  }, [qc, t]);

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
    void navigate({ to: "/profile/settings" });
  }, [navigate]);

  const go = useCallback((to: string) => {
    setOpen(false);
    void navigate({ to });
  }, [navigate]);

  const handleLogout = useCallback(() => {
    setOpen(false);
    qc.clear();
    logout();
    localStorage.removeItem("evey-recent-searches");
    void userServiceLogout({}).catch((err) => console.warn("logout failed", err));
    void navigate({ to: "/login" });
  }, [qc, logout, navigate]);

  const handleJoinRoom = useCallback(async () => {
    const code = joinCode.trim();
    if (code.length < 4) return;
    setJoining(true);
    try {
      const info = await syncPlayServiceJoinByCode({ code });
      setJoinOpen(false);
      setJoinCode("");
      if (info.mediaType === "music") {
        void navigate({ to: "/syncplay" });
      } else if (info.mediaId) {
        void navigate({ to: "/player/$mediaId", params: { mediaId: info.mediaId }, search: { room: info.roomId } });
      }
    } catch {
      toast.error(t("media.syncplay.joinError", "Could not join - check the code"));
    } finally {
      setJoining(false);
    }
  }, [joinCode, navigate, t]);

  type UserMenuItem = { label: string; icon: React.ReactNode; action: () => void; testid: string };
  const userItems: UserMenuItem[] = [
    { label: t("common.nav.settings"), icon: <Settings size={16} />, action: () => go("/profile/settings"), testid: "topbar-user-menu-settings" },
    { label: t("common.nav.watchHistory"), icon: <Clock size={16} />, action: () => go("/profile/history"), testid: "topbar-user-menu-history" },
    { label: t("common.nav.myStats"), icon: <BarChart3 size={16} />, action: () => go("/profile/stats"), testid: "topbar-user-menu-stats" },
    { label: t("common.nav.quickConnect"), icon: <QrCode size={16} />, action: () => go("/quickconnect"), testid: "topbar-user-menu-quickconnect" },
    { label: t("common.nav.joinRoom", "Join a room"), icon: <Users size={16} />, action: () => { setOpen(false); setJoinOpen(true); }, testid: "topbar-user-menu-joinroom" },
  ];
  if (isAdmin) {
    userItems.push({ label: t("common.nav.admin"), icon: <LayoutList size={16} />, action: () => go("/admin"), testid: "topbar-user-menu-admin" });
  }

  if (!active) return null;

  return (
    <div ref={menuRef} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="tap-target flex items-center justify-center cursor-pointer hover:opacity-90 transition-opacity-fast"
        aria-label={t("common.nav.userMenu", "User menu")}
        aria-expanded={open}
        aria-haspopup="true"
        data-testid="profile-switcher-btn"
      >
        <span className="w-8 h-8 rounded-full bg-[var(--accent)] flex items-center justify-center text-[var(--filter-active-text)] text-sm font-medium overflow-hidden">
          {profileAvatarUrl(active) ? (
            <img src={profileAvatarUrl(active)!} alt="" className="w-full h-full object-cover" />
          ) : (
            profileInitial(active)
          )}
        </span>
      </button>
      {open && (
        <div
          className="absolute right-0 top-full mt-2 w-56 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] shadow-lg py-1 z-overlay animate-fade-in"
          role="menu"
          data-testid="profile-switcher-menu"
        >
          {others.length > 0 && (
            <>
              <button
                onClick={() => {
                  setOpen(false);
                  setSwitchOpen(true);
                }}
                role="menuitem"
                data-testid="profile-switcher-switch"
                className="w-full flex items-center gap-3 px-3 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface)] transition-colors-fast cursor-pointer"
              >
                <ArrowLeftRight size={16} className="text-[var(--text-muted)] shrink-0" />
                {t("common.nav.switchProfile", "Switch profile")}
              </button>
              <div className="border-t border-[var(--border)] my-1" />
            </>
          )}
          <button
            onClick={handleManage}
            role="menuitem"
            data-testid="profile-switcher-manage"
            className="w-full flex items-center gap-3 px-3 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface)] transition-colors-fast cursor-pointer"
          >
            <Users size={16} className="text-[var(--text-muted)] shrink-0" />
            {t("common.nav.manageProfiles", "Manage Profiles")}
          </button>
          <div className="border-t border-[var(--border)] my-1" />
          {userItems.map((item) => (
            <button
              key={item.label}
              onClick={item.action}
              role="menuitem"
              data-testid={item.testid}
              className="w-full flex items-center gap-3 px-3 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface)] transition-colors-fast cursor-pointer"
            >
              <span className="text-[var(--text-muted)] shrink-0">{item.icon}</span>
              {item.label}
            </button>
          ))}
          <div className="border-t border-[var(--border)] my-1" />
          <button
            onClick={handleLogout}
            role="menuitem"
            data-testid="logout-btn"
            className="w-full flex items-center gap-3 px-3 py-2 text-sm text-[var(--color-danger)] hover:bg-[var(--surface)] transition-colors-fast cursor-pointer"
          >
            <LogOut size={16} className="shrink-0" />
            {t("common.nav.signOut")}
          </button>
        </div>
      )}
      {switchOpen && (
        <Modal
          open
          onClose={() => setSwitchOpen(false)}
          title={t("common.nav.switchProfile", "Switch profile")}
        >
          <ul className="flex flex-col gap-1" data-testid="profile-switcher-modal-list">
            {profiles.map((p) => {
              const isActive = p.id === active.id;
              return (
                <li key={p.id}>
                  <button
                    onClick={() => {
                      if (isActive) return;
                      void handleSwitch(p);
                    }}
                    disabled={switching !== null || isActive}
                    data-testid={`profile-switcher-item-${p.id}`}
                    className="w-full flex items-center gap-3 px-3 py-2 rounded-[var(--radius-sm)] text-sm text-[var(--text)] hover:bg-[var(--surface)] transition-colors-fast cursor-pointer disabled:cursor-default"
                  >
                    <ProfileAvatar profile={p} size={36} />
                    <span className="truncate flex-1 text-left">{p.name}</span>
                    {isActive && <Check size={16} className="text-[var(--accent)] shrink-0" />}
                    {switching === p.id && (
                      <span className="text-xs text-[var(--text-muted)]">
                        {t("common.loading", "Loading...")}
                      </span>
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
        </Modal>
      )}
      {pinTarget && (
        <PinPromptDialog
          target={pinTarget}
          fallback={active ?? undefined}
          onCancel={() => setPinTarget(null)}
          onVerified={async (target, pinToken) => {
            setPinTarget(null);
            await doSwitch(target, pinToken);
          }}
        />
      )}
      {joinOpen && (
        <Modal open onClose={() => setJoinOpen(false)} title={t("common.nav.joinRoom", "Join a room")}>
          <form onSubmit={(e) => { e.preventDefault(); void handleJoinRoom(); }} className="flex flex-col gap-4">
            <Input
              label={t("media.syncplay.codeLabel", "Code")}
              value={joinCode}
              onChange={(e) => setJoinCode(normalizeRoomCode(e.target.value).slice(0, ROOM_CODE_LENGTH))}
              placeholder="ABC123"
              autoCapitalize="characters"
              autoComplete="off"
              autoFocus
              className="font-mono tracking-widest text-center text-lg"
            />
            <div className="flex justify-end gap-2">
              <Button variant="ghost" type="button" onClick={() => setJoinOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" loading={joining} disabled={joinCode.trim().length < 4}>
                {t("media.syncplay.joinRoom", "Join")}
              </Button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}
