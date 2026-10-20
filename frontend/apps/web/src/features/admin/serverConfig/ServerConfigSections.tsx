import { type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Folder, TriangleAlert } from "lucide-react";
import { Spinner } from "@/components/ui/Spinner";
import { EmptyState } from "@/components/ui/EmptyState";
import { SettingsGroup } from "@/components/ui/SettingsGroup";
import { FeedConfigCard } from "@/features/admin/FeedConfigCard";
import { ConfigRow, AddEntryRow } from "./ConfigEntriesTable";
import { LocaleSettingsCard } from "./LocaleSettingsCard";
import { MemoryProfileCard } from "./MemoryProfileCard";
import { MkvgoCard } from "./MkvgoCard";
import { BackgroundJobsCard } from "./BackgroundJobsCard";
import { CpuBudgetCard } from "./CpuBudgetCard";
import { ApiKeysCard } from "./ApiKeysCard";
import { SmtpSettingsCard } from "./SmtpSettingsCard";
import { SsoSettingsCard } from "./SsoSettingsCard";
import { ArtworkSettingsCard } from "./ArtworkSettingsCard";
import { TrickplaySettingsCard } from "./TrickplaySettingsCard";
import { IntroDetectionSettingsCard } from "./IntroDetectionSettingsCard";
import { ParserTokensCard } from "./ParserTokensCard";
import { UpdateCheckCard } from "./UpdateCheckCard";
import { ServerSettingsGroup } from "./serverSettings";
import type { ConfigEntry } from "./hooks";

// The sections of the server configuration page, in menu order. Each one
// gathers the settings an admin looks for together, so nobody has to scroll
// a single long list of switches to find one.
export const SERVER_CONFIG_SECTIONS = [
  "general",
  "libraries",
  "services",
  "accounts",
  "performance",
  "data",
  "home",
  "advanced",
] as const;

export type ServerConfigSectionId = (typeof SERVER_CONFIG_SECTIONS)[number];

interface SectionProps {
  entries: ConfigEntry[] | undefined;
  entriesLoading: boolean;
}

function GeneralSection({ entries }: SectionProps) {
  const { t } = useTranslation();
  return (
    <>
      <ServerSettingsGroup
        title={t("admin.settingsLayout.groupServer")}
        fields={["serverName", "logLevel"]}
        saveTestId="server-settings-save-general"
        data-testid="group-server"
      />
      <SettingsGroup title={t("admin.serverConfig.languageRegion")} description={t("admin.serverConfig.languageRegionDesc")} data-testid="group-locale">
        <LocaleSettingsCard entries={entries} />
      </SettingsGroup>
      <SettingsGroup title={t("admin.update.title")} data-testid="group-updates">
        <UpdateCheckCard entries={entries} />
      </SettingsGroup>
    </>
  );
}

function LibrariesSection({ entries }: SectionProps) {
  const { t } = useTranslation();
  return (
    <>
      <ServerSettingsGroup
        title={t("admin.settingsLayout.groupScanning")}
        description={t("admin.settingsLayout.groupScanningDesc")}
        fields={["libraryWatcherEnabled", "periodicScanInterval"]}
        saveTestId="server-settings-save-scanning"
        data-testid="group-scanning"
      />
      <ServerSettingsGroup
        title={t("admin.settingsLayout.groupSkipping")}
        description={t("admin.settingsLayout.groupSkippingDesc")}
        fields={["skipPreviewEnabled", "introAutoSkipEnabled"]}
        saveTestId="server-settings-save-skipping"
        data-testid="group-skipping"
      />
      <SettingsGroup title={t("admin.serverConfig.introProvidersTitle")} data-testid="group-intro-providers">
        <IntroDetectionSettingsCard entries={entries} />
      </SettingsGroup>
      <SettingsGroup title={t("admin.serverConfig.artworkTitle")} data-testid="group-artwork">
        <ArtworkSettingsCard entries={entries} />
      </SettingsGroup>
      <SettingsGroup title={t("admin.serverConfig.trickplayTitle")} description={t("admin.serverConfig.trickplayDesc")} data-testid="group-trickplay">
        <TrickplaySettingsCard entries={entries} />
      </SettingsGroup>
      <SettingsGroup
        title={t("admin.serverConfig.parserTokensTitle")}
        description={t("admin.serverConfig.parserTokensDesc")}
        info={t("admin.serverConfig.parserTokensInfo")}
        data-testid="group-parser"
      >
        <ParserTokensCard entries={entries} />
      </SettingsGroup>
    </>
  );
}

function ServicesSection({ entries }: SectionProps) {
  const { t } = useTranslation();
  return (
    <SettingsGroup title={t("admin.serverConfig.apiKeys.title")} data-testid="group-api-keys">
      <ApiKeysCard entries={entries} />
    </SettingsGroup>
  );
}

