import { type ReactNode } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Switch, Text, View } from "react-native";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useMutation } from "@tanstack/react-query";
import { eveyFetch, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import { latestLabel, useCheckUpdateNow, useUpdateStatus, type UpdateStatus } from "@evey/client";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../../providers/ThemeProvider";
import { SettingRow } from "../settings/SettingRow";
import { SettingsGroup } from "../settings/SettingsGroup";
import { ServerSettingsGroup } from "../settings/ServerSettingsGroup";
import { LocaleSettingsCard } from "./LocaleSettingsCard";
import { MemoryProfileCard } from "./MemoryProfileCard";
import { MkvgoCard } from "./MkvgoCard";
import { SmtpSettingsCard } from "./SmtpSettingsCard";
import { SsoSettingsCard } from "./SsoSettingsCard";
import { BackgroundJobsCard } from "./BackgroundJobsCard";
import { CpuBudgetCard } from "./CpuBudgetCard";
import { ParserTokensCard } from "./ParserTokensCard";
import { ConfigEntriesCard } from "./ConfigEntriesCard";
import { ApiKeysCard } from "./ApiKeysCard";
import type { ConfigEntry, SaveEntry, SaveEntryAsync } from "./types";

// The sections of the server configuration screen, in menu order: the same
// eight as the web page (features/admin/serverConfig/ServerConfigSections),
// each gathering the settings an admin looks for together.
export const SERVER_CONFIG_SECTIONS = ["general", "libraries", "services", "accounts", "performance", "data", "home", "advanced"] as const;

export type ServerConfigSectionId = (typeof SERVER_CONFIG_SECTIONS)[number];

/** The raw config-entry writes the screen owns, handed to every card. */
export interface ConfigEntryOps {
  entries: ConfigEntry[] | undefined;
  getEntryValue: (key: string) => string;
  saveEntry: SaveEntry;
  saveEntryAsync: SaveEntryAsync;
  deleteEntry: (key: string) => void;
  deleteEntryAsync: (key: string) => Promise<unknown>;
  quietSaveEntry: SaveEntryAsync;
  quietDeleteEntry: (key: string) => Promise<unknown>;
  savePending: boolean;
  deletePending: boolean;
  quietPending: boolean;
}

function toggleValue(on: boolean): string {
  return on ? "true" : "false";
}

// EntrySwitch is a SettingRow whose switch writes one "true"/"false" config
// entry at once (no Save): the same gesture as the web's cards.
function EntrySwitch({ ops, entryKey, on, label, info, hint, restart, testID }: {
  ops: ConfigEntryOps;
  entryKey: string;
  on: boolean;
  label: string;
  info: string;
  hint?: string;
  restart?: boolean;
  testID: string;
}) {
  const theme = useTheme();
  return (
    <SettingRow label={label} info={info} hint={hint} restart={restart} kind="toggle" testID={`setting-row-${testID}`}>
      <Switch
        value={on}
        onValueChange={(v) => ops.saveEntry({ key: entryKey, value: toggleValue(v) })}
        disabled={ops.savePending}
        accessibilityLabel={label}
        testID={testID}
        trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
      />
    </SettingRow>
  );
}

function SecondaryButton({ label, onPress, disabled, loading, testID }: { label: string; onPress: () => void; disabled?: boolean; loading?: boolean; testID?: string }) {
  const theme = useTheme();
  const off = disabled || loading;
  return (
    <Pressable
      onPress={onPress}
      disabled={off}
      accessibilityRole="button"
      accessibilityLabel={label}
      testID={testID}
      style={[styles.secondary, { borderColor: theme.colors.border, borderRadius: theme.radii.md, opacity: off ? 0.5 : 1 }]}
    >
      {loading ? (
        <ActivityIndicator size="small" color={theme.colors.text} />
      ) : (
        <Text style={{ color: theme.colors.text, fontSize: FONT_SIZE.xs, fontWeight: "600" }}>{label}</Text>
      )}
    </Pressable>
  );
}

type Translate = ReturnType<typeof useTranslation>["t"];

function updateStatusLine(st: UpdateStatus | undefined, t: Translate): string {
  if (!st) return "";
  switch (st.state) {
    case "update_available":
      return t("admin.update.available", { latest: latestLabel(st) });
    case "up_to_date":
      return t("admin.update.upToDate");
    case "disabled":
      return t("admin.update.disabled");
    case "pending":
      return t("admin.update.pending");
    default:
      return t("admin.update.unknown", { reason: st.reason ?? "" });
  }
}

// UpdateRows: the switch for the periodic "newer image published?" check and
// its last answer. Evey only reads the registry; it never updates itself.
function UpdateRows({ ops }: { ops: ConfigEntryOps }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const { data } = useUpdateStatus();
  const checkNow = useCheckUpdateNow();
  const enabled = ops.getEntryValue("update_check.enabled") !== "false";
  return (
    <>
      <EntrySwitch ops={ops} entryKey="update_check.enabled" on={enabled} label={t("admin.update.enabled")} info={t("admin.update.noAuto")} hint={t("admin.update.desc")} testID="update-check-toggle" />
      <View style={styles.statusLine}>
        <View style={styles.flex}>
          <Text style={{ color: theme.colors.text, fontSize: 13 }} testID="update-status-line">
            {updateStatusLine(data, t)}
          </Text>
          {data?.checkedAt ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
              {t("admin.update.checkedAt", { when: new Date(data.checkedAt).toLocaleString() })}
            </Text>
          ) : null}
        </View>
        <SecondaryButton label={t("admin.update.checkNow")} onPress={() => checkNow.mutate()} disabled={!enabled} loading={checkNow.isPending} />
      </View>
    </>
  );
}

