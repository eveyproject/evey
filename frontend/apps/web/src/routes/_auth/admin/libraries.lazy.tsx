import { useState, useEffect } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQueryClient, useQuery, useMutation } from "@tanstack/react-query";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceDeleteLibrary,
  useLibraryServiceScanLibrary,
  libraryServiceListMediaItems,
  eveyFetch,
} from "@evey/api";
import { useAuthStore, toast } from "@evey/store";
import type { V1Library } from "@evey/api/src/generated/library.schemas";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { Input } from "@/components/ui/Input";
import { LibraryForm } from "@/features/admin/LibraryForm";

export const Route = createLazyFileRoute("/_auth/admin/libraries")({
  component: LibrariesPage,
});

function libraryTypeLabel(type?: string): string {
  switch (type) {
    case "LIBRARY_TYPE_MOVIES": return "Movies";
    case "LIBRARY_TYPE_SHOWS": return "Shows";
    case "LIBRARY_TYPE_MUSIC": return "Music";
    default: return "Library";
  }
}

function formatDate(iso?: string): string {
  if (!iso) return "\u2014";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

interface LibraryStat {
  id: string;
  lastScannedAt?: string | null;
}

interface ScanSchedule {
  libraryId: string;
  cron: string;
  enabled: boolean;
  lastRunAt?: string | null;
  nextRunAt?: string | null;
}

const PRESET_SCHEDULES = [
  { value: "0 */6 * * *", label: "Every 6 hours" },
  { value: "0 */12 * * *", label: "Every 12 hours" },
  { value: "0 3 * * *", label: "Daily (3 AM)" },
  { value: "0 3 * * 0", label: "Weekly (Sunday 3 AM)" },
  { value: "custom", label: "Custom cron..." },
];

function useLibraryStats() {
  const token = useAuthStore((s) => s.accessToken);
  return useQuery<LibraryStat[]>({
    queryKey: ["/api/v1/admin/library-stats"],
    queryFn: async () => {
      const res = await fetch(`/api/v1/admin/library-stats`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!res.ok) return [];
      const json = await res.json() as { stats: LibraryStat[] };
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
  const queryClient = useQueryClient();
  const { data: schedule, isLoading } = useSchedule(library.id!);
  const [preset, setPreset] = useState("");
  const [customCron, setCustomCron] = useState("");
  const [enabled, setEnabled] = useState(true);

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
      toast.success("Schedule saved");
      onClose();
    },
    onError: (err: Error) => toast.error(err.message || "Failed to save schedule"),
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
      toast.success("Schedule removed");
      onClose();
    },
    onError: (err: Error) => toast.error(err.message || "Failed to remove schedule"),
  });

  if (isLoading) {
    return <div className="flex justify-center py-8"><Spinner size="md" /></div>;
  }

  return (
    <div className="flex flex-col gap-4">
      <Select
        label="Scan frequency"
        value={preset}
        onChange={(v) => {
          setPreset(v);
          if (v !== "custom") setCustomCron("");
        }}
        placeholder="-- select frequency --"
        options={PRESET_SCHEDULES}
      />

      {preset === "custom" && (
        <Input
          label="Cron expression"
          placeholder="0 */6 * * *"
          value={customCron}
          onChange={(e) => setCustomCron(e.target.value)}
        />
      )}

      <label className="flex items-center gap-2 text-sm" style={{ color: "var(--text)" }}>
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
          className="rounded"
        />
        Enabled
      </label>

      {schedule && (
        <div className="text-xs space-y-1" style={{ color: "var(--text-muted)" }}>
          {schedule.lastRunAt && <p>Last run: {formatDate(schedule.lastRunAt)}</p>}
          {schedule.nextRunAt && <p>Next run: {formatDate(schedule.nextRunAt)}</p>}
        </div>
      )}

      <div className="flex justify-between pt-2">
        <div>
          {schedule && (
            <Button
              variant="danger"
              loading={deleteMutation.isPending}
              onClick={() => deleteMutation.mutate()}
            >
              Remove
            </Button>
          )}
        </div>
        <div className="flex gap-2">
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button
            loading={saveMutation.isPending}
            disabled={!cronValue}
            onClick={() => saveMutation.mutate()}
          >
            Save
          </Button>
        </div>
      </div>
    </div>
  );
}

