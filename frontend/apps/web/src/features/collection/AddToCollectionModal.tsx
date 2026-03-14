import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { toast } from "@evey/store";
import { eveyFetch } from "@evey/api";
import { useCollections } from "./api";
import { useState } from "react";

interface Props {
  open: boolean;
  onClose: () => void;
  mediaItemIds: string[];
}

export function AddToCollectionModal({ open, onClose, mediaItemIds }: Props) {
  const { data, isLoading } = useCollections();
  const collections = data?.collections ?? [];
  const [pending, setPending] = useState(false);

  async function handleAdd(collectionId: string, collectionName: string) {
    setPending(true);
    try {
      await Promise.all(
        mediaItemIds.map((id) =>
          eveyFetch<unknown>({
            url: `/api/v1/collections/${collectionId}/items`,
            method: "POST",
            data: { mediaItemId: id },
          })
        )
      );
      toast.success(
        `${mediaItemIds.length} item${mediaItemIds.length > 1 ? "s" : ""} added to "${collectionName}"`
      );
      onClose();
    } catch {
      toast.error("Failed to add to collection");
    } finally {
      setPending(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Add to Collection">
      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner />
        </div>
      ) : collections.length === 0 ? (
        <p className="text-sm text-center py-6" style={{ color: "var(--text-muted)" }}>
          No collections available.
        </p>
      ) : (
        <div className="space-y-1">
          {collections.map((col) => (
            <button
              key={col.id}
              onClick={() => void handleAdd(col.id, col.name)}
              disabled={pending}
              className="w-full flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] text-left transition-colors hover:bg-[var(--surface-raised)]"
              style={{ color: "var(--text)" }}
            >
              {col.posterUrl && (
                <img
                  src={col.posterUrl}
                  alt={col.name}
                  className="w-8 h-12 object-cover flex-shrink-0 rounded"
                />
              )}
              <span className="text-sm font-medium truncate">{col.name}</span>
            </button>
          ))}
        </div>
      )}
      {pending && (
        <div className="flex justify-center pt-4">
          <Button variant="ghost" disabled>Adding…</Button>
        </div>
      )}
    </Modal>
  );
}