function IntroProviderRows({ ops }: { ops: ConfigEntryOps }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const enabled = ops.getEntryValue("intro_providers.enabled") === "true";
  const tidbKey = ops.getEntryValue("api.tidb_key");
  const submit = ops.getEntryValue("intro_providers.tidb_submit_enabled") === "true";
  return (
    <>
      <EntrySwitch
        ops={ops}
        entryKey="intro_providers.enabled"
        on={enabled}
        label={t("admin.serverConfig.introProvidersEnabled")}
        info={t("admin.serverConfig.introProvidersInfo")}
        hint={t("admin.serverConfig.introProvidersDesc")}
        restart
        testID="intro-providers-toggle"
      />
      {enabled ? (
        <View style={styles.statusBlock}>
          <Text style={{ fontSize: 12, color: theme.colors.textMuted }}>
            {t("admin.serverConfig.introProvidersTidbStatus")}{" "}
            <Text style={{ color: tidbKey ? theme.colors.accent : theme.colors.warning }}>
              {tidbKey ? t("admin.serverConfig.introProvidersTidbAuthed") : t("admin.serverConfig.introProvidersTidbAnonymous")}
            </Text>
          </Text>
          <Text style={{ fontSize: 12, color: theme.colors.textMuted }}>{t("admin.serverConfig.introProvidersIntrodbStatus")}</Text>
        </View>
      ) : null}
      {enabled && tidbKey ? (
        <EntrySwitch
          ops={ops}
          entryKey="intro_providers.tidb_submit_enabled"
          on={submit}
          label={t("admin.serverConfig.introProvidersTidbSubmit")}
          info={t("admin.serverConfig.introProvidersTidbSubmitDesc")}
          testID="intro-tidb-submit-toggle"
        />
      ) : null}
    </>
  );
}

function ArtworkRows({ ops }: { ops: ConfigEntryOps }) {
  const { t } = useTranslation();
  // Default-true: only "false" disables (the server's resolveAutoDownloadDefault).
  const autoDownload = ops.getEntryValue("artwork.auto_download") !== "false";
  const retry = useMutation({
    mutationFn: () => eveyFetch<{ jobId: string; total: number }>({ url: "/api/v1/admin/artworks/retry-missing", method: "POST" }),
    onSuccess: () => toast.success(t("admin.serverConfig.artworkRetryStarted")),
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });
  return (
    <>
      <EntrySwitch
        ops={ops}
        entryKey="artwork.auto_download"
        on={autoDownload}
        label={t("admin.serverConfig.artworkAutoDownload")}
        info={t("admin.serverConfig.artworkInfo")}
        hint={t("admin.serverConfig.artworkDesc")}
        testID="artwork-auto-download-toggle"
      />
      <SettingRow label={t("admin.serverConfig.artworkRetryNow")} info={t("admin.serverConfig.artworkRetryDesc")} kind="toggle" testID="setting-row-artwork-retry">
        <SecondaryButton label={t("admin.settingsLayout.runNow")} onPress={() => retry.mutate()} loading={retry.isPending} testID="artwork-retry-btn" />
      </SettingRow>
    </>
  );
}

interface SectionProps {
  ops: ConfigEntryOps;
}

