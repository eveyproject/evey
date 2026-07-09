import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListMediaItems,
} from "@evey/api";
import type { V1DashboardInfo } from "@evey/api";
import {
  StatCard,
  getUsageColor,
  MovieIcon,
  SeriesIcon,
  UsersIcon,
  StreamIcon,
  CpuIcon,
  MemoryIcon,
  StorageIcon,
  ThroughputIcon,
  TheoreticalIcon,
  formatBps,
} from "./DashboardStatCard";
import { NowPlaying } from "./DashboardNowPlaying";
import { QuickActions } from "./DashboardQuickActions";
import { RecentActivity } from "./DashboardRecentActivity";
import { PlayActivityChart, StorageDonut, PeakHours } from "./DashboardCharts";
import { ActiveUsersSummary } from "./DashboardActiveUsers";
import { OnlineNow } from "./DashboardOnline";
import { PopularMedia } from "./DashboardPopularMedia";

interface DashboardStatsProps {
  info: V1DashboardInfo;
}

export function DashboardStats({ info }: DashboardStatsProps) {
  const { t } = useTranslation();
  const { data: moviesData } = useLibraryServiceListMediaItems({
    type: "MEDIA_ITEM_TYPE_MOVIE" as never,
    "pagination.page": 1,
    "pagination.pageSize": 1,
  });
  const { data: seriesData } = useLibraryServiceListMediaItems({
    type: "MEDIA_ITEM_TYPE_SERIES" as never,
    "pagination.page": 1,
    "pagination.pageSize": 1,
  });

  const movieCount = moviesData?.pagination?.totalItems ?? 0;
  const seriesCount = seriesData?.pagination?.totalItems ?? 0;

  return (
    <div className="flex flex-col gap-8">
      <NowPlaying />

      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
        <StatCard icon={<MovieIcon />} label={t("admin.dashboardStats.movies")} value={movieCount} />
        <StatCard icon={<SeriesIcon />} label={t("admin.dashboardStats.series")} value={seriesCount} />
        <StatCard icon={<UsersIcon />} label={t("admin.dashboardStats.users")} value={info.totalUsers ?? 0} />
        <StatCard icon={<StreamIcon />} label={t("admin.dashboardStats.activeStreams")} value={info.activeStreams ?? 0} />
        <StatCard icon={<CpuIcon />} label={t("admin.dashboardStats.cpu")} value={`${(info.cpuPercent ?? 0).toFixed(1)}%`} valueColor={getUsageColor(info.cpuPercent ?? 0)} />
        <StatCard icon={<MemoryIcon />} label={t("admin.dashboardStats.memory")} value={`${(info.memoryPercent ?? 0).toFixed(1)}%`} valueColor={getUsageColor(info.memoryPercent ?? 0)} />
        <StatCard
          icon={<StreamIcon />}
          label={t("admin.dashboardStats.transcodes")}
          value={info.activeTranscodes ?? 0}
        />
        <StatCard
          icon={<StorageIcon />}
          label={t("admin.dashboardStats.totalMedia")}
          value={info.totalMediaItems ?? 0}
        />
        <StatCard
          icon={<ThroughputIcon />}
          label={t("admin.dashboardStats.throughput")}
          value={formatBps(info.streamingBps)}
        />
        <StatCard
          icon={<TheoreticalIcon />}
          label={t("admin.dashboardStats.theoretical")}
          value={formatBps(info.activeSessionsBps)}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <PlayActivityChart />
        <StorageDonut storage={info.storage ?? []} />
        <PeakHours />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
        <OnlineNow />
        <ActiveUsersSummary />
      </div>

      <QuickActions />

      <PopularMedia />
      <RecentActivity />
    </div>
  );
}
