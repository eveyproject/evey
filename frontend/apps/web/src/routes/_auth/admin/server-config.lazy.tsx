import { Folder } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Spinner } from "@/components/ui/Spinner";
import { EmptyState } from "@/components/ui/EmptyState";
import { PageHeader } from "@/components/ui/PageHeader";
import { CollapsibleCard } from "@/components/ui/CollapsibleCard";
import { FeedConfigCard } from "@/features/admin/FeedConfigCard";
import { useServerConfig } from "@/features/admin/serverConfig/hooks";
import { ConfigRow, AddEntryRow } from "@/features/admin/serverConfig/ConfigEntriesTable";
import { LocaleSettingsCard } from "@/features/admin/serverConfig/LocaleSettingsCard";
import { MemoryProfileCard } from "@/features/admin/serverConfig/MemoryProfileCard";
import { MkvgoCard } from "@/features/admin/serverConfig/MkvgoCard";
import { BackgroundJobsCard } from "@/features/admin/serverConfig/BackgroundJobsCard";
import { CpuBudgetCard } from "@/features/admin/serverConfig/CpuBudgetCard";
import { ApiKeysCard } from "@/features/admin/serverConfig/ApiKeysCard";
import { SmtpSettingsCard } from "@/features/admin/serverConfig/SmtpSettingsCard";
import { ArtworkSettingsCard } from "@/features/admin/serverConfig/ArtworkSettingsCard";
import { TrickplaySettingsCard } from "@/features/admin/serverConfig/TrickplaySettingsCard";
import { IntroDetectionSettingsCard } from "@/features/admin/serverConfig/IntroDetectionSettingsCard";
import { ParserTokensCard } from "@/features/admin/serverConfig/ParserTokensCard";
import { ServerSettingsCard } from "@/features/admin/serverConfig/ServerSettingsCard";

export const Route = createLazyFileRoute("/_auth/admin/server-config")({
  component: ServerConfigPage,
});

function ServerConfigPage() {
  const { t } = useTranslation();
  const { data: entries, isLoading } = useServerConfig();

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.serverConfig.title")} subtitle={t("admin.serverConfig.subtitle")} />

      <CollapsibleCard
        title={t("admin.serverSettings.title")}
        alwaysOpen
      >
        <ServerSettingsCard />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.languageRegion")}
        subtitle={t("admin.serverConfig.languageRegionDesc")}
      >
        <LocaleSettingsCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.memoryProfileTitle")}
        subtitle={t("admin.serverConfig.memoryProfileSubtitle")}
      >
        <MemoryProfileCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.mkvgoTitle")}
        subtitle={t("admin.serverConfig.mkvgoSubtitle")}
      >
        <MkvgoCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.cpuBudget.title")}
        subtitle={t("admin.serverConfig.cpuBudget.subtitle")}
      >
        <CpuBudgetCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.backgroundJobs.title")}
        subtitle={t("admin.serverConfig.backgroundJobs.subtitle")}
      >
        <BackgroundJobsCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.feedConfig.title")}
        subtitle={t("admin.serverConfig.feedConfig.subtitle")}
      >
        <FeedConfigCard />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.apiKeys.title")}
        subtitle={t("admin.serverConfig.apiKeys.description")}
      >
        <ApiKeysCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.emailSmtp")}
        subtitle={t("admin.serverConfig.emailSmtpDesc")}
      >
        <SmtpSettingsCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.artworkTitle")}
        subtitle={t("admin.serverConfig.artworkDesc")}
        info={t("admin.serverConfig.artworkInfo")}
      >
        <ArtworkSettingsCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.trickplayTitle")}
        subtitle={t("admin.serverConfig.trickplayDesc")}
      >
        <TrickplaySettingsCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.introProvidersTitle")}
        subtitle={t("admin.serverConfig.introProvidersDesc")}
        info={t("admin.serverConfig.introProvidersInfo")}
      >
        <IntroDetectionSettingsCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.parserTokensTitle", "Parser tokens")}
        subtitle={t(
          "admin.serverConfig.parserTokensDesc",
          "Extra release tags the filename parser recognises beyond its baseline list.",
        )}
        info={t("admin.serverConfig.parserTokensInfo")}
      >
        <ParserTokensCard entries={entries} />
      </CollapsibleCard>

      {isLoading ? (
        <div className="flex justify-center py-16"><Spinner size="lg" /></div>
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]">
          <table className="w-full text-sm table-cards">
            <thead>
              <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
                {[t("admin.serverConfig.tableHeaders.key"), t("admin.serverConfig.tableHeaders.value"), t("admin.serverConfig.tableHeaders.updated"), t("admin.serverConfig.tableHeaders.actions")].map((h) => (
                  <th key={h} className="px-4 py-2 text-left font-medium text-[var(--text-muted)]">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {entries && entries.length === 0 ? (
                <tr>
                  <td colSpan={4}>
                    <EmptyState icon={<Folder size={48} strokeWidth={1.5} />} title={t("admin.serverConfig.noEntries")} description={t("admin.serverConfig.noEntriesDesc")} />
                  </td>
                </tr>
              ) : (
                entries?.map((e) => <ConfigRow key={e.key} entry={e} />)
              )}
              <AddEntryRow />
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
