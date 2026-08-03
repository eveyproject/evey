import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { toast } from "@evey/store";
import { collectionServiceAddCollectionItem, getCollectionServiceGetCollectionItemsQueryKey } from "@evey/api";
import { useAllCollections } from "./api";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

interface Props {
  open: boolean;
  onClose: () => void;
  mediaItemIds: string[];
}

export function AddToCollectionModal({ open, onClose, mediaItemIds }: Props) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading } = useAllCollections();
  const collections = data?.collections ?? [];
  const [pending, setPending] = useState(false);

  async function handleAdd(collectionId: string, collectionName: string) {
    setPending(true);
    try {
      await Promise.all(
        mediaItemIds.map((id) =>
          collectionServiceAddCollectionItem(collectionId, { mediaItemId: id, sortOrder: 0 })
        )
      );
      // Direct service call bypasses the mutation hook's auto-invalidation, so
      // refresh the target collection's items - otherwise the collection page
      // shows stale contents after the add.
      qc.invalidateQueries({ queryKey: getCollectionServiceGetCollectionItemsQueryKey(collectionId) });
      toast.success(
        t("media.collections.itemsAdded", { count: mediaItemIds.length, name: collectionName })
      );
      onClose();
    } catch (err) {
      console.warn("add to collection failed", err);
      toast.error(t("media.collections.addFailed"));
    } finally {
      setPending(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("media.contextMenu.addToCollection")}>
      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner />
        </div>
      ) : collections.length === 0 ? (
        <p className="text-sm text-center py-6 text-[var(--text-muted)]">
          {t("media.collections.noCollectionsAvailable")}
        </p>
      ) : (
        <div className="space-y-1">
          {collections.map((col) => (
            <button
              key={col.id}
              onClick={() => void handleAdd(col.id, col.name)}
              disabled={pending}
              className="w-full flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] text-left transition-colors-fast hover:bg-[var(--surface-raised)] text-[var(--text)]"
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
          <Button variant="ghost" disabled>{t("media.collections.adding")}</Button>
        </div>
      )}
    </Modal>
  );
}