function AccountsSection({ entries }: SectionProps) {
  const { t } = useTranslation();
  return (
    <>
      <ServerSettingsGroup
        title={t("admin.settingsLayout.groupSignIn")}
        description={t("admin.settingsLayout.groupSignInDesc")}
        fields={["passwordRecoveryEnabled", "loginRateLimit", "globalRateLimit"]}
        saveTestId="server-settings-save-sign-in"
        data-testid="group-sign-in"
      />
      <SettingsGroup title={t("admin.sso.title")} description={t("admin.sso.subtitle")} data-testid="group-sso">
        <SsoSettingsCard />
      </SettingsGroup>
      <SettingsGroup title={t("admin.serverConfig.emailSmtp")} description={t("admin.serverConfig.emailSmtpDesc")} data-testid="group-smtp">
        <SmtpSettingsCard entries={entries} />
      </SettingsGroup>
    </>
  );
}

function PerformanceSection({ entries }: SectionProps) {
  const { t } = useTranslation();
  return (
    <>
      <p className="text-xs text-[var(--text-muted)]">{t("admin.serverSettings.transcodeMovedHint")}</p>
      <SettingsGroup
        title={t("admin.serverConfig.memoryProfileTitle")}
        titleId="memory-profile-title"
        description={t("admin.serverConfig.memoryProfileSubtitle")}
        info={t("admin.serverConfig.memoryProfileDesc")}
        data-testid="group-memory"
      >
        <MemoryProfileCard entries={entries} labelledBy="memory-profile-title" />
      </SettingsGroup>
      <SettingsGroup title={t("admin.serverConfig.cpuBudget.title")} description={t("admin.serverConfig.cpuBudget.subtitle")} data-testid="group-cpu">
        <CpuBudgetCard entries={entries} />
      </SettingsGroup>
      <SettingsGroup
        title={t("admin.serverConfig.backgroundJobs.title")}
        description={t("admin.serverConfig.backgroundJobs.subtitle")}
        data-testid="group-background"
      >
        <BackgroundJobsCard entries={entries} />
      </SettingsGroup>
      <SettingsGroup
        title={t("admin.settingsLayout.groupMediaAnalysis")}
        description={t("admin.serverConfig.mkvgoSubtitle")}
        data-testid="group-mkvgo"
      >
        <MkvgoCard entries={entries} />
      </SettingsGroup>
    </>
  );
}

function DataSection() {
  const { t } = useTranslation();
  return (
    <>
      <ServerSettingsGroup
        title={t("admin.settingsLayout.groupRetention")}
        description={t("admin.settingsLayout.groupRetentionDesc")}
        fields={["auditRetentionDays", "metricsRetentionDays", "qoeRetentionDays", "playSessionRetentionDays"]}
        saveTestId="server-settings-save-retention"
        data-testid="group-retention"
      />
      <ServerSettingsGroup
        title={t("admin.settingsLayout.groupBackups")}
        fields={["backupIncludeTelemetry"]}
        saveTestId="server-settings-save-backups"
        data-testid="group-backups"
      />
    </>
  );
}

function HomeSection() {
  const { t } = useTranslation();
  return (
    <SettingsGroup
      title={t("admin.serverConfig.feedConfig.title")}
      description={t("admin.serverConfig.feedConfig.subtitle")}
      info={t("admin.settingsHelp.homeFeeds")}
      data-testid="group-feeds"
    >
      <FeedConfigCard />
    </SettingsGroup>
  );
}

function AdvancedSection({ entries, entriesLoading }: SectionProps) {
  const { t } = useTranslation();
  return (
    <SettingsGroup
      title={t("admin.serverConfig.entriesTitle")}
      description={t("admin.serverConfig.subtitle")}
      info={t("admin.settingsHelp.rawEntries")}
      data-testid="group-raw-entries"
    >
      <p className="mb-3 flex items-start gap-2 text-xs font-medium text-[var(--color-warning)]" data-testid="raw-entries-warning">
        <TriangleAlert size={14} className="mt-0.5 shrink-0" aria-hidden="true" />
        {t("admin.settingsLayout.rawEntriesWarning")}
      </p>
      {entriesLoading ? (
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
    </SettingsGroup>
  );
}

const SECTION_BODY: Record<ServerConfigSectionId, (p: SectionProps) => ReactNode> = {
  general: GeneralSection,
  libraries: LibrariesSection,
  services: ServicesSection,
  accounts: AccountsSection,
  performance: PerformanceSection,
  data: DataSection,
  home: HomeSection,
  advanced: AdvancedSection,
};

// ServerConfigSection renders one section: its one-line purpose, then its
// groups of settings.
export function ServerConfigSection({ id, ...props }: SectionProps & { id: ServerConfigSectionId }) {
  const { t } = useTranslation();
  const Body = SECTION_BODY[id];
  return (
    <div className="flex flex-col gap-4" data-testid={`section-${id}`}>
      <p className="text-sm text-[var(--text-muted)]">{t(`admin.settingsLayout.sections.${id}.desc`)}</p>
      <Body {...props} />
    </div>
  );
}
