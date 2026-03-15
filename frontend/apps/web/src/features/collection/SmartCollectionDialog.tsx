import { useState, useEffect } from "react";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { useCreatePlaylist, useUpdatePlaylist, type PlaylistRule, type Playlist } from "@/features/playlist/api";
import { toast } from "@evey/store";

type RuleField = "genre" | "year" | "type";
type YearOperator = "gte" | "lte" | "gt" | "lt" | "eq";
type StringOperator = "eq";

interface GenreRule { field: "genre"; operator: StringOperator; value: string }
interface YearRule { field: "year"; operator: YearOperator; value: string }
interface TypeRule { field: "type"; operator: StringOperator; value: string }
export type SmartRule = GenreRule | YearRule | TypeRule;

export interface SmartFilter {
  id: string;
  name: string;
  rules: SmartRule[];
  createdAt: string;
}

export function playlistToSmartFilter(pl: Playlist): SmartFilter {
  return {
    id: pl.id,
    name: pl.name,
    rules: (pl.rules ?? []).filter(isSmartRule),
    createdAt: pl.created_at,
  };
}

function isSmartRule(r: PlaylistRule): r is SmartRule {
  return r.field === "genre" || r.field === "year" || r.field === "type";
}

const MEDIA_TYPES = [
  { value: "MEDIA_ITEM_TYPE_MOVIE", label: "Movie" },
  { value: "MEDIA_ITEM_TYPE_SERIES", label: "Series" },
  { value: "MEDIA_ITEM_TYPE_EPISODE", label: "Episode" },
  { value: "MEDIA_ITEM_TYPE_ALBUM", label: "Album" },
  { value: "MEDIA_ITEM_TYPE_ARTIST", label: "Artist" },
  { value: "MEDIA_ITEM_TYPE_TRACK", label: "Track" },
];

const FIELD_LABELS: Record<RuleField, string> = {
  genre: "Genre",
  year: "Year",
  type: "Type",
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
  return (
    <div className="flex items-center gap-2">
      <span
        className="text-sm font-medium w-12 flex-shrink-0"
        style={{ color: "var(--text-muted)" }}
      >
        {FIELD_LABELS[rule.field]}
      </span>

      {rule.field === "year" ? (
        <select
          value={rule.operator}
          onChange={(e) => onChange({ ...rule, operator: e.target.value as YearOperator })}
          className="text-sm px-2 py-1.5 rounded-[var(--radius-md)] w-16 flex-shrink-0"
          style={{
            backgroundColor: "var(--surface-raised)",
            border: "1px solid var(--border)",
            color: "var(--text)",
          }}
        >
          {YEAR_OPERATORS.map((op) => (
            <option key={op.value} value={op.value}>{op.label}</option>
          ))}
        </select>
      ) : (
        <span
          className="text-sm px-2 py-1.5 w-8 text-center flex-shrink-0"
          style={{ color: "var(--text-muted)" }}
        >
          =
        </span>
      )}

      {rule.field === "type" ? (
        <select
          value={rule.value}
          onChange={(e) => onChange({ ...rule, value: e.target.value })}
          className="flex-1 text-sm px-2 py-1.5 rounded-[var(--radius-md)]"
          style={{
            backgroundColor: "var(--surface-raised)",
            border: "1px solid var(--border)",
            color: "var(--text)",
          }}
        >
          {MEDIA_TYPES.map((t) => (
            <option key={t.value} value={t.value}>{t.label}</option>
          ))}
        </select>
      ) : (
        <input
          type={rule.field === "year" ? "number" : "text"}
          value={rule.value}
          min={rule.field === "year" ? 1900 : undefined}
          max={rule.field === "year" ? new Date().getFullYear() + 2 : undefined}
          placeholder={rule.field === "genre" ? "e.g. Action" : "e.g. 2020"}
          onChange={(e) => onChange({ ...rule, value: e.target.value })}
          className="flex-1 text-sm px-3 py-1.5 rounded-[var(--radius-md)]"
          style={{
            backgroundColor: "var(--surface-raised)",
            border: "1px solid var(--border)",
            color: "var(--text)",
          }}
        />
      )}

      <button
        type="button"
        onClick={onRemove}
        aria-label="Remove rule"
        className="flex-shrink-0 p-1 rounded hover:bg-[var(--surface-raised)] transition-colors"
        style={{ color: "var(--text-muted)" }}
      >
        <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  );
}