function LibraryRow({ library, stats }: { library: V1Library; stats: LibraryStat[] }) {
  const queryClient = useQueryClient();
  const [scanDone, setScanDone] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showSchedule, setShowSchedule] = useState(false);
  const { data: schedule } = useSchedule(library.id!);

  const deleteLib = useLibraryServiceDeleteLibrary({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] });
        queryClient.invalidateQueries({ queryKey: ["/api/v1/admin/library-stats"] });
        toast.success(`Library "${library.name}" deleted`);
      },
      onError: (err: Error) => toast.error(err.message || "Failed to delete library"),
    },
  });

  const scanLib = useLibraryServiceScanLibrary({
    mutation: {
      onSuccess: () => {
        setScanDone(true);
        queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] });
        queryClient.invalidateQueries({ queryKey: ["/api/v1/media"] });
        queryClient.invalidateQueries({ queryKey: ["/api/v1/admin/library-stats"] });
        toast.success("Scan started");
      },
      onError: (err: Error) => toast.error(err.message || "Failed to start scan"),
    },
  });

  const { data: mediaData } = useQuery({
    queryKey: ["/api/v1/media", { libraryId: library.id, page: 1, pageSize: 1 }],
    queryFn: ({ signal }) =>
      libraryServiceListMediaItems(
        { libraryId: library.id, "pagination.page": 1, "pagination.pageSize": 1 },
        signal,
      ),
    enabled: !!library.id,
  });

  const totalItems = mediaData?.pagination?.totalItems;
  const libStat = stats.find((s) => s.id === library.id);

  return (
    <tr style={{ borderBottom: "1px solid var(--border)" }}>
      <td className="px-4 py-3" style={{ color: "var(--text)" }}>
        {library.name}
      </td>
      <td className="px-4 py-3">
        <Badge variant="info">{libraryTypeLabel(library.type)}</Badge>
      </td>
      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-muted)" }}>
        {library.paths?.join(", ") ?? "\u2014"}
      </td>
      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-muted)" }}>
        {totalItems != null ? `${totalItems} items` : <span className="opacity-50">{"\u2014"}</span>}
      </td>
      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-muted)" }}>
        {formatDate(libStat?.lastScannedAt ?? undefined)}
      </td>
      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-muted)" }}>
        {schedule?.enabled ? (
          <span title={`Cron: ${schedule.cron}`}>
            {schedule.nextRunAt ? formatDate(schedule.nextRunAt) : "Enabled"}
          </span>
        ) : (
          <span className="opacity-50">Off</span>
        )}
      </td>
      <td className="px-4 py-3 whitespace-nowrap">
        <div className="flex items-center gap-2">
          {scanLib.isPending ? (
            <div className="flex items-center gap-2 text-sm" style={{ color: "var(--text-muted)" }}>
              <Spinner size="sm" />
              <span>Scanning...</span>
            </div>
          ) : (
            <Button
              variant="secondary"
              onClick={() => {
                setScanDone(false);
                scanLib.mutate({ id: library.id! });
              }}
            >
              {scanDone ? "Scan again" : "Scan"}
            </Button>
          )}
          <Button variant="secondary" onClick={() => setShowSchedule(true)}>
            Schedule
          </Button>
          <Button
            variant="danger"
            loading={deleteLib.isPending}
            onClick={() => setShowDeleteConfirm(true)}
          >
            Delete
          </Button>
        </div>
      </td>
      <Modal open={showSchedule} onClose={() => setShowSchedule(false)} title={`Scan Schedule \u2014 ${library.name}`}>
        <ScheduleModal library={library} onClose={() => setShowSchedule(false)} />
      </Modal>
      <Modal open={showDeleteConfirm} onClose={() => setShowDeleteConfirm(false)} title="Delete Library">
        <p className="text-sm text-[var(--text-muted)] mb-4">
          Delete library "{library.name}"? This cannot be undone.
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setShowDeleteConfirm(false)}>Cancel</Button>
          <Button
            variant="danger"
            loading={deleteLib.isPending}
            onClick={() => {
              deleteLib.mutate({ id: library.id! });
              setShowDeleteConfirm(false);
            }}
          >
            Delete
          </Button>
        </div>
      </Modal>
    </tr>
  );
}

function LibrariesPage() {
  const [showModal, setShowModal] = useState(false);
  const queryClient = useQueryClient();
  const { data, isLoading } = useLibraryServiceListLibraries();
  const { data: stats = [] } = useLibraryStats();
  const libraries = data?.libraries ?? [];

  function handleCreateSuccess() {
    queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] });
    setShowModal(false);
    toast.success("Library created");
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          Libraries
        </h1>
        <Button onClick={() => setShowModal(true)}>Add Library</Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <div
          className="overflow-x-auto rounded-[var(--radius-lg)]"
          style={{ border: "1px solid var(--border)" }}
        >
          <table className="w-full text-sm">
            <thead>
              <tr style={{ backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
                {["Name", "Type", "Paths", "Items", "Last Scanned", "Next Scan", "Actions"].map((h) => (
                  <th
                    key={h}
                    className="px-4 py-2 text-left font-medium"
                    style={{ color: "var(--text-muted)" }}
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
                    className="px-4 py-8 text-center"
                    style={{ color: "var(--text-muted)" }}
                  >
                    No libraries. Add one to get started.
                  </td>
                </tr>
              ) : (
                libraries.map((lib) => <LibraryRow key={lib.id} library={lib} stats={stats} />)
              )}
            </tbody>
          </table>
        </div>
      )}

      <Modal open={showModal} onClose={() => setShowModal(false)} title="Add Library">
        <LibraryForm onSuccess={handleCreateSuccess} />
      </Modal>
    </div>
  );
}
