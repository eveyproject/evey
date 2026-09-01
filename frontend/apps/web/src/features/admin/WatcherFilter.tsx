import { useTranslation } from "react-i18next";
import { useUserServiceListUsers, useUserServiceListAllProfiles } from "@evey/api";

// A watcher is either a whole account or one profile of it. Encoding both in a
// single <select> value keeps the two ids the API takes (user_id, profile_id)
// out of the component's callers.
export type Watcher = { userId?: string; profileId?: string };

export function encodeWatcher(w: Watcher): string {
  if (w.profileId) return `profile:${w.profileId}`;
  if (w.userId) return `user:${w.userId}`;
  return "";
}

export function decodeWatcher(value: string): Watcher {
  if (value.startsWith("profile:")) return { profileId: value.slice("profile:".length) };
  if (value.startsWith("user:")) return { userId: value.slice("user:".length) };
  return {};
}

// WatcherFilter answers "whose activity am I looking at?". Empty = everyone,
// which is what an admin sees by default.
export function WatcherFilter({ value, onChange }: { value: Watcher; onChange: (w: Watcher) => void }) {
  const { t } = useTranslation();
  const { data: userData } = useUserServiceListUsers();
  const { data: profileData } = useUserServiceListAllProfiles();
  const users = userData?.users ?? [];
  const profiles = profileData?.profiles ?? [];

  return (
    <label className="flex items-center gap-2 text-sm text-[var(--text-muted)]">
      {t("admin.activity.watcher")}
      <select
        aria-label={t("admin.activity.watcher")}
        value={encodeWatcher(value)}
        onChange={(e) => onChange(decodeWatcher(e.target.value))}
        className="rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface)] px-2 py-1 text-[var(--text)]"
      >
        <option value="">{t("admin.activity.watcherAll")}</option>
        {users.map((u) => (
          <optgroup key={u.id} label={u.username ?? u.id ?? ""}>
            <option value={`user:${u.id}`}>
              {t("admin.activity.watcherWholeAccount", { name: u.username })}
            </option>
            {profiles
              .filter((p) => p.userId === u.id)
              .map((p) => (
                <option key={p.id} value={`profile:${p.id}`}>
                  {p.name}
                </option>
              ))}
          </optgroup>
        ))}
      </select>
    </label>
  );
}