function GeneralSection({ ops }: SectionProps) {
  const { t } = useTranslation();
  return (
    <>
      <ServerSettingsGroup title={t("admin.settingsLayout.groupServer")} fields={["serverName", "logLevel"]} saveTestID="server-settings-save-general" testID="group-server" />
      <SettingsGroup title={t("admin.serverConfig.languageRegion")} description={t("admin.serverConfig.languageRegionDesc")} testID="group-locale">
        <LocaleSettingsCard getEntryValue={ops.getEntryValue} saveEntry={ops.saveEntry} saving={ops.savePending} />
      </SettingsGroup>
      <SettingsGroup title={t("admin.update.title")} testID="group-updates">
        <UpdateRows ops={ops} />
      </SettingsGroup>
    </>
  );
}

function LibrariesSection({ ops }: SectionProps) {
  const { t } = useTranslation();
  const trickplay = ops.getEntryValue("trickplay.enabled") !== "false";
  return (
    <>
      <ServerSettingsGroup
        title={t("admin.settingsLayout.groupScanning")}
        description={t("admin.settingsLayout.groupScanningDesc")}
        fields={["libraryWatcherEnabled", "periodicScanInterval"]}
        saveTestID="server-settings-save-scanning"
        testID="group-scanning"
      />
      <ServerSettingsGroup
        title={t("admin.settingsLayout.groupSkipping")}
        description={t("admin.settingsLayout.groupSkippingDesc")}
        fields={["skipPreviewEnabled", "introAutoSkipEnabled"]}
        saveTestID="server-settings-save-skipping"
        testID="group-skipping"
      />
      <SettingsGroup title={t("admin.serverConfig.introProvidersTitle")} testID="group-intro-providers">
        <IntroProviderRows ops={ops} />
      </SettingsGroup>
      <SettingsGroup title={t("admin.serverConfig.artworkTitle")} testID="group-artwork">
        <ArtworkRows ops={ops} />
      </SettingsGroup>
      <SettingsGroup title={t("admin.serverConfig.trickplayTitle")} description={t("admin.serverConfig.trickplayDesc")} testID="group-trickplay">
        <EntrySwitch
          ops={ops}
          entryKey="trickplay.enabled"
          on={trickplay}
          label={t("admin.serverConfig.trickplayEnabled")}
          info={t("admin.settingsHelp.trickplay")}
          hint={t("admin.serverConfig.trickplayCpuHint")}
          testID="trickplay-toggle"
        />
      </SettingsGroup>
      <SettingsGroup
        title={t("admin.serverConfig.parserTokensTitle")}
        description={t("admin.serverConfig.parserTokensDesc")}
        info={t("admin.serverConfig.parserTokensInfo")}
        testID="group-parser"
      >
        <ParserTokensCard entries={ops.entries} saveEntry={ops.saveEntry} saving={ops.savePending} />
      </SettingsGroup>
    </>
  );
}

function ServicesSection({ ops }: SectionProps) {
  const { t } = useTranslation();
  return (
    <SettingsGroup title={t("admin.serverConfig.apiKeys.title")} description={t("admin.serverConfig.apiKeys.description")} testID="group-api-keys">
      <ApiKeysCard getEntryValue={ops.getEntryValue} saveEntry={ops.saveEntry} deleteEntry={ops.deleteEntry} saving={ops.savePending || ops.deletePending} />
    </SettingsGroup>
  );
}

function AccountsSection({ ops }: SectionProps) {
  const { t } = useTranslation();
  return (
    <>
      <ServerSettingsGroup
        title={t("admin.settingsLayout.groupSignIn")}
        description={t("admin.settingsLayout.groupSignInDesc")}
        fields={["passwordRecoveryEnabled", "loginRateLimit", "globalRateLimit"]}
        saveTestID="server-settings-save-sign-in"
        testID="group-sign-in"
      />
      <SettingsGroup title={t("admin.sso.title")} description={t("admin.sso.subtitle")} testID="group-sso">
        <SsoSettingsCard />
      </SettingsGroup>
      <SettingsGroup title={t("admin.serverConfig.emailSmtp")} description={t("admin.serverConfig.emailSmtpDesc")} testID="group-smtp">
        <SmtpSettingsCard entries={ops.entries} saveEntry={ops.quietSaveEntry} saving={ops.quietPending} />
      </SettingsGroup>
    </>
  );
}

