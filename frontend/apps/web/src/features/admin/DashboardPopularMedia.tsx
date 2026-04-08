import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useActivityServiceGetServerStats } from "@evey/api";

export function PopularMedia() {
  const { t } = useTranslation();
  const { data } = useActivityServiceGetServerStats({ popularLimit: 5 });
  const popular = data?.popularMedia ?? [];
  if (popular.length === 0) return null;

  return (
    <div>
      <h3 className="text-sm font-semibold mb-3 text-[var(--text)]">
        {t("admin.dashboardStats.popularMedia")}
      </h3>
      <div className="flex flex-col gap-1">
        {popular.map((m, i) => (
          <Link
            key={m.mediaItemId ?? i}
            to="/media/$mediaId"
            params={{ mediaId: m.mediaItemId ?? "" }}
            className="flex items-center gap-3 px-3 py-2 rounded-[var(--radius-md)] hover:bg-[var(--surface-raised)] transition-colors-fast"
          >
            <span className="text-sm font-mono w-6 text-center text-[var(--accent)]">
              {i + 1}
            </span>
            <span className="text-sm flex-1 truncate text-[var(--text)]">
              {m.title ?? "Unknown"}
            </span>
            <span className="text-xs text-[var(--text-muted)]">
              {t("admin.activity.plays", { count: m.playCount ?? 0 })}
            </span>
            {m.uniqueViewers != null && m.uniqueViewers > 0 && (
              <span className="text-xs text-[var(--text-muted)]">
                {t("admin.dashboardStats.viewers", { count: m.uniqueViewers })}
              </span>
            )}
          </Link>
        ))}
      </div>
    </div>
  );
}