interface SmartCollectionDialogProps {
  open: boolean;
  onClose: () => void;
  onSaved: (filter: SmartFilter) => void;
  editing?: SmartFilter;
}

export function SmartCollectionDialog({ open, onClose, onSaved, editing }: SmartCollectionDialogProps) {
  const [name, setName] = useState(editing?.name ?? "");
  const [rules, setRules] = useState<SmartRule[]>(editing?.rules ?? []);
  const [nameError, setNameError] = useState("");

  const createPlaylist = useCreatePlaylist();
  const updatePlaylist = useUpdatePlaylist();

  useEffect(() => {
    if (open) {
      setName(editing?.name ?? "");
      setRules(editing?.rules ?? []);
      setNameError("");
    }
  }, [open, editing]);

  const available: RuleField[] = (["genre", "year", "type"] as RuleField[]).filter(
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
    if (!name.trim()) { setNameError("Name is required"); return; }

    const validRules = rules.filter((r) => r.value.trim() !== "");
    const isPending = createPlaylist.isPending || updatePlaylist.isPending;
    if (isPending) return;

    try {
      if (editing) {
        await updatePlaylist.mutateAsync({
          id: editing.id,
          name: name.trim(),
          rules: validRules,
        });
        const updated: SmartFilter = { ...editing, name: name.trim(), rules: validRules };
        onSaved(updated);
        toast.success("Smart filter updated");
      } else {
        const result = await createPlaylist.mutateAsync({
          name: name.trim(),
          type: "smart",
          rules: validRules,
        });
        const newFilter: SmartFilter = {
          id: (result as unknown as { id: string }).id ?? crypto.randomUUID(),
          name: name.trim(),
          rules: validRules,
          createdAt: new Date().toISOString(),
        };
        onSaved(newFilter);
        toast.success("Smart filter created");
      }
      onClose();
    } catch {
      toast.error("Failed to save smart filter");
    }
  }

  const isSaving = createPlaylist.isPending || updatePlaylist.isPending;

  return (
    <Modal open={open} onClose={onClose} title={editing ? "Edit smart filter" : "New smart filter"}>
      <div className="flex flex-col gap-5">
        <Input
          label="Name"
          value={name}
          onChange={(e) => { setName(e.target.value); setNameError(""); }}
          placeholder="e.g. Action movies after 2015"
          error={nameError}
          autoFocus
        />

        <div className="flex flex-col gap-2">
          <span className="text-sm font-medium" style={{ color: "var(--text-muted)" }}>
            Rules
          </span>
          {rules.length === 0 && (
            <p className="text-sm" style={{ color: "var(--text-muted)" }}>
              No rules yet. Add at least one rule to filter items.
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
                  className="flex items-center gap-1 text-xs px-2 py-1 rounded-[var(--radius-md)] transition-colors"
                  style={{
                    backgroundColor: "var(--surface-raised)",
                    border: "1px solid var(--border)",
                    color: "var(--accent)",
                  }}
                >
                  <svg className="w-3 h-3" fill="none" stroke="currentColor" strokeWidth={2.5} viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
                  </svg>
                  {FIELD_LABELS[f]}
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 pt-1">
          <Button variant="secondary" onClick={onClose} type="button">
            Cancel
          </Button>
          <Button onClick={handleSave} type="button" loading={isSaving}>
            {editing ? "Save changes" : "Create"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