function PerformanceSection({ ops }: SectionProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  return (
    <>
      <Text style={[styles.note, { color: theme.colors.textMuted }]}>{t("admin.serverSettings.transcodeMovedHint")}</Text>
      <SettingsGroup
        title={t("admin.serverConfig.memoryProfileTitle")}
        description={t("admin.serverConfig.memoryProfileSubtitle")}
        testID="group-memory"
      >
        <MemoryProfileCard getEntryValue={ops.getEntryValue} saveEntry={ops.saveEntry} saving={ops.savePending} />
      </SettingsGroup>
      <SettingsGroup title={t("admin.serverConfig.cpuBudget.title")} description={t("admin.serverConfig.cpuBudget.subtitle")} testID="group-cpu">
        <CpuBudgetCard
          entries={ops.entries}
          saveEntry={ops.saveEntry}
          deleteEntry={ops.deleteEntry}
          saving={ops.savePending || ops.deletePending}
        />
      </SettingsGroup>
      <SettingsGroup title={t("admin.serverConfig.backgroundJobs.title")} description={t("admin.serverConfig.backgroundJobs.subtitle")} testID="group-background">
        <BackgroundJobsCard
          entries={ops.entries}
          saveEntry={ops.saveEntryAsync}
          deleteEntry={ops.deleteEntryAsync}
          quietSaveEntry={ops.quietSaveEntry}
          quietDeleteEntry={ops.quietDeleteEntry}
          saving={ops.savePending || ops.deletePending || ops.quietPending}
        />
      </SettingsGroup>
      <SettingsGroup title={t("admin.settingsLayout.groupMediaAnalysis")} description={t("admin.serverConfig.mkvgoSubtitle")} testID="group-mkvgo">
        <MkvgoCard getEntryValue={ops.getEntryValue} saveEntry={ops.saveEntry} saving={ops.savePending} />
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
        saveTestID="server-settings-save-retention"
        testID="group-retention"
      />
      <ServerSettingsGroup
        title={t("admin.settingsLayout.groupBackups")}
        fields={["backupIncludeTelemetry"]}
        saveTestID="server-settings-save-backups"
        testID="group-backups"
      />
    </>
  );
}

// The home rows have their own screen on the phone (a reorderable list does
// not fit inside a settings card): the section explains them and opens it.
function HomeSection() {
  const { t } = useTranslation();
  const router = useRouter();
  const title = t("admin.serverConfig.feedConfig.title");
  return (
    <SettingsGroup title={title} description={t("admin.serverConfig.feedConfig.subtitle")} testID="group-feeds">
      <SettingRow label={title} info={t("admin.settingsHelp.homeFeeds")} kind="toggle" testID="setting-row-home-feeds">
        <SecondaryButton label={t("common.edit")} onPress={() => router.push("/admin/feeds")} testID="home-feeds-open" />
      </SettingRow>
    </SettingsGroup>
  );
}

function AdvancedSection({ ops }: SectionProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  return (
    <SettingsGroup
      title={t("admin.serverConfig.entriesTitle")}
      description={t("admin.serverConfig.subtitle")}
      info={t("admin.settingsHelp.rawEntries")}
      testID="group-raw-entries"
    >
      <Text style={[styles.warning, { color: theme.colors.warning }]} testID="raw-entries-warning">
        {t("admin.settingsLayout.rawEntriesWarning")}
      </Text>
      <ConfigEntriesCard
        entries={ops.entries}
        saveEntry={ops.saveEntry}
        deleteEntry={ops.deleteEntry}
        saving={ops.savePending || ops.deletePending}
      />
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
export function ServerConfigSection({ id, ops }: SectionProps & { id: ServerConfigSectionId }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const Body = SECTION_BODY[id];
  return (
    <View testID={`section-${id}`}>
      <Text style={[styles.sectionDesc, { color: theme.colors.textMuted }]}>{t(`admin.settingsLayout.sections.${id}.desc`)}</Text>
      <Body ops={ops} />
    </View>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1, minWidth: 0 },
  secondary: { borderWidth: 1, paddingHorizontal: 12, minHeight: 40, alignItems: "center", justifyContent: "center" },
  statusLine: { flexDirection: "row", alignItems: "center", gap: 12, paddingVertical: 12 },
  statusBlock: { paddingVertical: 10, gap: 3 },
  note: { fontSize: 12, marginBottom: 12 },
  warning: { fontSize: 12, fontWeight: "600", paddingVertical: 10 },
  sectionDesc: { fontSize: 13, marginBottom: 12 },
});
