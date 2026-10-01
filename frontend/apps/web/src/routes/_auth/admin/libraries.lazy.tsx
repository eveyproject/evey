import { useState, useEffect } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQueryClient, useQuery, useMutation } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceDeleteLibrary,
  deleteLibraryUnbounded,
  useLibraryServiceScanLibrary,
  eveyFetch,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
import { scanFailedPaths } from "@evey/client";
import type { V1Library } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { PageHeader } from "@/components/ui/PageHeader";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { Input } from "@/components/ui/Input";
import { Checkbox } from "@/components/ui/Checkbox";
import { LibraryForm } from "@/features/admin/LibraryForm";
import { useDemoLock } from "@/hooks/useDemoMode";

export const Route = createLazyFileRoute("/_auth/admin/libraries")({
  component: LibrariesPage,
});

function libraryTypeLabel(type: string | undefined, t: (key: string) => string): string {
  switch (type) {
    case "LIBRARY_TYPE_MOVIES": return t("admin.libraries.types.movies");
    case "LIBRARY_TYPE_SHOWS": return t("admin.libraries.types.shows");
    case "LIBRARY_TYPE_MUSIC": return t("admin.libraries.types.music");
    case "LIBRARY_TYPE_PHOTOS": return t("admin.libraries.types.photos");
    default: return t("admin.libraries.types.library");
  }
}

// Only the movie/show enrichment chain has an external provider (TMDB) to
// re-match against; music artwork is fetched by its own scan-time pipeline and
// photo libraries carry EXIF only, so the match buttons would lie there.
function supportsProviderMatching(type: string | undefined): boolean {
  return type === "LIBRARY_TYPE_MOVIES" || type === "LIBRARY_TYPE_SHOWS";
}

