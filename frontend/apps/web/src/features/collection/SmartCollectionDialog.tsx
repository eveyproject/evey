import { useState, useEffect } from "react";
import { X, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { useCreateCollection, useUpdateCollection, type CollectionRule } from "./api";
import { toast } from "@evey/store";

type RuleField = "genre" | "year" | "type" | "rating_gte" | "library_id";
type YearOperator = "gte" | "lte" | "gt" | "lt" | "eq";
type StringOperator = "eq";

interface GenreRule { field: "genre"; operator: StringOperator; value: string }
interface YearRule { field: "year"; operator: YearOperator; value: string }
interface TypeRule { field: "type"; operator: StringOperator; value: string }
interface RatingGteRule { field: "rating_gte"; operator: "gte"; value: string }
interface LibraryIdRule { field: "library_id"; operator: StringOperator; value: string }
export type SmartRule = GenreRule | YearRule | TypeRule | RatingGteRule | LibraryIdRule;

export interface SmartCollection {
  id: string;
  name: string;
  rules: SmartRule[];
  sortBy?: string;
  createdAt: string;
}

const MEDIA_TYPES = [
  { value: "MEDIA_ITEM_TYPE_MOVIE", labelKey: "media.playlist.typeMovie" },
  { value: "MEDIA_ITEM_TYPE_SERIES", labelKey: "media.playlist.typeSeries" },
  { value: "MEDIA_ITEM_TYPE_EPISODE", labelKey: "media.playlist.typeEpisode" },
  { value: "MEDIA_ITEM_TYPE_ALBUM", labelKey: "media.playlist.typeAlbum" },
  { value: "MEDIA_ITEM_TYPE_ARTIST", labelKey: "media.playlist.typeArtist" },
  { value: "MEDIA_ITEM_TYPE_TRACK", labelKey: "media.playlist.typeTrack" },
];

const FIELD_LABEL_KEYS: Record<RuleField, string> = {
  genre: "media.playlist.fieldGenre",
  year: "media.playlist.fieldYear",
  type: "media.playlist.fieldType",
  rating_gte: "media.playlist.fieldRating",
  library_id: "media.playlist.fieldLibrary",
};

const YEAR_OPERATORS: { value: YearOperator; label: string }[] = [
  { value: "gte", label: ">=" },
  { value: "lte", label: "<=" },
  { value: "gt", label: ">" },
  { value: "lt", label: "<" },
  { value: "eq", label: "=" },
];

function defaultRule(field: RuleField): SmartRule {
  if (field === "genre") return { field: "genre", operator: "eq", value: "" };
  if (field === "year") return { field: "year", operator: "gte", value: "" };
  if (field === "rating_gte") return { field: "rating_gte", operator: "gte", value: "" };
  if (field === "library_id") return { field: "library_id", operator: "eq", value: "" };
  return { field: "type", operator: "eq", value: "MEDIA_ITEM_TYPE_MOVIE" };
}

const usedFields = (rules: SmartRule[]): Set<RuleField> =>
  new Set(rules.map((r) => r.field));

interface RuleRowProps {
  rule: SmartRule;
  onChange: (r: SmartRule) => void;
  onRemove: () => void;
}

function RuleRow({ rule, onChange, onRemove }: RuleRowProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      <span
        className="text-sm font-medium w-16 flex-shrink-0 text-[var(--text-muted)]"
      >
        {t(FIELD_LABEL_KEYS[rule.field])}
      </span>

      {rule.field === "year" ? (
        <select
          value={rule.operator}
          onChange={(e) => onChange({ ...rule, operator: e.target.value as YearOperator })}
          aria-label={`${t(FIELD_LABEL_KEYS[rule.field])} ${t("media.playlist.op")}`}
          className="text-sm px-2 py-1.5 rounded-[var(--radius-md)] w-16 flex-shrink-0 bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
        >
          {YEAR_OPERATORS.map((op) => (
            <option key={op.value} value={op.value}>{op.label}</option>
          ))}
        </select>
      ) : (
        <span
          className="text-sm px-2 py-1.5 w-8 text-center flex-shrink-0 text-[var(--text-muted)]"
        >
          {rule.field === "rating_gte" ? ">=" : "="}
        </span>
      )}

      {rule.field === "type" ? (
        <select
          value={rule.value}
          onChange={(e) => onChange({ ...rule, value: e.target.value })}
          aria-label={`${t(FIELD_LABEL_KEYS[rule.field])} ${t("media.playlist.value")}`}
          className="flex-1 text-sm px-2 py-1.5 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
        >
          {MEDIA_TYPES.map((mt) => (
            <option key={mt.value} value={mt.value}>{t(mt.labelKey)}</option>
          ))}
        </select>
      ) : (
        <input
          type={rule.field === "year" || rule.field === "rating_gte" ? "number" : "text"}
          value={rule.value}
          min={rule.field === "year" ? 1900 : rule.field === "rating_gte" ? 0 : undefined}
          max={rule.field === "year" ? new Date().getFullYear() + 2 : rule.field === "rating_gte" ? 10 : undefined}
          step={rule.field === "rating_gte" ? 0.1 : undefined}
          placeholder={
            rule.field === "genre" ? t("media.playlist.phGenre") :
            rule.field === "year" ? t("media.playlist.phYear") :
            rule.field === "rating_gte" ? t("media.playlist.phRating") :
            rule.field === "library_id" ? t("media.playlist.phLibrary") : ""
          }
          onChange={(e) => onChange({ ...rule, value: e.target.value })}
          aria-label={`${t(FIELD_LABEL_KEYS[rule.field])} ${t("media.playlist.value")}`}
          className="flex-1 text-sm px-3 py-1.5 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
        />
      )}

      <button
        type="button"
        onClick={onRemove}
        aria-label={t("media.playlist.removeRule")}
        className="flex-shrink-0 p-1 rounded hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text-muted)]"
      >
        <X size={16} />
      </button>
    </div>
  );
}

