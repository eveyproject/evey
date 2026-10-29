import { useState, useEffect, useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useLibraryAccessServiceGetUserLibraryAccess,
  useLibraryAccessServiceRevokeAllUserLibraryAccess,
  useLibraryAccessServiceSetUserLibraryAccess,
  getLibraryAccessServiceGetUserLibraryAccessQueryKey,
} from "@evey/api";
import { toast } from "@evey/store";
import type { V1User } from "@evey/api";
import { Checkbox } from "@/components/ui/Checkbox";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { SegmentedControl } from "@/components/ui/SegmentedControl";

type AccessMode = "all" | "selected";

export function LibraryAccessModal({
  user,
  open,
  onClose,
}: {
  user: V1User;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  // "All libraries" and "no library ticked" are two different answers, so the
  // admin picks the mode explicitly: an empty selection used to be saved as
  // a revoke-all, which lifts the restriction and opens the whole catalogue.
  const [mode, setMode] = useState<AccessMode>("all");
  const [saving, setSaving] = useState(false);

  const { data, isLoading } = useLibraryAccessServiceGetUserLibraryAccess(user.id ?? "", {
    query: { enabled: open && !!user.id },
  });

  useEffect(() => {
    if (data) {
      setSelected(new Set(data.libraryIds ?? []));
      // Restricted with nothing left (grants revoked, libraries deleted): the
      // server says so with a flag, since the empty list alone reads as
      // unrestricted.
      setMode(data.restricted || (data.libraryIds?.length ?? 0) > 0 ? "selected" : "all");
    }
  }, [data]);

  const isUnrestricted = mode === "all";
  const showNoLibrary = mode === "selected" && selected.size === 0;
  const libraries = data?.libraries ?? [];

  const handleToggle = useCallback((libId: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(libId)) {
        next.delete(libId);
      } else {
        next.add(libId);
      }
      return next;
    });
  }, []);

  const revoke = useLibraryAccessServiceRevokeAllUserLibraryAccess();
  const setAccess = useLibraryAccessServiceSetUserLibraryAccess();

  async function handleSave() {
    if (!user.id) return;
    setSaving(true);
    try {
      if (mode === "all") {
        await revoke.mutateAsync({ userId: user.id });
      } else {
        await setAccess.mutateAsync({ userId: user.id, data: { libraryIds: Array.from(selected) } });
      }
      // Invalidate this user's cached access so reopening the modal (or any
      // access-dependent view) reflects the change instead of stale data.
      qc.invalidateQueries({ queryKey: getLibraryAccessServiceGetUserLibraryAccessQueryKey(user.id) });
      toast.success(t("admin.users.libraryAccessUpdated"));
      onClose();
    } catch {
      toast.error(t("admin.users.failedLibraryAccess"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("admin.users.libraryAccessTitle", { name: user.username })}>
      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner size="md" />
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--text-muted)]">
            {showNoLibrary
              ? t("admin.users.noLibrariesDesc")
              : isUnrestricted
                ? t("admin.users.unrestrictedDesc")
                : t("admin.users.restrictedDesc", { count: selected.size, libraryWord: selected.size === 1 ? "library" : "libraries" })}
          </p>

          <SegmentedControl
            label={t("admin.users.libraryAccessTitle", { name: user.username })}
            options={[
              { value: "all", label: t("admin.users.accessAll") },
              { value: "selected", label: t("admin.users.accessSelected") },
            ]}
            value={mode}
            onChange={(v) => setMode(v as AccessMode)}
          />

          {showNoLibrary ? (
            <Badge variant="warning">{t("admin.users.noLibraries")}</Badge>
          ) : isUnrestricted ? (
            <Badge variant="success">{t("admin.users.unrestricted")}</Badge>
          ) : null}

          {!isUnrestricted && (
            <div className="flex flex-col gap-2">
              {libraries.map((lib) => (
                <div
                  key={lib.id}
                  className="px-3 py-2 rounded-[var(--radius-md)] hover:opacity-80 bg-[var(--surface)]"
                >
                  <Checkbox
                    label={lib.name ?? ""}
                    checked={selected.has(lib.id ?? "")}
                    onChange={() => handleToggle(lib.id ?? "")}
                  />
                </div>
              ))}
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="ghost" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button onClick={handleSave} loading={saving}>
              {t("common.save")}
            </Button>
          </div>
        </div>
      )}
    </Modal>
  );
}