function formatDate(iso?: string): string {
  if (!iso) return "-";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

interface LibraryStat {
  id: string;
  lastScannedAt?: string | null;
  itemCount?: number;
}

interface ScanSchedule {
  libraryId: string;
  cron: string;
  enabled: boolean;
  lastRunAt?: string | null;
  nextRunAt?: string | null;
}

function useLibraryStats() {
  return useQuery<LibraryStat[]>({
    queryKey: ["/api/v1/admin/library-stats"],
    queryFn: async () => {
      const json = await eveyFetch<{ stats: LibraryStat[] }>({ url: "/api/v1/admin/library-stats", method: "GET" });
      return json.stats ?? [];
    },
    staleTime: 30_000,
  });
}

function useSchedule(libraryId: string) {
  return useQuery<ScanSchedule | null>({
    queryKey: ["/api/v1/libraries", libraryId, "schedule"],
    queryFn: async () => {
      const data = await eveyFetch<{ schedule: ScanSchedule | null }>({
        url: `/api/v1/libraries/${libraryId}/schedule`,
        method: "GET",
      });
      return data.schedule;
    },
    staleTime: 30_000,
  });
}

function ScheduleModal({ library, onClose }: { library: V1Library; onClose: () => void }) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const queryClient = useQueryClient();
  const { data: schedule, isLoading } = useSchedule(library.id!);
  const [preset, setPreset] = useState("");
  const [customCron, setCustomCron] = useState("");
  const [enabled, setEnabled] = useState(true);

  const PRESET_SCHEDULES = [
    { value: "0 */6 * * *", label: t("admin.libraries.presets.every6h") },
    { value: "0 */12 * * *", label: t("admin.libraries.presets.every12h") },
    { value: "0 3 * * *", label: t("admin.libraries.presets.daily3am") },
    { value: "0 3 * * 0", label: t("admin.libraries.presets.weekly3am") },
    { value: "custom", label: t("admin.libraries.presets.customCron") },
  ];

  useEffect(() => {
    if (schedule) {
      setEnabled(schedule.enabled);
      const match = PRESET_SCHEDULES.find((p) => p.value === schedule.cron);
      if (match) {
        setPreset(schedule.cron);
      } else {
        setPreset("custom");
        setCustomCron(schedule.cron);
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- PRESET_SCHEDULES uses t() so its identity changes every render; only sync when schedule prop changes
  }, [schedule]);

  const cronValue = preset === "custom" ? customCron : preset;

  const saveMutation = useMutation({
    mutationFn: async () => {
      await eveyFetch<{ schedule: ScanSchedule }>({
        url: `/api/v1/libraries/${library.id}/schedule`,
        method: "PUT",
        data: { cron: cronValue, enabled },
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries", library.id, "schedule"] });
      toast.success(t("admin.libraries.scheduleSaved"));
      onClose();
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.libraries.failedSchedule"))),
  });

  const deleteMutation = useMutation({
    mutationFn: async () => {
      await eveyFetch<void>({
        url: `/api/v1/libraries/${library.id}/schedule`,
        method: "DELETE",
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries", library.id, "schedule"] });
      toast.success(t("admin.libraries.scheduleRemoved"));
      onClose();
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.libraries.failedRemoveSchedule"))),
  });

  if (isLoading) {
    return <div className="flex justify-center py-8"><Spinner size="md" /></div>;
  }

  return (
    <div className="flex flex-col gap-4">
      <Select
        label={t("admin.libraries.scanFrequency")}
        value={preset}
        onChange={(v) => {
          setPreset(v);
          if (v !== "custom") setCustomCron("");
        }}
        placeholder={t("admin.libraries.selectFrequency")}
        options={PRESET_SCHEDULES}
      />

      {preset === "custom" && (
        <Input
          label={t("admin.libraries.cronExpression")}
          placeholder="0 */6 * * *"
          value={customCron}
          onChange={(e) => setCustomCron(e.target.value)}
        />
      )}

      <Checkbox
        label={t("common.enabled")}
        checked={enabled}
        onChange={setEnabled}
      />

      {schedule && (
        <div className="text-xs space-y-1 text-[var(--text-muted)]">
          {schedule.lastRunAt && <p>{t("admin.libraries.lastRun", { date: formatDate(schedule.lastRunAt) })}</p>}
          {schedule.nextRunAt && <p>{t("admin.libraries.nextRun", { date: formatDate(schedule.nextRunAt) })}</p>}
        </div>
      )}

      <div className="flex justify-between pt-2">
        <div>
          {schedule && (
            <Button
              variant="danger"
              loading={deleteMutation.isPending}
              onClick={() => deleteMutation.mutate()}
              {...lock}
            >
              {t("common.remove")}
            </Button>
          )}
        </div>
        <div className="flex gap-2">
          <Button variant="ghost" onClick={onClose}>{t("common.cancel")}</Button>
          <Button
            loading={saveMutation.isPending}
            disabled={!cronValue}
            onClick={() => saveMutation.mutate()}
            {...lock}
          >
            {t("common.save")}
          </Button>
        </div>
      </div>
    </div>
  );
}

function LibraryRow({ library, stats }: { library: V1Library; stats: LibraryStat[] }) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const queryClient = useQueryClient();
  const [scanDone, setScanDone] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showSchedule, setShowSchedule] = useState(false);
  const [showEdit, setShowEdit] = useState(false);
  const { data: schedule } = useSchedule(library.id!);

  const deleteLib = useLibraryServiceDeleteLibrary({
    mutation: {
      // The cascade runs inside the request: no fetcher timeout on it.
      mutationFn: ({ id }) => deleteLibraryUnbounded(id),
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] });
        queryClient.invalidateQueries({ queryKey: ["/api/v1/admin/library-stats"] });
        toast.success(t("admin.libraries.libraryDeleted", { name: library.name }));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.libraries.failedDelete"))),
    },
  });

  const scanLib = useLibraryServiceScanLibrary({
    mutation: {
      onSuccess: (data) => {
        setScanDone(true);
        queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] });
        queryClient.invalidateQueries({ queryKey: ["/api/v1/media"] });
        queryClient.invalidateQueries({ queryKey: ["/api/v1/admin/library-stats"] });
        const failedPaths = scanFailedPaths(data);
        if (failedPaths.length > 0) {
          toast.warning(t("admin.libraries.scanPartial", { paths: failedPaths.join(", ") }));
          return;
        }
        toast.success(t("admin.libraries.scanStarted"));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.libraries.failedScan"))),
    },
  });

  const refreshMetadata = useMutation({
    mutationFn: () =>
      eveyFetch({
        url: "/api/v1/admin/media/refresh-metadata",
        method: "POST",
        data: { libraryId: library.id },
      }),
    onSuccess: () => toast.success(t("admin.libraries.refreshAllMetadataStarted")),
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  const matchOrphans = useMutation<{ queued?: number }, Error>({
    mutationFn: () =>
      eveyFetch({
        url: "/api/v1/admin/media/match-orphans",
        method: "POST",
        data: { libraryId: library.id },
      }),
    onSuccess: (resp) => {
      const queued = resp?.queued ?? 0;
      if (queued === 0) {
        toast.info(t("admin.libraries.matchOrphansEmpty"));
      } else {
        toast.success(t("admin.libraries.matchOrphansStarted", { count: queued }));
      }
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  const libStat = stats.find((s) => s.id === library.id);
  const totalItems = libStat?.itemCount;

  return (
    <tr className="border-b border-[var(--border)]">
      <td className="px-4 py-3 text-[var(--text)]" data-label={t("admin.libraries.tableHeaders.name")}>
        {library.name}
      </td>
      <td className="px-4 py-3" data-label={t("admin.libraries.tableHeaders.type")}>
        <Badge variant="info">{libraryTypeLabel(library.type, t)}</Badge>
      </td>
      <td className="px-4 py-3 text-sm text-[var(--text-muted)]" data-label={t("admin.libraries.tableHeaders.paths")}>
        {library.paths?.join(", ") ?? "-"}
      </td>
      <td className="px-4 py-3 text-sm text-[var(--text-muted)]" data-label={t("admin.libraries.tableHeaders.items")}>
        {totalItems != null ? t("admin.libraries.items", { count: totalItems }) : <span className="opacity-50">{"-"}</span>}
      </td>
      <td className="px-4 py-3 text-sm text-[var(--text-muted)]" data-label={t("admin.libraries.tableHeaders.lastScanned")}>
        {formatDate(libStat?.lastScannedAt ?? undefined)}
      </td>
      <td className="px-4 py-3 text-sm text-[var(--text-muted)]" data-label={t("admin.libraries.tableHeaders.nextScan")}>
        {schedule?.enabled ? (
          <span title={`Cron: ${schedule.cron}`}>
            {schedule.nextRunAt ? formatDate(schedule.nextRunAt) : t("common.enabled")}
          </span>
        ) : (
          <span className="opacity-50">{t("admin.libraries.off")}</span>
        )}
      </td>
      <td className="px-4 py-3 whitespace-nowrap" data-label={t("admin.libraries.tableHeaders.actions")}>
        <div className="flex items-center gap-2">
          {scanLib.isPending ? (
            <div className="flex items-center gap-2 text-sm text-[var(--text-muted)]">
              <Spinner size="sm" />
              <span>{t("admin.libraries.scanning")}</span>
            </div>
          ) : (
            <Button
              variant="secondary"
              onClick={() => {
                setScanDone(false);
                scanLib.mutate({ id: library.id! });
              }}
              {...lock}
            >
              {scanDone ? t("admin.libraries.scanAgain") : t("admin.libraries.scan")}
            </Button>
          )}
          <Button variant="secondary" onClick={() => setShowEdit(true)} {...lock}>
            {t("common.edit")}
          </Button>
          {supportsProviderMatching(library.type) && (
            <>
              <Button
                variant="secondary"
                loading={matchOrphans.isPending}
                onClick={() => matchOrphans.mutate()}
                title={t("admin.libraries.matchOrphansTooltip")}
                data-testid="match-orphans-btn"
                {...lock}
              >
                {t("admin.libraries.matchOrphans")}
              </Button>
              <Button
                variant="secondary"
                loading={refreshMetadata.isPending}
                onClick={() => refreshMetadata.mutate()}
                title={t("admin.libraries.refreshMetadataTooltip")}
                data-testid="refresh-library-metadata-btn"
                {...lock}
              >
                {t("admin.libraries.refreshMetadata")}
              </Button>
            </>
          )}
          <Button variant="secondary" onClick={() => setShowSchedule(true)} {...lock}>
            {t("admin.libraries.schedule")}
          </Button>
          <Button
            variant="danger"
            loading={deleteLib.isPending}
            onClick={() => setShowDeleteConfirm(true)}
            {...lock}
          >
            {t("common.delete")}
          </Button>
        </div>
      </td>
      <Modal open={showEdit} onClose={() => setShowEdit(false)} title={t("admin.libraries.editLibrary")}>
        <LibraryForm
          library={library}
          onSuccess={() => {
            queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] });
            setShowEdit(false);
            toast.success(t("admin.libraries.libraryUpdated", { name: library.name }));
          }}
        />
      </Modal>
      <Modal open={showSchedule} onClose={() => setShowSchedule(false)} title={t("admin.libraries.scanScheduleTitle", { name: library.name })}>
        <ScheduleModal library={library} onClose={() => setShowSchedule(false)} />
      </Modal>
      <Modal open={showDeleteConfirm} onClose={() => setShowDeleteConfirm(false)} title={t("admin.libraries.deleteTitle")}>
        <p className="text-sm text-[var(--text-muted)] mb-4">
          {t("admin.libraries.deleteConfirm", { name: library.name })}
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setShowDeleteConfirm(false)}>{t("common.cancel")}</Button>
          <Button
            variant="danger"
            loading={deleteLib.isPending}
            onClick={() => {
              deleteLib.mutate({ id: library.id! });
              setShowDeleteConfirm(false);
            }}
          >
            {t("common.delete")}
          </Button>
        </div>
      </Modal>
    </tr>
  );
}

function LibrariesPage() {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const [showModal, setShowModal] = useState(false);
  const queryClient = useQueryClient();
  const { data, isLoading } = useLibraryServiceListLibraries();
  const { data: stats = [] } = useLibraryStats();
  const libraries = data?.libraries ?? [];

  // Server-wide, not per library: saga membership is keyed on the TMDB
  // collection, which spans every library a title may live in.
  const relinkCollections = useMutation({
    mutationFn: () => eveyFetch({ url: "/api/v1/admin/collections/relink", method: "POST", data: {} }),
    onSuccess: () => toast.success(t("admin.libraries.relinkCollectionsStarted")),
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  function handleCreateSuccess() {
    queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] });
    setShowModal(false);
    toast.success(t("admin.libraries.libraryCreated"));
  }

  return (
    <div className="flex flex-col gap-6" data-testid="libraries-page">
      <PageHeader
        title={t("admin.libraries.title")}
        actions={
          <div className="flex gap-2">
            <Button
              variant="secondary"
              loading={relinkCollections.isPending}
              onClick={() => relinkCollections.mutate()}
              title={t("admin.libraries.relinkCollectionsTooltip")}
              data-testid="relink-collections-btn"
              {...lock}
            >
              {t("admin.libraries.relinkCollections")}
            </Button>
            <Button onClick={() => setShowModal(true)} data-testid="add-library-btn" {...lock}>{t("admin.libraries.addLibrary")}</Button>
          </div>
        }
      />

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <div
          className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
        >
          <table className="w-full text-sm table-cards">
            <thead>
              <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
                {[t("admin.libraries.tableHeaders.name"), t("admin.libraries.tableHeaders.type"), t("admin.libraries.tableHeaders.paths"), t("admin.libraries.tableHeaders.items"), t("admin.libraries.tableHeaders.lastScanned"), t("admin.libraries.tableHeaders.nextScan"), t("admin.libraries.tableHeaders.actions")].map((h) => (
                  <th
                    key={h}
                    className="px-4 py-2 text-left font-medium text-[var(--text-muted)]"
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {libraries.length === 0 ? (
                <tr>
                  <td
                    colSpan={7}
                    className="px-4 py-8 text-center text-[var(--text-muted)]"
                  >
                    {t("admin.libraries.noLibraries")}
                  </td>
                </tr>
              ) : (
                libraries.map((lib) => <LibraryRow key={lib.id} library={lib} stats={stats} />)
              )}
            </tbody>
          </table>
        </div>
      )}

      <Modal open={showModal} onClose={() => setShowModal(false)} title={t("admin.libraries.addLibrary")}>
        <LibraryForm onSuccess={handleCreateSuccess} />
      </Modal>
    </div>
  );
}
