import { useState } from "react";
import { X, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { toast } from "@evey/store";
import { useUpdatePlaylist, type PlaylistRule } from "./api";

type RuleField = "genre" | "year" | "type" | "library_id";
type YearOperator = "gte" | "lte" | "gt" | "lt" | "eq";
type StringOperator = "eq";

interface GenreRule { field: "genre"; operator: StringOperator; value: string }
interface YearRule { field: "year"; operator: YearOperator; value: string }
interface TypeRule { field: "type"; operator: StringOperator; value: string }
interface LibraryRule { field: "library_id"; operator: StringOperator; value: string }
type SmartRule = GenreRule | YearRule | TypeRule | LibraryRule;

const FIELD_LABEL_KEYS: Record<RuleField, string> = {
  genre: "media.playlist.fieldGenre",
  year: "media.playlist.fieldYear",
  type: "media.playlist.fieldType",
  library_id: "media.playlist.fieldLibrary",
};

const ALL_FIELDS: RuleField[] = ["genre", "year", "type", "library_id"];

const YEAR_OPERATORS: { value: YearOperator; label: string }[] = [
  { value: "gte", label: ">=" },
  { value: "lte", label: "<=" },
  { value: "gt", label: ">" },
  { value: "lt", label: "<" },
  { value: "eq", label: "=" },
];

const MEDIA_TYPES = [
  { value: "MEDIA_ITEM_TYPE_MOVIE", labelKey: "media.playlist.typeMovie" },
  { value: "MEDIA_ITEM_TYPE_SERIES", labelKey: "media.playlist.typeSeries" },
  { value: "MEDIA_ITEM_TYPE_EPISODE", labelKey: "media.playlist.typeEpisode" },
  { value: "MEDIA_ITEM_TYPE_ALBUM", labelKey: "media.playlist.typeAlbum" },
  { value: "MEDIA_ITEM_TYPE_ARTIST", labelKey: "media.playlist.typeArtist" },
  { value: "MEDIA_ITEM_TYPE_TRACK", labelKey: "media.playlist.typeTrack" },
];

function defaultRule(field: RuleField): SmartRule {
  if (field === "year") return { field: "year", operator: "gte", value: "" };
  if (field === "type") return { field: "type", operator: "eq", value: "MEDIA_ITEM_TYPE_MOVIE" };
  if (field === "library_id") return { field: "library_id", operator: "eq", value: "" };
  return { field: "genre", operator: "eq", value: "" };
}

function toSmartRule(r: PlaylistRule): SmartRule | null {
  if (r.field === "genre") return { field: "genre", operator: "eq", value: r.value };
  if (r.field === "year") return { field: "year", operator: (r.operator as YearOperator) || "gte", value: r.value };
  if (r.field === "type") return { field: "type", operator: "eq", value: r.value };
  if (r.field === "library_id") return { field: "library_id", operator: "eq", value: r.value };
  return null;
}

interface RuleRowProps {
  rule: SmartRule;
  onChange: (r: SmartRule) => void;
  onRemove: () => void;
}

function RuleRow({ rule, onChange, onRemove }: RuleRowProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2 py-1">
      <span
        className="text-sm font-medium w-16 flex-shrink-0 text-[var(--text-muted)]"
      >
        {t(FIELD_LABEL_KEYS[rule.field])}
      </span>

      {rule.field === "year" ? (
        <select
          value={rule.operator}
          onChange={(e) => onChange({ ...rule, operator: e.target.value as YearOperator })}
          aria-label={t("media.playlist.op")}
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
          =
        </span>
      )}

      {rule.field === "type" ? (
        <select
          value={rule.value}
          onChange={(e) => onChange({ ...rule, value: e.target.value })}
          aria-label={t("media.playlist.value")}
          className="flex-1 text-sm px-2 py-1.5 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
        >
          {MEDIA_TYPES.map((mt) => (
            <option key={mt.value} value={mt.value}>{t(mt.labelKey)}</option>
          ))}
        </select>
      ) : (
        <input
          type={rule.field === "year" ? "number" : "text"}
          value={rule.value}
          min={rule.field === "year" ? 1900 : undefined}
          max={rule.field === "year" ? new Date().getFullYear() + 2 : undefined}
          placeholder={rule.field === "genre" ? t("media.playlist.phGenre") : rule.field === "year" ? t("media.playlist.phYear") : t("media.playlist.phLibrary")}
          onChange={(e) => onChange({ ...rule, value: e.target.value })}
          aria-label={t("media.playlist.value")}
          className="flex-1 text-sm px-3 py-1.5 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
        />
      )}

      <button
        type="button"
        onClick={onRemove}
        aria-label={t("media.playlist.removeRule", { defaultValue: "Remove rule" })}
        className="flex-shrink-0 p-1 rounded hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text-muted)]"
      >
        <X size={16}  aria-hidden="true" />
      </button>
    </div>
  );
}

