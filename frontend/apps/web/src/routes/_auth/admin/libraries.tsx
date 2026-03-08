import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceDeleteLibrary,
  useLibraryServiceScanLibrary,
  libraryServiceListMediaItems,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import type { V1Library } from "@evey/api/src/generated/library.schemas";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { LibraryForm } from "@/features/admin/LibraryForm";

export const Route = createFileRoute("/_auth/admin/libraries")({
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
  if (!iso) return "—";
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

function LibraryRow({ library, stats }: { library: V1Library; stats: LibraryStat[] }) {
  const queryClient = useQueryClient();
  const [scanDone, setScanDone] = useState(false);

  const deleteLib = useLibraryServiceDeleteLibrary({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] });
        queryClient.invalidateQueries({ queryKey: ["/api/v1/admin/library-stats"] });
      },
    },
  });

  const scanLib = useLibraryServiceScanLibrary({
    mutation: {
      onSuccess: () => {
        setScanDone(true);
        queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] });
        queryClient.invalidateQueries({ queryKey: ["/api/v1/media"] });
        queryClient.invalidateQueries({ queryKey: ["/api/v1/admin/library-stats"] });
      },
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
        {library.paths?.join(", ") ?? "—"}
      </td>
      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-muted)" }}>
        {totalItems != null ? `${totalItems} items` : <span className="opacity-50">—</span>}
      </td>
      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-muted)" }}>
        {formatDate(libStat?.lastScannedAt ?? undefined)}
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
          <Button
            variant="danger"
            loading={deleteLib.isPending}
            onClick={() => {
              if (confirm(`Delete library "${library.name}"?`)) {
                deleteLib.mutate({ id: library.id! });
              }
            }}
          >
            Delete
          </Button>
        </div>
      </td>
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
                {["Name", "Type", "Paths", "Items", "Last Scanned", "Actions"].map((h) => (
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
                    colSpan={6}
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
