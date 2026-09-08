import { useEffect, useMemo, useState } from "react";
import { Avatar } from "@/components/ui/Avatar";
import { Radio, Shield, Square } from "lucide-react";
import { useTranslation } from "react-i18next";
import { usePresenceServiceListOnline, useAdminSessionServiceSendCastReceiverCommand, useUserServiceListAllProfiles, apiErrorMessage } from "@evey/api";
import type { V1OnlineUser } from "@evey/api";
import { toast } from "@evey/store";
import { ICON } from "@/constants/iconSizes";
import { deviceIcon, deviceLabel } from "./onlineDevice";
import { FetchErrorState } from "@/components/ui/FetchErrorState";

const REFRESH_INTERVAL = 5_000;

// OnlineNow is a compact dashboard card of who is connected right now (live
// heartbeat presence), sized to sit beside the Now Playing panel.
export function OnlineNow() {
  const { t } = useTranslation();
  const { data, isError, error, refetch } = usePresenceServiceListOnline({
    query: { refetchInterval: REFRESH_INTERVAL, refetchIntervalInBackground: false },
  });

  // "Nobody connected" and "I could not ask" are the same picture on a card an
  // admin reads to spot a session that should not be there. Only claim the
  // failure when there is nothing to show: a poll that fails while a previous
  // answer is still on screen must keep showing that answer, stale but true.
  const failed = isError && !data;

  // A cast receiver keeps beating for as long as its page is loaded, and a
  // television that is never really switched off never unloads it - so it
  // outlives its play session and disappears from Now Playing while still
  // holding the screen. This card is then the only place it can be reached.
  const stopReceiver = useAdminSessionServiceSendCastReceiverCommand({
    mutation: {
      onSuccess: () => toast.success(t("admin.online.receiverStopSent")),
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.online.receiverStopFailed"))),
    },
  });
  // Presence carries the profile ID but not its name, and an admin reading
  // this card wants to know WHO is watching, not a household account. The
  // admin profile list is already fetched by the neighbouring admin screens,
  // so this resolves the name from cache rather than widening the presence
  // payload. Absent (deleted profile, or a client that never sent one) simply
  // shows nothing extra rather than a raw id.
  const { data: profileData } = useUserServiceListAllProfiles();
  const profilesById = useMemo(() => {
    const map = new Map<string, { name?: string; isDefault?: boolean }>();
    for (const p of profileData?.profiles ?? []) {
      if (p.id) map.set(p.id, { name: p.name, isDefault: p.isDefault });
    }
    return map;
  }, [profileData]);

  const users = data?.users ?? [];
  const total = data?.total ?? users.length;

  function lastSeen(u: V1OnlineUser): string {
    const idle = u.idleSeconds ?? 0;
    if (idle < 5) return t("admin.online.justNow");
    if (idle < 60) return t("admin.online.secondsAgo", { count: idle });
    return t("admin.online.minutesAgo", { count: Math.floor(idle / 60) });
  }

  return (
    <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)] overflow-hidden flex flex-col">
      <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)]">
        <h3 className="text-sm font-semibold text-[var(--text)] flex items-center gap-2">
          <Radio size={ICON.md} className="text-[var(--accent)]" />
          {t("admin.online.title")}
        </h3>
        <span className="flex items-center gap-1.5 text-xs font-medium text-[var(--text-muted)]">
          <span className="inline-flex h-2 w-2 rounded-full bg-[var(--success)] shadow-[0_0_6px] shadow-[var(--success)]/70 animate-pulse" />
          {failed
            ? "-"
            : t(total === 1 ? "admin.online.countOne" : "admin.online.countOther", { count: total })}
        </span>
      </div>

      {failed ? (
        <FetchErrorState error={error} onRetry={() => void refetch()} />
      ) : users.length === 0 ? (
        <p className="text-sm text-[var(--text-muted)] px-4 py-6 text-center">{t("admin.online.empty")}</p>
      ) : (
        <ul className="flex flex-col max-h-[22rem] overflow-y-auto divide-y divide-[var(--border)]">
          {users.map((u) => {
            const Icon = deviceIcon(u.userAgent ?? "");
            const dev = deviceLabel(u.userAgent ?? "");
            // The household default profile is named after the account, which
            // says nothing: label it "Default" instead, the same word the
            // profile manager badges it with.
            const profile = u.profileId ? profilesById.get(u.profileId) : undefined;
            const profileLabel = profile
              ? (profile.isDefault ? t("auth.profile.manage.defaultProfileBadge") : profile.name)
              : undefined;
            return (
              <li
                // One row per DEVICE since the tracker keys on (user, user
                // agent): the same account on web + mobile is two rows.
                key={`${u.userId}|${u.userAgent ?? ""}`}
                className="flex items-center gap-3 px-4 py-2.5 hover:bg-[var(--surface-raised)] transition-colors"
              >
                <Avatar initial={(u.username ?? "?").charAt(0).toUpperCase()} size={32} />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5">
                    <span className="font-medium text-sm text-[var(--text)] truncate">{u.username || u.userId}</span>
                    {u.isAdmin && (
                      <Shield size={ICON.xs} className="shrink-0 text-[var(--accent)]" aria-label={t("admin.online.adminBadge")} />
                    )}
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-[var(--text-muted)] min-w-0">
                    <Icon size={ICON.xs} className="shrink-0" />
                    <span className="truncate">
                      {profileLabel ? `${profileLabel} · ` : ""}
                      {dev}
                      {u.clientIp ? ` · ${u.clientIp}` : ""}
                    </span>
                  </div>
                </div>
                <span className="shrink-0 text-xs text-[var(--text-muted)] tabular-nums">{lastSeen(u)}</span>
                {u.isCastReceiver && u.profileId && (
                  <StopReceiverButton
                    profileId={u.profileId}
                    onStop={(profileId) => stopReceiver.mutate({ profileId, data: { command: "stop" } })}
                  />
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

// StopReceiverButton releases a television nobody is watching any more.
//
// Confirm-to-act, like the Now Playing stop: one click arms, a second within a
// few seconds sends it. This ends someone else's playback on a screen the
// operator cannot see, so a single misplaced click must not do it.
//
// Delivery rides the receiver's next beacon, which is every 30s - the button
// reports that it was SENT, never that it was obeyed, because nothing here can
// know the latter.
function StopReceiverButton({ profileId, onStop }: { profileId: string; onStop: (profileId: string) => void }) {
  const { t } = useTranslation();
  const [armed, setArmed] = useState(false);
  useEffect(() => {
    if (!armed) return;
    const timer = setTimeout(() => setArmed(false), 4000);
    return () => clearTimeout(timer);
  }, [armed]);

  return (
    <button
      type="button"
      data-testid="stop-cast-receiver"
      className={`shrink-0 p-1.5 rounded-full transition-colors ${armed ? "bg-[var(--danger-bg,#dc2626)] text-[var(--on-danger,#fff)]" : "text-[var(--danger-bg,#dc2626)] hover:bg-[var(--surface)]"}`}
      onClick={() => {
        if (!armed) {
          setArmed(true);
          return;
        }
        setArmed(false);
        onStop(profileId);
      }}
      title={armed ? t("common.confirm") : t("admin.online.stopReceiver")}
      aria-label={armed ? t("common.confirm") : t("admin.online.stopReceiver")}
    >
      <Square size={14} fill="currentColor" aria-hidden="true" />
    </button>
  );
}
