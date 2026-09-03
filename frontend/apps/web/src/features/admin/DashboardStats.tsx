import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListMediaItems,
} from "@evey/api";
import type { V1DashboardInfo } from "@evey/api";
import {
  StatCard,
  getUsageColor,
  StreamIcon,
  CpuIcon,
  MemoryIcon,
  ThroughputIcon,
  formatBps,
} from "./DashboardStatCard";
import { NowPlaying } from "./DashboardNowPlaying";
import { QuickActions } from "./DashboardQuickActions";
import { PlayActivityChart, PeakHours } from "./DashboardCharts";
import { OnlineNow } from "./DashboardOnline";
import { BackgroundTasksPanel } from "./BackgroundTasksPanel";

interface DashboardStatsProps {
  info: V1DashboardInfo;
}

// The dashboard answers "what is happening on my server right now": who
// watches, who is connected, how loaded the box is, what Evey is doing on its
// own, and the shape of the last two weeks. Inventory counts ride a single
// line; storage, server details and the finer metrics live on System Health;
// popular media, active users and the audit trail live on Analytics and
// Activity - each of those used to be repeated here as well.
export function DashboardStats({ info }: DashboardStatsProps) {
  const { t } = useTranslation();
  const { data: moviesData, isError: moviesFailed } = useLibraryServiceListMediaItems({
    type: "MEDIA_ITEM_TYPE_MOVIE" as never,
    "pagination.page": 1,
    "pagination.pageSize": 1,
  });
  const { data: seriesData, isError: seriesFailed } = useLibraryServiceListMediaItems({
    type: "MEDIA_ITEM_TYPE_SERIES" as never,
    "pagination.page": 1,
    "pagination.pageSize": 1,
  });

  // A failed count and an empty library both used to render "0", which is the
  // one number an admin reads as "my library is gone". Same convention as the
  // measured-throughput card: "-" means not known, not zero.
  const movieCount = moviesFailed ? "-" : (moviesData?.pagination?.totalItems ?? 0);
  const seriesCount = seriesFailed ? "-" : (seriesData?.pagination?.totalItems ?? 0);
  const transcodes = info.activeTranscodes ?? 0;

  return (
    <div className="flex flex-col gap-6">
      <p className="text-sm text-[var(--text-muted)]" data-testid="dashboard-inventory">
        {t("admin.dashboardStats.inventory", {
          movies: movieCount,
          series: seriesCount,
          media: info.totalMediaItems ?? 0,
          users: info.totalUsers ?? 0,
        })}
      </p>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 items-start">
        <div className="lg:col-span-2">
          <NowPlaying />
        </div>
        <OnlineNow />
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          icon={<StreamIcon />}
          label={t("admin.dashboardStats.activeStreams")}
          value={info.activeStreams ?? 0}
          sub={transcodes > 0 ? t("admin.dashboardStats.transcodesSub", { count: transcodes }) : undefined}
        />
        <StatCard
          icon={<ThroughputIcon />}
          label={t("admin.dashboardStats.throughput")}
          value={formatBps(info.streamingBps)}
          sub={t("admin.dashboardStats.theoreticalSub", { rate: formatBps(info.activeSessionsBps) })}
        />
        <StatCard icon={<CpuIcon />} label={t("admin.dashboardStats.cpu")} value={`${(info.cpuPercent ?? 0).toFixed(1)}%`} valueColor={getUsageColor(info.cpuPercent ?? 0)} />
        <StatCard icon={<MemoryIcon />} label={t("admin.dashboardStats.memory")} value={`${(info.memoryPercent ?? 0).toFixed(1)}%`} valueColor={getUsageColor(info.memoryPercent ?? 0)} />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 items-start">
        <div className="lg:col-span-2">
          <BackgroundTasksPanel />
        </div>
        <QuickActions />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <PlayActivityChart />
        <PeakHours />
      </div>
    </div>
  );
}