interface Props {
  playlistId: string;
  initialRules: PlaylistRule[];
  onSaved?: () => void;
}

export function SmartPlaylistRulesEditor({ playlistId, initialRules, onSaved }: Props) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [rules, setRules] = useState<SmartRule[]>(() =>
    (initialRules ?? []).map(toSmartRule).filter((r): r is SmartRule => r !== null)
  );
  const update = useUpdatePlaylist({ ownErrorToast: true });

  const usedFields = new Set(rules.map((r) => r.field));
  const available = ALL_FIELDS.filter((f) => !usedFields.has(f));

  function startEdit() {
    setRules(
      (initialRules ?? []).map(toSmartRule).filter((r): r is SmartRule => r !== null)
    );
    setEditing(true);
  }

  function cancel() {
    setEditing(false);
  }

  function addRule(field: RuleField) {
    setRules((prev) => [...prev, defaultRule(field)]);
  }

  function updateRule(i: number, r: SmartRule) {
    setRules((prev) => prev.map((old, idx) => (idx === i ? r : old)));
  }

  function removeRule(i: number) {
    setRules((prev) => prev.filter((_, idx) => idx !== i));
  }

  async function save() {
    const validRules = rules.filter((r) => r.value.trim() !== "");
    try {
      await update.mutateAsync({ id: playlistId, rules: validRules });
      toast.success(t("media.playlist.rulesSaved"));
      setEditing(false);
      onSaved?.();
    } catch {
      toast.error(t("media.playlist.rulesSaveFailed"));
    }
  }

  if (!editing) {
    return (
      <div className="mb-6">
        <div className="flex items-center justify-between mb-2">
          <span className="text-sm font-medium text-[var(--text-muted)]">
            {t("media.playlist.rules")}
          </span>
          <Button type="button" variant="secondary" onClick={startEdit}>
            {t("media.playlist.editRules")}
          </Button>
        </div>
        {initialRules && initialRules.length > 0 ? (
          <div className="flex flex-wrap gap-2">
            {initialRules.map((rule, i) => (
              <span
                key={i}
                className="text-xs px-2 py-1 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
              >
                {FIELD_LABEL_KEYS[rule.field as RuleField] ? t(FIELD_LABEL_KEYS[rule.field as RuleField]) : rule.field} {rule.operator} {rule.value}
              </span>
            ))}
          </div>
        ) : (
          <p className="text-sm text-[var(--text-muted)]">
            {t("media.playlist.noRules")}
          </p>
        )}
      </div>
    );
  }

  return (
    <div
      className="mb-6 p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
    >
      <span className="text-sm font-medium block mb-3 text-[var(--text)]">
        {t("media.playlist.editRules")}
      </span>

      {rules.length === 0 && (
        <p className="text-sm mb-3 text-[var(--text-muted)]">
          {t("media.playlist.noRulesYet")}
        </p>
      )}

      <div className="flex flex-col gap-1 mb-3">
        {rules.map((rule, i) => (
          <RuleRow
            key={`${rule.field}-${i}`}
            rule={rule}
            onChange={(r) => updateRule(i, r)}
            onRemove={() => removeRule(i)}
          />
        ))}
      </div>

      {available.length > 0 && (
        <div className="flex gap-2 flex-wrap mb-4">
          {available.map((f) => (
            <button
              key={f}
              type="button"
              onClick={() => addRule(f)}
              aria-label={t("media.playlist.addFieldRule", { field: t(FIELD_LABEL_KEYS[f]), defaultValue: `Add ${t(FIELD_LABEL_KEYS[f])} rule` })}
              className="flex items-center gap-1 text-xs px-2 py-1 rounded-[var(--radius-md)] transition-colors-fast bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--accent)]"
            >
              <Plus size={12} strokeWidth={2.5}  aria-hidden="true" />
              {t(FIELD_LABEL_KEYS[f])}
            </button>
          ))}
        </div>
      )}

      <div className="flex gap-2">
        <Button type="button" onClick={save} loading={update.isPending}>
          {t("media.playlist.saveRules")}
        </Button>
        <Button type="button" variant="secondary" onClick={cancel}>
          {t("common.cancel")}
        </Button>
      </div>
    </div>
  );
}
