import { Modal } from "@/components/ui/Modal";
import { Spinner } from "@/components/ui/Spinner";
import { toast } from "@evey/store";
import { useCollections, useAddToCollection } from "./api";

interface Props {
  open: boolean;
  onClose: () => void;
  mediaItemId: string;
}

export function AddToCollectionModal({ open, onClose, mediaItemId }: Props) {
  const { data, isLoading } = useCollections();
  const addTo = useAddToCollection();

  const manualCollections = (data?.collections ?? []).filter((c) => c.type === "manual");

  async function handleAdd(collectionId: string, collectionName: string) {
    try {
      await addTo.mutateAsync({ collectionId, mediaItemId });
      toast.success(`Added to "${collectionName}"`);
      onClose();
    } catch {
      toast.error("Failed to add to collection");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Add to Collection">
      {isLoading ? (
        <div className="flex justify-center py-8"><Spinner /></div>
      ) : manualCollections.length === 0 ? (
        <div className="text-center py-6">
          <p className="text-sm" style={{ color: "var(--text-muted)" }}>No manual collections yet.</p>
        </div>
      ) : (
        <div className="space-y-2">
          {manualCollections.map((col) => (
            <button
              key={col.id}
              onClick={() => handleAdd(col.id, col.name)}
              disabled={addTo.isPending}
              className="w-full flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] text-left transition-colors hover:bg-[var(--surface-raised)]"
              style={{ color: "var(--text)" }}
            >
              <svg className="w-5 h-5 flex-shrink-0" style={{ color: "var(--accent)" }} fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" />
              </svg>
              <p className="text-sm font-medium truncate">{col.name}</p>
            </button>
          ))}
        </div>
      )}
    </Modal>
  );
}