interface SmartCollectionDialogProps {
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
  editing?: SmartCollection;
}

export function SmartCollectionDialog({ open, onClose, onSaved, editing }: SmartCollectionDialogProps) {
  const { t } = useTranslation();
  const [name, setName] = useState(editing?.name ?? "");
  const [rules, setRules] = useState<SmartRule[]>(editing?.rules ?? []);
  const [nameError, setNameError] = useState("");

  const createCollection = useCreateCollection();
  const updateCollection = useUpdateCollection();

  useEffect(() => {
    if (open) {
      setName(editing?.name ?? "");
      setRules(editing?.rules ?? []);
      setNameError("");
    }
  }, [open, editing]);

  const ALL_FIELDS: RuleField[] = ["genre", "year", "type", "rating_gte", "library_id"];
  const available: RuleField[] = ALL_FIELDS.filter(
    (f) => !usedFields(rules).has(f)
  );

  function addRule(field: RuleField) {
    setRules((prev) => [...prev, defaultRule(field)]);
  }

  function updateRule(i: number, r: SmartRule) {
    setRules((prev) => prev.map((old, idx) => (idx === i ? r : old)));
  }

  function removeRule(i: number) {
    setRules((prev) => prev.filter((_, idx) => idx !== i));
  }

  async function handleSave() {
    if (!name.trim()) { setNameError(t("media.collections.nameRequired")); return; }

    const validRules: CollectionRule[] = rules
      .filter((r) => r.value.trim() !== "")
      .map((r) => ({ field: r.field, operator: r.operator, value: r.value }));
    const isPending = createCollection.isPending || updateCollection.isPending;
    if (isPending) return;

    try {
      if (editing) {
        await updateCollection.mutateAsync({
          id: editing.id,
          name: name.trim(),
          rules: validRules,
        });
        toast.success(t("media.collections.smartUpdated"));
      } else {
        await createCollection.mutateAsync({
          name: name.trim(),
          rules: validRules,
        });
        toast.success(t("media.collections.smartCreated"));
      }
      onSaved();
      onClose();
    } catch {
      toast.error(t("media.collections.smartSaveFailed"));
    }
  }

  const isSaving = createCollection.isPending || updateCollection.isPending;

  return (
    <Modal open={open} onClose={onClose} title={editing ? t("media.collections.editSmartCollection") : t("media.collections.newSmartCollection")}>
      <div className="flex flex-col gap-5">
        <Input
          label={t("common.name")}
          value={name}
          onChange={(e) => { setName(e.target.value); setNameError(""); }}
          placeholder={t("media.playlist.phCollectionName")}
          error={nameError}
          autoFocus
        />

        <div className="flex flex-col gap-2">
          <span className="text-sm font-medium text-[var(--text-muted)]">
            {t("media.playlist.rules")}
          </span>
          {rules.length === 0 && (
            <p className="text-sm text-[var(--text-muted)]">
              {t("media.collections.noRulesYet")}
            </p>
          )}
          {rules.map((rule, i) => (
            <RuleRow
              key={`${rule.field}-${i}`}
              rule={rule}
              onChange={(r) => updateRule(i, r)}
              onRemove={() => removeRule(i)}
            />
          ))}

          {available.length > 0 && (
            <div className="flex gap-2 flex-wrap mt-1">
              {available.map((f) => (
                <button
                  key={f}
                  type="button"
                  onClick={() => addRule(f)}
                  className="flex items-center gap-1 text-xs px-2 py-1 rounded-[var(--radius-md)] transition-colors-fast bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--accent)]"
                >
                  <Plus size={12} strokeWidth={2.5} />
                  {t(FIELD_LABEL_KEYS[f])}
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 pt-1">
          <Button variant="secondary" onClick={onClose} type="button">
            {t("common.cancel")}
          </Button>
          <Button onClick={handleSave} type="button" loading={isSaving}>
            {editing ? t("media.collections.saveChanges") : t("common.create")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
