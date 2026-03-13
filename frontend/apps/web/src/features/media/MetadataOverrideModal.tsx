import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  useMetadataServiceListMetadataOverrides,
  useMetadataServiceSetMetadataOverride,
  useMetadataServiceRemoveMetadataOverride,
  getMetadataServiceListMetadataOverridesQueryKey,
} from "@evey/api";
import { toast } from "@evey/store";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Textarea } from "@/components/ui/Textarea";
import { Select } from "@/components/ui/Select";
import { Spinner } from "@/components/ui/Spinner";

const RATING_OPTIONS = [
  { value: "", label: "— none —" },
  { value: "G", label: "G" },
  { value: "PG", label: "PG" },
  { value: "PG-13", label: "PG-13" },
  { value: "R", label: "R" },
  { value: "NC-17", label: "NC-17" },
  { value: "TV-G", label: "TV-G" },
  { value: "TV-PG", label: "TV-PG" },
  { value: "TV-14", label: "TV-14" },
  { value: "TV-MA", label: "TV-MA" },
];

interface Field {
  name: string;
  label: string;
  type: "text" | "number" | "textarea" | "rating";
}

const FIELDS: Field[] = [
  { name: "title", label: "Title", type: "text" },
  { name: "original_title", label: "Original Title", type: "text" },
  { name: "overview", label: "Overview", type: "textarea" },
  { name: "year", label: "Year", type: "number" },
  { name: "rating", label: "Rating", type: "rating" },
  { name: "genres", label: "Genres (comma-separated)", type: "text" },
];

interface Props {
  open: boolean;
  onClose: () => void;
  mediaItemId: string;
}

export function MetadataOverrideModal({ open, onClose, mediaItemId }: Props) {
  const queryClient = useQueryClient();
  const { data, isLoading } = useMetadataServiceListMetadataOverrides(mediaItemId, {
    query: { enabled: open && !!mediaItemId },
  });
  const setOverride = useMetadataServiceSetMetadataOverride();
  const removeOverride = useMetadataServiceRemoveMetadataOverride();

  const overridesByField = Object.fromEntries(
    (data?.overrides ?? []).map((o) => [o.fieldName ?? "", o]),
  );

  const [draft, setDraft] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState<string | null>(null);
  const [removing, setRemoving] = useState<string | null>(null);

  function currentValue(fieldName: string): string {
    if (fieldName in draft) return draft[fieldName];
    return overridesByField[fieldName]?.customValue ?? "";
  }

  function setDraftField(fieldName: string, value: string) {
    setDraft((prev) => ({ ...prev, [fieldName]: value }));
  }

  async function handleSave(fieldName: string) {
    const value = currentValue(fieldName).trim();
    if (!value) return;
    setSaving(fieldName);
    try {
      await setOverride.mutateAsync({ data: { mediaItemId, fieldName, customValue: value } });
      queryClient.invalidateQueries({ queryKey: getMetadataServiceListMetadataOverridesQueryKey(mediaItemId) });
      queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${mediaItemId}`] });
      setDraft((prev) => { const n = { ...prev }; delete n[fieldName]; return n; });
      toast.success(`Override saved: ${fieldName}`);
    } catch {
      toast.error("Failed to save override");
    } finally {
      setSaving(null);
    }
  }

  async function handleRemove(overrideId: string, fieldName: string) {
    setRemoving(fieldName);
    try {
      await removeOverride.mutateAsync({ id: overrideId });
      queryClient.invalidateQueries({ queryKey: getMetadataServiceListMetadataOverridesQueryKey(mediaItemId) });
      queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${mediaItemId}`] });
      setDraft((prev) => { const n = { ...prev }; delete n[fieldName]; return n; });
      toast.success(`Override removed: ${fieldName}`);
    } catch {
      toast.error("Failed to remove override");
    } finally {
      setRemoving(null);
    }
  }

  function isDirty(fieldName: string): boolean {
    if (!(fieldName in draft)) return false;
    return draft[fieldName] !== (overridesByField[fieldName]?.customValue ?? "");
  }

  return (
    <Modal open={open} onClose={onClose} title="Metadata Overrides">
      {isLoading ? (
        <div className="flex justify-center py-8"><Spinner /></div>
      ) : (
        <div className="flex flex-col gap-5">
          <p className="text-xs" style={{ color: "var(--text-muted)" }}>
            Fields set here override provider metadata (TMDB/OMDb). Leave blank to use provider value.
          </p>
          {FIELDS.map((field) => {
            const existing = overridesByField[field.name];
            const value = currentValue(field.name);
            const dirty = isDirty(field.name);
            const isSavingThis = saving === field.name;
            const isRemovingThis = removing === field.name;

            return (
              <div key={field.name} className="flex flex-col gap-2">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium flex-1" style={{ color: "var(--text)" }}>
                    {field.label}
                  </span>
                  {existing?.id && (
                    <span
                      className="text-xs px-1.5 py-0.5 rounded-full"
                      style={{ backgroundColor: "var(--accent)", color: "var(--bg)", fontSize: "10px" }}
                    >
                      overridden
                    </span>
                  )}
                </div>

                {field.type === "textarea" ? (
                  <Textarea
                    value={value}
                    onChange={(e) => setDraftField(field.name, e.target.value)}
                    rows={3}
                    placeholder={existing ? existing.customValue : ""}
                  />
                ) : field.type === "rating" ? (
                  <Select
                    options={RATING_OPTIONS}
                    value={value}
                    onChange={(v) => setDraftField(field.name, v)}
                  />
                ) : (
                  <Input
                    type={field.type}
                    value={value}
                    onChange={(e) => setDraftField(field.name, e.target.value)}
                    placeholder={existing ? existing.customValue : ""}
                  />
                )}

                <div className="flex gap-2">
                  <Button
                    type="button"
                    variant="primary"
                    loading={isSavingThis}
                    disabled={!value.trim() || !dirty}
                    onClick={() => handleSave(field.name)}
                  >
                    Save
                  </Button>
                  {existing?.id && (
                    <Button
                      type="button"
                      variant="secondary"
                      loading={isRemovingThis}
                      onClick={() => handleRemove(existing.id!, field.name)}
                      className="text-red-400 hover:text-red-300"
                    >
                      Remove override
                    </Button>
                  )}
                </div>
              </div>
            );
          })}

          <div className="pt-2 border-t" style={{ borderColor: "var(--border)" }}>
            <Button type="button" variant="secondary" onClick={onClose}>
              Close
            </Button>
          </div>
        </div>
      )}
    </Modal>
  );
}
