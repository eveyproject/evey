import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { useCreateCollection, useUpdateCollection } from "./api";
import { toast } from "@evey/store";

export interface EditableCollection {
  id: string;
  name: string;
  overview?: string;
}

interface CollectionDialogProps {
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
  editing?: EditableCollection;
}

/**
 * Hand-built collection: just a name and a free-text description, no rules.
 * The counterpart to SmartCollectionDialog - a series can only be grouped by
 * hand, since TMDB collections are a film-only notion.
 */
export function CollectionDialog({ open, onClose, onSaved, editing }: CollectionDialogProps) {
  const { t } = useTranslation();
  const [name, setName] = useState(editing?.name ?? "");
  const [overview, setOverview] = useState(editing?.overview ?? "");
  const [nameError, setNameError] = useState("");

  const createCollection = useCreateCollection();
  const updateCollection = useUpdateCollection();

  useEffect(() => {
    if (open) {
      setName(editing?.name ?? "");
      setOverview(editing?.overview ?? "");
      setNameError("");
    }
  }, [open, editing]);

  const isSaving = createCollection.isPending || updateCollection.isPending;

  async function handleSave() {
    if (!name.trim()) { setNameError(t("media.collections.nameRequired")); return; }
    if (isSaving) return;

    try {
      if (editing) {
        const trimmedOverview = overview.trim();
        const initialOverview = (editing.overview ?? "").trim();
        await updateCollection.mutateAsync({
          id: editing.id,
          name: name.trim(),
          // Absent means "keep the existing synopsis" server-side - a saga's
          // TMDB overview must not be wiped by an edit that never touched it.
          overview: trimmedOverview !== initialOverview ? trimmedOverview : undefined,
        });
        toast.success(t("media.collections.updated"));
      } else {
        await createCollection.mutateAsync({
          name: name.trim(),
          overview: overview.trim() || undefined,
        });
        toast.success(t("media.collections.created"));
      }
      onSaved();
      onClose();
    } catch {
      toast.error(t(editing ? "media.collections.updateFailed" : "media.collections.createFailed"));
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={editing ? t("media.collections.editCollection") : t("media.collections.newCollection")}>
      <div className="flex flex-col gap-5">
        <Input
          label={t("common.name")}
          value={name}
          onChange={(e) => { setName(e.target.value); setNameError(""); }}
          error={nameError}
          autoFocus
        />

        <div className="flex flex-col gap-1">
          <label htmlFor="collection-overview" className="text-sm font-medium text-[var(--text-muted)]">
            {t("common.description")}
          </label>
          <textarea
            id="collection-overview"
            value={overview}
            onChange={(e) => setOverview(e.target.value)}
            rows={3}
            className="px-3 py-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent transition-shadow duration-200"
          />
        </div>

        <div className="flex justify-end gap-2 pt-1">
          <Button variant="secondary" onClick={onClose} type="button">
            {t("common.cancel")}
          </Button>
          <Button onClick={handleSave} type="button" loading={isSaving}>
            {editing ? t("common.save") : t("common.create")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
