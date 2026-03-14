import { useState, useId, useEffect } from "react";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { libraryServiceListMediaItems } from "@evey/api";

// Fields the server supports filtering on
type RuleField = "genre" | "year" | "type";
type YearOperator = ">=" | "<=" | "=";
type StringOperator = "=";
type TypeOperator = "=";

interface GenreRule { field: "genre"; op: StringOperator; value: string }
interface YearRule { field: "year"; op: YearOperator; value: string }
interface TypeRule { field: "type"; op: TypeOperator; value: string }
export type SmartRule = GenreRule | YearRule | TypeRule;

export interface SmartFilter {
  id: string;
  name: string;
  rules: SmartRule[];
  createdAt: string;
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

function buildParams(rules: SmartRule[]) {
  const params: Record<string, string | number> = {};
  for (const r of rules) {
    if (r.field === "genre" && r.value) params.genre = r.value;
    if (r.field === "year" && r.value) params.year = Number(r.value);
    if (r.field === "type" && r.value) params.type = r.value;
  }
  return params;
}

function defaultRule(field: RuleField): SmartRule {
  if (field === "genre") return { field: "genre", op: "=", value: "" };
  if (field === "year") return { field: "year", op: ">=", value: "" };
  return { field: "type", op: "=", value: "MEDIA_ITEM_TYPE_MOVIE" };
}

const usedFields = (rules: SmartRule[]): Set<RuleField> =>
  new Set(rules.map((r) => r.field));

interface RuleRowProps {
  rule: SmartRule;
  onChange: (r: SmartRule) => void;
  onRemove: () => void;
}

function RuleRow({ rule, onChange, onRemove }: RuleRowProps) {
  const id = useId();

  return (
    <div className="flex items-center gap-2">
      {/* field label */}
      <span
        className="text-sm font-medium w-12 flex-shrink-0"
        style={{ color: "var(--text-muted)" }}
      >
        {FIELD_LABELS[rule.field]}
      </span>

      {/* operator */}
      {rule.field === "year" ? (
        <select
          value={rule.op}
          onChange={(e) => onChange({ ...rule, op: e.target.value as YearOperator })}
          id={id}
          className="text-sm px-2 py-1.5 rounded-[var(--radius-md)] w-16 flex-shrink-0"
          style={{
            backgroundColor: "var(--surface-raised)",
            border: "1px solid var(--border)",
            color: "var(--text)",
          }}
        >
          <option value=">=">&ge;</option>
          <option value="<=">&le;</option>
          <option value="=">=</option>
        </select>
      ) : (
        <span
          className="text-sm px-2 py-1.5 w-8 text-center flex-shrink-0"
          style={{ color: "var(--text-muted)" }}
        >
          =
        </span>
      )}

      {/* value */}
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
  const [previewCount, setPreviewCount] = useState<number | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [nameError, setNameError] = useState("");

  // Reset when dialog opens/editing changes
  useEffect(() => {
    if (open) {
      setName(editing?.name ?? "");
      setRules(editing?.rules ?? []);
      setPreviewCount(null);
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
    setPreviewCount(null);
  }

  async function runPreview() {
    const params = buildParams(rules);
    if (Object.keys(params).length === 0) { setPreviewCount(null); return; }
    setPreviewLoading(true);
    try {
      const res = await libraryServiceListMediaItems({ ...params, "pagination.pageSize": 1 });
      setPreviewCount(res.pagination?.totalItems ?? (res.items?.length ?? 0));
    } catch {
      setPreviewCount(null);
    } finally {
      setPreviewLoading(false);
    }
  }

  function handleSave() {
    if (!name.trim()) { setNameError("Name is required"); return; }
    const filter: SmartFilter = {
      id: editing?.id ?? crypto.randomUUID(),
      name: name.trim(),
      rules,
      createdAt: editing?.createdAt ?? new Date().toISOString(),
    };
    onSaved(filter);
    onClose();
  }

  const hasRules = rules.length > 0;

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

        {/* Rule list */}
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

        {/* Live preview */}
        {hasRules && (
          <div className="flex items-center gap-3">
            <Button variant="secondary" onClick={runPreview} loading={previewLoading} type="button">
              Preview
            </Button>
            {previewCount !== null && !previewLoading && (
              <span className="text-sm" style={{ color: "var(--text-muted)" }}>
                {previewCount} {previewCount === 1 ? "item" : "items"} match
              </span>
            )}
          </div>
        )}

        {/* Footer */}
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="secondary" onClick={onClose} type="button">
            Cancel
          </Button>
          <Button onClick={handleSave} type="button">
            {editing ? "Save changes" : "Create"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
