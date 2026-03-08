import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceDeleteLibrary,
  useLibraryServiceScanLibrary,
} from "@evey/api";
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

function LibraryRow({ library }: { library: V1Library }) {
  const queryClient = useQueryClient();

  const deleteLib = useLibraryServiceDeleteLibrary({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] }),
    },
  });

  const scanLib = useLibraryServiceScanLibrary({
    mutation: {},
  });

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
      <td className="px-4 py-3 whitespace-nowrap">
        <div className="flex gap-2">
          <Button
            variant="secondary"
            loading={scanLib.isPending}
            onClick={() => scanLib.mutate({ id: library.id! })}
          >
            Scan
          </Button>
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
                {["Name", "Type", "Paths", "Actions"].map((h) => (
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
                    colSpan={4}
                    className="px-4 py-8 text-center"
                    style={{ color: "var(--text-muted)" }}
                  >
                    No libraries. Add one to get started.
                  </td>
                </tr>
              ) : (
                libraries.map((lib) => <LibraryRow key={lib.id} library={lib} />)
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
